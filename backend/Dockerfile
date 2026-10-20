FROM node:24-alpine AS frontend-builder

WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ ./
# The tests gate every production image: if one fails, the build (and deploy.sh) stops here.
RUN npm test
RUN npm run build


FROM golang:1.26.9-alpine AS backend-builder

RUN apk add --no-cache gcc musl-dev p7zip

WORKDIR /backend
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY backend/ ./
# The tests gate every production image: if one fails, the build (and deploy.sh) stops here.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go vet ./... && go test ./...
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o server ./cmd/server

# ---- runtime base: every CLI tool the backend shells out to (shared by dev + runtime) ----
FROM alpine:3.24 AS runtime-base

COPY --from=linuxserver/unrar:7.2.4 /usr/bin/unrar-alpine /usr/bin/unrar

RUN apk add --no-cache \
    p7zip \
    ca-certificates \
    tzdata \
    ffmpeg \
    rsvg-convert \
    imagemagick \
    imagemagick-heic \
    poppler-utils

# Fail the build if a tool is missing or broken, instead of a preview later.
RUN ffmpeg -version >/dev/null \
    && ffprobe -version >/dev/null \
    && 7z i >/dev/null \
    && unrar >/dev/null \
    && rsvg-convert --version >/dev/null \
    && magick -list format | grep -q HEIC \
    && pdftoppm -v 2>/dev/null


# ---- dev: runtime tools + Go toolchain + air live reload (used by docker-compose.dev.yml) ----
FROM runtime-base AS dev

COPY --from=golang:1.26.9-alpine /usr/local/go /usr/local/go

RUN GOPATH=/tmp/gopath GOCACHE=/tmp/gocache GOBIN=/usr/local/bin \
        /usr/local/go/bin/go install github.com/air-verse/air@v1.67.3 \
    && chmod -R u+w /tmp/gopath \
    && rm -rf /tmp/gopath /tmp/gocache \
    && mkdir -m 777 /go-cache

# World-writable cache dir so the container can run as the host user (uid set in compose)
ENV PATH=/usr/local/go/bin:$PATH \
    GOTOOLCHAIN=local \
    GOPATH=/go-cache \
    GOCACHE=/go-cache/build

WORKDIR /app/backend
CMD ["air"]


# ---- runtime ----
FROM runtime-base

# Lets deploy.sh find (and remove) this app's untagged old images.
LABEL org.opencontainers.image.title="files"

# su-exec: docker-entrypoint.sh starts the server as PUID:PGID, not root.
RUN apk add --no-cache su-exec

WORKDIR /app
COPY --from=backend-builder /backend/server .
COPY --from=frontend-builder /frontend/dist ./public
COPY --chmod=755 backend/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN --mount=type=bind,source=backend/docker-entrypoint_test.sh,target=/tmp/docker-entrypoint_test.sh \
    sh /tmp/docker-entrypoint_test.sh

RUN mkdir -p /data/users && chown 1000:1000 /data /data/users

ENV PORT=8080 \
    DB_PATH=/data/files.db \
    USERS_ROOT=/data/users \
    THUMB_CACHE_DIR=/data/thumbs \
    HLS_CACHE_DIR=/data/hls \
    CONV_CACHE_DIR=/data/conv \
    GIN_MODE=release \
    PUID=1000 \
    PGID=1000

# Healthy while the server answers and its database works.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO /dev/null "http://127.0.0.1:${PORT}/health" || exit 1

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./server"]
