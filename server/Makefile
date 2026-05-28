.PHONY: build run-dev test test-integration lint rules-lint migrate-up migrate-down seed-bgm cost-report clean check-layering

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
	AIBAO_LLM_DOUBAO_API_KEY=$$AIBAO_LLM_DOUBAO_API_KEY \
	AIBAO_TTS_MINIMAX_GROUP_ID=$$AIBAO_TTS_MINIMAX_GROUP_ID \
	AIBAO_TTS_MINIMAX_API_KEY=$$AIBAO_TTS_MINIMAX_API_KEY \
	AIBAO_STORAGE_COS_SECRET_ID=$$AIBAO_STORAGE_COS_SECRET_ID \
	AIBAO_STORAGE_COS_SECRET_KEY=$$AIBAO_STORAGE_COS_SECRET_KEY \
	AIBAO_STORAGE_COS_BUCKET=$$AIBAO_STORAGE_COS_BUCKET \
	AIBAO_STORAGE_COS_REGION=$$AIBAO_STORAGE_COS_REGION \
	AIBAO_STORAGE_COS_APPID=$$AIBAO_STORAGE_COS_APPID \
	./$(BINARY)

test:
	$(GO) test -race -count=1 -cover $(PKG)

test-integration:
	$(GO) test -race -count=1 -tags=integration $(PKG)

lint:
	golangci-lint run

rules-lint:
	$(GO) run ./cmd/rules-lint

migrate-up:
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL_DEV)" up

migrate-down:
	migrate -path $(MIGRATE_DIR) -database "$(DB_URL_DEV)" down 1

seed-bgm:
	AIBAO_CONFIG=$(CONFIG_DEV) \
	AIBAO_POSTGRES_PASSWORD=aibao \
	$(GO) run ./cmd/seed-bgm -manifest=safety/bgm_manifest.yaml -config=$(CONFIG_DEV)

cost-report:
	@$(GO) build -o bin/cost-report ./cmd/cost-report

clean:
	rm -rf bin logs

# Plan 11B §3.5: enforce gateway/ does not import service/repository/api.
check-layering:
	@bash scripts/check_layering.sh
