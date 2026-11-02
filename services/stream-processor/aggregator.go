package main

import (
	"fmt"
	"math"
	"time"
)

// Thresholds configure when an alert is raised.
type Thresholds struct {
	TireMaxC  float64
	BrakeMaxC float64
	FuelLowL  float64
}

// lapAccumulator collects running totals for the lap a car is currently on.
type lapAccumulator struct {
	sessionID string
	carID     string
	lap       int
	partial   bool // true when the processor joined mid-lap, so the totals are incomplete

	first, last time.Time
	samples     int

	sumSpeed, maxSpeed, maxRPM float64
	fuelStart, fuelEnd         float64
	sumFL, sumFR, sumRL, sumRR float64
	maxTire, maxBrake          float64
	pitted                     bool
	alerted                    map[string]bool // alert kinds already raised this lap
}

func newLap(e TelemetryEvent, partial bool) *lapAccumulator {
	return &lapAccumulator{
		sessionID: e.SessionID,
		carID:     e.CarID,
		lap:       e.Lap,
		partial:   partial,
		first:     e.Timestamp,
		fuelStart: e.FuelLevelLiters,
		alerted:   make(map[string]bool),
	}
}

func (a *lapAccumulator) add(e TelemetryEvent) {
	a.last = e.Timestamp
	a.samples++
	a.sumSpeed += e.SpeedKmh
	a.maxSpeed = math.Max(a.maxSpeed, e.SpeedKmh)
	a.maxRPM = math.Max(a.maxRPM, e.RPM)
	a.fuelEnd = e.FuelLevelLiters
	a.sumFL += e.TireFlTemp
	a.sumFR += e.TireFrTemp
	a.sumRL += e.TireRlTemp
	a.sumRR += e.TireRrTemp
	a.maxTire = math.Max(a.maxTire, maxTire(e))
	a.maxBrake = math.Max(a.maxBrake, e.BrakeTemp)
	a.pitted = a.pitted || e.InPit
}

func (a *lapAccumulator) metrics() LapMetrics {
	n := float64(a.samples)
	m := LapMetrics{
		SessionID:   a.sessionID,
		CarID:       a.carID,
		Lap:         a.lap,
		StartedAt:   a.first,
		EndedAt:     a.last,
		LapTimeS:    a.last.Sub(a.first).Seconds(),
		Samples:     a.samples,
		AvgSpeedKmh: a.sumSpeed / n,
		MaxSpeedKmh: a.maxSpeed,
		MaxRPM:      a.maxRPM,
		FuelStartL:  a.fuelStart,
		FuelEndL:    a.fuelEnd,
		AvgTireFL:   a.sumFL / n,
		AvgTireFR:   a.sumFR / n,
		AvgTireRL:   a.sumRL / n,
		AvgTireRR:   a.sumRR / n,
		MaxTireC:    a.maxTire,
		MaxBrakeC:   a.maxBrake,
		Pitted:      a.pitted,
	}
	// A refuel during the lap makes start-minus-end negative; report net burn only.
	m.FuelUsedL = math.Max(0, a.fuelStart-a.fuelEnd)
	return m
}

// Aggregator turns a per-car event stream into completed laps and alerts.
// It is not safe for concurrent use; the processor feeds it from one goroutine.
type Aggregator struct {
	th   Thresholds
	cars map[string]*lapAccumulator // keyed by session_id|car_id
}

func NewAggregator(th Thresholds) *Aggregator {
	return &Aggregator{th: th, cars: make(map[string]*lapAccumulator)}
}

// Process consumes one event. It returns the lap that this event closed (if any)
// and any alerts this event raised.
func (g *Aggregator) Process(e TelemetryEvent) (*LapMetrics, []Alert) {
	key := e.SessionID + "|" + e.CarID
	cur, ok := g.cars[key]

	var done *LapMetrics
	switch {
	case !ok:
		// First event we've seen for this car. Unless it's lap 0, we joined mid-lap.
		cur = newLap(e, e.Lap > 0)
		g.cars[key] = cur
	case e.Lap > cur.lap:
		if !cur.partial && cur.samples > 0 {
			m := cur.metrics()
			done = &m
		}
		cur = newLap(e, false)
		g.cars[key] = cur
	case e.Lap < cur.lap:
		// Out-of-order / redelivered event from a lap already closed: ignore.
		return nil, nil
	}

	cur.add(e)
	return done, g.checkAlerts(cur, e)
}

func (g *Aggregator) checkAlerts(cur *lapAccumulator, e TelemetryEvent) []Alert {
	var out []Alert
	raise := func(kind string, value, threshold float64, detail string) {
		if cur.alerted[kind] {
			return
		}
		cur.alerted[kind] = true
		out = append(out, Alert{
			SessionID: e.SessionID, CarID: e.CarID, Lap: e.Lap,
			Kind: kind, Value: value, Threshold: threshold,
			Detail: detail, RaisedAt: e.Timestamp,
		})
	}

	if t := maxTire(e); t > g.th.TireMaxC {
		raise("tire_overheat", t, g.th.TireMaxC, fmt.Sprintf("tire at %.1f°C (limit %.0f°C)", t, g.th.TireMaxC))
	}
	if e.BrakeTemp > g.th.BrakeMaxC {
		raise("brake_overheat", e.BrakeTemp, g.th.BrakeMaxC, fmt.Sprintf("brakes at %.0f°C (limit %.0f°C)", e.BrakeTemp, g.th.BrakeMaxC))
	}
	if !e.InPit && e.FuelLevelLiters < g.th.FuelLowL {
		raise("fuel_low", e.FuelLevelLiters, g.th.FuelLowL, fmt.Sprintf("%.1f L left (limit %.0f L)", e.FuelLevelLiters, g.th.FuelLowL))
	}
	return out
}

func maxTire(e TelemetryEvent) float64 {
	return math.Max(math.Max(e.TireFlTemp, e.TireFrTemp), math.Max(e.TireRlTemp, e.TireRrTemp))
}
