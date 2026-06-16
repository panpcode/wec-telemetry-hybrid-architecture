# ── Build stage ─────────────────────────────────────────────────────────────
# The ui.html file must be present at build time because it is embedded into
# the binary via //go:embed — the Go compiler reads it during go build.
FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod ./
COPY *.go *.html ./
RUN go mod tidy && go build -o dashboard .

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/dashboard .

# wget is used by the docker-compose healthcheck.
RUN apk add --no-cache wget

CMD ["./dashboard"]
