package main

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/models"
)

// empezar deja la base en su estado inicial y sin archivo.
func empezar() {
	crearBaseInicial()
	archivoBD = ""
}

func esRechazo(err error) bool {
	_, ok := err.(*errorNegocio)
	return ok
}

func contarAsientos() (ocurrencias, libres, ocupados int) {
	for _, evento := range bd.Eventos {
		for _, oc := range evento.Ocurrencias {
			ocurrencias++
			libres += oc.AsientosLibres
			ocupados += oc.AsientosOcupados
		}
	}
	return
}

func TestCargaInicial(t *testing.T) {
	empezar()
	if len(bd.Eventos) != 50 {
		t.Fatalf("eventos = %d, se esperaban 50", len(bd.Eventos))
	}
	ocurrencias, libres, ocupados := contarAsientos()
	if ocurrencias != 70 { // 20 eventos con dos ocurrencias + 30 con una
		t.Fatalf("ocurrencias = %d, se esperaban 70", ocurrencias)
	}
	if libres != 7000 || ocupados != 0 {
		t.Fatalf("asientos: %d libres y %d ocupados", libres, ocupados)
	}
	eventos, err := consultarMes(mesInicial)
	if err != nil || len(eventos) != 50 {
		t.Fatalf("consulta del mes inicial: %d eventos, err=%v", len(eventos), err)
	}
	dobles := 0
	for _, e := range eventos {
		if len(e.Ocurrencias) == 2 {
			dobles++
		}
	}
	if dobles != 20 {
		t.Fatalf("eventos con dos ocurrencias = %d, se esperaban 20", dobles)
	}
	if eventos[0].ID != "E1" || eventos[49].ID != "E50" {
		t.Fatalf("orden incorrecto: %s ... %s", eventos[0].ID, eventos[49].ID)
	}
}

func TestConsultaMes(t *testing.T) {
	empezar()
	if eventos, err := consultarMes("2026-11"); err != nil || len(eventos) != 0 {
		t.Fatalf("un mes sin eventos debe dar lista vacía: %d, err=%v", len(eventos), err)
	}
	for _, mes := range []string{"2026-13", "octubre", "", "2026-1"} {
		if _, err := consultarMes(mes); !esRechazo(err) {
			t.Errorf("el mes %q debía ser rechazado, err=%v", mes, err)
		}
	}
}

func TestReservaYRechazos(t *testing.T) {
	empezar()
	r, err := reservar("E1", "E1-1", "C1", 3)
	if err != nil || r.NumAsientos != 3 || r.Estado != "confirmada" || r.ID != "R1" {
		t.Fatalf("reserva inválida: %+v, err=%v", r, err)
	}
	if libres, _ := disponibilidad("E1", "E1-1"); libres != 97 {
		t.Fatalf("libres = %d, se esperaban 97", libres)
	}
	casos := []struct {
		nombre   string
		evento   string
		ocurr    string
		cantidad int
	}{
		{"sin cupo", "E1", "E1-1", 98},
		{"evento inexistente", "E99", "E99-1", 1},
		{"ocurrencia inexistente", "E1", "E1-9", 1},
		{"cantidad cero", "E1", "E1-1", 0},
		{"cantidad negativa", "E1", "E1-1", -2},
	}
	for _, c := range casos {
		if _, err := reservar(c.evento, c.ocurr, "C1", c.cantidad); !esRechazo(err) {
			t.Errorf("%s: debía ser un rechazo, err=%v", c.nombre, err)
		}
	}
	// Los rechazos no deben haber cambiado nada.
	if libres, _ := disponibilidad("E1", "E1-1"); libres != 97 {
		t.Fatalf("un rechazo cambió los asientos: libres = %d", libres)
	}
}

// Con reservas al mismo tiempo nunca se deben vender más asientos de los que existen.
func TestReservasConcurrentesSinSobreventa(t *testing.T) {
	empezar()
	var aceptadas atomic.Int64
	var espera sync.WaitGroup
	for g := 0; g < 20; g++ {
		espera.Add(1)
		go func() {
			defer espera.Done()
			for i := 0; i < 20; i++ {
				if _, err := reservar("E21", "E21-1", "C", 1); err == nil {
					aceptadas.Add(1)
				}
			}
		}()
	}
	espera.Wait()
	if aceptadas.Load() != 100 { // 400 intentos para 100 asientos
		t.Fatalf("reservas aceptadas = %d, debían ser exactamente 100", aceptadas.Load())
	}
	_, libres, ocupados := contarAsientos()
	if libres+ocupados != 7000 || ocupados != 100 || len(bd.Reservas) != 100 {
		t.Fatalf("se rompió la cuenta: %d libres, %d ocupados, %d reservas", libres, ocupados, len(bd.Reservas))
	}
}

func TestGuardarYCargar(t *testing.T) {
	empezar()
	archivoBD = filepath.Join(t.TempDir(), "carpeta", "bd.json")
	if _, err := reservar("E2", "E2-2", "C7", 5); err != nil {
		t.Fatal(err)
	}
	if err := guardar(); err != nil {
		t.Fatal(err)
	}

	crearBaseInicial() // se "pierde" la memoria, como si se reiniciara el programa
	existe, err := cargar()
	if err != nil || !existe {
		t.Fatalf("cargar: existe=%v err=%v", existe, err)
	}
	if len(bd.Reservas) != 1 || bd.Reservas[0].IDCliente != "C7" {
		t.Fatalf("no se recuperó la reserva: %+v", bd.Reservas)
	}
	if libres, _ := disponibilidad("E2", "E2-2"); libres != 95 {
		t.Fatalf("libres = %d, se esperaban 95", libres)
	}
	if r, _ := reservar("E2", "E2-2", "C8", 1); r.ID != "R2" {
		t.Fatalf("la numeración debía continuar en R2, fue %s", r.ID)
	}

	archivoBD = filepath.Join(t.TempDir(), "no-existe.json")
	if existe, err := cargar(); existe || err != nil {
		t.Fatalf("un archivo que no existe no es un error: existe=%v err=%v", existe, err)
	}
}

func TestAtender(t *testing.T) {
	empezar()
	silencioso = true
	pedir := func(sol models.SolicitudPersistencia) models.RespuestaPersistencia {
		datos, _ := json.Marshal(sol)
		return atender(datos)
	}
	if r := pedir(models.SolicitudPersistencia{Accion: models.AccionConsultarMes, Mes: "2026-10"}); r.Codigo != models.CodOK || len(r.Eventos) != 50 {
		t.Fatalf("consultar_mes: %s, %d eventos", r.Codigo, len(r.Eventos))
	}
	if r := pedir(models.SolicitudPersistencia{Accion: models.AccionDisponibilidad, IDEvento: "E3", IDOcurrencia: "E3-1"}); r.Codigo != models.CodOK || r.Disponibles != 100 {
		t.Fatalf("disponibilidad: %s, %d", r.Codigo, r.Disponibles)
	}
	if r := pedir(models.SolicitudPersistencia{Accion: models.AccionReservar, IDEvento: "E3", IDOcurrencia: "E3-1", IDCliente: "C", NumAsientos: 2}); r.Codigo != models.CodOK || r.Reserva == nil {
		t.Fatalf("reservar: %s %q", r.Codigo, r.Mensaje)
	}
	if r := pedir(models.SolicitudPersistencia{Accion: models.AccionReservar, IDEvento: "E3", IDOcurrencia: "E3-1", NumAsientos: 500}); r.Codigo != models.CodRechazada {
		t.Fatalf("sin cupo debía ser RECHAZADA y fue %s", r.Codigo)
	}
	if r := pedir(models.SolicitudPersistencia{Accion: "borrar_todo"}); r.Codigo != models.CodError {
		t.Fatalf("una acción desconocida debía ser ERROR y fue %s", r.Codigo)
	}
	if r := atender([]byte("esto no es json")); r.Codigo != models.CodError {
		t.Fatalf("un mensaje mal formado debía ser ERROR y fue %s", r.Codigo)
	}
}
