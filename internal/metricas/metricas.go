// Package metricas calcula las métricas de la Tabla 1: tiempo promedio de
// respuesta, desviación estándar y transacciones por minuto. También
// guarda y lee los resultados de los clientes en archivos CSV.
package metricas

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/models"
)

// Registro es el resultado de una solicitud medida por un cliente.
type Registro struct {
	Momento   time.Time // cuándo llegó la respuesta
	Cliente   string
	Operacion string
	Latencia  time.Duration
	Codigo    string
	Mensaje   string
}

// Fila es el resumen de una operación (consulta o reserva).
type Fila struct {
	Operacion  string
	Procesadas int // respondidas con OK o RECHAZADA
	Rechazadas int
	Errores    int
	MediaMs    float64
	DesvMs     float64
	MinMs      float64
	MaxMs      float64
	TPM        float64
}

// Describir calcula la media y la desviación estándar muestral (dividiendo
// entre n-1) de una lista de valores.
func Describir(valores []float64) (media, desv float64) {
	if len(valores) == 0 {
		return 0, 0
	}
	suma := 0.0
	for _, v := range valores {
		suma += v
	}
	media = suma / float64(len(valores))
	if len(valores) < 2 {
		return media, 0
	}
	sumaCuadrados := 0.0
	for _, v := range valores {
		sumaCuadrados += (v - media) * (v - media)
	}
	desv = math.Sqrt(sumaCuadrados / float64(len(valores)-1))
	return media, desv
}

// Ventana devuelve el tiempo entre el primer y el último registro.
func Ventana(registros []Registro) time.Duration {
	if len(registros) == 0 {
		return 0
	}
	primero := registros[0].Momento
	ultimo := registros[0].Momento
	for _, r := range registros {
		if r.Momento.Before(primero) {
			primero = r.Momento
		}
		if r.Momento.After(ultimo) {
			ultimo = r.Momento
		}
	}
	return ultimo.Sub(primero)
}

// Resumir calcula, por operación, el tiempo de respuesta promedio, la
// desviación estándar y el TPM. Las solicitudes con ERROR (timeout, servicio
// caído) se cuentan aparte y no entran en los tiempos. Si ventana es <= 0 se
// usa el tiempo entre el primer y el último registro.
func Resumir(registros []Registro, ventana time.Duration) []Fila {
	if ventana <= 0 {
		ventana = Ventana(registros)
	}
	var filas []Fila
	for _, operacion := range []string{models.OpConsulta, models.OpReserva} {
		fila := Fila{Operacion: operacion}
		var tiempos []float64 // en milisegundos
		for _, r := range registros {
			if r.Operacion != operacion {
				continue
			}
			if r.Codigo != models.CodOK && r.Codigo != models.CodRechazada {
				fila.Errores++
				continue
			}
			if r.Codigo == models.CodRechazada {
				fila.Rechazadas++
			}
			tiempos = append(tiempos, float64(r.Latencia)/float64(time.Millisecond))
		}
		fila.Procesadas = len(tiempos)
		fila.MediaMs, fila.DesvMs = Describir(tiempos)
		for i, t := range tiempos {
			if i == 0 || t < fila.MinMs {
				fila.MinMs = t
			}
			if i == 0 || t > fila.MaxMs {
				fila.MaxMs = t
			}
		}
		if ventana > 0 {
			fila.TPM = float64(len(tiempos)) / ventana.Minutes()
		} else {
			fila.TPM = float64(len(tiempos))
		}
		filas = append(filas, fila)
	}
	return filas
}

// EscribirCSV escribe un registro por línea.
func EscribirCSV(w io.Writer, registros []Registro) error {
	escritor := csv.NewWriter(w)
	escritor.Write([]string{"timestamp_ms", "cliente", "operacion", "latencia_ms", "codigo", "mensaje"})
	for _, r := range registros {
		escritor.Write([]string{
			strconv.FormatInt(r.Momento.UnixMilli(), 10),
			r.Cliente,
			r.Operacion,
			strconv.FormatFloat(float64(r.Latencia)/float64(time.Millisecond), 'f', 3, 64),
			r.Codigo,
			r.Mensaje,
		})
	}
	escritor.Flush()
	return escritor.Error()
}

// LeerCSV lee un archivo escrito por EscribirCSV.
func LeerCSV(r io.Reader) ([]Registro, error) {
	lineas, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(lineas) == 0 {
		return nil, errors.New("el archivo está vacío")
	}
	var registros []Registro
	for i, linea := range lineas[1:] { // la primera línea es el encabezado
		if len(linea) < 6 {
			return nil, fmt.Errorf("línea %d: se esperaban 6 columnas y hay %d", i+2, len(linea))
		}
		ms, err := strconv.ParseInt(linea[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("línea %d: timestamp inválido: %v", i+2, err)
		}
		latencia, err := strconv.ParseFloat(linea[3], 64)
		if err != nil {
			return nil, fmt.Errorf("línea %d: latencia inválida: %v", i+2, err)
		}
		registros = append(registros, Registro{
			Momento:   time.UnixMilli(ms),
			Cliente:   linea[1],
			Operacion: linea[2],
			Latencia:  time.Duration(latencia * float64(time.Millisecond)),
			Codigo:    linea[4],
			Mensaje:   linea[5],
		})
	}
	return registros, nil
}
