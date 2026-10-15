// Package tests tiene las pruebas de integración: compilan los programas, los
// lanzan como procesos separados en localhost (igual que en el despliegue real,
// pero en una sola máquina) y los usan como lo haría un cliente.

package tests

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/metricas"
	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/models"
	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/mqutil"
)

var directorioBinarios string // aquí se compilan los programas

// TestMain compila los programas una sola vez antes de correr las pruebas.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "srb-binarios")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for _, programa := range []string{"persistencia", "rmc", "gestor_transacciones", "cliente"} {
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, programa), "../cmd/"+programa)
		if salida, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("no se pudo compilar %s: %v\n%s\n", programa, err, salida)
			os.Exit(1)
		}
	}
	directorioBinarios = dir
	codigo := m.Run()
	os.RemoveAll(dir)
	os.Exit(codigo)
}

// lanzar arranca un programa y lo detiene al terminar la prueba.
func lanzar(t *testing.T, programa string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(filepath.Join(directorioBinarios, programa), args...)
	cmd.Dir = t.TempDir()
	if err := cmd.Start(); err != nil {
		t.Fatalf("no se pudo lanzar %s: %v", programa, err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return cmd
}

// esperarPuerto espera hasta que el programa abra el puerto.
func esperarPuerto(t *testing.T, direccion string) {
	t.Helper()
	limite := time.Now().Add(10 * time.Second)
	for time.Now().Before(limite) {
		conexion, err := net.DialTimeout("tcp", direccion, 300*time.Millisecond)
		if err == nil {
			conexion.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("el puerto %s no se abrió", direccion)
}

// sistema arranca persistencia (base+4), RMC (base+2) y GT (base).
// Si conRMC es false no arranca el RMC (para simular que se cayó).
// Devuelve la dirección del GT y el proceso del GT.
func sistema(t *testing.T, base int, conRMC bool, archivoBD, archivoMetricas string) (string, *exec.Cmd) {
	t.Helper()
	direccion := func(desplazamiento int) string { return fmt.Sprintf("127.0.0.1:%d", base+desplazamiento) }

	lanzar(t, "persistencia", "-bind", "tcp://"+direccion(4), "-archivo", archivoBD, "-reiniciar", "-q", "-estado-cada", "0")
	esperarPuerto(t, direccion(4))
	if conRMC {
		lanzar(t, "rmc", "-bind", "tcp://"+direccion(2), "-persistencia", "tcp://"+direccion(4), "-timeout", "2s", "-q")
		esperarPuerto(t, direccion(2))
	}
	gt := lanzar(t, "gestor_transacciones", "-bind", "tcp://"+direccion(0), "-rmc", "tcp://"+direccion(2),
		"-timeout", "2s", "-reporte-cada", "0", "-q", "-metricas", archivoMetricas)
	esperarPuerto(t, direccion(0))
	return "tcp://" + direccion(0), gt
}

// pedir envía una solicitud y devuelve la respuesta.
func pedir(t *testing.T, r *mqutil.Requester, sol models.Solicitud) models.Respuesta {
	t.Helper()
	datos, _ := json.Marshal(sol)
	bytesRespuesta, err := r.Enviar(datos)
	if err != nil {
		t.Fatalf("error de comunicación: %v", err)
	}
	var respuesta models.Respuesta
	if err := json.Unmarshal(bytesRespuesta, &respuesta); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	return respuesta
}

func TestFlujoCompleto(t *testing.T) {
	archivoMetricas := filepath.Join(t.TempDir(), "metricas_gt.csv")
	gt, procesoGT := sistema(t, 19100, true, filepath.Join(t.TempDir(), "bd.json"), archivoMetricas)
	cliente := mqutil.NuevoRequester(gt, 5*time.Second)
	defer cliente.Cerrar()

	// Consulta: debe devolver los 50 eventos del mes.
	r := pedir(t, cliente, models.Solicitud{IDSolicitud: "t-1", Operacion: models.OpConsulta, IDCliente: "T", Mes: "2026-10"})
	if r.Codigo != models.CodOK || len(r.Eventos) != 50 || r.IDSolicitud != "t-1" {
		t.Fatalf("consulta inesperada: %s %q con %d eventos", r.Codigo, r.Mensaje, len(r.Eventos))
	}

	// Reserva válida.
	r = pedir(t, cliente, models.Solicitud{IDSolicitud: "t-2", Operacion: models.OpReserva, IDCliente: "T", IDEvento: "E1", IDOcurrencia: "E1-1", NumAsientos: 3})
	if r.Codigo != models.CodOK || r.Reserva == nil || r.Reserva.NumAsientos != 3 || r.Reserva.IDCliente != "T" {
		t.Fatalf("reserva inesperada: %s %q %+v", r.Codigo, r.Mensaje, r.Reserva)
	}

	// La consulta siguiente debe mostrar el cambio.
	r = pedir(t, cliente, models.Solicitud{IDSolicitud: "t-3", Operacion: models.OpConsulta, IDCliente: "T", Mes: "2026-10"})
	if libres := r.Eventos[0].Ocurrencias[0].AsientosLibres; libres != 97 {
		t.Fatalf("libres después de la reserva = %d, se esperaban 97", libres)
	}

	// Casos que deben ser rechazados por el negocio.
	rechazos := map[string]models.Solicitud{
		"sin cupo":               {Operacion: models.OpReserva, IDEvento: "E1", IDOcurrencia: "E1-1", NumAsientos: 101},
		"evento inexistente":     {Operacion: models.OpReserva, IDEvento: "E99", IDOcurrencia: "E99-1", NumAsientos: 1},
		"ocurrencia inexistente": {Operacion: models.OpReserva, IDEvento: "E1", IDOcurrencia: "E1-9", NumAsientos: 1},
		"cantidad cero":          {Operacion: models.OpReserva, IDEvento: "E1", IDOcurrencia: "E1-1", NumAsientos: 0},
		"mes inválido":           {Operacion: models.OpConsulta, Mes: "2026-99"},
	}
	for nombre, sol := range rechazos {
		sol.IDCliente = "T"
		if r := pedir(t, cliente, sol); r.Codigo != models.CodRechazada {
			t.Errorf("%s: se esperaba RECHAZADA y fue %s (%q)", nombre, r.Codigo, r.Mensaje)
		}
	}

	// Un mes sin eventos es válido: OK con lista vacía.
	if r := pedir(t, cliente, models.Solicitud{Operacion: models.OpConsulta, IDCliente: "T", Mes: "2026-11"}); r.Codigo != models.CodOK || len(r.Eventos) != 0 {
		t.Errorf("mes sin eventos: %s con %d eventos", r.Codigo, len(r.Eventos))
	}

	// Operaciones de la Entrega 2: ERROR controlado, el sistema no se cae.
	for _, operacion := range []string{models.OpModificacion, models.OpCancelacion} {
		if r := pedir(t, cliente, models.Solicitud{Operacion: operacion, IDCliente: "T"}); r.Codigo != models.CodError {
			t.Errorf("%s debía responder ERROR (no soportada) y fue %s", operacion, r.Codigo)
		}
	}

	// Un mensaje que no es JSON: el GT responde ERROR y sigue funcionando.
	if bytesRespuesta, err := cliente.Enviar([]byte("esto no es json")); err != nil || len(bytesRespuesta) == 0 {
		t.Fatalf("el GT debía responder al mensaje mal formado: %v", err)
	}
	if r := pedir(t, cliente, models.Solicitud{Operacion: models.OpConsulta, IDCliente: "T", Mes: "2026-10"}); r.Codigo != models.CodOK {
		t.Fatal("el sistema debía seguir funcionando después del mensaje mal formado")
	}

	// Con Ctrl+C el GT guarda sus métricas en el CSV.
	procesoGT.Process.Signal(os.Interrupt)
	time.Sleep(1500 * time.Millisecond)
	contenido, err := os.ReadFile(archivoMetricas)
	if err != nil || !strings.Contains(string(contenido), "consulta") || !strings.Contains(string(contenido), "reserva") {
		t.Fatalf("el GT no guardó sus métricas: %v %q", err, contenido)
	}
}

// La base de datos debe sobrevivir a un reinicio de la persistencia.
func TestPersistenciaSobreviveReinicio(t *testing.T) {
	archivoBD := filepath.Join(t.TempDir(), "bd.json")
	gt, _ := sistema(t, 19200, true, archivoBD, "")
	cliente := mqutil.NuevoRequester(gt, 5*time.Second)
	defer cliente.Cerrar()
	r := pedir(t, cliente, models.Solicitud{Operacion: models.OpReserva, IDCliente: "T", IDEvento: "E5", IDOcurrencia: "E5-1", NumAsientos: 4})
	if r.Codigo != models.CodOK {
		t.Fatalf("la reserva falló: %s %q", r.Codigo, r.Mensaje)
	}

	// Se "cae" la persistencia de golpe y se vuelve a levantar sin -reiniciar.
	lanzar(t, "persistencia", "-bind", "tcp://127.0.0.1:19215", "-archivo", archivoBD, "-q", "-estado-cada", "0")
	esperarPuerto(t, "127.0.0.1:19215")
	directo := mqutil.NuevoRequester("tcp://127.0.0.1:19215", 5*time.Second)
	defer directo.Cerrar()
	datos, _ := json.Marshal(models.SolicitudPersistencia{Accion: models.AccionDisponibilidad, IDEvento: "E5", IDOcurrencia: "E5-1"})
	bytesRespuesta, err := directo.Enviar(datos)
	if err != nil {
		t.Fatal(err)
	}
	var respuesta models.RespuestaPersistencia
	json.Unmarshal(bytesRespuesta, &respuesta)
	if respuesta.Codigo != models.CodOK || respuesta.Disponibles != 96 {
		t.Fatalf("la persistencia no recuperó los datos: %s, %d libres (se esperaban 96)", respuesta.Codigo, respuesta.Disponibles)
	}
}

// Detección de fallas: si el RMC no está, el GT debe responder ERROR por
// timeout en lugar de quedarse colgado.
func TestRMCCaidoSeDetectaConTimeout(t *testing.T) {
	gt, _ := sistema(t, 19300, false, "", "")
	cliente := mqutil.NuevoRequester(gt, 10*time.Second)
	defer cliente.Cerrar()

	inicio := time.Now()
	r := pedir(t, cliente, models.Solicitud{IDSolicitud: "t-9", Operacion: models.OpConsulta, IDCliente: "T", Mes: "2026-10"})
	if r.Codigo != models.CodError || r.IDSolicitud != "t-9" {
		t.Fatalf("se esperaba un ERROR controlado, fue %s %q", r.Codigo, r.Mensaje)
	}
	if demora := time.Since(inicio); demora > 8*time.Second {
		t.Fatalf("detectar la falla tardó demasiado: %s", demora)
	}
}

// Varios clientes reservan la misma ocurrencia a la vez: nunca se vende más de lo que existe.
func TestConcurrenciaSinSobreventa(t *testing.T) {
	gt, _ := sistema(t, 19400, true, "", "")
	var aceptadas atomic.Int64
	var espera sync.WaitGroup
	for numero := 0; numero < 8; numero++ {
		espera.Add(1)
		go func(numero int) {
			defer espera.Done()
			cliente := mqutil.NuevoRequester(gt, 10*time.Second)
			defer cliente.Cerrar()
			for i := 0; i < 20; i++ {
				datos, _ := json.Marshal(models.Solicitud{Operacion: models.OpReserva, IDCliente: fmt.Sprintf("C%d", numero),
					IDEvento: "E30", IDOcurrencia: "E30-1", NumAsientos: 1})
				bytesRespuesta, err := cliente.Enviar(datos)
				if err != nil {
					t.Errorf("cliente %d: %v", numero, err)
					return
				}
				var respuesta models.Respuesta
				json.Unmarshal(bytesRespuesta, &respuesta)
				if respuesta.Codigo == models.CodOK {
					aceptadas.Add(1)
				}
			}
		}(numero)
	}
	espera.Wait()
	if aceptadas.Load() != 100 { // 8 x 20 = 160 intentos para 100 asientos
		t.Fatalf("reservas aceptadas = %d, debían ser exactamente 100", aceptadas.Load())
	}
}

// El generador de carga contra el sistema real.
func TestGeneradorDeCarga(t *testing.T) {
	gt, _ := sistema(t, 19500, true, "", "")
	prefijo := filepath.Join(t.TempDir(), "res")
	cmd := exec.Command(filepath.Join(directorioBinarios, "cliente"), "-gt", gt, "-id", "T", "-clientes", "3", "-solo-consulta", "2",
		"-rpm", "300", "-pct-consulta", "50", "-duracion", "4s", "-semilla", "42", "-q", "-salida", prefijo)
	if salida, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("el cliente falló: %v\n%s", err, salida)
	}

	archivo, err := os.Open(prefijo + "_T.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer archivo.Close()
	registros, err := metricas.LeerCSV(archivo)
	if err != nil {
		t.Fatal(err)
	}
	filas := metricas.Resumir(registros, 4*time.Second)
	consulta, reserva := filas[0], filas[1]
	if consulta.Errores+reserva.Errores != 0 {
		t.Fatalf("hubo errores del sistema: %+v", filas)
	}
	if consulta.Procesadas < 20 || reserva.Procesadas < 1 {
		t.Fatalf("muy pocas solicitudes: %+v", filas)
	}
	if consulta.MediaMs <= 0 || consulta.TPM <= 0 {
		t.Fatalf("métricas vacías: %+v", consulta)
	}
	// Los clientes "solo consulta" (T-C1 y T-C2) nunca deben haber reservado.
	for _, r := range registros {
		if (r.Cliente == "T-C1" || r.Cliente == "T-C2") && r.Operacion != models.OpConsulta {
			t.Fatalf("el cliente %s solo debía consultar", r.Cliente)
		}
	}
}
