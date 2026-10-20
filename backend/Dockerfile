FROM node:22-alpine AS frontend-builder

WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ ./
# The tests gate every production image: if one fails, the build (and deploy.sh) stops here.
RUN npm test
RUN npm run build


FROM golang:1.25-alpine AS backend-builder

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
FROM alpine:3.20 AS runtime-base

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


# ---- dev: runtime tools + Go toolchain + air live reload (used by docker-compose.dev.yml) ----
FROM runtime-base AS dev

COPY --from=golang:1.25-alpine /usr/local/go /usr/local/go

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

WORKDIR /app
COPY --from=backend-builder /backend/server .
COPY --from=frontend-builder /frontend/dist ./public

RUN mkdir -p /data/users

ENV PORT=8080 \
    DB_PATH=/data/files.db \
    USERS_ROOT=/data/users \
    GIN_MODE=release

CMD ["./server"]
