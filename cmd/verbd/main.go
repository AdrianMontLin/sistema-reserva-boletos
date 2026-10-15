// Muestra el estado de la base de datos a partir del archivo JSON que escribe
// la Persistencia. Sirve en la sustentación para ver cómo cambia la BD.
//
// Uso: ./bin/verbd data/srb_db.json              (todas las ocurrencias)
//
//	./bin/verbd -ocupadas data/srb_db.json   (solo las que tienen reservas)
//	./bin/verbd -reservas data/srb_db.json   (lista de reservas)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/models"
)

func main() {
	soloOcupadas := flag.Bool("ocupadas", false, "mostrar solo las ocurrencias con asientos ocupados")
	verReservas := flag.Bool("reservas", false, "mostrar la lista de reservas")
	flag.Parse()

	ruta := "data/srb_db.json"
	if flag.NArg() > 0 {
		ruta = flag.Arg(0)
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	var bd models.BaseDeDatos
	if err := json.Unmarshal(datos, &bd); err != nil {
		fmt.Fprintf(os.Stderr, "el archivo %s no es válido: %v\n", ruta, err)
		os.Exit(1)
	}

	ocurrencias, libres, ocupados := 0, 0, 0
	for _, evento := range bd.Eventos {
		for _, oc := range evento.Ocurrencias {
			ocurrencias++
			libres += oc.AsientosLibres
			ocupados += oc.AsientosOcupados
		}
	}
	fmt.Printf("Guardado: %s\n", bd.Guardado)
	fmt.Printf("%d eventos | %d ocurrencias | asientos: %d libres + %d ocupados = %d | %d reservas\n\n",
		len(bd.Eventos), ocurrencias, libres, ocupados, libres+ocupados, len(bd.Reservas))

	tabla := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tabla.Flush()
	if *verReservas {
		fmt.Fprintln(tabla, "Reserva\tOcurrencia\tCliente\tAsientos\tEstado")
		for _, r := range bd.Reservas {
			fmt.Fprintf(tabla, "%s\t%s\t%s\t%d\t%s\n", r.ID, r.IDOcurrencia, r.IDCliente, r.NumAsientos, r.Estado)
		}
		return
	}
	fmt.Fprintln(tabla, "Evento\tOcurrencia\tFecha\tLibres\tOcupados")
	for _, evento := range bd.Eventos {
		for _, oc := range evento.Ocurrencias {
			if *soloOcupadas && oc.AsientosOcupados == 0 {
				continue
			}
			fmt.Fprintf(tabla, "%s\t%s\t%s\t%d\t%d\n", evento.Nombre, oc.ID, oc.Fecha, oc.AsientosLibres, oc.AsientosOcupados)
		}
	}
}
