# ── Build stage ─────────────────────────────────────────────────────────────
# schema.sql is embedded into the binary via //go:embed.
FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go *.sql ./
RUN go build -o stream-processor .

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/stream-processor .
CMD ["./stream-processor"]
