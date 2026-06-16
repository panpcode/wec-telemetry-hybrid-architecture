package main

import (
	"math"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

// CarSimulator maintains the physics state for a single car across its race.
type CarSimulator struct {
	config CarConfig

	// Session identity
	sessionID string

	// Track progress
	lap         int
	lapDistance float64 // km driven in the current lap
	lapStart    time.Time

	// Dynamics
	speed         float64 // km/h
	rpm           float64
	throttle      float64 // 0–1
	brakePressure float64 // bar

	// Temperatures (°C)
	tireTempFL, tireTempFR float64
	tireTempRL, tireTempRR float64
	brakeTemp              float64

	// Consumables
	fuelLevel    float64 // liters remaining
	fuelBurnRate float64 // L/h (most recent step)

	// Pit stop state
	inPit      bool
	pitElapsed float64 // seconds accumulated in current pit stop
	lastPitLap int     // lap on which the last pit stop ended

	eventCount int
}

// NewCarSimulator initialises a simulator ready for the first lap.
func NewCarSimulator(config CarConfig) *CarSimulator {
	return &CarSimulator{
		config:    config,
		sessionID: "SESSION_" + time.Now().Format("20060102_150405"),

		fuelLevel:  config.FuelCapacity,
		tireTempFL: 70, tireTempFR: 70,
		tireTempRL: 70, tireTempRR: 70,
		brakeTemp:  100,
		lapStart:   time.Now(),

		// Pre-offset so the first pit stop happens after PitEveryLaps laps.
		lastPitLap: -config.PitEveryLaps,
	}
}

// clamp restricts v to the [lo, hi] range.
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// safeRound rounds to n decimal places; returns 0 for NaN/Inf to avoid
// corrupted JSON in the telemetry stream.
func safeRound(v float64, n int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	f := math.Pow(10, float64(n))
	return math.Round(v*f) / f
}

// inBrakingZone returns true when lapProgress falls inside any of the
// track's defined braking zones.
func inBrakingZone(progress float64, zones []BrakingZone) bool {
	for _, z := range zones {
		if progress >= z.Start && progress <= z.End {
			return true
		}
	}
	return false
}

// Step advances the simulation by dt seconds and returns the resulting event.
func (cs *CarSimulator) Step(dt float64) TelemetryEvent {
	// Guard against extreme dt that would blow up the physics.
	dt = clamp(dt, 0, 0.1)
	track := cs.config.Track

	// ── PIT STOP ───────────────────────────────────────────────────────────────
	// While in the pit: car is stationary, tires cool, tank refills.
	// The pit stop ends after PitDurationSec of accumulated time.
	if cs.inPit {
		cs.pitElapsed += dt

		// Refuel at ~3 L/s (realistic fast-fill rig rate).
		cs.fuelLevel = clamp(cs.fuelLevel+3.0*dt, 0, cs.config.FuelCapacity)

		// Tires cool toward 70 °C baseline while the car sits still.
		cs.tireTempFL += (70 - cs.tireTempFL) * 0.15
		cs.tireTempFR += (70 - cs.tireTempFR) * 0.15
		cs.tireTempRL += (70 - cs.tireTempRL) * 0.15
		cs.tireTempRR += (70 - cs.tireTempRR) * 0.15

		cs.speed = 0
		cs.rpm = 0
		cs.throttle = 0
		cs.brakePressure = 0
		cs.fuelBurnRate = 0

		if cs.pitElapsed >= cs.config.PitDurationSec {
			// Pit complete — return to track.
			cs.inPit = false
			cs.pitElapsed = 0
			cs.lastPitLap = cs.lap
		}
		return cs.buildEvent(0, false)
	}

	// ── THROTTLE & BRAKE ───────────────────────────────────────────────────────
	// Compute lap progress (0–1) and decide throttle/brake based on whether
	// the car is approaching a corner or on a straight.
	lapProgress := math.Mod(cs.lapDistance, track.LengthKm) / track.LengthKm
	if math.IsNaN(lapProgress) {
		lapProgress = 0
	}

	if inBrakingZone(lapProgress, track.BrakingZones) {
		// Driver lifting and braking for a corner.
		cs.throttle = math.Max(0, cs.throttle-0.15)
		cs.brakePressure = math.Min(20, cs.brakePressure+0.2)
	} else {
		// Full throttle on the straight.
		cs.throttle = math.Min(1.0, cs.throttle+0.1)
		cs.brakePressure = math.Max(0, cs.brakePressure-0.25)
	}

	// ── SPEED ──────────────────────────────────────────────────────────────────
	// Throttle position → target RPM → speed, with brake deceleration and
	// a sinusoidal corner-speed penalty layered on top.
	targetRPM := 3000 + cs.throttle*6500
	cs.rpm = clamp(cs.rpm+(targetRPM-cs.rpm)*0.1*dt*1000, 0, cs.config.MaxRPM*1.1)
	cs.speed = (cs.rpm / cs.config.MaxRPM) * track.MaxSpeedKmh
	cs.speed *= (1 - clamp(cs.brakePressure*0.01, 0, 1))
	cornerPenalty := math.Abs(math.Sin(lapProgress*math.Pi*4)) * 0.3
	cs.speed *= (1 - cornerPenalty)
	cs.speed = clamp(cs.speed, 0, track.MaxSpeedKmh*1.2)

	// ── DISTANCE & LAP COUNTING ────────────────────────────────────────────────
	distKm := (cs.speed / 3600) * dt
	cs.lapDistance += distKm

	if cs.lapDistance >= track.LengthKm {
		cs.lap++
		cs.lapDistance = 0
		cs.lapStart = time.Now()

		// Check whether a pit stop is due at this lap crossing.
		if cs.config.PitEveryLaps > 0 && cs.lap-cs.lastPitLap >= cs.config.PitEveryLaps {
			cs.inPit = true
			cs.pitElapsed = 0
		}
	}

	// ── TIRE TEMPERATURES ──────────────────────────────────────────────────────
	// Thermal load is a function of speed (aerodynamic + road friction) and
	// brake pressure (heat transferred to rubber). Each tire drifts toward the
	// target temperature with small Gaussian noise per step.
	thermalLoad := (cs.speed/track.MaxSpeedKmh)*100 + cs.brakePressure*3
	thermalLoad = clamp(thermalLoad, 0, 200)
	target := 70 + thermalLoad
	noise := func() float64 { return rand.NormFloat64() * 0.1 }

	cs.tireTempFL = clamp(cs.tireTempFL+(target-cs.tireTempFL)*0.05+noise(), 30, 150)
	cs.tireTempFR = clamp(cs.tireTempFR+(target-cs.tireTempFR)*0.05+noise(), 30, 150)
	cs.tireTempRL = clamp(cs.tireTempRL+(target-cs.tireTempRL)*0.05+noise(), 30, 150)
	cs.tireTempRR = clamp(cs.tireTempRR+(target-cs.tireTempRR)*0.05+noise(), 30, 150)

	// ── FUEL ───────────────────────────────────────────────────────────────────
	// Burn rate peaks under high throttle and high RPM; unit is L/h.
	cs.fuelBurnRate = cs.throttle*150 + (cs.rpm/cs.config.MaxRPM)*50
	cs.fuelLevel -= (cs.fuelBurnRate / 3600) * dt
	cs.fuelLevel = clamp(cs.fuelLevel, 0, cs.config.FuelCapacity*1.01)

	// ── BRAKE TEMPERATURE ─────────────────────────────────────────────────────
	cs.brakeTemp += cs.brakePressure * 100 * dt
	cs.brakeTemp *= 0.98 // passive cooling
	cs.brakeTemp = clamp(cs.brakeTemp, 80, 500)

	sector := int(lapProgress * float64(track.Sectors))
	return cs.buildEvent(sector, true)
}

// buildEvent assembles a TelemetryEvent from the current simulator state.
// GPS coordinates trace a rough ellipse around the circuit's geographic centre.
func (cs *CarSimulator) buildEvent(sector int, onTrack bool) TelemetryEvent {
	cs.eventCount++
	track := cs.config.Track

	angle := (cs.lapDistance / track.LengthKm) * 2 * math.Pi
	radius := track.LengthKm / (2 * math.Pi) * 0.01 // approx degrees
	gpsLat := track.GpsCenterLat + math.Sin(angle)*radius
	gpsLon := track.GpsCenterLon + math.Cos(angle)*radius*1.5

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
		FuelFlowLph:      safeRound(cs.fuelBurnRate, 1),
		BrakeTemp:        safeRound(cs.brakeTemp, 1),
		GpsLat:           safeRound(gpsLat, 6),
		GpsLong:          safeRound(gpsLon, 6),
		OnTrack:          onTrack,
		InPit:            cs.inPit,
		SchemaVersion:    "1.0",
	}
}
