package main

import "time"

// TelemetryEvent mirrors the payload the ingestion service publishes on TELEMETRY.{CAR_ID}.
// Only the fields the processor aggregates are decoded.
type TelemetryEvent struct {
	Timestamp       time.Time `json:"timestamp"`
	CarID           string    `json:"car_id"`
	SessionID       string    `json:"session_id"`
	Lap             int       `json:"lap"`
	SpeedKmh        float64   `json:"speed_kmh"`
	RPM             float64   `json:"rpm"`
	TireFlTemp      float64   `json:"tire_fl_temp"`
	TireFrTemp      float64   `json:"tire_fr_temp"`
	TireRlTemp      float64   `json:"tire_rl_temp"`
	TireRrTemp      float64   `json:"tire_rr_temp"`
	FuelLevelLiters float64   `json:"fuel_level_liters"`
	BrakeTemp       float64   `json:"brake_temp"`
	InPit           bool      `json:"in_pit"`
}

// LapMetrics is one completed lap for one car, written to the lap_metrics table.
type LapMetrics struct {
	SessionID   string
	CarID       string
	Lap         int
	StartedAt   time.Time
	EndedAt     time.Time
	LapTimeS    float64
	Samples     int
	AvgSpeedKmh float64
	MaxSpeedKmh float64
	MaxRPM      float64
	FuelStartL  float64
	FuelEndL    float64
	FuelUsedL   float64
	AvgTireFL   float64
	AvgTireFR   float64
	AvgTireRL   float64
	AvgTireRR   float64
	MaxTireC    float64
	MaxBrakeC   float64
	Pitted      bool
}

// Alert is a threshold breach, written to the alerts table.
type Alert struct {
	SessionID string
	CarID     string
	Lap       int
	Kind      string
	Value     float64
	Threshold float64
	Detail    string
	RaisedAt  time.Time
}
