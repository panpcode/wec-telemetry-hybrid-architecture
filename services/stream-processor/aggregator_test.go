package main

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 6, 14, 15, 0, 0, 0, time.UTC)

func ev(lap int, sec float64, fuel float64) TelemetryEvent {
	return TelemetryEvent{
		Timestamp: t0.Add(time.Duration(sec * float64(time.Second))),
		CarID:     "CAR_01", SessionID: "S1", Lap: lap,
		SpeedKmh: 200, RPM: 8000,
		TireFlTemp: 90, TireFrTemp: 92, TireRlTemp: 88, TireRrTemp: 89,
		FuelLevelLiters: fuel, BrakeTemp: 300,
	}
}

func defaultTh() Thresholds { return Thresholds{TireMaxC: 120, BrakeMaxC: 450, FuelLowL: 8} }

func TestLapClosesOnLapChange(t *testing.T) {
	g := NewAggregator(defaultTh())
	for i, sec := range []float64{0, 100, 200} {
		if lap, _ := g.Process(ev(0, sec, 75-float64(i))); lap != nil {
			t.Fatalf("lap closed early at sample %d", i)
		}
	}
	lap, _ := g.Process(ev(1, 210, 72))
	if lap == nil {
		t.Fatal("expected lap 0 to close")
	}
	if lap.Lap != 0 || lap.Samples != 3 || lap.LapTimeS != 200 {
		t.Errorf("got lap=%d samples=%d time=%.0f", lap.Lap, lap.Samples, lap.LapTimeS)
	}
	if lap.FuelUsedL != 2 {
		t.Errorf("fuel used = %.2f, want 2", lap.FuelUsedL)
	}
}

func TestPartialFirstLapIsSkipped(t *testing.T) {
	g := NewAggregator(defaultTh())
	g.Process(ev(5, 0, 50)) // joined mid-lap 5
	if lap, _ := g.Process(ev(6, 10, 49)); lap != nil {
		t.Fatal("partial lap 5 should not be emitted")
	}
	if lap, _ := g.Process(ev(7, 20, 48)); lap == nil || lap.Lap != 6 {
		t.Fatal("lap 6 should be emitted in full")
	}
}

func TestOldLapEventsIgnored(t *testing.T) {
	g := NewAggregator(defaultTh())
	g.Process(ev(0, 0, 75))
	g.Process(ev(1, 10, 74))
	if lap, alerts := g.Process(ev(0, 5, 75)); lap != nil || alerts != nil {
		t.Fatal("event from closed lap should be ignored")
	}
}

func TestAlertsRaisedOncePerLap(t *testing.T) {
	g := NewAggregator(defaultTh())
	hot := ev(0, 0, 75)
	hot.TireFrTemp = 130
	_, a := g.Process(hot)
	if len(a) != 1 || a[0].Kind != "tire_overheat" {
		t.Fatalf("want one tire_overheat, got %+v", a)
	}
	hot.Timestamp = hot.Timestamp.Add(time.Second)
	if _, a := g.Process(hot); len(a) != 0 {
		t.Fatal("same alert raised twice in one lap")
	}
	next := ev(1, 100, 75)
	next.TireFrTemp = 130
	if _, a := g.Process(next); len(a) != 1 {
		t.Fatal("alert should re-arm on the next lap")
	}
}

func TestFuelLowIgnoredInPit(t *testing.T) {
	g := NewAggregator(defaultTh())
	e := ev(0, 0, 3)
	e.InPit = true
	if _, a := g.Process(e); len(a) != 0 {
		t.Fatal("fuel_low must not fire while refuelling in the pit")
	}
	e.InPit = false
	e.Timestamp = e.Timestamp.Add(time.Second)
	if _, a := g.Process(e); len(a) != 1 || a[0].Kind != "fuel_low" {
		t.Fatalf("want fuel_low, got %+v", a)
	}
}
