.PHONY: help build run test vet fmt tidy migrate-up migrate-down clean docker-build docker-run docker-down validate-coolify

GO          ?= go
BIN_DIR     ?= bin
SERVER_BIN  ?= $(BIN_DIR)/evogo-connect
CLI_BIN     ?= $(BIN_DIR)/connect
MIGRATE     ?= $(GO) run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate
DATABASE_URL ?= postgres://connect:connect@localhost:5432/evogo_connect?sslmode=disable

help: ## Mostra esta ajuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Compila server e CLI
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(SERVER_BIN) ./cmd/evogo-connect
	$(GO) build -o $(CLI_BIN) ./cmd/connect-cli
	@echo "OK: $(SERVER_BIN) + $(CLI_BIN)"

run: build ## Roda o servidor local
	./$(SERVER_BIN)

test: ## Roda a suite de testes
	$(GO) test -race -count=1 ./...

vet: ## go vet
	$(GO) vet ./...

fmt: ## gofmt -w em tudo
	$(GO) fmt ./...

tidy: ## go mod tidy
	$(GO) mod tidy

migrate-up: ## Aplica migrations
	$(MIGRATE) -database "$(DATABASE_URL)" -path migrations up

migrate-down: ## Reverte última migration
	$(MIGRATE) -database "$(DATABASE_URL)" -path migrations down 1

docker-build: ## Builda imagem Docker
	docker build -f deploy/Dockerfile -t evogo-connect:dev .

docker-run: ## Sobe stack completa (Postgres + connector)
	docker compose -f deploy/docker-compose.yml up -d

docker-down: ## Derruba stack
	docker compose -f deploy/docker-compose.yml down

validate-coolify: ## Valida o compose de produção para Coolify
	bash scripts/validate-coolify-compose.sh

clean: ## Limpa artefatos
	rm -rf $(BIN_DIR) coverage.out coverage.html
