// Package mqutil tiene lo que comparten los programas que hablan por ZeroMQ.

package mqutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	zmq4 "github.com/go-zeromq/zmq4"
)

// ErrTimeout se devuelve cuando el otro lado no responde a tiempo. Así es como
// el sistema detecta que un componente falló (modelo de fallos).
var ErrTimeout = errors.New("tiempo de espera agotado")

// Requester manda solicitudes REQ/REP a una dirección y espera la respuesta
// con un tiempo límite. Cada Requester lo usa una sola goroutine a la vez.
//
// Si hay un timeout o un error, se cierra el socket y se crea uno nuevo en la
// siguiente solicitud: un socket REQ queda bloqueado esperando una respuesta
// que nunca llegó, así que no se puede seguir usando.
type Requester struct {
	direccion string
	timeout   time.Duration
	socket    zmq4.Socket
	cancelar  context.CancelFunc
}

// NuevoRequester crea un Requester. La conexión se hace en la primera solicitud.
func NuevoRequester(direccion string, timeout time.Duration) *Requester {
	return &Requester{direccion: direccion, timeout: timeout}
}

// conectar crea el socket y se conecta; intenta hasta 3 veces.
func (r *Requester) conectar() error {
	var ultimoError error
	for intento := 1; intento <= 3; intento++ {
		ctx, cancelar := context.WithCancel(context.Background())
		socket := zmq4.NewReq(ctx, zmq4.WithDialerRetry(200*time.Millisecond), zmq4.WithDialerMaxRetries(3))
		err := socket.Dial(r.direccion)
		if err == nil {
			r.socket = socket
			r.cancelar = cancelar
			return nil
		}
		ultimoError = err
		socket.Close()
		cancelar()
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("no se pudo conectar a %s: %w", r.direccion, ultimoError)
}

// Cerrar cierra el socket (si estaba abierto).
func (r *Requester) Cerrar() {
	if r.socket != nil {
		r.socket.Close()
	}
	if r.cancelar != nil {
		r.cancelar()
	}
	r.socket = nil
	r.cancelar = nil
}

// Enviar manda los datos y devuelve la respuesta.
func (r *Requester) Enviar(datos []byte) ([]byte, error) {
	if r.socket == nil {
		if err := r.conectar(); err != nil {
			return nil, err
		}
	}
	socket := r.socket

	if err := socket.Send(zmq4.NewMsg(datos)); err != nil {
		r.Cerrar()
		return nil, fmt.Errorf("enviar a %s: %w", r.direccion, err)
	}

	// Recv se queda bloqueado hasta que llegue la respuesta, así que lo
	// hacemos en otra goroutine para poder aplicar el tiempo límite.
	type resultado struct {
		mensaje zmq4.Msg
		err     error
	}
	canal := make(chan resultado, 1)
	go func() {
		mensaje, err := socket.Recv()
		canal <- resultado{mensaje, err}
	}()

	select {
	case res := <-canal:
		if res.err != nil {
			r.Cerrar()
			return nil, fmt.Errorf("recibir de %s: %w", r.direccion, res.err)
		}
		if len(res.mensaje.Frames) == 0 {
			return nil, fmt.Errorf("respuesta vacía de %s", r.direccion)
		}
		return res.mensaje.Frames[0], nil
	case <-time.After(r.timeout):
		r.Cerrar()
		return nil, fmt.Errorf("%w (%s, %s)", ErrTimeout, r.direccion, r.timeout)
	}
}
