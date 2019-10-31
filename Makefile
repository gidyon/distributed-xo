SERVER_OUT := "game"
SERVER_PKG_BUILD := "github.com/gidyon/tik-tak-toe"


build_server_prod: ## Build a production binary for server
	CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -a -installsuffix cgo -ldflags '-s' -v -o $(SERVER_OUT) $(SERVER_PKG_BUILD)
	
build_server: ## Build the binary file for server
	@go build -i -v -o $(SERVER_OUT) $(SERVER_PKG_BUILD)

clean_server: ## Remove server binary
	@rm -f $(SERVER_OUT)

docker_build: ## Create a docker image for the service
ifdef tag
	@docker build --no-cache -t gidyon/xo:$(tag) .
else
	@docker build --no-cache -t gidyon/xo:latest .
endif

docker_build_prod: build_server_prod docker_build

docker_tag:
	@docker tag gidyon/xo:$(tag) gidyon/xo:$(tag)

docker_push:
	@docker push gidyon/xo:$(tag)

docker_build_and_push: docker_build_prod docker_tag docker_push 
	
help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
