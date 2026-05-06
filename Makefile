APP_NAME    = involens
VERSION     ?= SNAPSHOT
INGESTION   = ingestion
API         = api

GOENV   = CGO_ENABLED=0 GOOS=linux GOARCH=amd64
LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.appName=$(APP_NAME)"

.DEFAULT_GOAL := help

.PHONY: build
build: build-ingestion build-api ## Build all binaries

.PHONY: build-ingestion
build-ingestion: ## Build the ingestion binary
	$(GOENV) go build $(LDFLAGS) -a -installsuffix cgo -o bin/$(INGESTION) ./cmd/ingestion

.PHONY: build-api
build-api: ## Build the api binary
	$(GOENV) go build $(LDFLAGS) -a -installsuffix cgo -o bin/$(API) ./cmd/api

.PHONY: run-ingestion
run-ingestion: ## Run the ingestion service locally
	go run ./cmd/ingestion

.PHONY: run-api
run-api: ## Run the api service locally
	go run ./cmd/api

.PHONY: test
test: ## Run unit tests
	go test -race -v ./...

.PHONY: tidy
tidy: ## Tidy go modules
	go mod tidy

.PHONY: fmt
fmt: ## Run gofmt on all Go files
	gofmt -l -w .

.PHONY: docker-build
docker-build: docker-build-ingestion docker-build-api ## Build all docker images

.PHONY: docker-build-ingestion
docker-build-ingestion: ## Build the ingestion docker image
	docker build -f Dockerfile.ingestion -t $(APP_NAME)-$(INGESTION):$(VERSION) .

.PHONY: docker-build-api
docker-build-api: ## Build the api docker image
	docker build -f Dockerfile.api -t $(APP_NAME)-$(API):$(VERSION) .

.PHONY: docker-up
docker-up: ## Start all services with docker compose (mongo + ingestion + api)
	docker compose up -d

.PHONY: docker-up-app
docker-up-app: ## Start only the app containers (use when mongo already runs externally)
	docker compose up -d ingestion api

.PHONY: docker-down
docker-down: ## Stop all services
	docker compose down

.PHONY: docker-down-app
docker-down-app: ## Stop only the app containers
	docker compose stop ingestion api && docker compose rm -f ingestion api

.PHONY: docker-logs
docker-logs: ## Tail docker compose logs for app services
	docker compose logs -f ingestion api

.PHONY: docker-restart
docker-restart: ## Rebuild and restart the app containers
	docker compose up -d --build ingestion api

.PHONY: help
help: ## Show available targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'
