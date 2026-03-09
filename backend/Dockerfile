FROM node:22-alpine AS frontend-builder

WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build


FROM golang:1.25-alpine AS backend-builder

RUN apk add --no-cache gcc musl-dev p7zip

WORKDIR /backend
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY backend/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o server ./cmd/server

# ---- runtime ----
FROM alpine:3.20

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

WORKDIR /app
COPY --from=backend-builder /backend/server .
COPY --from=frontend-builder /frontend/dist ./public

RUN mkdir -p /data/users

ENV PORT=8080 \
    DB_PATH=/data/files.db \
    USERS_ROOT=/data/users \
    GIN_MODE=release

CMD ["./server"]
