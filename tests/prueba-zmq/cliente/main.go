package main

import (
	"context"
	"fmt"
	"log"

	zmq4 "github.com/go-zeromq/zmq4"
)

func main() {
	ctx := context.Background()
	socket := zmq4.NewReq(ctx)
	defer socket.Close()

	err := socket.Dial("tcp://127.0.0.1:9999")
	if err != nil {
		log.Fatalf("error al conectar: %v", err)
	}

	err = socket.Send(zmq4.NewMsgString("Hola desde el cliente!"))
	if err != nil {
		log.Fatalf("error al enviar: %v", err)
	}

	msg, err := socket.Recv()
	if err != nil {
		log.Fatalf("error al recibir respuesta: %v", err)
	}
	fmt.Printf("Respuesta del servidor: %s\n", string(msg.Frames[0]))
}
