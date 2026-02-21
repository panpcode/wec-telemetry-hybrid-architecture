package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// TelemetryEvent represents a single ECU telemetry reading
type TelemetryEvent struct {
	EventID          string    `json:"event_id"`
	Timestamp        time.Time `json:"timestamp"`
	CarID            string    `json:"car_id"`
	SessionID        string    `json:"session_id"`
	Lap              int       `json:"lap"`
	Sector           int       `json:"sector"`
	SpeedKmh         float64   `json:"speed_kmh"`
	RPM              float64   `json:"rpm"`
	ThrottlePercent  float64   `json:"throttle_percent"`
	BrakePressureBar float64   `json:"brake_pressure_bar"`
	TireFlTemp       float64   `json:"tire_fl_temp"`
	TireFrTemp       float64   `json:"tire_fr_temp"`
	TireRlTemp       float64   `json:"tire_rl_temp"`
	TireRrTemp       float64   `json:"tire_rr_temp"`
	FuelLevelLiters  float64   `json:"fuel_level_liters"`
	FuelFlowLph      float64   `json:"fuel_flow_lph"`
	BrakeTemp        float64   `json:"brake_temp"`
	GpsLat           float64   `json:"gps_lat"`
	GpsLong          float64   `json:"gps_long"`
	OnTrack          bool      `json:"on_track"`
	SchemaVersion    string    `json:"schema_version"`
}

// ValidationError holds validation result details
type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

// ValidationResult holds the validation outcome
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// Server holds server state and NATS connection
type Server struct {
	nc              *nats.Conn
	js              nats.JetStreamContext
	addr            string
	natsURL         string
	eventCount      int64
	eventCountMutex sync.Mutex
}

// NewServer creates and initializes a new Server
func NewServer(addr string, natsURL string) *Server {
	return &Server{
		addr:    addr,
		natsURL: natsURL,
	}
}

// Connect establishes NATS connection
func (s *Server) Connect() error {
	nc, err := nats.Connect(s.natsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS at %s: %w", s.natsURL, err)
	}
	s.nc = nc

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream context: %w", err)
	}
	s.js = js

	log.Printf("✅ Connected to NATS at %s", s.natsURL)
	return nil
}

// Close closes NATS connection
func (s *Server) Close() {
	if s.nc != nil {
		s.nc.Close()
	}
}

// ValidateEvent checks if a telemetry event is within acceptable ranges
func (s *Server) ValidateEvent(event TelemetryEvent) ValidationResult {
	result := ValidationResult{Valid: true, Errors: []ValidationError{}}

	// Required fields
	if event.EventID == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "event_id",
			Message: "required",
		})
	}

	if event.CarID == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "car_id",
			Message: "required",
		})
	}

	if event.SchemaVersion == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "schema_version",
			Message: "required",
		})
	}

	// Validate ranges
	if event.SpeedKmh < 0 || event.SpeedKmh > 400 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "speed_kmh",
			Message: "must be 0-400",
			Value:   event.SpeedKmh,
		})
	}

	if event.RPM < 0 || event.RPM > 10000 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "rpm",
			Message: "must be 0-10000",
			Value:   event.RPM,
		})
	}

	if event.ThrottlePercent < 0 || event.ThrottlePercent > 100 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "throttle_percent",
			Message: "must be 0-100",
			Value:   event.ThrottlePercent,
		})
	}

	if event.BrakePressureBar < 0 || event.BrakePressureBar > 25 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "brake_pressure_bar",
			Message: "must be 0-25",
			Value:   event.BrakePressureBar,
		})
	}

	// Tire temperatures
	for _, temp := range []float64{event.TireFlTemp, event.TireFrTemp, event.TireRlTemp, event.TireRrTemp} {
		if temp < 0 || temp > 200 {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   "tire_temp",
				Message: "must be 0-200°C",
				Value:   temp,
			})
			break
		}
	}

	if event.BrakeTemp < 0 || event.BrakeTemp > 600 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "brake_temp",
			Message: "must be 0-600°C",
			Value:   event.BrakeTemp,
		})
	}

	if event.FuelLevelLiters < 0 || event.FuelLevelLiters > 100 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "fuel_level_liters",
			Message: "must be 0-100",
			Value:   event.FuelLevelLiters,
		})
	}

	if event.FuelFlowLph < 0 || event.FuelFlowLph > 250 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "fuel_flow_lph",
			Message: "must be 0-250",
			Value:   event.FuelFlowLph,
		})
	}

	// Check for NaN/Inf
	floatFields := map[string]float64{
		"speed_kmh":        event.SpeedKmh,
		"rpm":              event.RPM,
		"throttle_percent": event.ThrottlePercent,
		"brake_pressure":   event.BrakePressureBar,
		"tire_fl_temp":     event.TireFlTemp,
		"tire_fr_temp":     event.TireFrTemp,
		"tire_rl_temp":     event.TireRlTemp,
		"tire_rr_temp":     event.TireRrTemp,
		"fuel_level":       event.FuelLevelLiters,
		"fuel_flow":        event.FuelFlowLph,
		"brake_temp":       event.BrakeTemp,
	}

	for field, value := range floatFields {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   field,
				Message: "NaN or Infinity value",
				Value:   value,
			})
		}
	}

	return result
}

// IngestHandler receives and validates telemetry events
func (s *Server) IngestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event TelemetryEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		log.Printf("❌ Failed to decode event: %v", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate event
	validation := s.ValidateEvent(event)
	if !validation.Valid {
		log.Printf("❌ Event validation failed for %s: %+v", event.CarID, validation.Errors)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  "validation failed",
			"errors": validation.Errors,
		})
		return
	}

	// Publish to NATS JetStream
	stream := fmt.Sprintf("TELEMETRY.%s", event.CarID)
	eventJSON, _ := json.Marshal(event)

	_, err := s.js.Publish(stream, eventJSON)
	if err != nil {
		log.Printf("❌ Failed to publish to NATS stream %s: %v", stream, err)
		http.Error(w, "failed to publish event", http.StatusInternalServerError)
		return
	}

	// Increment counter
	s.eventCountMutex.Lock()
	s.eventCount++
	s.eventCountMutex.Unlock()

	// Return 202 Accepted
	w.WriteHeader(http.StatusAccepted)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "accepted",
		"event_id": event.EventID,
		"car_id":   event.CarID,
		"stream":   stream,
	})
}

// HealthHandler returns service health status
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check NATS connection
	natsHealthy := false
	if s.nc != nil {
		natsHealthy = s.nc.IsConnected()
	}

	s.eventCountMutex.Lock()
	count := s.eventCount
	s.eventCountMutex.Unlock()

	status := map[string]interface{}{
		"status":          "healthy",
		"nats_connected":  natsHealthy,
		"events_ingested": count,
		"timestamp":       time.Now().UTC(),
	}

	if !natsHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
		status["status"] = "degraded"
	}

	json.NewEncoder(w).Encode(status)
}

// Start begins the HTTP server
func (s *Server) Start() error {
	http.HandleFunc("/telemetry/ingest", s.IngestHandler)
	http.HandleFunc("/health", s.HealthHandler)

	log.Printf("🚀 Ingestion Service listening on %s", s.addr)
	return http.ListenAndServe(s.addr, nil)
}
