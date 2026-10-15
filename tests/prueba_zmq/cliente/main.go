// Prueba de humo de ZeroMQ: cliente REQ que se conecta al servidor de prueba.

package main

import (
	"context"
	"fmt"
	"log"

	zmq4 "github.com/go-zeromq/zmq4"
)

func main() {
	socket := zmq4.NewReq(context.Background())
	defer socket.Close()

	if err := socket.Dial("tcp://127.0.0.1:9999"); err != nil {
		log.Fatalf("error al conectar: %v", err)
	}
	if err := socket.Send(zmq4.NewMsgString("Hola desde el cliente!")); err != nil {
		log.Fatalf("error al enviar: %v", err)
	}
	msg, err := socket.Recv()
	if err != nil {
		log.Fatalf("error al recibir respuesta: %v", err)
	}
	fmt.Printf("Respuesta del servidor: %s\n", string(msg.Frames[0]))
}
