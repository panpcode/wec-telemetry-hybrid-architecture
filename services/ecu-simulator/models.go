package main

import (
	"time"
)

// TelemetryEvent represents a single ECU telemetry reading
type TelemetryEvent struct {
	EventID              string    `json:"event_id"`
	Timestamp            time.Time `json:"timestamp"`
	CarID                string    `json:"car_id"`
	SessionID            string    `json:"session_id"`
	Lap                  int       `json:"lap"`
	Sector               int       `json:"sector"`
	SpeedKmh             float64   `json:"speed_kmh"`
	RPM                  float64   `json:"rpm"`
	ThrottlePercent      float64   `json:"throttle_percent"`
	BrakePressureBar     float64   `json:"brake_pressure_bar"`
	TireFlTemp           float64   `json:"tire_fl_temp"`
	TireFrTemp           float64   `json:"tire_fr_temp"`
	TireRlTemp           float64   `json:"tire_rl_temp"`
	TireRrTemp           float64   `json:"tire_rr_temp"`
	FuelLevelLiters      float64   `json:"fuel_level_liters"`
	FuelFlowLph          float64   `json:"fuel_flow_lph"`
	BrakeTemp            float64   `json:"brake_temp"`
	GpsLat               float64   `json:"gps_lat"`
	GpsLong              float64   `json:"gps_long"`
	OnTrack              bool      `json:"on_track"`
	SchemaVersion        string    `json:"schema_version"`
}

// CarConfig holds configuration for a simulated car
type CarConfig struct {
	CarID       string
	TrackLength float64 // km
	MaxSpeed    float64 // km/h
	MaxRPM      float64
	FuelCapacity float64 // liters
}

// DefaultCarConfig returns default racing car configuration
func DefaultCarConfig(carID string) CarConfig {
	return CarConfig{
		CarID:        carID,
		TrackLength:  13.6, // Le Mans track length
		MaxSpeed:     330,  // km/h
		MaxRPM:       9500,
		FuelCapacity: 75.0,
	}
}
