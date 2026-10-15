// Servicio de Persistencia (Nodo 3).
//
// Guarda los eventos y las reservas en memoria y en un archivo JSON, y responde
// a las solicitudes del RMC por un socket REP (puerto 9904 por defecto).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	zmq4 "github.com/go-zeromq/zmq4"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/models"
)

// Datos de la carga inicial, según el enunciado.
const (
	mesInicial            = "2026-10"
	totalEventos          = 50
	eventosDobles         = 20 // eventos que tienen dos ocurrencias en el mes
	asientosPorOcurrencia = 100
)

var (
	bd      models.BaseDeDatos // la base de datos en memoria
	candado sync.Mutex         // protege bd: la usan el ciclo principal y el manejador de Ctrl+C

	archivoBD  string // dónde se guarda el JSON ("" = no se guarda)
	silencioso bool   // si es true no se imprime cada operación
)

// errorNegocio es un rechazo por una regla del negocio (no es una falla del sistema).
type errorNegocio struct{ mensaje string }

func (e *errorNegocio) Error() string { return e.mensaje }

func rechazo(formato string, args ...any) error {
	return &errorNegocio{fmt.Sprintf(formato, args...)}
}

// ---------------------------------------------------------------------------
// Base de datos
// ---------------------------------------------------------------------------

// crearBaseInicial llena la base con 50 eventos: 20 con dos ocurrencias y 30
// con una, cada una con 100 asientos libres.
func crearBaseInicial() {
	categorias := []string{
		"Concierto", "Obra de teatro", "Torneo de fútbol", "Cine-foro", "Exposición de arte",
		"Torneo de baloncesto", "Recital de poesía", "Carrera atlética", "Festival de danza", "Torneo de ajedrez",
	}
	bd = models.BaseDeDatos{SiguienteReserva: 1}
	for i := 1; i <= totalEventos; i++ {
		id := "E" + strconv.Itoa(i)
		evento := models.Evento{ID: id, Nombre: fmt.Sprintf("%s (%s)", categorias[(i-1)%len(categorias)], id)}
		if i <= eventosDobles {
			dia := 1 + i%14
			evento.Ocurrencias = []models.Ocurrencia{
				nuevaOcurrencia(id+"-1", dia),
				nuevaOcurrencia(id+"-2", dia+14),
			}
		} else {
			evento.Ocurrencias = []models.Ocurrencia{nuevaOcurrencia(id+"-1", 1+(i*3)%28)}
		}
		bd.Eventos = append(bd.Eventos, evento)
	}
}

func nuevaOcurrencia(id string, dia int) models.Ocurrencia {
	return models.Ocurrencia{
		ID:              id,
		Fecha:           fmt.Sprintf("%s-%02d", mesInicial, dia),
		AsientosTotales: asientosPorOcurrencia,
		AsientosLibres:  asientosPorOcurrencia,
	}
}

// cargar lee el archivo JSON. Devuelve false si el archivo no existe.
func cargar() (bool, error) {
	datos, err := os.ReadFile(archivoBD)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var leida models.BaseDeDatos
	if err := json.Unmarshal(datos, &leida); err != nil {
		return false, fmt.Errorf("el archivo %s está dañado: %v", archivoBD, err)
	}
	bd = leida
	if bd.SiguienteReserva < 1 {
		bd.SiguienteReserva = len(bd.Reservas) + 1
	}
	return true, nil
}

// guardar escribe la base de datos en el archivo JSON. Se escribe primero en
// un archivo temporal y luego se renombra, para no dejar el archivo a medias
// si el programa se cae justo mientras escribe.
func guardar() error {
	if archivoBD == "" {
		return nil
	}
	candado.Lock()
	defer candado.Unlock()

	bd.Guardado = time.Now().Format(time.RFC3339)
	datos, err := json.MarshalIndent(bd, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(archivoBD), 0o755); err != nil {
		return err
	}
	temporal := archivoBD + ".tmp"
	if err := os.WriteFile(temporal, datos, 0o644); err != nil {
		return err
	}
	return os.Rename(temporal, archivoBD)
}

// buscarOcurrencia devuelve un puntero a la ocurrencia dentro de la base.
// Quien la llame debe tener el candado.
func buscarOcurrencia(idEvento, idOcurrencia string) (*models.Ocurrencia, error) {
	for i := range bd.Eventos {
		if bd.Eventos[i].ID != idEvento {
			continue
		}
		for j := range bd.Eventos[i].Ocurrencias {
			if bd.Eventos[i].Ocurrencias[j].ID == idOcurrencia {
				return &bd.Eventos[i].Ocurrencias[j], nil
			}
		}
		return nil, rechazo("la ocurrencia %q no existe para el evento %s", idOcurrencia, idEvento)
	}
	return nil, rechazo("el evento %q no existe", idEvento)
}

var formatoMes = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// consultarMes devuelve los eventos que tienen alguna ocurrencia en el mes
// (formato AAAA-MM), mostrando solo las ocurrencias de ese mes.
func consultarMes(mes string) ([]models.Evento, error) {
	if !formatoMes.MatchString(mes) {
		return nil, rechazo("mes inválido: se espera el formato AAAA-MM (recibido %q)", mes)
	}
	candado.Lock()
	defer candado.Unlock()

	var resultado []models.Evento
	for _, evento := range bd.Eventos {
		var delMes []models.Ocurrencia
		for _, oc := range evento.Ocurrencias {
			if strings.HasPrefix(oc.Fecha, mes+"-") {
				delMes = append(delMes, oc)
			}
		}
		if len(delMes) > 0 {
			resultado = append(resultado, models.Evento{ID: evento.ID, Nombre: evento.Nombre, Ocurrencias: delMes})
		}
	}
	return resultado, nil
}

// disponibilidad devuelve cuántos asientos libres quedan en una ocurrencia.
func disponibilidad(idEvento, idOcurrencia string) (int, error) {
	candado.Lock()
	defer candado.Unlock()

	oc, err := buscarOcurrencia(idEvento, idOcurrencia)
	if err != nil {
		return 0, err
	}
	return oc.AsientosLibres, nil
}

// reservar descuenta los asientos si hay cupo. Comprobar el cupo y descontarlo
// se hace con el candado tomado todo el tiempo, así dos reservas al mismo
// tiempo nunca pueden vender más asientos de los que existen.
func reservar(idEvento, idOcurrencia, idCliente string, cantidad int) (*models.Reserva, error) {
	if cantidad <= 0 {
		return nil, rechazo("cantidad de asientos inválida: %d", cantidad)
	}
	candado.Lock()
	defer candado.Unlock()

	oc, err := buscarOcurrencia(idEvento, idOcurrencia)
	if err != nil {
		return nil, err
	}
	if oc.AsientosLibres < cantidad {
		return nil, rechazo("no hay asientos suficientes: quedan %d y se pidieron %d", oc.AsientosLibres, cantidad)
	}
	oc.AsientosLibres -= cantidad
	oc.AsientosOcupados += cantidad

	reserva := models.Reserva{
		ID:           "R" + strconv.Itoa(bd.SiguienteReserva),
		IDEvento:     idEvento,
		IDOcurrencia: idOcurrencia,
		IDCliente:    idCliente,
		NumAsientos:  cantidad,
		Estado:       models.EstadoConfirmada,
	}
	bd.SiguienteReserva++
	bd.Reservas = append(bd.Reservas, reserva)
	return &reserva, nil
}

// resumen devuelve una línea con el estado de la base de datos.
func resumen() string {
	candado.Lock()
	defer candado.Unlock()

	ocurrencias, libres, ocupados := 0, 0, 0
	for _, evento := range bd.Eventos {
		for _, oc := range evento.Ocurrencias {
			ocurrencias++
			libres += oc.AsientosLibres
			ocupados += oc.AsientosOcupados
		}
	}
	return fmt.Sprintf("%d eventos, %d ocurrencias | asientos: %d libres, %d ocupados (de %d) | %d reservas",
		len(bd.Eventos), ocurrencias, libres, ocupados, libres+ocupados, len(bd.Reservas))
}

// ---------------------------------------------------------------------------
// Atender las solicitudes del RMC
// ---------------------------------------------------------------------------

func registrar(formato string, args ...any) {
	if !silencioso {
		log.Printf(formato, args...)
	}
}

// respuestaDeError arma la respuesta para un error: RECHAZADA si es una regla
// del negocio, ERROR si es otra cosa.
func respuestaDeError(err error) models.RespuestaPersistencia {
	if _, esRechazo := err.(*errorNegocio); esRechazo {
		return models.RespuestaPersistencia{Codigo: models.CodRechazada, Mensaje: err.Error()}
	}
	return models.RespuestaPersistencia{Codigo: models.CodError, Mensaje: err.Error()}
}

func atender(datos []byte) models.RespuestaPersistencia {
	var sol models.SolicitudPersistencia
	if err := json.Unmarshal(datos, &sol); err != nil {
		return models.RespuestaPersistencia{Codigo: models.CodError, Mensaje: "solicitud mal formada: " + err.Error()}
	}

	switch sol.Accion {

	case models.AccionConsultarMes:
		eventos, err := consultarMes(sol.Mes)
		if err != nil {
			registrar("consultar_mes %q -> %v", sol.Mes, err)
			return respuestaDeError(err)
		}
		registrar("consultar_mes %s -> %d eventos", sol.Mes, len(eventos))
		return models.RespuestaPersistencia{Codigo: models.CodOK, Eventos: eventos}

	case models.AccionDisponibilidad:
		libres, err := disponibilidad(sol.IDEvento, sol.IDOcurrencia)
		if err != nil {
			registrar("disponibilidad %s -> %v", sol.IDOcurrencia, err)
			return respuestaDeError(err)
		}
		registrar("disponibilidad %s -> %d libres", sol.IDOcurrencia, libres)
		return models.RespuestaPersistencia{Codigo: models.CodOK, Disponibles: libres}

	case models.AccionReservar:
		reserva, err := reservar(sol.IDEvento, sol.IDOcurrencia, sol.IDCliente, sol.NumAsientos)
		if err != nil {
			registrar("reservar %s x%d (cliente %s) -> RECHAZADA: %v", sol.IDOcurrencia, sol.NumAsientos, sol.IDCliente, err)
			return respuestaDeError(err)
		}
		if err := guardar(); err != nil { // se guarda después de cada reserva
			log.Printf("error al guardar %s: %v", archivoBD, err)
		}
		libres, _ := disponibilidad(sol.IDEvento, sol.IDOcurrencia)
		registrar("reservar %s x%d (cliente %s) -> %s confirmada, quedan %d libres", sol.IDOcurrencia, sol.NumAsientos, sol.IDCliente, reserva.ID, libres)
		return models.RespuestaPersistencia{Codigo: models.CodOK, Mensaje: "reserva confirmada", Reserva: reserva}

	default:
		return models.RespuestaPersistencia{Codigo: models.CodError, Mensaje: fmt.Sprintf("acción no reconocida: %q", sol.Accion)}
	}
}

func main() {
	bind := flag.String("bind", "tcp://*:9904", "dirección donde escucha (REP)")
	archivo := flag.String("archivo", "data/srb_db.json", "archivo JSON de la base de datos (vacío = solo memoria)")
	reiniciar := flag.Bool("reiniciar", false, "ignorar el archivo y empezar con la carga inicial (50 eventos)")
	estadoCada := flag.Duration("estado-cada", 10*time.Second, "cada cuánto muestra el estado de la BD (0 = nunca)")
	q := flag.Bool("q", false, "silencioso: no imprime cada operación")
	flag.Parse()
	archivoBD = *archivo
	silencioso = *q
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.SetPrefix("[PERSISTENCIA] ")

	// 1. Preparar la base de datos: carga inicial, o la guardada en el archivo.
	crearBaseInicial()
	if !*reiniciar && archivoBD != "" {
		existe, err := cargar()
		if err != nil {
			log.Fatal(err)
		}
		if existe {
			log.Printf("base de datos recuperada desde %s", archivoBD)
		} else {
			log.Printf("no había archivo %s: se usa la carga inicial", archivoBD)
		}
	}
	log.Printf("ESTADO INICIAL: %s", resumen())

	// 2. Abrir el puerto.
	socket := zmq4.NewRep(context.Background())
	if err := socket.Listen(*bind); err != nil {
		log.Fatalf("no se pudo escuchar en %s: %v", *bind, err)
	}
	log.Printf("escuchando en %s (REP)", *bind)

	// 3. Con Ctrl+C: guardar, mostrar el estado final y salir.
	senal := make(chan os.Signal, 1)
	signal.Notify(senal, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-senal
		if err := guardar(); err != nil {
			log.Printf("error al guardar: %v", err)
		}
		log.Printf("ESTADO FINAL: %s", resumen())
		os.Exit(0)
	}()

	// 4. Mostrar el estado cada cierto tiempo.
	if *estadoCada > 0 {
		go func() {
			for {
				time.Sleep(*estadoCada)
				log.Printf("ESTADO: %s", resumen())
			}
		}()
	}

	// 5. Atender solicitudes, una a la vez (REQ/REP).
	for {
		mensaje, err := socket.Recv()
		if err != nil {
			log.Printf("error al recibir: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var datos []byte
		if len(mensaje.Frames) > 0 {
			datos = mensaje.Frames[0]
		}
		respuesta := atender(datos)
		bytesRespuesta, _ := json.Marshal(respuesta)
		if err := socket.Send(zmq4.NewMsg(bytesRespuesta)); err != nil {
			log.Printf("error al responder: %v", err)
		}
	}
}
