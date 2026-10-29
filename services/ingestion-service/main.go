package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Flags override env vars; env vars (set by docker-compose) override the defaults.
	httpAddr := flag.String("http", getenv("HTTP_ADDR", ":8080"), "HTTP server address (env HTTP_ADDR)")
	natsURL := flag.String("nats", getenv("NATS_URL", "nats://localhost:4222"), "NATS server URL (env NATS_URL)")
	flag.Parse()

	log.Println("🏎️  WEC Ingestion Service Starting...")

	// Create server
	server := NewServer(*httpAddr, *natsURL)

	// Connect to NATS
	if err := server.Connect(); err != nil {
		log.Fatalf("❌ Failed to connect to NATS: %v", err)
	}
	defer server.Close()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("\n✋ Received signal: %v", sig)
		server.Close()
		os.Exit(0)
	}()

	// Start HTTP server (blocks)
	if err := server.Start(); err != nil {
		log.Fatalf("❌ HTTP server error: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
