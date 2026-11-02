package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// Processor drains the TELEMETRY stream through a durable pull consumer,
// aggregates laps, and writes results to Postgres.
//
// Messages are ack'd only after everything they produced is stored, so a
// crash or DB outage leads to redelivery rather than lost laps. Inserts are
// idempotent, so redelivery never creates duplicates.
type Processor struct {
	nc    *nats.Conn
	sub   *nats.Subscription
	agg   *Aggregator
	store *Store
	batch int
}

func NewProcessor(natsURL string, store *Store, agg *Aggregator, batch int) (*Processor, error) {
	nc, err := nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("NATS connect: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("JetStream context: %w", err)
	}
	if err := ensureStream(js); err != nil {
		nc.Close()
		return nil, fmt.Errorf("ensure stream: %w", err)
	}
	sub, err := js.PullSubscribe("TELEMETRY.*", "stream-processor", nats.BindStream("TELEMETRY"))
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("PullSubscribe: %w", err)
	}
	return &Processor{nc: nc, sub: sub, agg: agg, store: store, batch: batch}, nil
}

func (p *Processor) Close() { p.nc.Drain() } //nolint:errcheck

// Run blocks until ctx is cancelled.
func (p *Processor) Run(ctx context.Context) {
	for ctx.Err() == nil {
		msgs, err := p.sub.Fetch(p.batch, nats.MaxWait(2*time.Second))
		if errors.Is(err, nats.ErrTimeout) || len(msgs) == 0 {
			continue
		}
		if err != nil {
			log.Printf("Fetch error: %v — sleeping 2s", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, m := range msgs {
			var e TelemetryEvent
			if err := json.Unmarshal(m.Data, &e); err != nil || e.CarID == "" {
				_ = m.Term() // malformed: never redeliver
				continue
			}
			lap, raised := p.agg.Process(e)
			if err := p.persist(ctx, lap, raised); err != nil {
				// Leave this message un-ack'd; JetStream redelivers it after AckWait.
				log.Printf("DB write failed (will be redelivered): %v", err)
				_ = m.Nak()
				continue
			}
			_ = m.Ack()
			if lap != nil {
				log.Printf("🏁 %s lap %d: %.1fs, %.2f L, avg %.0f km/h, pitted=%v",
					lap.CarID, lap.Lap, lap.LapTimeS, lap.FuelUsedL, lap.AvgSpeedKmh, lap.Pitted)
			}
			for _, a := range raised {
				log.Printf("⚠️  %s lap %d %s: %s", a.CarID, a.Lap, a.Kind, a.Detail)
			}
		}
	}
}

func (p *Processor) persist(ctx context.Context, lap *LapMetrics, alerts []Alert) error {
	if lap != nil {
		if err := p.store.InsertLap(ctx, *lap); err != nil {
			return fmt.Errorf("insert lap: %w", err)
		}
	}
	for _, a := range alerts {
		if err := p.store.InsertAlert(ctx, a); err != nil {
			return fmt.Errorf("insert alert: %w", err)
		}
	}
	return nil
}

// ensureStream mirrors the ingestion service so start-up order doesn't matter.
func ensureStream(js nats.JetStreamContext) error {
	if _, err := js.StreamInfo("TELEMETRY"); err == nil {
		return nil
	}
	_, err := js.AddStream(&nats.StreamConfig{
		Name:      "TELEMETRY",
		Subjects:  []string{"TELEMETRY.*"},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		MaxBytes:  10 * 1024 * 1024 * 1024,
	})
	if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return err
	}
	return nil
}
