// Gestor de Transacciones (GT) — Nodo 2.
//
// Es la puerta de entrada del sistema: recibe las solicitudes de los clientes
// (REP, puerto 9900 por defecto), las enruta por el canal síncrono hacia el RMC
// (REQ), espera la respuesta y se la devuelve al cliente. Además mide las
// transacciones procesadas por minuto y los tiempos de servicio.
// El canal asíncrono (cancelaciones) se implementa en la Entrega 2.

package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	zmq4 "github.com/go-zeromq/zmq4"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/metricas"
	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/models"
	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/mqutil"
)

// estadisticas guarda lo que mide el GT de una operación.
type estadisticas struct {
	tiempos    []float64 // milisegundos de cada transacción procesada
	rechazadas int       // procesadas pero rechazadas por una regla del negocio
	errores    int       // fallas del sistema (RMC caído, timeout, ...)
	primera    time.Time // cuándo terminó la primera transacción
	ultima     time.Time // cuándo terminó la última
}

var (
	rmc        *mqutil.Requester
	silencioso bool

	candado sync.Mutex // protege los datos de abajo
	datos   = map[string]*estadisticas{
		models.OpConsulta: {},
		models.OpReserva:  {},
	}
)

// registrarTransaccion suma una transacción a las estadísticas de su operación.
func registrarTransaccion(operacion, codigo string, duracion time.Duration) {
	candado.Lock()
	defer candado.Unlock()
	e := datos[operacion]
	if codigo != models.CodOK && codigo != models.CodRechazada {
		e.errores++
		return
	}
	if codigo == models.CodRechazada {
		e.rechazadas++
	}
	ahora := time.Now()
	if len(e.tiempos) == 0 {
		e.primera = ahora
	}
	e.ultima = ahora
	e.tiempos = append(e.tiempos, float64(duracion)/float64(time.Millisecond))
}

// transacciones por minuto calcula las transacciones por minuto entre la primera y la última.
func (e *estadisticas) tpm() float64 {
	ventana := e.ultima.Sub(e.primera)
	if ventana <= 0 {
		return float64(len(e.tiempos))
	}
	return float64(len(e.tiempos)) / ventana.Minutes()
}

func imprimirMetricas(titulo string) {
	candado.Lock()
	defer candado.Unlock()
	log.Printf("---- %s (medidas en el GT) ----", titulo)
	for _, operacion := range []string{models.OpConsulta, models.OpReserva} {
		e := datos[operacion]
		media, desv := metricas.Describir(e.tiempos)
		log.Printf("%-9s procesadas=%-5d rechazadas=%-4d errores=%-3d media=%.2f ms  desv=%.2f ms  TPM=%.1f",
			operacion, len(e.tiempos), e.rechazadas, e.errores, media, desv, e.tpm())
	}
}

func escribirCSV(ruta string) error {
	candado.Lock()
	defer candado.Unlock()
	archivo, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer archivo.Close()
	escritor := csv.NewWriter(archivo)
	escritor.Write([]string{"operacion", "procesadas", "rechazadas", "errores", "media_ms", "desv_ms", "tpm"})
	for _, operacion := range []string{models.OpConsulta, models.OpReserva} {
		e := datos[operacion]
		media, desv := metricas.Describir(e.tiempos)
		escritor.Write([]string{
			operacion, strconv.Itoa(len(e.tiempos)), strconv.Itoa(e.rechazadas), strconv.Itoa(e.errores),
			strconv.FormatFloat(media, 'f', 2, 64), strconv.FormatFloat(desv, 'f', 2, 64), strconv.FormatFloat(e.tpm(), 'f', 2, 64),
		})
	}
	escritor.Flush()
	return escritor.Error()
}

func codificar(r models.Respuesta) []byte {
	bytesRespuesta, _ := json.Marshal(r)
	return bytesRespuesta
}

// atender procesa una solicitud del cliente y devuelve la respuesta en bytes.
func atender(solicitud []byte) []byte {
	inicio := time.Now()

	var sol models.Solicitud
	if err := json.Unmarshal(solicitud, &sol); err != nil {
		return codificar(models.Respuesta{Codigo: models.CodError, Mensaje: "solicitud mal formada: " + err.Error()})
	}

	// En la Entrega 1 solo se aceptan consultas y reservas.
	if sol.Operacion != models.OpConsulta && sol.Operacion != models.OpReserva {
		mensaje := fmt.Sprintf("operación %q no soportada en la Entrega 1", sol.Operacion)
		if sol.Operacion == models.OpCancelacion {
			mensaje = "la cancelación (canal asíncrono) se implementa en la Entrega 2"
		}
		return codificar(models.Respuesta{IDSolicitud: sol.IDSolicitud, Codigo: models.CodError, Mensaje: mensaje})
	}

	// Canal síncrono: se reenvía la solicitud tal cual al RMC y se espera la respuesta.
	bytesRespuesta, err := rmc.Enviar(solicitud)
	var respuesta models.Respuesta
	if err != nil {
		respuesta = models.Respuesta{IDSolicitud: sol.IDSolicitud, Codigo: models.CodError, Mensaje: "RMC no disponible: " + err.Error()}
		bytesRespuesta = codificar(respuesta)
	} else if err := json.Unmarshal(bytesRespuesta, &respuesta); err != nil {
		respuesta = models.Respuesta{IDSolicitud: sol.IDSolicitud, Codigo: models.CodError, Mensaje: "respuesta ilegible del RMC"}
		bytesRespuesta = codificar(respuesta)
	}

	duracion := time.Since(inicio)
	registrarTransaccion(sol.Operacion, respuesta.Codigo, duracion)
	if !silencioso {
		log.Printf("%-8s %-10s -> %-9s %6.1f ms  %s", sol.Operacion, sol.IDSolicitud, respuesta.Codigo,
			float64(duracion)/float64(time.Millisecond), respuesta.Mensaje)
	}
	return bytesRespuesta
}

func main() {
	bind := flag.String("bind", "tcp://*:9900", "dirección donde escucha a los clientes (REP, canal síncrono)")
	direccionRMC := flag.String("rmc", "tcp://127.0.0.1:9902", "dirección del servicio RMC (REQ)")
	timeout := flag.Duration("timeout", 10*time.Second, "espera máxima al RMC")
	reporteCada := flag.Duration("reporte-cada", 15*time.Second, "cada cuánto imprime las métricas (0 = nunca)")
	archivoMetricas := flag.String("metricas", "metricas_gt.csv", "CSV con las métricas finales (vacío = no escribe)")
	q := flag.Bool("q", false, "silencioso: no imprime cada transacción")
	flag.Parse()
	silencioso = *q
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.SetPrefix("[GT] ")

	rmc = mqutil.NuevoRequester(*direccionRMC, *timeout)

	socket := zmq4.NewRep(context.Background())
	if err := socket.Listen(*bind); err != nil {
		log.Fatalf("no se pudo escuchar en %s: %v", *bind, err)
	}
	log.Printf("escuchando en %s (REP, canal síncrono) | RMC en %s (timeout %s)", *bind, *direccionRMC, *timeout)

	// Con Ctrl+C: mostrar las métricas finales, guardarlas y salir.
	senal := make(chan os.Signal, 1)
	signal.Notify(senal, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-senal
		imprimirMetricas("MÉTRICAS FINALES")
		if *archivoMetricas != "" {
			if err := escribirCSV(*archivoMetricas); err != nil {
				log.Printf("no se pudo escribir %s: %v", *archivoMetricas, err)
			} else {
				log.Printf("métricas guardadas en %s", *archivoMetricas)
			}
		}
		os.Exit(0)
	}()

	// Imprimir las métricas cada cierto tiempo.
	if *reporteCada > 0 {
		go func() {
			for {
				time.Sleep(*reporteCada)
				imprimirMetricas("MÉTRICAS")
			}
		}()
	}

	// Atender solicitudes una a la vez (REQ/REP).
	for {
		mensaje, err := socket.Recv()
		if err != nil {
			log.Printf("error al recibir: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var solicitud []byte
		if len(mensaje.Frames) > 0 {
			solicitud = mensaje.Frames[0]
		}
		if err := socket.Send(zmq4.NewMsg(atender(solicitud))); err != nil {
			log.Printf("error al responder: %v", err)
		}
	}
}
