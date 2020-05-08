package main

import (
	"log"
	"net"
	"time"

	pb "github.com/gravitational/oom"
	"google.golang.org/grpc"
)

const (
	port = ":50051"
)

// server is used to implement helloworld.GreeterServer.
type server struct {
}

// SayHello implements helloworld.GreeterServer
func (s *server) CreateStream(stream pb.OOM_CreateStreamServer) error {
	defer stream.SendAndClose(&pb.Response{})
	for {
		time.Sleep(time.Second)
		request, err := stream.Recv()
		if err != nil {
			return err
		}
		log.Printf("Got message: %v\n", request.Data)
	}
}

func main() {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterOOMServer(s, &server{})
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
