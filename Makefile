.PHONY: run-ingestion run-api build test docker-up docker-down

run-ingestion:
	go run ./cmd/ingestion/...

run-api:
	go run ./cmd/api/...

build:
	go build -o bin/ingestion ./cmd/ingestion/...
	go build -o bin/api ./cmd/api/...

test:
	go test ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down
