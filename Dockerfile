# =============================================================================
# Build uniTerm Web from this repository (Wails v3 -tags server + multi-user gateway).
#
#   docker compose up -d --build
#
# Frontend bindings are a gitignored generated artifact: run wails3 generate, then npm build.
# =============================================================================

# Stage 1: Go source + generate the frontend bindings
FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS bindings-generator
# proxy.golang.org is unreachable from some networks; goproxy.cn serves the same modules.
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
RUN for i in 1 2 3; do apk add --no-cache git ca-certificates && break; [ "$i" = 3 ] && exit 1; sleep 5; done
WORKDIR /src
COPY go.mod go.sum ./
RUN for i in 1 2 3; do go mod download && break; [ "$i" = 3 ] && exit 1; sleep 5; done
COPY . .
# Keep in sync with the wails version in go.mod
RUN for i in 1 2 3; do go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0 && break; [ "$i" = 3 ] && exit 1; sleep 5; done
RUN /go/bin/wails3 generate bindings -f "-tags server,production" -clean=true

# =============================================================================
# Stage 2: Build Vue 3 + Pinia + Element Plus + xterm.js frontend
# =============================================================================
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN for i in 1 2 3; do npm ci && break; [ "$i" = 3 ] && exit 1; sleep 5; done
COPY --from=bindings-generator /src/frontend/bindings ./bindings
COPY frontend/ ./
RUN npm run build

# =============================================================================
# Stage 3: Headless worker (-tags server) + multi-user gateway
# =============================================================================
FROM bindings-generator AS backend-builder
# Go cross-compiles: the toolchain runs natively on the build machine and
# only the output targets TARGETARCH — no QEMU emulation in any build stage.
ARG TARGETARCH
ARG VERSION=dev
COPY --from=frontend-builder /src/frontend/dist ./frontend/dist
# Compress the binaries: -s -w strips first, then UPX/LZMA lands the image
# well under half the size. Trade-off: slightly slower first run and the
# occasional antivirus false positive on the packed ELF.
RUN apk add --no-cache upx
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -tags "server production" -ldflags="-s -w -X main.Version=${VERSION}" -o /out/uniterm-server .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /out/uniterm-gateway ./cmd/uniterm-gateway
RUN upx --lzma -q /out/uniterm-server /out/uniterm-gateway

# =============================================================================
# Stage 4: Runtime image with the CLI tools the web console shells rely on
# =============================================================================
FROM alpine:3.21
# EXTRA_PACKAGES lets deployments opt into heavyweight CLI tools (docker-cli,
# kubectl) that the web console shells can drive. They are NOT installed by
# default: a mounted docker.sock plus any tenant's local terminal equals host
# root, and every tenant worker currently shares the container's root user.
ARG EXTRA_PACKAGES=""
RUN for i in 1 2 3; do apk add --no-cache \
    ca-certificates \
    tzdata \
    wget \
    $EXTRA_PACKAGES \
    && break; [ "$i" = 3 ] && exit 1; sleep 5; done \
    && mkdir -p /data/gateway /data/users

WORKDIR /app
COPY --from=backend-builder /out/uniterm-server /app/uniterm-server
COPY --from=backend-builder /out/uniterm-gateway /app/uniterm-gateway
COPY --from=frontend-builder /src/frontend/dist /app/frontend/dist

ENV UNITERM_DATA_DIR=/data \
    UNITERM_GATEWAY_ADDR=0.0.0.0:8080 \
    UNITERM_SERVER_BIN=/app/uniterm-server \
    UNITERM_ADMIN_USER=admin \
    UNITERM_WORKER_IDLE_MINUTES=60

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/uniterm-gateway"]
