.PHONY: build build-server build-worker build-migrate build-dev-idp dev-idp test test-integration lint lint-migrations migrate migrate-down db-up db-down audit bench-physics clean

# Default DATABASE_URL for local dev (docker-compose db on port 5433)
DATABASE_URL ?= postgres://fb_user:fb_pass@localhost:5433/futurebuild_os?sslmode=disable

## Build
build: build-server build-worker

build-server:
	go build -o bin/server ./cmd/server

build-worker:
	go build -o bin/worker ./cmd/worker

build-migrate:
	go build -o bin/migrate ./cmd/migrate

# Mock OIDC issuer for staging and sales demos. NOT for production.
build-dev-idp:
	go build -o bin/dev-idp ./cmd/dev-idp

# Run the dev-idp on :8083. Point BuildOS at it with:
#   BRAIN_JWKS_URL=http://localhost:8083/jwks BRAIN_ISSUER_URL=http://localhost:8083
dev-idp: build-dev-idp
	./bin/dev-idp

## Test (unit only — no Docker required)
test:
	go test ./... -count=1

## Integration tests — spawns ephemeral Postgres containers via Testcontainers.
## Requires Docker. Tests gated behind the `integration` build tag.
test-integration:
	go test -tags=integration -count=1 ./...

## Lint
lint:
	golangci-lint run ./...

lint-migrations:
	bash scripts/lint-migrations.sh

## Database
db-up:
	docker compose up -d db

db-down:
	docker compose down

migrate: build-migrate
	DATABASE_URL=$(DATABASE_URL) ./bin/migrate up

migrate-down: build-migrate
	DATABASE_URL=$(DATABASE_URL) ./bin/migrate down

## Physics Engine Benchmarks (CI hard gate)
bench-physics:
	@echo "Running physics engine benchmarks..."
	go test -bench=BenchmarkCPM -benchtime=10x ./internal/physics/... -run='^$$' | \
		go run ./tools/bench-gate/main.go --cpm80=200ms --cpm200=500ms

## Audit (lint + migration lint + test + physics benchmarks)
audit: lint-migrations test bench-physics
	@echo "Audit: ALL PASSED"

## Clean
clean:
	rm -rf bin/
