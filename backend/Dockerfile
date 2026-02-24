FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev p7zip

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o server ./cmd/server

# ---- runtime ----
FROM alpine:3.20

RUN apk add --no-cache p7zip ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/server .

RUN mkdir -p /data/users

ENV PORT=8080 \
    DB_PATH=/data/files.db \
    USERS_ROOT=/data/users \
    GIN_MODE=release

EXPOSE 8080

CMD ["./server"]
