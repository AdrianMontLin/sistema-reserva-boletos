// Servicio RMC: Reserva, Modificación y Consulta (Nodo 2).
//
// Atiende las solicitudes síncronas del Gestor de Transacciones (REP, puerto
// 9902 por defecto) y pregunta o actualiza los datos en la Persistencia (REQ).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	zmq4 "github.com/go-zeromq/zmq4"

	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/models"
	"github.com/AdrianMontLin/sistema-reserva-boletos/internal/mqutil"
)

var (
	persistencia *mqutil.Requester // conexión con la persistencia
	silencioso   bool
)

func registrar(formato string, args ...any) {
	if !silencioso {
		log.Printf(formato, args...)
	}
}

// preguntarAPersistencia manda una solicitud a la persistencia y devuelve su respuesta.
func preguntarAPersistencia(sol models.SolicitudPersistencia) (models.RespuestaPersistencia, error) {
	var respuesta models.RespuestaPersistencia
	datos, err := json.Marshal(sol)
	if err != nil {
		return respuesta, err
	}
	bytesRespuesta, err := persistencia.Enviar(datos)
	if err != nil {
		return respuesta, err
	}
	if err := json.Unmarshal(bytesRespuesta, &respuesta); err != nil {
		return respuesta, fmt.Errorf("respuesta ilegible de la persistencia: %v", err)
	}
	return respuesta, nil
}

// errorDePersistencia es la respuesta cuando no se pudo hablar con la persistencia.
func errorDePersistencia(sol models.Solicitud, err error) models.Respuesta {
	return models.Respuesta{
		IDSolicitud: sol.IDSolicitud,
		Codigo:      models.CodError,
		Mensaje:     "servicio de persistencia no disponible: " + err.Error(),
	}
}

// consultar recibe la consulta, la envía a la persistencia y devuelve el resultado.
func consultar(sol models.Solicitud) models.Respuesta {
	resp, err := preguntarAPersistencia(models.SolicitudPersistencia{
		IDSolicitud: sol.IDSolicitud, Accion: models.AccionConsultarMes, Mes: sol.Mes,
	})
	if err != nil {
		registrar("consulta %s (%s) -> ERROR: %v", sol.Mes, sol.IDCliente, err)
		return errorDePersistencia(sol, err)
	}
	registrar("consulta %s (%s) -> %s, %d eventos", sol.Mes, sol.IDCliente, resp.Codigo, len(resp.Eventos))
	return models.Respuesta{IDSolicitud: sol.IDSolicitud, Codigo: resp.Codigo, Mensaje: resp.Mensaje, Eventos: resp.Eventos}
}

// reservar sigue los pasos del enunciado:
//  1. consulta a la persistencia la disponibilidad en tiempo real,
//  2. si hay asientos, confirma la reserva en la base de datos,
//  3. responde el resultado (OK o RECHAZADA).
func reservar(sol models.Solicitud) models.Respuesta {
	if sol.IDEvento == "" || sol.IDOcurrencia == "" || sol.NumAsientos <= 0 {
		return models.Respuesta{IDSolicitud: sol.IDSolicitud, Codigo: models.CodRechazada,
			Mensaje: "reserva incompleta: se requieren id_evento, id_ocurrencia y num_asientos > 0"}
	}

	// Paso 1: ¿hay cupo?
	disp, err := preguntarAPersistencia(models.SolicitudPersistencia{
		IDSolicitud: sol.IDSolicitud, Accion: models.AccionDisponibilidad,
		IDEvento: sol.IDEvento, IDOcurrencia: sol.IDOcurrencia,
	})
	if err != nil {
		registrar("reserva %s x%d (%s) -> ERROR: %v", sol.IDOcurrencia, sol.NumAsientos, sol.IDCliente, err)
		return errorDePersistencia(sol, err)
	}
	if disp.Codigo != models.CodOK {
		registrar("reserva %s x%d (%s) -> %s: %s", sol.IDOcurrencia, sol.NumAsientos, sol.IDCliente, disp.Codigo, disp.Mensaje)
		return models.Respuesta{IDSolicitud: sol.IDSolicitud, Codigo: disp.Codigo, Mensaje: disp.Mensaje}
	}
	if disp.Disponibles < sol.NumAsientos {
		mensaje := fmt.Sprintf("no hay asientos suficientes: disponibles %d, solicitados %d", disp.Disponibles, sol.NumAsientos)
		registrar("reserva %s x%d (%s) -> RECHAZADA: %s", sol.IDOcurrencia, sol.NumAsientos, sol.IDCliente, mensaje)
		return models.Respuesta{IDSolicitud: sol.IDSolicitud, Codigo: models.CodRechazada, Mensaje: mensaje}
	}

	// Paso 2: confirmar la reserva. La persistencia vuelve a comprobar el cupo,
	// por si otro cliente se llevó los últimos asientos entre el paso 1 y este.
	res, err := preguntarAPersistencia(models.SolicitudPersistencia{
		IDSolicitud: sol.IDSolicitud, Accion: models.AccionReservar,
		IDEvento: sol.IDEvento, IDOcurrencia: sol.IDOcurrencia, IDCliente: sol.IDCliente, NumAsientos: sol.NumAsientos,
	})
	if err != nil {
		registrar("reserva %s x%d (%s) -> ERROR: %v", sol.IDOcurrencia, sol.NumAsientos, sol.IDCliente, err)
		return errorDePersistencia(sol, err)
	}
	registrar("reserva %s x%d (%s) -> %s", sol.IDOcurrencia, sol.NumAsientos, sol.IDCliente, res.Codigo)
	return models.Respuesta{IDSolicitud: sol.IDSolicitud, Codigo: res.Codigo, Mensaje: res.Mensaje, Reserva: res.Reserva}
}

// atender decide qué hacer según la operación pedida.
func atender(datos []byte) models.Respuesta {
	var sol models.Solicitud
	if err := json.Unmarshal(datos, &sol); err != nil {
		return models.Respuesta{Codigo: models.CodError, Mensaje: "solicitud mal formada: " + err.Error()}
	}
	switch sol.Operacion {
	case models.OpConsulta:
		return consultar(sol)
	case models.OpReserva:
		return reservar(sol)
	default:
		return models.Respuesta{IDSolicitud: sol.IDSolicitud, Codigo: models.CodError,
			Mensaje: fmt.Sprintf("operación %q no soportada por el RMC en la Entrega 1", sol.Operacion)}
	}
}

func main() {
	bind := flag.String("bind", "tcp://*:9902", "dirección donde escucha al GT (REP)")
	direccionPersistencia := flag.String("persistencia", "tcp://127.0.0.1:9904", "dirección de la persistencia (REQ)")
	timeout := flag.Duration("timeout", 5*time.Second, "espera máxima a la persistencia")
	q := flag.Bool("q", false, "silencioso: no imprime cada operación")
	flag.Parse()
	silencioso = *q
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.SetPrefix("[RMC] ")

	persistencia = mqutil.NuevoRequester(*direccionPersistencia, *timeout)

	socket := zmq4.NewRep(context.Background())
	if err := socket.Listen(*bind); err != nil {
		log.Fatalf("no se pudo escuchar en %s: %v", *bind, err)
	}
	log.Printf("escuchando en %s (REP) | persistencia en %s (timeout %s)", *bind, *direccionPersistencia, *timeout)

	// Atender solicitudes una a la vez (REQ/REP).
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
