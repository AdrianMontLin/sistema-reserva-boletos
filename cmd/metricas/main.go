// Junta los CSV que genera cmd/cliente de uno o varios nodos y calcula, por
// operación, las métricas de la Tabla 1: tiempo promedio de respuesta,
// desviación estándar y transacciones procesadas por minuto.
//
// Uso: ./bin/metricas resultados_N1.csv resultados_N3.csv
//
//	./bin/metricas -ventana 2m resultados/*.csv

package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/metricas"
)

func main() {
	ventana := flag.Duration("ventana", 0, "ventana de medición para el TPM (0 = de la primera a la última solicitud)")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "uso: metricas [-ventana 2m] archivo.csv [archivo2.csv ...]")
		os.Exit(1)
	}

	var todos []metricas.Registro
	for _, ruta := range flag.Args() {
		archivo, err := os.Open(ruta)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		registros, err := metricas.LeerCSV(archivo)
		archivo.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error en %s: %v\n", ruta, err)
			os.Exit(1)
		}
		todos = append(todos, registros...)
	}

	clientes := map[string]bool{}
	for _, r := range todos {
		clientes[r.Cliente] = true
	}
	v := *ventana
	if v <= 0 {
		v = metricas.Ventana(todos)
	}
	fmt.Printf("Archivos: %d | clientes: %d | solicitudes: %d | ventana: %s\n\n",
		flag.NArg(), len(clientes), len(todos), v.Round(time.Millisecond))

	tabla := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tabla, "Operación\tProcesadas\tRechazadas\tErrores\tMedia (ms)\tDesv. est. (ms)\tMín (ms)\tMáx (ms)\tTPM")
	for _, f := range metricas.Resumir(todos, v) {
		fmt.Fprintf(tabla, "%s\t%d\t%d\t%d\t%.2f\t%.2f\t%.2f\t%.2f\t%.1f\n",
			f.Operacion, f.Procesadas, f.Rechazadas, f.Errores, f.MediaMs, f.DesvMs, f.MinMs, f.MaxMs, f.TPM)
	}
	tabla.Flush()
}
