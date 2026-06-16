# ── Build stage ─────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o ingestion-service .

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/ingestion-service .

# wget is used by the docker-compose healthcheck.
RUN apk add --no-cache wget

CMD ["./ingestion-service"]
