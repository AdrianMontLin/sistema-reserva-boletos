// Prueba de humo de ZeroMQ: servidor REP en el puerto 9999.

package main

import (
	"context"
	"fmt"
	"log"

	zmq4 "github.com/go-zeromq/zmq4"
)

func main() {
	socket := zmq4.NewRep(context.Background())
	defer socket.Close()

	if err := socket.Listen("tcp://*:9999"); err != nil {
		log.Fatalf("error al escuchar: %v", err)
	}
	fmt.Println("Servidor de prueba escuchando en el puerto 9999...")

	for {
		msg, err := socket.Recv()
		if err != nil {
			log.Fatalf("error al recibir: %v", err)
		}
		fmt.Printf("Mensaje recibido: %s\n", string(msg.Frames[0]))
		if err := socket.Send(zmq4.NewMsgString("Hola desde el servidor!")); err != nil {
			log.Fatalf("error al responder: %v", err)
		}
	}
}
