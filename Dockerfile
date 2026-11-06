# syntax=docker/dockerfile:1

# Build the Vue application from the lockfile; local build artifacts are ignored.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web-builder
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# Cross-compile the server with the freshly built frontend embedded in it.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS server-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=web-builder /build/web/dist/ ./cmd/server/web/
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/airy ./cmd/server

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates ffmpeg tzdata \
    && addgroup -S -g 10001 airy \
    && adduser -S -D -H -u 10001 -G airy airy \
    && mkdir -p /app /data /mnt \
    && chown airy:airy /data
COPY --from=server-builder /out/airy /app/airy

ENV PORT=8096 \
    IS_DOCKER=true \
    GIN_MODE=release \
    TZ=Asia/Shanghai \
    FFMPEG_PATH=/usr/bin/ffmpeg \
    FFPROBE_PATH=/usr/bin/ffprobe

# SQLite's default relative path resolves here. Mount /data to persist it.
# Without DB_* settings SQLite is used; configure DB_* for PostgreSQL.
WORKDIR /data
USER 10001:10001
EXPOSE 8096

# Use GET (not HEAD) because the health endpoint is a Gin GET route.
# TLS checks are local liveness checks and also allow privately issued certs.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD if [ -n "$TLS_CERT_FILE" ]; then \
          wget -q -O /dev/null --no-check-certificate "https://127.0.0.1:${PORT}/api/health"; \
        else \
          wget -q -O /dev/null "http://127.0.0.1:${PORT}/api/health"; \
        fi || exit 1

ENTRYPOINT ["/app/airy"]
