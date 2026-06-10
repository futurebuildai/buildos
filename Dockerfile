# BuildOS production Dockerfile.
#
# Two binaries (server + worker) ship from one image; the entrypoint
# selects which to run via the BUILDOS_ROLE env var (default: server).
# Building two images per release would double the supply-chain
# surface — same binary set, same CVE scan, lower operational cost.
#
# Stage 1 — builder: golang:alpine for the toolchain. We deliberately
# pin to a specific minor (1.26.x) so a Go release with breaking ABI
# behavior can't sneak into a customer fork's nightly rebuild without
# review.
#
# Stage 2 — runtime: distroless/static. No shell, no package manager,
# no C library. Smaller attack surface than alpine (which still ships
# busybox + apk), and the gcr.io/distroless/static-debian12 base is
# Google-maintained with a public security posture and signed releases.
#
# Build flags:
#   -tags=prod           — enables the //go:build prod constraint that
#                          excludes the DEV_AUTH_MODE=header bypass path
#                          (D8 hardening; see internal/api/middleware/auth.go).
#   -trimpath            — strips local filesystem paths from the binary
#                          for reproducible builds + no $HOME in panics.
#   -ldflags '-s -w'     — strip debug info; drops ~30% off binary size
#                          and prevents gdb/delve attaches in prod.
#   -ldflags '-X main.version=...' — burned-in build version for /health.
#
# Multi-arch: this Dockerfile builds for whatever TARGETPLATFORM the
# `docker buildx build --platform linux/amd64,linux/arm64` invocation
# specifies. The BuildKit-injected vars do the right thing for cross
# compilation under Go.

# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26
ARG ALPINE_VERSION=3.22
ARG NODE_VERSION=20
ARG DISTROLESS_TAG=nonroot

# ----------------------------------------------------------------
# Stage 0 — web console build (Phase 0a: same-origin SPA serving).
# Vite emits a static bundle (web/dist) that the server serves itself
# (WEB_DIST_DIR); no separate static host. Runs on $BUILDPLATFORM —
# the output is platform-independent, so multi-arch builds run it once.
# npm ci is lockfile-pinned; `npm run build` typechecks then builds.
# ----------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS webbuilder

WORKDIR /web

# Lockfile layer first so dependency downloads cache across rebuilds.
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci

COPY web/ .
# Build, then strip source maps: they embed the complete original
# TypeScript (sourcesContent) and anything under dist/ is served
# unauthenticated by the server. sourcemap:'hidden' (vite.config.ts)
# already keeps the sourceMappingURL comment out of the bundles, so
# deleting the .map files leaves no dangling references.
RUN npm run build && find dist -name '*.map' -delete

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

# git + ca-certificates for go modules over HTTPS during build.
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Cache the module layer separately. Module deps change less often
# than source; this lets `docker build` skip the download step on
# most rebuilds.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .

# BuildKit sets TARGETOS / TARGETARCH from --platform.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# CGO disabled → fully static binary, runs on distroless/static.
# tags=prod is the build-tag flag that strips the dev-auth bypass.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
        -tags=prod \
        -trimpath \
        -ldflags="-s -w \
            -X main.version=${VERSION} \
            -X main.commit=${COMMIT} \
            -X main.buildDate=${BUILD_DATE}" \
        -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
        -tags=prod \
        -trimpath \
        -ldflags="-s -w \
            -X main.version=${VERSION} \
            -X main.commit=${COMMIT} \
            -X main.buildDate=${BUILD_DATE}" \
        -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
        -tags=prod \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/migrate ./cmd/migrate

# Tiny entrypoint that selects which binary to run by reading
# BUILDOS_ROLE. Single image, dual purpose, no shell needed in the
# runtime stage (the entrypoint is a Go binary too).
#
# Secrets reach the process via env vars or a file-secret mount
# (resolved by internal/config.SecretSource). Multi-line PEM values
# (JWT_PRIVATE_KEY_PEM, JWT_PUBLIC_KEY_PEM) are read directly from the
# environment — no env-to-file materialization needed.
COPY <<'EOF' /src/cmd/buildos-entrypoint/main.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// version stamped via -ldflags at build time.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	role := os.Getenv("BUILDOS_ROLE")
	if role == "" {
		role = "server"
	}
	bin := ""
	switch role {
	case "server":
		bin = "/usr/local/bin/server"
	case "worker":
		bin = "/usr/local/bin/worker"
	case "migrate":
		bin = "/usr/local/bin/migrate"
	default:
		fmt.Fprintf(os.Stderr, "buildos-entrypoint: unknown BUILDOS_ROLE %q (want server / worker / migrate)\n", role)
		os.Exit(64) // EX_USAGE
	}
	if _, err := os.Stat(bin); err != nil {
		fmt.Fprintf(os.Stderr, "buildos-entrypoint: %s missing: %v\n", bin, err)
		os.Exit(127)
	}
	fmt.Fprintf(os.Stderr, "buildos %s commit=%s built=%s starting role=%s\n", version, commit, buildDate, role)
	args := append([]string{bin}, os.Args[1:]...)
	if err := syscall.Exec(bin, args, os.Environ()); err != nil {
		// Fall back to exec.Command on platforms where syscall.Exec
		// isn't available (none in our targets, but defensively).
		cmd := exec.Command(bin, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "buildos-entrypoint: exec %s failed: %v\n", bin, err)
			os.Exit(1)
		}
	}
}
EOF

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
        -trimpath \
        -ldflags="-s -w \
            -X main.version=${VERSION} \
            -X main.commit=${COMMIT} \
            -X main.buildDate=${BUILD_DATE}" \
        -o /out/buildos-entrypoint ./cmd/buildos-entrypoint

# ----------------------------------------------------------------
# Stage 2 — runtime image
# ----------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:${DISTROLESS_TAG}

# OCI labels for image registries / SBOM tools to pick up. Most are
# overridable by the release workflow via --label flags so the values
# match the actual git ref / build moment.
LABEL org.opencontainers.image.title="BuildOS"
LABEL org.opencontainers.image.description="Single-tenant residential construction management backend"
LABEL org.opencontainers.image.source="https://github.com/futurebuildai/buildos"
LABEL org.opencontainers.image.licenses="proprietary"
LABEL org.opencontainers.image.vendor="FutureBuild AI"

# Distroless ships a `nonroot` user (uid 65532) by default. The
# COPY --chown=nonroot ensures the binaries are readable + executable
# by that uid without needing a shell to chmod after the fact.
COPY --from=builder --chown=nonroot:nonroot /out/server  /usr/local/bin/server
COPY --from=builder --chown=nonroot:nonroot /out/worker  /usr/local/bin/worker
COPY --from=builder --chown=nonroot:nonroot /out/migrate /usr/local/bin/migrate
COPY --from=builder --chown=nonroot:nonroot /out/buildos-entrypoint /usr/local/bin/buildos-entrypoint

# Migrations are read at runtime by cmd/migrate. Bake them into the
# image so a fork deployment doesn't need a separate volume mount or
# config-map for SQL files.
COPY --chown=nonroot:nonroot migrations/ /var/lib/buildos/migrations/

# Built web console, served same-origin by the server role (Phase 0a).
# WEB_DIST_DIR points the server at it; worker/migrate ignore it.
COPY --from=webbuilder --chown=nonroot:nonroot /web/dist /var/lib/buildos/web/
ENV WEB_DIST_DIR=/var/lib/buildos/web

# Run as the distroless nonroot uid. Even if the entrypoint is
# compromised, the process can't read /etc/shadow or write outside
# the writable scratch dirs we provide.
USER nonroot:nonroot

# Server listens on 8080 by default. Worker has no listening port.
EXPOSE 8080

# Healthcheck targets the liveness endpoint — orchestrators (k8s,
# Nomad, ECS) will use their own readiness probe against /ready.
# Distroless has no curl/wget; we use the healthcheck-shim Go binary
# (TODO once the entrypoint pattern is proven). For now, k8s
# livenessProbe + readinessProbe handle this; the HEALTHCHECK
# directive is informational for `docker run` / docker-compose.
# (Distroless can't actually exec the HEALTHCHECK because there's no
# shell. Leaving it commented as documentation.)
# HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
#   CMD ["/usr/local/bin/server", "--health-check"]

ENTRYPOINT ["/usr/local/bin/buildos-entrypoint"]
