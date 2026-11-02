# WEC Real-Time Telemetry Platform

A hybrid on-track + cloud telemetry system designed for endurance racing (WEC). Built to handle high-frequency ECU data streams during a race weekend — with sub-100ms latency on track and full historical replay in the cloud.

---

## The Problem

A WEC team needs two things simultaneously:

- **On track**: race engineers making fuel and tire decisions in real time, with no dependency on internet connectivity
- **At HQ**: the analytics team querying lap-by-lap data after the race for ML models and strategy preparation

A single architecture cannot serve both well. Cloud-only introduces latency and offline risk. Pure local storage has no path to historical analysis. This project solves that with a hybrid design.

---

## Architecture

```
ON-TRACK (trackside server, Docker)
─────────────────────────────────────────────────

  ECU / Sensor Gateway
       │
       │  HTTP POST  every 50ms
       ▼
  Ingestion Service (Go)
  — validates schema
  — publishes to NATS JetStream
       │
       ▼
  NATS JetStream  (disk-backed, offline-capable)
       │
       ├──▶  Stream Processor (Go)
       │     — lap metrics, anomaly detection
       │     — writes to PostgreSQL
       │
       └──▶  Parquet Sinker (Go)
             — batches raw events to local disk

  Cloud Uploader (Go)
  — async, non-blocking
  — uploads when connectivity available
       │
       │  Parquet over HTTPS
       ▼

CLOUD (AWS)
─────────────────────────────────────────────────

  S3 Data Lake
  — partitioned by session / car / lap
       │
       ├──▶  Athena  (SQL queries on raw data)
       └──▶  Glue    (schema discovery, ETL)
```

**Key design decisions:**

- NATS JetStream runs locally — the pipeline keeps working with no internet
- Raw events are never mutated; all analytics derive from the same immutable stream
- The cloud uploader is decoupled — it reads from JetStream independently and retries on failure
- Parquet on S3 is directly queryable by Athena with no ETL step required

---

## Data Model

Every ECU reading is a self-contained event published at 20 Hz per car:

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-06-16T14:30:45.123Z",
  "car_id": "CAR_001",
  "session_id": "SESSION_20260616_LM",
  "lap": 12,
  "sector": 2,
  "speed_kmh": 285.5,
  "rpm": 8200,
  "throttle_percent": 95.2,
  "brake_pressure_bar": 0.0,
  "tire_fl_temp": 98.5,
  "tire_fr_temp": 99.2,
  "tire_rl_temp": 97.8,
  "tire_rr_temp": 98.1,
  "fuel_level_liters": 42.5,
  "fuel_flow_lph": 120.0,
  "brake_temp": 420.0,
  "gps_lat": 47.9497,
  "gps_long": 0.2076,
  "on_track": true,
  "schema_version": "1.0"
}
```

---

## Tech Stack

| Layer | Technology | Rationale |
|---|---|---|
| Event broker | NATS JetStream | Single binary, disk-backed, durable consumers, offline-first |
| Backend services | Go | Predictable latency, native NATS client, small Docker footprint |
| Computed metrics | PostgreSQL | Indexed lap summaries for dashboard queries |
| Raw event archive | AWS S3 (Parquet) | Columnar, compressed, directly queryable |
| Cloud analytics | AWS Athena + Glue | SQL on S3 without a separate warehouse |
| Containerisation | Docker Compose | Reproducible local stack on any trackside laptop |

---

## What Is Implemented

| Service | Status | Description |
|---|---|---|
| ECU Simulator | Done | Physics-based car model. Simulates up to N cars at 20 Hz — speed, RPM, tire temps, fuel burn, GPS, lap tracking. |
| Ingestion Service | Done | HTTP endpoint that validates incoming events and publishes to NATS JetStream. |
| NATS JetStream | Done | Configured stream `TELEMETRY.*` with disk-backed storage and durable consumers. |
| Cloud Uploader | Done | Durable JetStream consumer; batches events as gzipped NDJSON and uploads to MinIO/S3 with Hive-style partitions (`session_id=/car_id=`). Parquet is a planned upgrade. |
| Pit-Wall Dashboard | Done | Go WebSocket server + embedded HTML UI on :3000, live per-car speed, tires, fuel, pit status. |
| Docker Compose | Done | Full stack: NATS, MinIO + bucket init, ingestion, simulator, uploader, dashboard. |
| Stream Processor | Done | Durable JetStream consumer; per-lap metrics (lap time, fuel, speed, tire/brake temps, pit flag) and threshold alerts written idempotently to PostgreSQL. |
| Query API | Planned | REST endpoint for historical lap and stint queries. |

---

## Running It Locally

**Whole stack with Docker Compose:**
```bash
docker-compose up --build
# Dashboard:      http://localhost:3000
# MinIO console:  http://localhost:9001 (minioadmin / minioadmin)
# Ingestion:      http://localhost:8080/health
```

Or run individual services without Docker:

**Start NATS:**
```bash
docker run -p 4222:4222 -p 8222:8222 nats -js
```

**Start the ingestion service:**
```bash
cd services/ingestion-service
go run .
# Listening on :8080, connected to NATS
```

**Start the ECU simulator (2 cars, Le Mans layout):**
```bash
cd services/ecu-simulator
go run .
# Events flowing to http://localhost:8080/telemetry/ingest at 20 Hz
```

**Watch events live:**
```bash
nats sub 'TELEMETRY.>'
```

**Check ingestion health:**
```bash
curl http://localhost:8080/health
```

---

## Project Structure

```
wec-telemetry-hybrid-architecture/
├── architecture/
│   ├── Architecture.md          — full design document
│   └── ECU_DATA_GENERATION.md   — ECU simulation approach
├── services/
│   ├── ecu-simulator/           — car physics simulator (Go)
│   ├── ingestion-service/       — HTTP ingest + NATS publish (Go)
│   ├── cloud-uploader/          — JetStream → MinIO/S3 archiver (Go)
│   ├── dashboard/               — live pit-wall UI over WebSocket (Go)
│   └── stream-processor/        — lap metrics + alerts → PostgreSQL (Go)
├── docker-compose.yml
├── Makefile
└── README.md
```

---

## Target Scale

Designed for a single WEC team with 1–3 cars. Requirements that shaped the design:

- 3 cars × 20 Hz = ~60 events/sec sustained; bursts to ~200 events/sec
- Sub-100ms end-to-end latency on track (ECU → dashboard)
- 48-hour offline buffer on the trackside server
- Full raw event replay from any point in a session
- Post-race Athena queries without loading data into a separate warehouse

---

## Roadmap

Next up:

1. **Query API**: REST access to lap and stint history from PostgreSQL.
2. **Parquet output** from the cloud uploader, plus Athena/Glue and Terraform for production S3.

Full design rationale: [architecture/Architecture.md](architecture/Architecture.md)
