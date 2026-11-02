package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// Store writes lap metrics and alerts to PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// OpenStore connects to Postgres (retrying while it starts up) and applies the schema.
func OpenStore(ctx context.Context, dsn string) (*Store, error) {
	var pool *pgxpool.Pool
	var err error
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err = pgxpool.New(ctx, dsn)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				break
			}
			pool.Close()
		}
		log.Printf("Postgres not ready (attempt %d): %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// InsertLap is idempotent: a lap redelivered after a restart is ignored.
func (s *Store) InsertLap(ctx context.Context, m LapMetrics) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO lap_metrics (
			session_id, car_id, lap, started_at, ended_at, lap_time_s, samples,
			avg_speed_kmh, max_speed_kmh, max_rpm,
			fuel_start_l, fuel_end_l, fuel_used_l,
			avg_tire_fl_c, avg_tire_fr_c, avg_tire_rl_c, avg_tire_rr_c,
			max_tire_c, max_brake_c, pitted
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (session_id, car_id, lap) DO NOTHING`,
		m.SessionID, m.CarID, m.Lap, m.StartedAt, m.EndedAt, m.LapTimeS, m.Samples,
		m.AvgSpeedKmh, m.MaxSpeedKmh, m.MaxRPM,
		m.FuelStartL, m.FuelEndL, m.FuelUsedL,
		m.AvgTireFL, m.AvgTireFR, m.AvgTireRL, m.AvgTireRR,
		m.MaxTireC, m.MaxBrakeC, m.Pitted,
	)
	return err
}

// InsertAlert is idempotent per (session, car, lap, kind).
func (s *Store) InsertAlert(ctx context.Context, a Alert) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO alerts (session_id, car_id, lap, kind, value, threshold, detail, raised_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (session_id, car_id, lap, kind) DO NOTHING`,
		a.SessionID, a.CarID, a.Lap, a.Kind, a.Value, a.Threshold, a.Detail, a.RaisedAt,
	)
	return err
}
