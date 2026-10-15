// Package models define las estructuras de datos que usan todos los programas:
// los eventos, las reservas y los mensajes que viajan por ZeroMQ (en formato JSON).

package models

// ---------- Datos del negocio ----------

// Evento es un evento cultural o deportivo del mes. Puede tener una o dos
// ocurrencias (fechas) dentro del mismo mes.
type Evento struct {
	ID          string       `json:"id"` // ej: "E1"
	Nombre      string       `json:"nombre"`
	Ocurrencias []Ocurrencia `json:"ocurrencias"`
}

// Ocurrencia es una fecha concreta de un evento. Los asientos se controlan
// por ocurrencia: libres + ocupados = totales.
type Ocurrencia struct {
	ID               string `json:"id"`    // ej: "E1-1"
	Fecha            string `json:"fecha"` // formato AAAA-MM-DD
	AsientosTotales  int    `json:"asientos_totales"`
	AsientosLibres   int    `json:"asientos_libres"`
	AsientosOcupados int    `json:"asientos_ocupados"`
}

// Reserva son los puestos que un cliente reservó para una ocurrencia.
type Reserva struct {
	ID           string `json:"id"`
	IDEvento     string `json:"id_evento"`
	IDOcurrencia string `json:"id_ocurrencia"`
	IDCliente    string `json:"id_cliente"`
	NumAsientos  int    `json:"num_asientos"`
	Estado       string `json:"estado"`
}

// EstadoConfirmada es el estado de una reserva recién hecha.
const EstadoConfirmada = "confirmada"

// BaseDeDatos es lo que la persistencia guarda en el archivo JSON.
type BaseDeDatos struct {
	Guardado         string    `json:"guardado"`
	SiguienteReserva int       `json:"siguiente_reserva"`
	Eventos          []Evento  `json:"eventos"`
	Reservas         []Reserva `json:"reservas"`
}

// ---------- Mensajes Cliente -> GT -> RMC ----------

// Operaciones que puede pedir un cliente.
const (
	OpConsulta     = "consulta"
	OpReserva      = "reserva"
	OpModificacion = "modificacion" // Entrega 2
	OpCancelacion  = "cancelacion"  // Entrega 2
)

// Códigos de resultado:
//   - OK: la operación se hizo.
//   - RECHAZADA: el sistema funcionó pero una regla del negocio lo impide (ej. no hay cupo).
//   - ERROR: falló algo del sistema (servicio caído, timeout, operación no soportada).
const (
	CodOK        = "OK"
	CodRechazada = "RECHAZADA"
	CodError     = "ERROR"
)

// Solicitud es el mensaje que envía el cliente. Según la operación se llenan
// unos campos u otros: Mes para consultas; IDEvento, IDOcurrencia y
// NumAsientos para reservas.
type Solicitud struct {
	IDSolicitud  string `json:"id_solicitud"` // único por solicitud, sirve para rastrearla en los logs
	Operacion    string `json:"operacion"`
	IDCliente    string `json:"id_cliente"`
	Mes          string `json:"mes,omitempty"` // AAAA-MM
	IDEvento     string `json:"id_evento,omitempty"`
	IDOcurrencia string `json:"id_ocurrencia,omitempty"`
	NumAsientos  int    `json:"num_asientos,omitempty"`
}

// Respuesta es lo que recibe el cliente.
type Respuesta struct {
	IDSolicitud string   `json:"id_solicitud,omitempty"`
	Codigo      string   `json:"codigo"`
	Mensaje     string   `json:"mensaje,omitempty"`
	Eventos     []Evento `json:"eventos,omitempty"` // en las consultas
	Reserva     *Reserva `json:"reserva,omitempty"` // en las reservas
}

// ---------- Mensajes RMC -> Persistencia ----------

// Acciones que el RMC le pide a la persistencia.
const (
	AccionConsultarMes   = "consultar_mes"
	AccionDisponibilidad = "disponibilidad"
	AccionReservar       = "reservar"
)

// SolicitudPersistencia es lo que el RMC envía a la persistencia.
type SolicitudPersistencia struct {
	IDSolicitud  string `json:"id_solicitud,omitempty"`
	Accion       string `json:"accion"`
	Mes          string `json:"mes,omitempty"`
	IDEvento     string `json:"id_evento,omitempty"`
	IDOcurrencia string `json:"id_ocurrencia,omitempty"`
	IDCliente    string `json:"id_cliente,omitempty"`
	NumAsientos  int    `json:"num_asientos,omitempty"`
}

// RespuestaPersistencia es lo que la persistencia le devuelve al RMC.
type RespuestaPersistencia struct {
	Codigo      string   `json:"codigo"`
	Mensaje     string   `json:"mensaje,omitempty"`
	Eventos     []Evento `json:"eventos,omitempty"`
	Reserva     *Reserva `json:"reserva,omitempty"`
	Disponibles int      `json:"disponibles"`
}
