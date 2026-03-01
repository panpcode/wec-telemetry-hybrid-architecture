package main

import (
	"math"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

// CarSimulator simulates a racing car with realistic physics
type CarSimulator struct {
	config        CarConfig
	sessionID     string
	lap           int
	lapDistance   float64 // km
	speed         float64 // km/h
	rpm           float64
	fuelLevel     float64 // liters
	throttle      float64 // 0-1
	brakePressure float64 // 0-20 bar

	// Tire state
	tireTempFL float64
	tireTempFR float64
	tireTempRL float64
	tireTempRR float64

	// Brake state
	brakeTemp float64 // celsius

	// Statistics
	lapStartTime time.Time
	eventCount   int
}

// NewCarSimulator creates a new car simulator
func NewCarSimulator(config CarConfig) *CarSimulator {
	return &CarSimulator{
		config:       config,
		sessionID:    "SESSION_" + time.Now().Format("20060102_150405"),
		lap:          0,
		fuelLevel:    config.FuelCapacity,
		tireTempFL:   70,
		tireTempFR:   70,
		tireTempRL:   70,
		tireTempRR:   70,
		brakeTemp:    100,
		lapStartTime: time.Now(),
	}
}

// clamp restricts a float64 value to a min/max range
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// safeRound rounds and validates a float, returning 0 if NaN/Inf
func safeRound(value float64, decimals int) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	multiplier := math.Pow(10, float64(decimals))
	return math.Round(value*multiplier) / multiplier
}

// Step simulates one timestep (dt in seconds)
func (cs *CarSimulator) Step(dt float64) TelemetryEvent {
	// Clamp dt to prevent physics explosion
	dt = clamp(dt, 0, 0.1)

	// Calculate lap progress (0 to 1)
	lapProgress := math.Mod(cs.lapDistance, cs.config.TrackLength) / cs.config.TrackLength
	if math.IsNaN(lapProgress) {
		lapProgress = 0
	}

	// ===== ACCELERATION/BRAKING LOGIC =====
	// Simulates a simple track: brake in corners, accelerate on straights
	if lapProgress < 0.3 || (0.5 < lapProgress && lapProgress < 0.7) {
		// Braking zones
		cs.throttle = math.Max(0, cs.throttle-0.15)
		cs.brakePressure = math.Min(20, cs.brakePressure+0.2)
	} else {
		// Acceleration zones
		cs.throttle = math.Min(1.0, cs.throttle+0.1)
		cs.brakePressure = math.Max(0, cs.brakePressure-0.25)
	}

	// ===== SPEED CALCULATION =====
	// Throttle → RPM → Speed
	targetRPM := 3000 + (cs.throttle * 6500) // 3000-9500 RPM range
	cs.rpm = clamp(cs.rpm+(targetRPM-cs.rpm)*0.1*dt*1000, 0, cs.config.MaxRPM*1.1)

	// RPM → Speed (simplified: 8000 RPM = max speed)
	cs.speed = (cs.rpm / cs.config.MaxRPM) * cs.config.MaxSpeed

	// Braking decelerates (each bar of pressure reduces speed)
	cs.speed *= (1 - clamp(cs.brakePressure*0.01, 0, 1))

	// Corner slowdown (sinusoidal speed variation on track)
	cornerPenalty := math.Abs(math.Sin(lapProgress*math.Pi*4)) * 0.3
	cs.speed *= (1 - cornerPenalty)
	cs.speed = clamp(cs.speed, 0, cs.config.MaxSpeed*1.2)

	// ===== DISTANCE & LAP TRACKING =====
	distanceKm := (cs.speed / 3600) * dt // km traveled in dt
	cs.lapDistance += distanceKm

	// Lap complete?
	if cs.lapDistance >= cs.config.TrackLength {
		cs.lap++
		cs.lapDistance = 0
		cs.lapStartTime = time.Now()
	}

	// ===== TIRE TEMPERATURE =====
	// Based on speed + brake pressure + lateral g's
	thermalLoad := (cs.speed/cs.config.MaxSpeed)*100 + cs.brakePressure*3
	thermalLoad = clamp(thermalLoad, 0, 200)

	for i := 0; i < 4; i++ {
		targetTemp := 70 + thermalLoad
		// Reduce noise amplitude to prevent NaN propagation
		noise := rand.NormFloat64() * 0.1

		switch i {
		case 0: // FL
			cs.tireTempFL += (targetTemp - cs.tireTempFL) * 0.05
			cs.tireTempFL += noise
		case 1: // FR
			cs.tireTempFR += (targetTemp - cs.tireTempFR) * 0.05
			cs.tireTempFR += noise
		case 2: // RL
			cs.tireTempRL += (targetTemp - cs.tireTempRL) * 0.05
			cs.tireTempRL += noise
		case 3: // RR
			cs.tireTempRR += (targetTemp - cs.tireTempRR) * 0.05
			cs.tireTempRR += noise
		}
	}
	// Clamp all tire temps to valid ranges
	cs.tireTempFL = clamp(cs.tireTempFL, 30, 150)
	cs.tireTempFR = clamp(cs.tireTempFR, 30, 150)
	cs.tireTempRL = clamp(cs.tireTempRL, 30, 150)
	cs.tireTempRR = clamp(cs.tireTempRR, 30, 150)

	// ===== FUEL CONSUMPTION =====
	// liters/hour based on throttle + RPM
	fuelBurnRate := (cs.throttle * 150) + (cs.rpm / cs.config.MaxRPM * 50) // L/h
	cs.fuelLevel -= (fuelBurnRate / 3600) * dt
	cs.fuelLevel = clamp(cs.fuelLevel, 0, cs.config.FuelCapacity*1.01)

	// ===== BRAKE TEMPERATURE =====
	brakeHeat := cs.brakePressure * 100
	cs.brakeTemp += brakeHeat * dt
	cs.brakeTemp *= 0.98 // Cool down
	cs.brakeTemp = clamp(cs.brakeTemp, 80, 500)

	// ===== GENERATE EVENT =====
	cs.eventCount++
	sector := int((lapProgress) * 3)

	gpsLat := 48.2645 + (math.Sin(float64(cs.lap)*0.1) * 0.01)
	gpsLong := 11.6265 + (math.Cos(float64(cs.lap)*0.1) * 0.01)

	return TelemetryEvent{
		EventID:          uuid.New().String(),
		Timestamp:        time.Now().UTC(),
		CarID:            cs.config.CarID,
		SessionID:        cs.sessionID,
		Lap:              cs.lap,
		Sector:           sector,
		SpeedKmh:         safeRound(cs.speed, 1),
		RPM:              safeRound(cs.rpm, 0),
		ThrottlePercent:  safeRound(cs.throttle*100, 1),
		BrakePressureBar: safeRound(cs.brakePressure, 2),
		TireFlTemp:       safeRound(cs.tireTempFL, 1),
		TireFrTemp:       safeRound(cs.tireTempFR, 1),
		TireRlTemp:       safeRound(cs.tireTempRL, 1),
		TireRrTemp:       safeRound(cs.tireTempRR, 1),
		FuelLevelLiters:  safeRound(cs.fuelLevel, 2),
		FuelFlowLph:      safeRound(fuelBurnRate, 1),
		BrakeTemp:        safeRound(cs.brakeTemp, 1),
		GpsLat:           gpsLat,
		GpsLong:          gpsLong,
		OnTrack:          true,
		SchemaVersion:    "1.0",
	}
}
