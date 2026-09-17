package main

import (
	"context"
	"fmt"
	"log"

	zmq4 "github.com/go-zeromq/zmq4"
)

func main() {
	ctx := context.Background()
	socket := zmq4.NewRep(ctx)
	defer socket.Close()

	err := socket.Listen("tcp://*:9999")
	if err != nil {
		log.Fatalf("error al escuchar: %v", err)
	}
	fmt.Println("Servidor de prueba escuchando en el puerto 9999...")

	for {
		msg, err := socket.Recv()
		if err != nil {
			log.Fatalf("error al recibir: %v", err)
		}
		fmt.Printf("Mensaje recibido: %s\n", string(msg.Frames[0]))

		err = socket.Send(zmq4.NewMsgString("Hola desde el servidor!"))
		if err != nil {
			log.Fatalf("error al responder: %v", err)
		}
	}
}
