# =============================================================================
# 从当前仓库源码构建 uniTerm Web（Wails v3 -tags server + 多用户网关）
#
#   docker compose up -d --build
#
# 前端 bindings 是 gitignore 的生成物，所以必须先用 wails3 生成再 npm build。
# =============================================================================

# Stage 1: Go 源码 + 生成前端 bindings
FROM golang:1.26.2-alpine AS bindings-generator
# proxy.golang.org 在部分网络不可达；goproxy.cn 提供相同模块。
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
RUN for i in 1 2 3; do apk add --no-cache git ca-certificates && break; [ "$i" = 3 ] && exit 1; sleep 5; done
WORKDIR /src
COPY go.mod go.sum ./
RUN for i in 1 2 3; do go mod download && break; [ "$i" = 3 ] && exit 1; sleep 5; done
COPY . .
# 与 go.mod 中的 wails 版本保持一致
RUN for i in 1 2 3; do go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0 && break; [ "$i" = 3 ] && exit 1; sleep 5; done
RUN /go/bin/wails3 generate bindings -f "-tags server,production" -clean=true

# =============================================================================
# Stage 2: Build Vue 3 + Pinia + Element Plus + xterm.js frontend
# =============================================================================
FROM node:22-alpine AS frontend-builder
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
COPY --from=frontend-builder /src/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 GOOS=linux go build -tags "server production" -ldflags="-s -w" -o /out/uniterm-server .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/uniterm-gateway ./cmd/uniterm-gateway

# =============================================================================
# Stage 4: Runtime image with the CLI tools the web console shells rely on
# =============================================================================
FROM alpine:3.21
RUN for i in 1 2 3; do apk add --no-cache \
    ca-certificates \
    tzdata \
    bash \
    openssh-client \
    curl \
    wget \
    git \
    sqlite \
    docker-cli \
    kubectl \
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