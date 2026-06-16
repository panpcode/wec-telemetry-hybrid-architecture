# ── Build stage ─────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
WORKDIR /app

# Copy module files and download dependencies before copying source so that
# Docker can cache this layer and skip the download on every code change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o ecu-simulator .

# ── Runtime stage ────────────────────────────────────────────────────────────
# Minimal Alpine image — the Go binary is statically linked so no libc needed.
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/ecu-simulator .
CMD ["./ecu-simulator"]
