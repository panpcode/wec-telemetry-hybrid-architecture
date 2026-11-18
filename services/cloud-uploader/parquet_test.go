package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
)

func TestEncodeParquetRoundTrip(t *testing.T) {
	ts := time.Date(2026, 6, 14, 15, 0, 0, 123_000_000, time.UTC)
	in := []json.RawMessage{
		json.RawMessage(`{"event_id":"a","timestamp":"` + ts.Format(time.RFC3339Nano) + `","car_id":"CAR_001","session_id":"S1","lap":3,"speed_kmh":287.4,"tire_fl_temp":98.5,"in_pit":true,"schema_version":"1.0"}`),
		json.RawMessage(`not json`),
	}

	data, rows, skipped, err := encodeParquet(in)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 || skipped != 1 {
		t.Fatalf("rows=%d skipped=%d, want 1/1", rows, skipped)
	}

	out, err := parquet.Read[Row](bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	got := out[0]
	if got.EventID != "a" || got.Lap != 3 || got.SpeedKmh != 287.4 || got.TireFlTemp != 98.5 || !got.InPit {
		t.Errorf("unexpected row: %+v", got)
	}
	if !got.Timestamp.Equal(ts) {
		t.Errorf("timestamp %v, want %v (millisecond precision)", got.Timestamp, ts)
	}
}
