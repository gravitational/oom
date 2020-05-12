package main

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"

	pb "github.com/gravitational/oom"
)

const (
	address = "localhost:10000"
)

func main() {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{pb.Certificate()},
			RootCAs:      pb.CertPool(),
			ServerName:   address,
			// could be empty or non empty
			//NextProtos:   []string{"http/1.1"},
		},
	}
	client := &http.Client{
		Transport: transport,
	}
	re, err := client.Get("https://" + address)
	if err != nil {
		panic(err)
	}
	data, err := ioutil.ReadAll(re.Body)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Got response: %v %v\n", re.Status, string(data))
}
