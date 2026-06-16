# ── Build stage ─────────────────────────────────────────────────────────────
# go mod tidy is used instead of go mod download because this is a new module
# without a committed go.sum — tidy resolves and pins all transitive deps.
FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod ./
COPY *.go ./
RUN go mod tidy && go build -o cloud-uploader .

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/cloud-uploader .
CMD ["./cloud-uploader"]
