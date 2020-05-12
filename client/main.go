package main

import (
	"context"
	"log"
	"strings"

	pb "github.com/gravitational/oom"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	address     = "localhost:10000"
	defaultName = "world"
)

func main() {
	creds := credentials.NewClientTLSFromCert(pb.CertPool(), address)

	// Set up a connection to the server.
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewOOMClient(conn)

	stream, err := client.CreateStream(context.Background())
	if err != nil {
		log.Fatalf("Could not create stream: %v", err)
	}
	data := strings.Repeat("hello", 10)
	i := 0
	for {
		i++
		err := stream.Send(&pb.Wrapper{Req: &pb.Wrapper_Request{Request: &pb.Request{Data: data}}})
		if err != nil {
			log.Fatalf("Failed to send: %v", err)
		}
		if i%1000 == 0 {
			log.Printf("Sent %v items", i)
		}
	}

	go stream.RecvMsg(&pb.Response{})
}
