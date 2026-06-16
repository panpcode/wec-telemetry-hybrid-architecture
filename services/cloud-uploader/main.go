package main

import (
	"log"
	"os"
	"strconv"
)

func main() {
	cfg := Config{
		NatsURL:         getenv("NATS_URL", "nats://localhost:4222"),
		S3Endpoint:      getenv("S3_ENDPOINT", "http://localhost:9000"),
		S3Bucket:        getenv("S3_BUCKET", "wec-telemetry"),
		S3AccessKey:     getenv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:     getenv("S3_SECRET_KEY", "minioadmin"),
		S3UseSSL:        getenvBool("S3_USE_SSL", false),
		BatchSize:       getenvInt("BATCH_SIZE", 500),
		BatchTimeoutSec: getenvInt("BATCH_TIMEOUT_SEC", 10),
	}

	log.Printf("Starting cloud uploader")
	log.Printf("  NATS     : %s", cfg.NatsURL)
	log.Printf("  S3       : %s / %s", cfg.S3Endpoint, cfg.S3Bucket)
	log.Printf("  Batch    : %d events or %ds (whichever comes first)", cfg.BatchSize, cfg.BatchTimeoutSec)

	u, err := NewUploader(cfg)
	if err != nil {
		log.Fatalf("init failed: %v", err)
	}
	defer u.Close()

	// Run forever — pull from NATS, batch, upload, ack.
	u.Run()
}

// Config holds all environment-driven configuration for the uploader.
type Config struct {
	NatsURL         string
	S3Endpoint      string
	S3Bucket        string
	S3AccessKey     string
	S3SecretKey     string
	S3UseSSL        bool
	BatchSize       int
	BatchTimeoutSec int
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
