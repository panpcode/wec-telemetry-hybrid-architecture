package main

import (
	"math"
	"math/rand"
	"testing"
)

// lapStats summarises one simulated lap.
type lapStats struct {
	timeS, avgSpeed, maxSpeed, fuel, maxTire, maxBrake float64
	pitted                                             bool
}

// simulate runs the physics offline (no wall-clock sleeping) and returns per-lap stats.
func simulate(t *testing.T, track TrackConfig, laps, pitEvery int, seed int64) []lapStats {
	t.Helper()
	const dt = 0.05 // 20 Hz, same as production
	rng := rand.New(rand.NewSource(seed))
	cs := NewCarSimulator(DefaultCarConfig("CAR_T", track, pitEvery, rng), "S", seed)

	var out []lapStats
	cur := lapStats{}
	n, sumSpeed, fuelStart := 0, 0.0, cs.fuelLevel
	for steps := 0; len(out) < laps && steps < 10_000_000; steps++ {
		e := cs.Step(dt)
		checkLimits(t, e)
		if e.Lap > len(out) { // lap just closed
			cur.avgSpeed = sumSpeed / float64(n)
			cur.fuel = math.Max(0, fuelStart-e.FuelLevelLiters)
			out = append(out, cur)
			cur, n, sumSpeed, fuelStart = lapStats{}, 0, 0, e.FuelLevelLiters
		}
		n++
		cur.timeS += dt
		sumSpeed += e.SpeedKmh
		cur.maxSpeed = math.Max(cur.maxSpeed, e.SpeedKmh)
		cur.maxTire = math.Max(cur.maxTire, math.Max(math.Max(e.TireFlTemp, e.TireFrTemp), math.Max(e.TireRlTemp, e.TireRrTemp)))
		cur.maxBrake = math.Max(cur.maxBrake, e.BrakeTemp)
		cur.pitted = cur.pitted || e.InPit
	}
	return out
}

// checkLimits mirrors the ingestion service's validation so the simulator
// never emits an event that would be rejected.
func checkLimits(t *testing.T, e TelemetryEvent) {
	t.Helper()
	bad := e.SpeedKmh < 0 || e.SpeedKmh > 400 ||
		e.RPM < 0 || e.RPM > 10000 ||
		e.ThrottlePercent < 0 || e.ThrottlePercent > 100 ||
		e.BrakePressureBar < 0 || e.BrakePressureBar > 25 ||
		e.BrakeTemp < 0 || e.BrakeTemp > 600 ||
		e.FuelLevelLiters < 0 || e.FuelLevelLiters > 100
	for _, tt := range []float64{e.TireFlTemp, e.TireFrTemp, e.TireRlTemp, e.TireRrTemp} {
		bad = bad || tt < 0 || tt > 200
	}
	if bad {
		t.Fatalf("event outside ingestion limits: %+v", e)
	}
}

func TestPhysicsRealisticPerTrack(t *testing.T) {
	cases := []struct {
		track          string
		minLap, maxLap float64 // plausible Hypercar lap times (s), with margin
	}{
		{"le_mans", 195, 260},
		{"spa", 115, 160},
		{"imola", 85, 120},
	}
	for _, c := range cases {
		t.Run(c.track, func(t *testing.T) {
			laps := simulate(t, TrackByName(c.track), 10, 8, 42)
			for i, l := range laps {
				t.Logf("lap %2d: %6.1fs avg %5.1f max %5.1f km/h fuel %4.2fL tire %5.1f°C brake %5.1f°C pit=%v",
					i, l.timeS, l.avgSpeed, l.maxSpeed, l.fuel, l.maxTire, l.maxBrake, l.pitted)
			}
			// Lap 1 is a clean flying lap (lap 0 starts from standstill).
			fly := laps[1]
			if fly.timeS < c.minLap || fly.timeS > c.maxLap {
				t.Errorf("flying lap %.1fs outside [%v, %v]", fly.timeS, c.minLap, c.maxLap)
			}
			if fly.maxTire > 115 {
				t.Errorf("fresh-ish tyres peaking at %.1f°C — should sit in the 85–110 window", fly.maxTire)
			}
			if fly.maxBrake > 450 {
				t.Errorf("brakes peaking at %.1f°C on a normal lap — should stay under the 450 alert", fly.maxBrake)
			}
		})
	}
}

func TestFirstPitAfterConfiguredLaps(t *testing.T) {
	laps := simulate(t, TrackByName("imola"), 6, 4, 1)
	for i, l := range laps {
		// Pit entry happens on crossing the line at the end of lap 3, so lap 4 contains the stop.
		if want := i == 4; l.pitted != want {
			t.Errorf("lap %d pitted=%v, want %v", i, l.pitted, want)
		}
	}
}

func TestCarsDiffer(t *testing.T) {
	a := simulate(t, TrackByName("imola"), 2, 0, 1)
	b := simulate(t, TrackByName("imola"), 2, 0, 2)
	if a[1].timeS == b[1].timeS && a[1].maxBrake == b[1].maxBrake {
		t.Error("two cars with different seeds produced identical laps")
	}
}
