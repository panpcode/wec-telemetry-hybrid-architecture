package main

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"
)

// Row is one telemetry sample as stored in Parquet.
//
// session_id and car_id are deliberately absent: they are encoded in the
// object path (session_id=…/car_id=…), which is the Hive partitioning
// convention that DuckDB, Athena and Spark all read. Storing them in the
// file as well would duplicate the column.
type Row struct {
	EventID          string    `json:"event_id"           parquet:"event_id"`
	Timestamp        time.Time `json:"timestamp"          parquet:"timestamp,timestamp(millisecond)"`
	Lap              int32     `json:"lap"                parquet:"lap"`
	Sector           int32     `json:"sector"             parquet:"sector"`
	SpeedKmh         float64   `json:"speed_kmh"          parquet:"speed_kmh"`
	RPM              float64   `json:"rpm"                parquet:"rpm"`
	ThrottlePercent  float64   `json:"throttle_percent"   parquet:"throttle_percent"`
	BrakePressureBar float64   `json:"brake_pressure_bar" parquet:"brake_pressure_bar"`
	TireFlTemp       float64   `json:"tire_fl_temp"       parquet:"tire_fl_temp"`
	TireFrTemp       float64   `json:"tire_fr_temp"       parquet:"tire_fr_temp"`
	TireRlTemp       float64   `json:"tire_rl_temp"       parquet:"tire_rl_temp"`
	TireRrTemp       float64   `json:"tire_rr_temp"       parquet:"tire_rr_temp"`
	FuelLevelLiters  float64   `json:"fuel_level_liters"  parquet:"fuel_level_liters"`
	FuelFlowLph      float64   `json:"fuel_flow_lph"      parquet:"fuel_flow_lph"`
	BrakeTemp        float64   `json:"brake_temp"         parquet:"brake_temp"`
	GpsLat           float64   `json:"gps_lat"            parquet:"gps_lat"`
	GpsLong          float64   `json:"gps_long"           parquet:"gps_long"`
	OnTrack          bool      `json:"on_track"           parquet:"on_track"`
	InPit            bool      `json:"in_pit"             parquet:"in_pit"`
	SchemaVersion    string    `json:"schema_version"     parquet:"schema_version"`
}

// encodeParquet decodes JSON payloads into rows and writes one zstd-compressed
// Parquet file. Payloads that fail to decode are skipped and counted.
func encodeParquet(payloads []json.RawMessage) (data []byte, rows int, skipped int, err error) {
	batch := make([]Row, 0, len(payloads))
	for _, p := range payloads {
		var r Row
		if json.Unmarshal(p, &r) != nil {
			skipped++
			continue
		}
		batch = append(batch, r)
	}

	var buf bytes.Buffer
	w := parquet.NewGenericWriter[Row](&buf, parquet.Compression(&zstd.Codec{}))
	if _, err := w.Write(batch); err != nil {
		return nil, 0, skipped, err
	}
	if err := w.Close(); err != nil {
		return nil, 0, skipped, err
	}
	return buf.Bytes(), len(batch), skipped, nil
}
