package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// init ensures the uuid package is linked (it provides only side-effects here).
func init() { _ = uuid.New() }

func main() {
	// ── Config from environment ─────────────────────────────────────────────
	ingestionURL := getenv("INGESTION_URL", "http://localhost:8080")
	numCars := getenvInt("NUM_CARS", 2)
	trackName := getenv("TRACK", "le_mans")
	simHz := getenvInt("SIM_HZ", 20)
	simDuration := getenvDuration("SIM_DURATION", 10*time.Minute)
	pitEveryLaps := getenvInt("PIT_EVERY_LAPS", 8)

	interval := time.Duration(1000/simHz) * time.Millisecond
	track := TrackByName(trackName)

	log.Printf("WEC ECU Simulator")
	log.Printf("  Track    : %s (%.3f km, max %.0f km/h)", track.Name, track.LengthKm, track.MaxSpeedKmh)
	log.Printf("  Cars     : %d", numCars)
	log.Printf("  Rate     : %d Hz (%v interval)", simHz, interval)
	log.Printf("  Duration : %v", simDuration)
	log.Printf("  Pit stop : every %d laps", pitEveryLaps)
	log.Printf("  Endpoint : %s", ingestionURL)

	// ── Create one simulator per car ────────────────────────────────────────
	cars := make([]*CarSimulator, numCars)
	for i := range cars {
		carID := fmt.Sprintf("CAR_%03d", i+1)
		cfg := DefaultCarConfig(carID, track, pitEveryLaps)
		cars[i] = NewCarSimulator(cfg)
		log.Printf("  Created simulator for %s", carID)
	}

	client := &http.Client{Timeout: 5 * time.Second}

	// ── Simulation loop ─────────────────────────────────────────────────────
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	start := time.Now()
	var eventsSent atomic.Int64

	log.Printf("Simulation running...")

	for range ticker.C {
		if time.Since(start) >= simDuration {
			log.Printf("Simulation complete. Events sent: %d", eventsSent.Load())
			os.Exit(0)
		}

		// Each car gets its own goroutine per tick so they don't block each other.
		for _, sim := range cars {
			event := sim.Step(interval.Seconds())
			go func(e TelemetryEvent) {
				if sendEvent(client, ingestionURL, e) {
					eventsSent.Add(1)
				}
			}(event)
		}

		// Log throughput every 200 events.
		if n := eventsSent.Load(); n > 0 && n%200 == 0 {
			elapsed := time.Since(start).Round(time.Second)
			log.Printf("Sent %d events in %v (%.1f ev/s)", n, elapsed, float64(n)/time.Since(start).Seconds())
		}
	}
}

// sendEvent serialises the event and POSTs it to the ingestion service.
// Returns true on success.
func sendEvent(client *http.Client, baseURL string, event TelemetryEvent) bool {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return false
	}

	resp, err := client.Post(baseURL+"/telemetry/ingest", "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("POST error: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		log.Printf("unexpected status %d for %s", resp.StatusCode, event.CarID)
		return false
	}
	return true
}

// ── Env helpers ─────────────────────────────────────────────────────────────

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
