VERSION ?= $(shell git describe --tags --always --dirty --match=v0* 2> /dev/null || \
			cat $(CURDIR)/.version 2> /dev/null || echo v0)
PLATFORM ?= linux/amd64

build:
	@cd bxgateway && go mod vendor
	@docker build --platform $(PLATFORM) -f bxgateway/Dockerfile --build-arg SG_VERSION=$(VERSION)d .
	@cd bxgateway && rm -rf vendor
