package main

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"

	pb "github.com/gravitational/oom"
)

const (
	address = "https://localhost:10000"
)

func main() {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{pb.Certificate()},
			RootCAs:      pb.CertPool(),
		},
	}
	client := &http.Client{
		Transport: transport,
	}
	re, err := client.Get(address)
	if err != nil {
		panic(err)
	}
	data, err := ioutil.ReadAll(re.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Got response: %v %v\n", re.Status, string(data))
}
