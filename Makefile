build:
	@cd bxgateway && go mod vendor
	@docker build --platform linux/amd64 -f bxgateway/Dockerfile .
	@cd bxgateway && rm -rf vendor
