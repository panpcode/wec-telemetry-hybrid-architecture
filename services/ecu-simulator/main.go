package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

const (
	// Simulation parameters
	SIMULATION_RATE_HZ  = 20 // 20 events per second (50ms intervals)
	SIMULATION_INTERVAL = 50 * time.Millisecond

	// Default configuration
	INGESTION_URL       = "http://localhost:8080"
	NUM_CARS            = 2                // Number of cars to simulate
	SIMULATION_DURATION = 10 * time.Minute // How long to run
)

// init ensures uuid package is imported
func init() {
	_ = uuid.New()
}

func main() {
	log.Println("🏎️  WEC ECU Simulator Starting...")

	// Create simulators for configured number of cars
	cars := make(map[string]*CarSimulator)
	for i := 1; i <= NUM_CARS; i++ {
		carID := fmt.Sprintf("CAR_%03d", i)
		config := DefaultCarConfig(carID)
		cars[carID] = NewCarSimulator(config)
		log.Printf("✅ Created simulator for %s", carID)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Main simulation loop
	ticker := time.NewTicker(SIMULATION_INTERVAL)
	defer ticker.Stop()

	startTime := time.Now()
	eventCount := 0

	log.Printf("🚀 Starting simulation loop (%.1f Hz, duration: %v)",
		float64(1000)/float64(SIMULATION_INTERVAL.Milliseconds()),
		SIMULATION_DURATION)

	for range ticker.C {
		// Check if we should stop
		if time.Since(startTime) > SIMULATION_DURATION {
			log.Printf("✋ Simulation complete!")
			log.Printf("📊 Total events sent: %d", eventCount)
			os.Exit(0)
		}

		// Generate event for each car
		for _, simulator := range cars {
			event := simulator.Step(SIMULATION_INTERVAL.Seconds())

			// Send to ingestion service
			go sendEvent(client, event, &eventCount)
		}

		// Print status every 100 events
		if eventCount%100 == 0 && eventCount > 0 {
			log.Printf("📡 Events sent: %d (elapsed: %v)",
				eventCount,
				time.Since(startTime).Round(time.Second))
		}
	}
}

// sendEvent sends a telemetry event to the ingestion service
func sendEvent(client *http.Client, event TelemetryEvent, counter *int) {
	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("❌ Failed to marshal event: %v", err)
		return
	}

	// Send HTTP POST
	resp, err := client.Post(
		INGESTION_URL+"/telemetry/ingest",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		log.Printf("❌ Failed to send event to %s: %v", INGESTION_URL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		log.Printf("⚠️  Ingestion service returned status %d", resp.StatusCode)
		return
	}

	*counter++
}
