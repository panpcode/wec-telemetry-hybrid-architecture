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
	rng    *rand.Rand // per-car source so cars don't produce identical noise

	// Session identity (shared by every car in the run)
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
	tireAgeLaps  int     // laps completed on the current set of tyres

	// Lap-to-lap driver variation, re-drawn at every line crossing.
	lapPace  float64 // ±~0.4 % on top speed
	lapBrake float64 // ±~4 % on braking effort

	// Pit stop state
	inPit      bool
	pitElapsed float64 // seconds accumulated in current pit stop
	lastPitLap int     // lap on which the last pit stop ended

	eventCount int
}

// Physics constants shared by all cars.
const (
	maxBrakeBar     = 20.0  // peak pedal pressure (ingestion accepts up to 25)
	brakeDecelKmhS  = 140.0 // deceleration at full brake pressure (~4 g)
	accelKmhS       = 45.0  // full-throttle acceleration from low speed
	cornerSpeedFrac = 0.40  // apex speed as a fraction of the car's top speed
	shiftRPM        = 9200.0
	idleRPM         = 3000.0
	gears           = 7
	ambientBrakeC   = 150.0
	pitTireC        = 70.0
)

// NewCarSimulator initialises a simulator ready for the first lap.
func NewCarSimulator(config CarConfig, sessionID string, seed int64) *CarSimulator {
	return &CarSimulator{
		config:    config,
		rng:       rand.New(rand.NewSource(seed)),
		sessionID: sessionID,

		fuelLevel:  config.FuelCapacity,
		tireTempFL: pitTireC, tireTempFR: pitTireC,
		tireTempRL: pitTireC, tireTempRR: pitTireC,
		brakeTemp: ambientBrakeC,
		lapStart:  time.Now(),
		lapPace:   1,
		lapBrake:  1,
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

// approach moves v toward target by at most rate*dt.
func approach(v, target, rate, dt float64) float64 {
	step := rate * dt
	if v < target {
		return math.Min(target, v+step)
	}
	return math.Max(target, v-step)
}

// relax moves v toward target with time constant 1/k seconds (first-order lag).
// Stable for any dt because the blend factor is capped at 1.
func relax(v, target, k, dt float64) float64 {
	return v + (target-v)*math.Min(1, k*dt)
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

// gearRPM maps road speed to engine RPM through a 7-speed gearbox:
// each gear tops out at shiftRPM, giving the familiar saw-tooth trace.
func gearRPM(speed, vmax float64) float64 {
	for g := 1; g <= gears; g++ {
		top := vmax * float64(g) / gears
		if speed <= top || g == gears {
			return math.Max(idleRPM, shiftRPM*speed/top)
		}
	}
	return idleRPM
}

// Step advances the simulation by dt seconds and returns the resulting event.
func (cs *CarSimulator) Step(dt float64) TelemetryEvent {
	// Guard against extreme dt that would blow up the physics.
	dt = clamp(dt, 0, 0.1)
	track := cs.config.Track
	noise := func(sd float64) float64 { return cs.rng.NormFloat64() * sd }

	// ── PIT STOP ───────────────────────────────────────────────────────────────
	// While in the pit: car is stationary, tyres are changed, tank refills.
	if cs.inPit {
		cs.pitElapsed += dt
		cs.fuelLevel = clamp(cs.fuelLevel+3.0*dt, 0, cs.config.FuelCapacity) // ~3 L/s rig
		cs.brakeTemp = relax(cs.brakeTemp, ambientBrakeC, 0.1, dt)
		cs.speed, cs.rpm, cs.throttle, cs.brakePressure, cs.fuelBurnRate = 0, 0, 0, 0, 0

		// Fresh tyres go on halfway through the stop.
		if cs.pitElapsed >= cs.config.PitDurationSec/2 && cs.tireAgeLaps > 0 {
			cs.tireAgeLaps = 0
			cs.tireTempFL, cs.tireTempFR, cs.tireTempRL, cs.tireTempRR = pitTireC, pitTireC, pitTireC, pitTireC
		}
		if cs.pitElapsed >= cs.config.PitDurationSec {
			cs.inPit = false
			cs.pitElapsed = 0
			cs.lastPitLap = cs.lap
		}
		return cs.buildEvent(0, false)
	}

	// ── DRIVER INPUTS & SPEED ──────────────────────────────────────────────────
	// In a braking zone the driver brakes hard down to apex speed, then holds
	// a partial throttle through the corner. Elsewhere it's flat out.
	lapProgress := clamp(cs.lapDistance/track.LengthKm, 0, 1)
	vmax := track.MaxSpeedKmh * cs.config.Pace * cs.lapPace
	brakeEffort := cs.config.BrakeAggression * cs.lapBrake
	apex := vmax * cornerSpeedFrac

	if inBrakingZone(lapProgress, track.BrakingZones) {
		if cs.speed > apex*1.03 {
			cs.throttle = approach(cs.throttle, 0, 8, dt)
			cs.brakePressure = approach(cs.brakePressure, maxBrakeBar*brakeEffort, 80, dt)
		} else {
			cs.throttle = approach(cs.throttle, 0.35, 3, dt)
			cs.brakePressure = approach(cs.brakePressure, 0, 80, dt)
		}
	} else {
		cs.throttle = approach(cs.throttle, 1, 4, dt)
		cs.brakePressure = approach(cs.brakePressure, 0, 80, dt)
	}
	cs.brakePressure = clamp(cs.brakePressure, 0, maxBrakeBar*1.15)

	brakeFrac := cs.brakePressure / maxBrakeBar
	accel := cs.throttle * accelKmhS * (1 - math.Pow(cs.speed/vmax, 2))
	cs.speed += (accel - brakeFrac*brakeDecelKmhS) * dt
	if cs.brakePressure > 0 {
		cs.speed = math.Max(cs.speed, apex*0.97) // drivers don't over-slow a corner
	}
	cs.speed = clamp(cs.speed, 0, vmax)

	cs.rpm = clamp(gearRPM(cs.speed, vmax)+noise(25), 0, cs.config.MaxRPM)

	// ── DISTANCE & LAP COUNTING ────────────────────────────────────────────────
	cs.lapDistance += (cs.speed / 3600) * dt
	if cs.lapDistance >= track.LengthKm {
		cs.lap++
		cs.tireAgeLaps++
		cs.lapDistance = 0
		cs.lapStart = time.Now()
		cs.lapPace = 1 + clamp(noise(0.004), -0.01, 0.01)
		cs.lapBrake = 1 + clamp(noise(0.04), -0.1, 0.1)

		if cs.config.PitEveryLaps > 0 && cs.lap-cs.lastPitLap >= cs.config.PitEveryLaps {
			cs.inPit = true
			cs.pitElapsed = 0
		}
	}

	// ── TYRE TEMPERATURES ──────────────────────────────────────────────────────
	// Operating window is ~85–105 °C. Speed adds load on every tyre, braking
	// heats the fronts, traction heats the rears, and worn tyres run hotter
	// (+0.9 °C per lap of stint) — so a long stint eventually triggers alerts.
	speedFrac := cs.speed / vmax
	base := 78 + 18*speedFrac + 0.9*float64(cs.tireAgeLaps)
	front := base + 16*brakeFrac
	rear := base + 8*cs.throttle*(1-speedFrac)
	k := 0.5 // ~2 s thermal time constant
	cs.tireTempFL = clamp(relax(cs.tireTempFL, front, k, dt)+noise(0.05), 30, 160)
	cs.tireTempFR = clamp(relax(cs.tireTempFR, front+3, k, dt)+noise(0.05), 30, 160)
	cs.tireTempRL = clamp(relax(cs.tireTempRL, rear-2, k, dt)+noise(0.05), 30, 160)
	cs.tireTempRR = clamp(relax(cs.tireTempRR, rear+1, k, dt)+noise(0.05), 30, 160)

	// ── FUEL ───────────────────────────────────────────────────────────────────
	// L/h: idle draw plus a load term from throttle × RPM (~150 L/h flat out),
	// giving ~13-lap stints at Le Mans on a 75 L tank.
	cs.fuelBurnRate = 10 + 140*cs.throttle*(cs.rpm/shiftRPM)
	cs.fuelLevel = clamp(cs.fuelLevel-(cs.fuelBurnRate/3600)*dt, 0, cs.config.FuelCapacity)

	// ── BRAKE TEMPERATURE ─────────────────────────────────────────────────────
	// Heat input scales with brake pressure × speed (kinetic energy being
	// dumped); airflow cooling scales with speed.
	heat := brakeFrac * speedFrac * 260 * brakeEffort
	cool := (cs.brakeTemp - ambientBrakeC) * (0.04 + 0.22*speedFrac)
	cs.brakeTemp = clamp(cs.brakeTemp+(heat-cool)*dt, 80, 590)

	// Use the position *after* this step's movement: on the step that crosses
	// the line, lapProgress still reflects the end of the previous lap.
	sector := int(cs.lapDistance / track.LengthKm * float64(track.Sectors))
	if sector >= track.Sectors {
		sector = track.Sectors - 1
	}
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
