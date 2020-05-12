package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"golang.org/x/net/http2"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gravitational/oom/multiplexer"

	pb "github.com/gravitational/oom"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	port    = ":10000"
	address = "localhost:10000"
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
		log.Printf("Got message: %v\n", request.GetRequest().Data)
	}
}

func main() {
	switch os.Args[1] {
	case "http2":
		mainHTTP()
	case "grpc":
		mainGRPC()
	case "grpcmux":
		mainGRPCMux()
	default:
		panic(fmt.Errorf("unsupported arg: %v", os.Args[1]))
	}
}

// mainGRPC is a straightforward GRPC TLS listener using
// grpc implementation of HTTP/2
func mainGRPC() {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	cert := pb.Certificate()
	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewServerTLSFromCert(&cert))}
	s := grpc.NewServer(opts...)
	pb.RegisterOOMServer(s, &server{})
	fmt.Printf("grpc with native transport (no multiplexing) on port: %v\n", port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

type adapter struct {
	grpcServer *grpc.Server
}

func (a *adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
		a.grpcServer.ServeHTTP(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// mainHTTP uses grpc ServeHTTP compatibiltiy handler that allows
// to use GRPC and HTTP/2 on the same listener, however it has
// problems with flow control (see README.md)
func mainHTTP() {
	adapter := &adapter{
		grpcServer: grpc.NewServer(),
	}
	pb.RegisterOOMServer(adapter.grpcServer, &server{})
	srv := &http.Server{
		Addr:    port,
		Handler: adapter,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{pb.Certificate()},
			NextProtos:   []string{"h2", "http/1.1"},
		},
	}
	conn, err := net.Listen("tcp", port)
	if err != nil {
		panic(err)
	}
	fmt.Printf("grpc with golang standard library http/2 transport on port: %v\n", port)
	if err := srv.Serve(tls.NewListener(conn, srv.TLSConfig)); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

// mainGRPCMux multiplexes GRPC and HTTP1/1 using detection
// after tls.Listener Accept negotiated a handshake
// and the router looks at the NextNegotiatedProtocol section
func mainGRPCMux() {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	mux, err := multiplexer.New(multiplexer.Config{
		Listener:   lis,
		DisableSSH: true,
	})
	go mux.Serve()
	if err != nil {
		panic(err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{pb.Certificate()},
		NextProtos:   []string{http2.NextProtoTLS, "http/1.1"},
	}

	tlsLis := multiplexer.NewTLSNextProtoListener(
		tls.NewListener(mux.TLS(), config))
	go tlsLis.Serve()

	opts := []grpc.ServerOption{
		grpc.Creds(&tlsCreds{
			config: config,
		})}
	s := grpc.NewServer(opts...)
	pb.RegisterOOMServer(s, &server{})
	fmt.Printf("grpc with native transport and multiplexing on port: %v\n", port)

	go func() {
		if err := s.Serve(tlsLis.HTTP2()); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		if err := http.Serve(tlsLis.HTTP(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	mux.Wait()
}

// tlsCreds is the credentials required for authenticating a connection using TLS.
type tlsCreds struct {
	// TLS configuration
	config *tls.Config
}

func (c tlsCreds) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{
		SecurityProtocol: "tls",
		SecurityVersion:  "1.2",
		ServerName:       c.config.ServerName,
	}
}

func (c *tlsCreds) ClientHandshake(ctx context.Context, authority string, rawConn net.Conn) (_ net.Conn, _ credentials.AuthInfo, err error) {
	return nil, nil, errors.New("not supported")
}

func (c *tlsCreds) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	tlsConn, ok := rawConn.(*tls.Conn)
	if !ok {
		return nil, nil, errors.New("expected TLS connection")
	}
	return WrapSyscallConn(rawConn, tlsConn), credentials.TLSInfo{tlsConn.ConnectionState()}, nil
}

func (c *tlsCreds) Clone() credentials.TransportCredentials {
	return &tlsCreds{
		config: c.config.Clone(),
	}
}

func (c *tlsCreds) OverrideServerName(serverNameOverride string) error {
	c.config.ServerName = serverNameOverride
	return nil
}

type sysConn = syscall.Conn

// syscallConn keeps reference of rawConn to support syscall.Conn for channelz.
// SyscallConn() (the method in interface syscall.Conn) is explicitly
// implemented on this type,
//
// Interface syscall.Conn is implemented by most net.Conn implementations (e.g.
// TCPConn, UnixConn), but is not part of net.Conn interface. So wrapper conns
// that embed net.Conn don't implement syscall.Conn. (Side note: tls.Conn
// doesn't embed net.Conn, so even if syscall.Conn is part of net.Conn, it won't
// help here).
type syscallConn struct {
	net.Conn
	// sysConn is a type alias of syscall.Conn. It's necessary because the name
	// `Conn` collides with `net.Conn`.
	sysConn
}

// WrapSyscallConn tries to wrap rawConn and newConn into a net.Conn that
// implements syscall.Conn. rawConn will be used to support syscall, and newConn
// will be used for read/write.
//
// This function returns newConn if rawConn doesn't implement syscall.Conn.
func WrapSyscallConn(rawConn, newConn net.Conn) net.Conn {
	sysConn, ok := rawConn.(syscall.Conn)
	if !ok {
		return newConn
	}
	return &syscallConn{
		Conn:    newConn,
		sysConn: sysConn,
	}
}
