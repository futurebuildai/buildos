.PHONY: build build-server build-worker build-migrate build-prod build-fork-init fork-init test test-integration test-prod lint lint-migrations lint-migrations-test migrate migrate-down db-up db-down audit bench-physics e2e-backend backup-db restore-db backup-db-test docker-build docker-run clean

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

# Fork lifecycle bootstrap tool — generates the JWT signing keypair,
# AES-256 vault master key, and bootstrap token for a new customer
# fork. Run once per fork during initial provisioning.
build-fork-init:
	go build -o bin/buildos-fork-init ./cmd/buildos-fork-init

# Convenience wrapper. Required: OUT=<dir>; optional: KID=<kid>,
# ORG_ID=<uuid>. Example:
#   make fork-init OUT=./forks/acme/secrets KID=acme-2026-q2
fork-init: build-fork-init
	@if [ -z "$(OUT)" ]; then echo "make fork-init: OUT=<dir> is required"; exit 64; fi
	./bin/buildos-fork-init \
		--out "$(OUT)" \
		$(if $(KID),--kid "$(KID)") \
		$(if $(ORG_ID),--org-id "$(ORG_ID)")

## Production build — same flags the Dockerfile uses.
## - tags=prod compiles out the DEV_AUTH_MODE=header bypass (D8).
## - trimpath strips local paths from the binary for reproducibility.
## - ldflags '-s -w' strips debug info; ~30% smaller binary, no
##   gdb/delve attaches in prod.
build-prod:
	@mkdir -p bin/prod
	CGO_ENABLED=0 go build -tags=prod -trimpath -ldflags="-s -w" -o bin/prod/server  ./cmd/server
	CGO_ENABLED=0 go build -tags=prod -trimpath -ldflags="-s -w" -o bin/prod/worker  ./cmd/worker
	CGO_ENABLED=0 go build -tags=prod -trimpath -ldflags="-s -w" -o bin/prod/migrate ./cmd/migrate
	@echo "prod binaries in bin/prod/ (CGO disabled, prod build tag, debug stripped)"

## Test (unit only — no Docker required)
test:
	go test ./... -count=1

## Integration tests — spawns ephemeral Postgres containers via Testcontainers.
## Requires Docker. Tests gated behind the `integration` build tag.
test-integration:
	go test -tags=integration -count=1 ./...

## Test the prod-build path: D8 hardening (claimsFromDevHeader stub),
## any other prod-only behaviors. Smaller suite than full `test` — only
## packages with prod-tagged test files.
test-prod:
	go test -tags=prod -count=1 ./internal/api/middleware/...

## Lint
lint:
	golangci-lint run ./...

lint-migrations:
	bash scripts/lint-migrations.sh

# Regression suite for the linter itself — runs the four fixture
# directories under scripts/lint-migrations-fixtures/ and asserts each
# passes or fails as expected.
lint-migrations-test:
	bash scripts/lint-migrations.test.sh

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

## Audit (lint + migration lint + test + prod-mode test + physics benchmarks
## + backup/restore retention+guard regression)
audit: lint-migrations lint-migrations-test test test-prod bench-physics backup-db-test
	@echo "Audit: ALL PASSED"

## Backup / DR — per-fork Postgres backup with retention + restore guard.
## See docs/dr-runbook.md for the policy, scheduling, and restore drill.
##
##   make backup-db                          # dump + sidecar + upload + prune
##   make backup-db PRUNE_ONLY=1             # retention sweep only, no dump
##   make restore-db DUMP=path/to.dump CONFIRM=1
backup-db:
	bash scripts/backup-db.sh $(if $(PRUNE_ONLY),--prune-only,)

restore-db:
	@if [ -z "$(DUMP)" ]; then echo "make restore-db: DUMP=<dump-file> is required"; exit 64; fi
	bash scripts/restore-db.sh "$(DUMP)" $(if $(CONFIRM),--confirm,)

# DB-free regression suite for the backup retention math + restore guards.
backup-db-test:
	bash scripts/backup-db.test.sh

## End-to-end backend harness — boots a live cmd/server against a
## migrated+seeded Postgres using NATIVE auth (claim → wizard → operate),
## then runs CMD against it and tears the server down. See
## scripts/e2e-backend.sh for the full contract (exports E2E_API_URL,
## E2E_BOOTSTRAP_TOKEN, E2E_OWNER_EMAIL, E2E_OWNER_PASSWORD).
##
## Local:  make e2e-backend CMD="npm --prefix web run test:e2e:live"
## With DB: make e2e-backend DB_UP=1 CMD="..."  (brings up the compose DB)
## No CMD:  make e2e-backend                    (boots + idles foreground)
e2e-backend:
	bash scripts/e2e-backend.sh $(if $(DB_UP),--db-up,) $(if $(CMD),-- $(CMD),)

## Docker — single multi-arch image, server/worker/migrate selectable
## via BUILDOS_ROLE env. Local builds default to the host arch only
## (multi-arch happens in CI on the release workflow).
##
## VERSION/COMMIT/BUILD_DATE are baked in via -ldflags so /health
## reports the exact build provenance. CI overrides these from the
## github.sha and tag values; local builds default to "dev" / git
## state / now.
DOCKER_IMAGE ?= buildos
DOCKER_TAG   ?= dev
VERSION      ?= dev
COMMIT       ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE   ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		.

# Smoke-run the built image. Doesn't connect to a real DB; the server
# will fail liveness on /ready (DB unreachable) but /health (process
# alive) will respond 200, confirming the binary boots.
docker-run:
	docker run --rm -p 8080:8080 \
		-e DATABASE_URL=postgres://disabled \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

## Clean
clean:
	rm -rf bin/
