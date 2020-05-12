## Description

This small program demonstrates mysterious problem I've encountered
while testing streaming GRPC.

Imagine a scenario when grpc client is really fast, but server is slow.
HTTP2 solves this problem by introducing flow control.

* [Client](client/main.go)

However, there is a difference between using native GRPC HTTP2 transport:

* [Native GRPC transport](server/main.go#L61)

Native GRPC HTTP2 transport implementation works, with fast client
you would see flow control kicking in on about 2K items sent:

```bash
$ go run main/server.go grpc
$ go run client/main.go
2020/05/11 20:08:43 Sent 1000 items
2020/05/11 20:08:43 Sent 2000 items
... stops
```


* [HTTP2 transport](server/main.go#L92)

Golang's HTTP2 transport using ServeHTTP adapter is often used
to support both HTTP/1.1 and HTTP2 + GRPC  on the same socket.

However, when using streams, the backpressure does not kick in:

```bash
$ go run main/server.go http2
$ go run client/main.go
2020/05/11 20:08:43 Sent 1000 items
2020/05/11 20:08:43 Sent 2000 items
2020/05/11 20:08:43 Sent 3000 items
2020/05/11 20:08:43 Sent 4000 items
never stops and OOMs
```

The server ends up eating all RAM consuming as fast as possible
and crashing with out of memory.

* [Native GRPC transport with mux](server/main.go#L118)

The solution is to multiplex GRPC and HTTP1/1 using detection
after tls.Listener Accept negotiated a handshake
and the router looks at the NextNegotiatedProtocol section
as demonstrated in mainGRPCMux


```bash
$ go run main/server.go grpcmux
$ go run client/main.go
2020/05/11 20:08:43 Sent 1000 items
2020/05/11 20:08:43 Sent 2000 items
2020/05/11 20:08:43 Sent 3000 items
works
```

```bash
$ go run http11client/main.go
HTTP/1.1 also works!
```
