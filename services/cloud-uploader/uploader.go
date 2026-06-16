package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/nats-io/nats.go"
)

// Uploader pulls telemetry events from NATS JetStream and uploads them to
// an S3-compatible object store (MinIO locally, real S3 in production).
type Uploader struct {
	cfg Config
	nc  *nats.Conn
	js  nats.JetStreamContext
	sub *nats.Subscription
	s3  *minio.Client
}

// NewUploader connects to NATS and MinIO, then binds a durable consumer
// to the TELEMETRY stream so uploads resume where they left off after restarts.
func NewUploader(cfg Config) (*Uploader, error) {
	// Connect to NATS with automatic reconnect so transient network blips
	// don't kill the process.
	nc, err := nats.Connect(cfg.NatsURL,
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

	// Ensure the stream exists — both the ingestion service and the uploader
	// call this so neither depends on the other starting first.
	if err := ensureStream(js); err != nil {
		nc.Close()
		return nil, fmt.Errorf("ensure stream: %w", err)
	}

	// Durable pull consumer — the consumer offset is persisted by NATS so that
	// if this service restarts, it picks up from the last ack'd message.
	sub, err := js.PullSubscribe("TELEMETRY.*", "cloud-uploader",
		nats.BindStream("TELEMETRY"),
	)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("PullSubscribe: %w", err)
	}

	// MinIO client — endpoint must be host:port without a scheme prefix.
	endpoint := stripScheme(cfg.S3Endpoint)
	s3, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3UseSSL,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("MinIO client: %w", err)
	}

	log.Printf("Connected to NATS and MinIO (%s)", endpoint)
	return &Uploader{cfg: cfg, nc: nc, js: js, sub: sub, s3: s3}, nil
}

// Run loops indefinitely: fetch → upload → ack.
// On upload failure, messages are NOT ack'd so NATS redelivers them.
func (u *Uploader) Run() {
	batchTimeout := time.Duration(u.cfg.BatchTimeoutSec) * time.Second
	log.Printf("Running — waiting for batches of up to %d msgs (timeout %v)",
		u.cfg.BatchSize, batchTimeout)

	for {
		msgs, err := u.sub.Fetch(u.cfg.BatchSize, nats.MaxWait(batchTimeout))
		if err == nats.ErrTimeout {
			// Normal: no messages arrived within the window.
			continue
		}
		if err != nil {
			log.Printf("Fetch error: %v — sleeping 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if len(msgs) == 0 {
			continue
		}

		if err := u.uploadBatch(msgs); err != nil {
			log.Printf("Upload failed (will retry): %v", err)
			// Do not ack — NATS will redeliver the batch on the next Fetch call.
			continue
		}

		// Ack everything only after the upload succeeds.
		for _, m := range msgs {
			_ = m.Ack()
		}
		log.Printf("Uploaded and ack'd %d messages", len(msgs))
	}
}

// uploadBatch groups messages by (session_id, car_id), compresses each group
// as gzipped NDJSON, and uploads to MinIO using Hive-style path partitioning
// so Athena can use partition projection directly.
//
// Path format: session_id={X}/car_id={Y}/{timestamp}.jsonl.gz
// To switch to real AWS S3, swap the minio client for the AWS SDK — the
// upload logic (PutObject call) stays the same.
func (u *Uploader) uploadBatch(msgs []*nats.Msg) error {
	// Group raw payloads by session + car.
	type group struct {
		sessionID string
		carID     string
		payloads  []json.RawMessage
	}
	groups := make(map[string]*group)

	for _, m := range msgs {
		var meta struct {
			SessionID string `json:"session_id"`
			CarID     string `json:"car_id"`
		}
		if err := json.Unmarshal(m.Data, &meta); err != nil {
			continue // skip malformed messages
		}
		key := meta.SessionID + "|" + meta.CarID
		if _, ok := groups[key]; !ok {
			groups[key] = &group{sessionID: meta.SessionID, carID: meta.CarID}
		}
		groups[key].payloads = append(groups[key].payloads, json.RawMessage(m.Data))
	}

	ts := time.Now().UTC().Format("20060102T150405Z")

	for _, g := range groups {
		buf, err := compressNDJSON(g.payloads)
		if err != nil {
			return fmt.Errorf("compress %s/%s: %w", g.sessionID, g.carID, err)
		}

		objectKey := fmt.Sprintf("session_id=%s/car_id=%s/%s.jsonl.gz",
			g.sessionID, g.carID, ts)

		_, err = u.s3.PutObject(
			context.Background(),
			u.cfg.S3Bucket,
			objectKey,
			bytes.NewReader(buf),
			int64(len(buf)),
			minio.PutObjectOptions{
				ContentType:     "application/gzip",
				ContentEncoding: "gzip",
			},
		)
		if err != nil {
			return fmt.Errorf("PutObject %s: %w", objectKey, err)
		}
		log.Printf("  s3://%s/%s  (%d events, %d bytes compressed)",
			u.cfg.S3Bucket, objectKey, len(g.payloads), len(buf))
	}
	return nil
}

// compressNDJSON writes each payload on its own line and gzip-compresses the result.
// NDJSON (newline-delimited JSON) is readable by Athena, pandas, and spark with no ETL.
func compressNDJSON(payloads []json.RawMessage) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	for _, p := range payloads {
		if _, err := gz.Write(p); err != nil {
			return nil, err
		}
		gz.Write([]byte("\n")) //nolint:errcheck
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ensureStream creates the TELEMETRY JetStream stream if it doesn't exist.
// Idempotent — safe to call from multiple services on startup.
func ensureStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo("TELEMETRY")
	if err == nil {
		return nil // already exists
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      "TELEMETRY",
		Subjects:  []string{"TELEMETRY.*"},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,       // 7-day rolling window
		MaxBytes:  10 * 1024 * 1024 * 1024,  // 10 GB hard cap
	})
	return err
}

// stripScheme removes http:// or https:// from a URL, returning host:port only
// (the format expected by the MinIO Go client).
func stripScheme(url string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, prefix) {
			return url[len(prefix):]
		}
	}
	return url
}

func (u *Uploader) Close() {
	if u.nc != nil {
		u.nc.Close()
	}
}
