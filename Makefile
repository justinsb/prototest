.PHONY: proto
proto:
	go get github.com/gogo/protobuf/protoc-gen-gofast
	mkdir -p gogo/
	PATH=~/go/bin:${PATH} protoc --gofast_out=gogo/ pb/prototest.proto