package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	natsURL := getenv("NATS_URL", "nats://localhost:4222")
	dsn := getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/telemetry?sslmode=disable")
	th := Thresholds{
		TireMaxC:  getenvFloat("TIRE_TEMP_MAX_C", 120),
		BrakeMaxC: getenvFloat("BRAKE_TEMP_MAX_C", 450),
		FuelLowL:  getenvFloat("FUEL_LOW_L", 8),
	}

	log.Printf("Starting stream processor")
	log.Printf("  NATS       : %s", natsURL)
	log.Printf("  Thresholds : tire>%.0f°C brake>%.0f°C fuel<%.0fL", th.TireMaxC, th.BrakeMaxC, th.FuelLowL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := OpenStore(ctx, dsn)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer store.Close()
	log.Printf("✅ Connected to Postgres, schema applied")

	p, err := NewProcessor(natsURL, store, NewAggregator(th), 200)
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer p.Close()
	log.Printf("✅ Consuming TELEMETRY.* as durable 'stream-processor'")

	p.Run(ctx)
	log.Printf("Shutting down")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvFloat(key string, fallback float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return fallback
}
