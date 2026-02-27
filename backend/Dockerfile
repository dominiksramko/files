FROM node:22-alpine AS frontend-builder

WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build


FROM golang:1.23-alpine AS backend-builder

RUN apk add --no-cache gcc musl-dev p7zip

WORKDIR /backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN go build -o server ./cmd/server

# ---- runtime ----
FROM alpine:3.20

RUN apk add --no-cache p7zip ca-certificates tzdata ffmpeg

WORKDIR /app
COPY --from=backend-builder /backend/server .
COPY --from=frontend-builder /frontend/dist ./public

RUN mkdir -p /data/users

ENV PORT=8080 \
    DB_PATH=/data/files.db \
    USERS_ROOT=/data/users \
    GIN_MODE=release

CMD ["./server"]
