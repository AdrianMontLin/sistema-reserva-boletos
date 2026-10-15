package metricas

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/models"
)

func casiIgual(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestDescribir(t *testing.T) {
	media, desv := Describir([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	if !casiIgual(media, 5) || !casiIgual(desv, 2.138) { // desviación muestral (n-1)
		t.Fatalf("media=%v desv=%v", media, desv)
	}
	if m, d := Describir(nil); m != 0 || d != 0 {
		t.Fatal("una lista vacía debe dar 0")
	}
	if _, d := Describir([]float64{7}); d != 0 {
		t.Fatal("con un solo valor la desviación es 0")
	}
}

func registrosDePrueba() []Registro {
	t0 := time.UnixMilli(1_700_000_000_000)
	return []Registro{
		{t0.Add(1 * time.Second), "N1-C1", models.OpConsulta, 10 * time.Millisecond, models.CodOK, ""},
		{t0.Add(2 * time.Second), "N1-C1", models.OpConsulta, 20 * time.Millisecond, models.CodOK, ""},
		{t0.Add(3 * time.Second), "N3-C1", models.OpReserva, 30 * time.Millisecond, models.CodRechazada, "sin cupo, quedan 0"},
		{t0.Add(4 * time.Second), "N3-C1", models.OpReserva, 5 * time.Second, models.CodError, "timeout"},
	}
}

func TestResumir(t *testing.T) {
	filas := Resumir(registrosDePrueba(), time.Minute)
	consulta, reserva := filas[0], filas[1]
	// consulta: 2 procesadas, media 15 ms, desviación muestral 7.07 ms, 2 en 1 minuto = 2 TPM
	if consulta.Procesadas != 2 || !casiIgual(consulta.MediaMs, 15) || !casiIgual(consulta.DesvMs, 7.07) || !casiIgual(consulta.TPM, 2) {
		t.Fatalf("consulta: %+v", consulta)
	}
	// reserva: 1 procesada (rechazada) y 1 error; el error no entra en los tiempos
	if reserva.Procesadas != 1 || reserva.Rechazadas != 1 || reserva.Errores != 1 || !casiIgual(reserva.MediaMs, 30) {
		t.Fatalf("reserva: %+v", reserva)
	}
	// sin ventana se usa el tiempo entre el primer y el último registro (3 s)
	if v := Ventana(registrosDePrueba()); v != 3*time.Second {
		t.Fatalf("ventana = %s, se esperaban 3s", v)
	}
}

func TestCSVIdaYVuelta(t *testing.T) {
	original := registrosDePrueba()
	var buffer bytes.Buffer
	if err := EscribirCSV(&buffer, original); err != nil {
		t.Fatal(err)
	}
	leidos, err := LeerCSV(&buffer)
	if err != nil || len(leidos) != len(original) {
		t.Fatalf("lectura: %d registros, err=%v", len(leidos), err)
	}
	if leidos[2].Mensaje != "sin cupo, quedan 0" { // el mensaje trae una coma
		t.Fatalf("se perdió el mensaje: %q", leidos[2].Mensaje)
	}
	a, b := Resumir(original, time.Minute), Resumir(leidos, time.Minute)
	for i := range a {
		if a[i].Procesadas != b[i].Procesadas || a[i].Errores != b[i].Errores || !casiIgual(a[i].MediaMs, b[i].MediaMs) {
			t.Fatalf("el resumen cambió al pasar por el CSV: %+v vs %+v", a[i], b[i])
		}
	}
}

func TestLeerCSVInvalido(t *testing.T) {
	if _, err := LeerCSV(strings.NewReader("")); err == nil {
		t.Fatal("un archivo vacío debe dar error")
	}
	malo := "timestamp_ms,cliente,operacion,latencia_ms,codigo,mensaje\nabc,C,consulta,1.0,OK,\n"
	if _, err := LeerCSV(strings.NewReader(malo)); err == nil {
		t.Fatal("un timestamp inválido debe dar error")
	}
}
