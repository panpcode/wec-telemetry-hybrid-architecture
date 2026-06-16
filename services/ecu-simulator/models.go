package main

import "time"

// TelemetryEvent is one sensor snapshot emitted by a car at ~20 Hz.
// Every field is present even if the car is stationary (pit stop).
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
	InPit            bool      `json:"in_pit"`
	SchemaVersion    string    `json:"schema_version"`
}

// BrakingZone is a stretch of circuit (expressed as 0–1 lap-progress fraction)
// where the driver brakes hard before a corner.
type BrakingZone struct {
	Start float64
	End   float64
}

// TrackConfig holds circuit-specific parameters that drive the physics model.
type TrackConfig struct {
	Name         string
	LengthKm     float64
	MaxSpeedKmh  float64
	Sectors      int
	BrakingZones []BrakingZone // fractions of lap where the car brakes
	GpsCenterLat float64       // geographic center used for GPS simulation
	GpsCenterLon float64
}

// TrackByName returns the config for a named circuit, defaulting to Le Mans.
func TrackByName(name string) TrackConfig {
	switch name {
	case "spa":
		return spaTrack()
	case "imola":
		return imolaTrack()
	default:
		return leMansTrack()
	}
}

// leMansTrack defines the Circuit de la Sarthe (13.626 km).
// Key features: very long Mulsanne / Hunaudières straight (top speed ~340 km/h),
// tight Ford Chicanes at the start, slow Arnage corner.
func leMansTrack() TrackConfig {
	return TrackConfig{
		Name:        "le_mans",
		LengthKm:    13.626,
		MaxSpeedKmh: 340,
		Sectors:     3,
		BrakingZones: []BrakingZone{
			{0.04, 0.10}, // Ford Chicanes (after start/finish straight)
			{0.44, 0.52}, // Tertre Rouge (end of Hunaudières)
			{0.68, 0.75}, // Indianapolis
			{0.82, 0.90}, // Arnage hairpin
		},
		GpsCenterLat: 47.9497,
		GpsCenterLon: 0.2076,
	}
}

// spaTrack defines Spa-Francorchamps (7.004 km).
// Challenging elevation changes; Eau Rouge / Raidillon section requires full commitment.
func spaTrack() TrackConfig {
	return TrackConfig{
		Name:        "spa",
		LengthKm:    7.004,
		MaxSpeedKmh: 295,
		Sectors:     3,
		BrakingZones: []BrakingZone{
			{0.02, 0.09}, // La Source hairpin
			{0.14, 0.20}, // Raidillon / Eau Rouge
			{0.45, 0.53}, // Pouhon high-speed chicane
			{0.72, 0.80}, // Bus Stop chicane
		},
		GpsCenterLat: 50.4372,
		GpsCenterLon: 5.9714,
	}
}

// imolaTrack defines Autodromo Enzo e Dino Ferrari (4.909 km).
// Tight, technical layout with little room for error — Tamburello, Tosa, Rivazza.
func imolaTrack() TrackConfig {
	return TrackConfig{
		Name:        "imola",
		LengthKm:    4.909,
		MaxSpeedKmh: 275,
		Sectors:     3,
		BrakingZones: []BrakingZone{
			{0.06, 0.14}, // Tamburello chicane
			{0.32, 0.42}, // Tosa hairpin
			{0.62, 0.72}, // Rivazza
		},
		GpsCenterLat: 44.3440,
		GpsCenterLon: 11.7167,
	}
}

// CarConfig holds per-car settings derived from team and circuit choices.
type CarConfig struct {
	CarID          string
	Track          TrackConfig
	MaxRPM         float64
	FuelCapacity   float64 // liters
	PitEveryLaps   int     // laps between pit stops (0 = no pit stops)
	PitDurationSec float64 // how long a stop takes in real seconds
}

// DefaultCarConfig returns sensible defaults for a WEC LMP2 / Hypercar.
func DefaultCarConfig(carID string, track TrackConfig, pitEveryLaps int) CarConfig {
	return CarConfig{
		CarID:          carID,
		Track:          track,
		MaxRPM:         9500,
		FuelCapacity:   75.0,
		PitEveryLaps:   pitEveryLaps,
		PitDurationSec: 30.0,
	}
}
