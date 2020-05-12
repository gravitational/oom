.PHONY: install
install:
	sudo apt-get -y install protobuf-compiler


.PHONY: generate
generate:
	protoc oom.proto --gofast_out=plugins=grpc:.
