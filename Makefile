VERSION ?= $(shell git describe --tags --always --dirty --match=v0* 2> /dev/null || \
			cat $(CURDIR)/.version 2> /dev/null || echo v0)

PROTOBUF_IMAGE=bloxroute/bdn-protobuf:v30.2

build:
	@cd bxgateway && go mod vendor
	@docker build --platform linux/amd64 -f bxgateway/Dockerfile --build-arg SG_VERSION=$(VERSION)d .
	@cd bxgateway && rm -rf vendor

.PHONY: genproto
.SILENT: genproto
genproto:
	docker run -v $(CURDIR)/pkg/protobuf:/go/protobuf --platform linux/amd64 $(PROTOBUF_IMAGE) \
		protoc --go_out=. --go_opt=paths=source_relative  --go-grpc_out=. --go-grpc_opt=paths=source_relative relay.proto types.proto
