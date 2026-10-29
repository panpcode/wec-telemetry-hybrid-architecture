# ── Build stage ─────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
COPY *.go ./
RUN go mod download && go build -o cloud-uploader .

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/cloud-uploader .
CMD ["./cloud-uploader"]
