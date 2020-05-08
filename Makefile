.PHONY: install
install:
	sudo apt-get -y install protobuf-compiler
	go get -u github.com/golang/protobuf/protoc-gen-go


.PHONY: generate
generate:
	protoc oom.proto --go_out=plugins=grpc:.

