.PHONY: build run-dev test test-integration lint migrate-up migrate-down clean

GO := go
BINARY := bin/aibao-server
PKG := ./...

CONFIG_DEV := config/config.dev.yaml
MIGRATE_DIR := migrations
DB_URL_DEV := postgres://aibao:aibao@127.0.0.1:5432/aibao?sslmode=disable

build:
	$(GO) build -o $(BINARY) ./cmd/server

run-dev: build
	AIBAO_CONFIG=$(CONFIG_DEV) \
	AIBAO_POSTGRES_PASSWORD=aibao \
	AIBAO_AUTH_JWT_SECRET=dev-jwt-secret-change-me \
	AIBAO_CRYPTO_PHONE_AES_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
	AIBAO_CRYPTO_SAFEHASH_SALT=dev-safehash-salt \
	./$(BINARY)

test:
	$(GO) test -race -count=1 -cover $(PKG)

test-integration:
	$(GO) test -race -count=1 -tags=integration $(PKG)

lint:
	golangci-lint run

migrate-up:
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL_DEV)" up

migrate-down:
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL_DEV)" down 1

clean:
	rm -rf bin logs
