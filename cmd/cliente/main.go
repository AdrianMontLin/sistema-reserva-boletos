// Cliente y generador de carga (Nodo 1 y Nodo 3).
//
// Simula varios clientes al mismo tiempo (cada uno es una goroutine con su
// propio socket REQ). Cada cliente envía consultas y reservas al Gestor de
// Transacciones a un ritmo fijo, mide cuánto tarda cada respuesta y al final
// se muestra el resumen (media, desviación estándar y TPM).
//
// Con -interactivo se pueden enviar solicitudes a mano.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/metricas"
	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/models"
	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/mqutil"
)

// Parámetros de la línea de comandos.
var (
	gt           = flag.String("gt", "tcp://127.0.0.1:9900", "dirección del Gestor de Transacciones")
	origen       = flag.String("id", "N1", "identifica el origen de los clientes (N1 = Nodo 1, N3 = Nodo 3)")
	clientes     = flag.Int("clientes", 1, "número de clientes simultáneos")
	soloConsulta = flag.Int("solo-consulta", 0, "cuántos de esos clientes hacen únicamente consultas")
	rpm          = flag.Float64("rpm", 20, "solicitudes por minuto por cliente (el enunciado pide al menos 20)")
	pctConsulta  = flag.Int("pct-consulta", 70, "porcentaje de consultas de los clientes mixtos (el resto son reservas)")
	duracion     = flag.Duration("duracion", 2*time.Minute, "duración de la prueba")
	mes          = flag.String("mes", "2026-10", "mes a consultar (AAAA-MM)")
	maxAsientos  = flag.Int("max-asientos", 4, "cada reserva pide entre 1 y este número de puestos")
	timeout      = flag.Duration("timeout", 15*time.Second, "espera máxima por respuesta")
	semilla      = flag.Int64("semilla", 0, "semilla aleatoria (0 = aleatoria)")
	salida       = flag.String("salida", "resultados", "prefijo del CSV con el detalle por solicitud (vacío = no escribe)")
	silencioso   = flag.Bool("q", false, "silencioso: no imprime cada solicitud")
	interactivo  = flag.Bool("interactivo", false, "modo interactivo: enviar solicitudes a mano")
)

var (
	registros        []metricas.Registro // resultado de todas las solicitudes
	candadoRegistros sync.Mutex          // protege la lista (la usan todos los clientes)
	detener          atomic.Bool         // se pone en true con Ctrl+C
)

// objetivo es una ocurrencia sobre la que se puede reservar.
type objetivo struct {
	evento     string
	ocurrencia string
	libres     int
}

// construirCatalogo arma la lista de ocurrencias a partir de una consulta.
func construirCatalogo(eventos []models.Evento) []objetivo {
	var catalogo []objetivo
	for _, evento := range eventos {
		for _, oc := range evento.Ocurrencias {
			catalogo = append(catalogo, objetivo{evento.ID, oc.ID, oc.AsientosLibres})
		}
	}
	return catalogo
}

// elegirObjetivo escoge una ocurrencia para reservar. El 90% de las veces
// elige una que todavía tenga cupo, el resto elige cualquiera, para que
// también se vean rechazos.
func elegirObjetivo(azar *rand.Rand, catalogo []objetivo) int {
	if azar.Intn(100) < 90 {
		var conCupo []int
		for i, o := range catalogo {
			if o.libres > 0 {
				conCupo = append(conCupo, i)
			}
		}
		if len(conCupo) > 0 {
			return conCupo[azar.Intn(len(conCupo))]
		}
	}
	return azar.Intn(len(catalogo))
}

// correrCliente es lo que hace cada cliente simulado hasta que se acaba el tiempo.
func correrCliente(numero int, fin time.Time) {
	id := fmt.Sprintf("%s-C%d", *origen, numero)
	esSoloConsulta := numero <= *soloConsulta
	azar := rand.New(rand.NewSource(*semilla + int64(numero)*7919))
	requester := mqutil.NuevoRequester(*gt, *timeout)
	defer requester.Cerrar()

	intervalo := time.Duration(float64(time.Minute) / *rpm)
	// Cada cliente empieza en un momento distinto dentro del primer intervalo.
	proxima := time.Now().Add(time.Duration(azar.Int63n(int64(intervalo))))
	var catalogo []objetivo

	for secuencia := 1; ; secuencia++ {
		if espera := time.Until(proxima); espera > 0 {
			time.Sleep(espera)
		}
		if time.Now().After(fin) || detener.Load() {
			return
		}

		// Elegir la operación. Los clientes "solo consulta" nunca reservan.
        // Sin catálogo no se puede reservar.
		operacion := models.OpReserva
		if esSoloConsulta || len(catalogo) == 0 || azar.Intn(100) < *pctConsulta {
			operacion = models.OpConsulta
		}
		sol := models.Solicitud{
			IDSolicitud: fmt.Sprintf("%s-%d", id, secuencia),
			Operacion:   operacion,
			IDCliente:   id,
		}
		elegido := 0
		if operacion == models.OpConsulta {
			sol.Mes = *mes
		} else {
			elegido = elegirObjetivo(azar, catalogo)
			sol.IDEvento = catalogo[elegido].evento
			sol.IDOcurrencia = catalogo[elegido].ocurrencia
			sol.NumAsientos = 1 + azar.Intn(*maxAsientos)
		}

		// Enviar y medir cuánto tarda la respuesta de extremo a extremo.
		datos, _ := json.Marshal(sol)
		inicio := time.Now()
		bytesRespuesta, err := requester.Enviar(datos)
		latencia := time.Since(inicio)

		registro := metricas.Registro{Momento: time.Now(), Cliente: id, Operacion: operacion, Latencia: latencia}
		var respuesta models.Respuesta
		if err != nil {
			registro.Codigo = models.CodError
			registro.Mensaje = err.Error()
		} else if err := json.Unmarshal(bytesRespuesta, &respuesta); err != nil {
			registro.Codigo = models.CodError
			registro.Mensaje = "respuesta ilegible"
		} else {
			registro.Codigo = respuesta.Codigo
			registro.Mensaje = respuesta.Mensaje
			if respuesta.Codigo == models.CodOK {
				if operacion == models.OpConsulta {
					catalogo = construirCatalogo(respuesta.Eventos)
				} else if elegido < len(catalogo) {
					catalogo[elegido].libres -= sol.NumAsientos
				}
			}
		}

		candadoRegistros.Lock()
		registros = append(registros, registro)
		candadoRegistros.Unlock()

		if !*silencioso {
			detalle := fmt.Sprintf("consulta %s (%d eventos)", *mes, len(respuesta.Eventos))
			if operacion == models.OpReserva {
				detalle = fmt.Sprintf("reserva %s x%d", sol.IDOcurrencia, sol.NumAsientos)
			}
			log.Printf("[%s] %-34s -> %-9s %7.1f ms %s", id, detalle, registro.Codigo,
				float64(latencia)/float64(time.Millisecond), registro.Mensaje)
		}

		// Mantener el ritmo, si el sistema fue más lento que el intervalo, no se acumula atraso.
		proxima = proxima.Add(intervalo)
		if ahora := time.Now(); proxima.Before(ahora) {
			proxima = ahora
		}
	}
}

// modoInteractivo permite enviar consultas y reservas escribiendo comandos.
func modoInteractivo() {
	requester := mqutil.NuevoRequester(*gt, *timeout)
	defer requester.Cerrar()
	fmt.Printf("Cliente interactivo %s conectado a %s\n", *origen, *gt)
	fmt.Println("Comandos:  consulta [AAAA-MM]   |   reserva <evento> <ocurrencia> <asientos>   |   salir")
	fmt.Println("Ejemplo:   reserva E1 E1-1 2")

	lector := bufio.NewScanner(os.Stdin)
	secuencia := 0
	for fmt.Print("> "); lector.Scan(); fmt.Print("> ") {
		campos := strings.Fields(lector.Text())
		if len(campos) == 0 {
			continue
		}
		secuencia++
		sol := models.Solicitud{IDSolicitud: fmt.Sprintf("%s-I%d", *origen, secuencia), IDCliente: *origen}

		switch strings.ToLower(campos[0]) {
		case "salir", "exit", "q":
			return
		case "consulta":
			sol.Operacion = models.OpConsulta
			sol.Mes = "2026-10"
			if len(campos) > 1 {
				sol.Mes = campos[1]
			}
		case "reserva":
			if len(campos) != 4 {
				fmt.Println("uso: reserva <evento> <ocurrencia> <asientos>")
				continue
			}
			asientos, err := strconv.Atoi(campos[3])
			if err != nil {
				fmt.Println("los asientos deben ser un número entero")
				continue
			}
			sol.Operacion = models.OpReserva
			sol.IDEvento = campos[1]
			sol.IDOcurrencia = campos[2]
			sol.NumAsientos = asientos
		default:
			fmt.Println("comando desconocido")
			continue
		}

		datos, _ := json.Marshal(sol)
		inicio := time.Now()
		bytesRespuesta, err := requester.Enviar(datos)
		ms := float64(time.Since(inicio)) / float64(time.Millisecond)
		if err != nil {
			fmt.Printf("ERROR de comunicación (%.1f ms): %v\n", ms, err)
			continue
		}
		var respuesta models.Respuesta
		if err := json.Unmarshal(bytesRespuesta, &respuesta); err != nil {
			fmt.Println("respuesta ilegible:", err)
			continue
		}
		fmt.Printf("%s (%.1f ms) %s\n", respuesta.Codigo, ms, respuesta.Mensaje)
		for _, evento := range respuesta.Eventos {
			for _, oc := range evento.Ocurrencias {
				fmt.Printf("  %-4s %-26s %-7s %s  libres=%3d ocupados=%3d\n",
					evento.ID, evento.Nombre, oc.ID, oc.Fecha, oc.AsientosLibres, oc.AsientosOcupados)
			}
		}
		if respuesta.Reserva != nil {
			r := respuesta.Reserva
			fmt.Printf("  reserva %s: %d asiento(s) en %s [%s]\n", r.ID, r.NumAsientos, r.IDOcurrencia, r.Estado)
		}
	}
}

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	if *interactivo {
		modoInteractivo()
		return
	}

	// Revisar los parámetros.
	if *clientes < 1 {
		log.Fatal("clientes debe ser >= 1")
	}
	if *soloConsulta < 0 || *soloConsulta > *clientes {
		log.Fatal("solo-consulta debe estar entre 0 y el número de clientes")
	}
	if *rpm <= 0 {
		log.Fatal("rpm debe ser > 0")
	}
	if *pctConsulta < 0 || *pctConsulta > 100 {
		log.Fatal("pct-consulta debe estar entre 0 y 100")
	}
	if *duracion <= 0 {
		log.Fatal("duracion debe ser > 0")
	}
	if *maxAsientos < 1 {
		*maxAsientos = 4
	}
	if *semilla == 0 {
		*semilla = time.Now().UnixNano()
	}

	// Con Ctrl+C se deja de enviar, pero todavía se muestra el resumen.
	senal := make(chan os.Signal, 1)
	signal.Notify(senal, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-senal
		detener.Store(true)
	}()

	fmt.Printf("Carga: %d clientes (%d solo consulta) x %.0f solicitudes/min durante %s hacia %s\n",
		*clientes, *soloConsulta, *rpm, *duracion, *gt)

	// Lanzar los clientes y esperar a que terminen.
	inicio := time.Now()
	fin := inicio.Add(*duracion)
	var espera sync.WaitGroup
	for numero := 1; numero <= *clientes; numero++ {
		espera.Add(1)
		go func(numero int) {
			defer espera.Done()
			correrCliente(numero, fin)
		}(numero)
	}
	espera.Wait()
	transcurrido := time.Since(inicio)

	// Mostrar el resumen.
	fmt.Printf("\n=== RESUMEN (origen %s, %d clientes, %.0f s) ===\n", *origen, *clientes, transcurrido.Seconds())
	tabla := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tabla, "Operación\tProcesadas\tRechazadas\tErrores\tMedia (ms)\tDesv. est. (ms)\tMín (ms)\tMáx (ms)\tTPM")
	for _, f := range metricas.Resumir(registros, transcurrido) {
		fmt.Fprintf(tabla, "%s\t%d\t%d\t%d\t%.2f\t%.2f\t%.2f\t%.2f\t%.1f\n",
			f.Operacion, f.Procesadas, f.Rechazadas, f.Errores, f.MediaMs, f.DesvMs, f.MinMs, f.MaxMs, f.TPM)
	}
	tabla.Flush()

	// Guardar el detalle de cada solicitud en un CSV.
	if *salida != "" {
		ruta := fmt.Sprintf("%s_%s.csv", *salida, *origen)
		archivo, err := os.Create(ruta)
		if err != nil {
			log.Fatalf("no se pudo crear %s: %v", ruta, err)
		}
		defer archivo.Close()
		if err := metricas.EscribirCSV(archivo, registros); err != nil {
			log.Fatalf("no se pudo escribir %s: %v", ruta, err)
		}
		fmt.Printf("\nDetalle por solicitud guardado en %s\n", ruta)
	}
}
