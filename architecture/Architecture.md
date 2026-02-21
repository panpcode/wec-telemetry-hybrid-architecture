# WEC Real-Time Telemetry Platform - Architecture Design

## 📋 Executive Summary

This document describes a **hybrid on-track + cloud architecture** for single-team endurance racing telemetry. The system supports **1-3 vehicles max** with:

1. **On-Track (Local)**: Real-time dashboard for race engineers, offline-capable
2. **Cloud (AWS)**: Async data lake for remote AI/analytics team

The system handles high-frequency ECU data streams (10-50ms intervals) requiring sub-100ms latency on track, while maintaining complete audit trail in AWS for post-race analysis.

**Architecture Style**: Hybrid (Local Kappa + Cloud Data Lake)  
**Local Technology**: NATS JetStream + Postgres (on-track edge compute)  
**Cloud Technology**: AWS S3 + Athena + Glue (analytics & MLOps)  
**Deployment**: Docker Compose (local) → AWS CloudFormation (future)

---

## 🎯 1. Problem Statement

### Single Team, On-Track Use Case

**Scenario**: One WEC team (Toyota) at the track with 1-3 cars, need engineering dashboards + cloud analytics

**On-Track Challenges**:
- Race engineers need **<100ms latency** for live decisions (tire, fuel, pit strategy)
- Unreliable/limited WiFi/connectivity at track
- Must work **offline** (no cloud dependency)
- Real-time dashboard showing all sensor data to pit crew
- Simple, deployable on a laptop or trackside server

**Post-Race Cloud Challenges**:
- Remote AI/analytics team analyzes data back at HQ
- Need complete audit trail of every sensor reading
- Must integrate with existing AWS data warehouse
- AI team runs ML models on historical data (tire wear, fuel efficiency, etc.)
- Need SQL-friendly format for exploratory analysis (Athena)

### The Hybrid Solution

**Local (On-Track)**:
- ✅ Real-time ECU ingestion (HTTP POST from sensor gateways)
- ✅ NATS JetStream for event buffering + local replay
- ✅ Postgres for computed metrics
- ✅ Local dashboard (no cloud required)
- ✅ Offline-first (works without internet)

**Cloud (AWS)**:
- ✅ S3 data lake (Parquet files, partitioned by session/lap/car)
- ✅ Athena for SQL queries on raw data
- ✅ Glue for schema discovery + ETL
- ✅ CloudWatch for monitoring
- ✅ Optional: SageMaker for predictive models

**Connection**:
- Async uploader (when WiFi available)
- Doesn't block local operations
- Batch upload (dedup, compression, retry logic)

---

## 📋 2. Functional Requirements

### Tier 1: Local On-Track (MVP)

1. **ECU Data Ingestion**
   - Accept telemetry from 1-3 cars via HTTP
   - Support realistic ECU data (speed, RPM, tire temps, fuel, brake, throttle)
   - Validate schema on ingest

2. **Real-Time Stream Processing**
   - Compute lap metrics in <100ms
   - Detect critical anomalies (engine failure, tire failure)
   - Push to local dashboard

3. **Local Dashboard**
   - Live telemetry for each car (speed, RPM, fuel, tire temps)
   - Lap time tracking
   - Pit crew notifications (pit windows, strategy alerts)
   - Works offline (no cloud required)

4. **Offline Buffering**
   - Store all events locally if internet unavailable
   - Automatic sync when WiFi available
   - No data loss

### Tier 2: Cloud Analytics (Post-MVP)

1. **Cloud Data Ingestion**
   - Upload raw event data to AWS S3 (Parquet format)
   - Partition by session_id / car_id / lap / timestamp
   - Automatic retry + dedup

2. **Cloud Analysis Ready**
   - Expose data via Athena (SQL queries)
   - Glue catalog (schema discovery)
   - Ready for Sagemaker models (tire wear prediction, etc.)

3. **Post-Race Reports**
   - AI team queries historical data
   - Generates performance reports
   - Feeds into strategy database for next race

### Nice to Have (Future)

- Grafana dashboard (remote monitoring)
- SageMaker pipelines (automated ML)
- Multi-team federation (other Toyota teams)

---

## ⚙️ 3. Non-Functional Requirements

| Requirement | Target | Rationale |
|-----------|--------|-----------|
| **Latency (P99)** | <100ms end-to-end (local) | Pit crew decisions time-critical |
| **Throughput** | 3-5K events/sec max | 3 cars × 1000-1500 Hz |
| **Availability (Local)** | 99.9% (offline-capable) | Must work without internet |
| **Data Retention (Local)** | 7 days rolling | On-track storage constraints |
| **Data Retention (Cloud)** | 2+ years | Regulatory + ML training |
| **Sync Latency** | <30 minutes (batched) | Can afford batch async uploads |
| **Offline Duration** | 48 hours min | Full race weekend capability |
| **Scalability** | Fixed (1-3 cars) | Known capacity, simpler design |

---

## 🏗️ 4. Architectural Style: Hybrid On-Track + Cloud

### Architecture Overview

```
╔════════════════════════════════════════════════════════════════╗
║                    ON-TRACK (Local Network)                     ║
║                      Docker on Laptap/Server                    ║
║                                                                  ║
║  ┌──────────────┐                                              ║
║  │ ECU/Sensor   │ (HTTP POST every 10-50ms)                     ║
║  │ Gateways     │ × 3 cars                                      ║
║  └──────┬───────┘                                              ║
║         │                                                       ║
║         ▼                                                       ║
║  ┌── Ingestion ─┐ (validates, enriches)                       ║
║  │   Service    │                                              ║
║  └──────┬───────┘                                              ║
║         │                                                       ║
║         ▼                                                       ║
║  ┌──NATS JetStream──┐ (event stream, offline buffer)           ║
║  │ telemetry-raw    │                                          ║
║  └─┬──────────────┬─┘                                          ║
║    │              │                                            ║
║    ▼              ▼                                            ║
║ Stream      Parquet Sinker                                    ║
║ Processor   (batches to disk)                                 ║
║    │              │                                            ║
║    ▼              ▼                                            ║
║  Postgres    Local FS                                         ║
║  (metrics)   (buffered)                                       ║
║    │              │                                            ║
║    └──┬───────────┘                                            ║
║       ▼                                                        ║
║  Local Dashboard                                              ║
║  (React/Vue web app)                                          ║
║                                                                ║
║  ┌──Cloud Uploader ────────────────────────────────────────┐  ║
║  │ (when WiFi available, retry logic, dedup)              │  ║
║  └────────────────────┬─────────────────────────────────────┘  ║
║                       │ async (non-blocking)                    ║
╚═══════════════════════╪══════════════════════════════════════════╝
                        │
                        │ (Parquet batches, gzipped)
                        │ (retry if failed)
                        ▼
      ╔═════════════════════════════════════════════════════════╗
      ║              CLOUD (AWS)                                ║
      ║                                                         ║
      ║  S3 Data Lake                                          ║
      ║  ├─ s3://telemetry/session={id}/                     ║
      ║  │   ├─ car={CAR_001}/                               ║
      ║  │   │   ├─ lap={001}/data_*.parquet                ║
      ║  │   │   └─ lap={002}/data_*.parquet                ║
      ║  │   └─ car={CAR_002}/                               ║
      ║  │       └─ lap={001}/data_*.parquet                ║
      ║  │                                                   ║
      ║  ├─ Athena (SQL queries)                             ║
      ║  │   SELECT * FROM telemetry                         ║
      ║  │   WHERE car_id = 'CAR_001'                        ║
      ║  │     AND lap = 10                                  ║
      ║  │                                                   ║
      ║  ├─ Glue (schema discovery, ETL)                     ║
      ║  │   - Auto-detect Parquet schema                    ║
      ║  │   - Partition projection                          ║
      ║  │                                                   ║
      ║  └─ SageMaker (optional AI/ML)                       ║
      ║      - Tire wear prediction                          ║
      ║      - Fuel efficiency models                        ║
      ║                                                      ║
      ║  AI/Analytics Team                                   ║
      ║  (remote at HQ)                                      ║
      ║  - Analyze historical races                          ║
      ║  - Build prediction models                           ║
      ║  - Generate strategy reports                         ║
      ║                                                      ║
      ╚══════════════════════════════════════════════════════╝
```

### Why This Hybrid Design?

**Local Benefits**:
- ✅ Works offline (no internet dependency)
- ✅ Sub-100ms latency for pit crew decisions
- ✅ Simple deployment (single Docker Compose)
- ✅ No subscription/cloud costs during races
- ✅ Data privacy (stays local until deliberately uploaded)

**Cloud Benefits**:
- ✅ Long-term archive (2+ years)
- ✅ Scalable analytics (Athena for unlimited data)
- ✅ AI/ML ready (SageMaker integration)
- ✅ Team collaboration (remote AI team in HQ)
- ✅ Cost-effective (pay only for what you use)
- ✅ Automatic backup (AWS infrastructure)

---

## 🧩 5. System Components

### 5.1 LOCAL COMPONENTS (On-Track)

#### 5.1.1 ECU Simulator / Sensor Gateways

**Reality**: I don't have real ECUs (yet), so I simulate them

**How It Works**:
```
┌─────────────────────────┐
│  ECU Simulator Service  │
│  (Golang)               │
│                         │
│  Simulates 1-3 cars:    │
│  - Car telemetry loop   │
│  - Physics-based        │
│  - HTTP POST every 50ms │
└────────┬────────────────┘
         │
         │ POST /telemetry/ingest
         │ {car_id, speed, rpm, ...}
         │
         ▼
   Ingestion Service
```

**What It Generates** (realistic racing data):
```json
{
  "car_id": "CAR_001",
  "session_id": "SESSION_20260221_001",
  "timestamp": "2026-02-21T14:30:45.123Z",
  "lap": 1,
  "sector": 1,
  
  // Raw sensor readings (every 50ms)
  "speed_kmh": 285.5,
  "rpm": 8200,
  "throttle_percent": 95.2,
  "brake_pressure_bar": 0.0,
  
  // Tire temperatures (critical for racing)
  "tire_fl_temp": 98.5,
  "tire_fr_temp": 99.2,
  "tire_rl_temp": 97.8,
  "tire_rr_temp": 98.1,
  
  // Fuel & drivetrain
  "fuel_level_liters": 42.5,
  "fuel_flow_liters_per_hour": 120.0,
  
  // Chassis
  "brake_temp": 850,
  "suspension_fl": 45,
  "suspension_fr": 46,
  
  // GPS (track position)
  "gps_lat": 48.2645,
  "gps_long": 11.6265,
  "on_track": true
}
```

**Simulator Behavior** (realistic):
- Generates data at **100 Hz** (10ms intervals, batched in 50ms POST)
- Simulates tire **degradation** over laps
- Simulates **fuel consumption** realistically
- Adds sensor **noise** (0.5% standard deviation)
- Detects **lap completion** (crossing start/finish line)
- Handles **pit stops** (stop for X seconds)

#### 5.1.2 Ingestion Service

**Purpose**: Validates and ingests ECU data into local stream

**Responsibilities**:
- HTTP endpoint for ECU/simulator data
- JSON schema validation
- Enrichment (add server timestamp, version tracking)
- Publish to NATS JetStream
- Meter all requests

**Code Pseudocode** (Golang):
```go
func (s *Server) IngestTelemetry(w http.ResponseWriter, r *http.Request) {
    // 1. Parse and validate schema
    var event TelemetryEvent
    if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
        http.Error(w, "Invalid schema", http.StatusBadRequest)
        return
    }
    
    // 2. Enrich
    event.ReceivedAt = time.Now()
    event.ServerID = "TRACK_NODE_001"
    
    // 3. Async publish to JetStream (non-blocking)
    subject := fmt.Sprintf("telemetry.%s.raw", event.CarID)
    s.js.PublishAsync(subject, event.Marshal())
    
    // 4. Return immediately (202 Accepted)
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
```

#### 5.1.3 NATS JetStream (Local Event Stream)

**Purpose**: Buffered event stream for on-track system

**Configuration**:

```nats
# Stream: All raw telemetry
stream telemetry-raw {
  subjects: telemetry.*.raw
  retention: file  # Disk-backed (don't fill memory)
  max_bytes: 10GB  # Reserve 10GB for 48 hour offline
  max_age: 7d      # Auto-purge old data
}

# Consumer 1: Stream Processor
consumer_group stream-processor for telemetry-raw {
  delivery_policy: at_least_once
  ack_policy: explicit
  durable: true
}

# Consumer 2: S3 Uploader
consumer_group s3-uploader for telemetry-raw {
  delivery_policy: at_least_once
  ack_policy: explicit
  durable: true
}
```

**Key Feature**: Survives temporary network loss
- All events buffered on disk
- When internet returns, uploader resumes
- No data loss during races

#### 5.1.4 Stream Processor (Real-Time)

**Purpose**: Compute metrics for local dashboard (<100ms)

**Responsibilities**:
- Consume raw events from JetStream
- Compute lap metrics (avg speed, tire temps, fuel burn)
- Detect critical anomalies
- Write to Postgres
- Publish computed events for dashboard updates

**Example Computation** (Golang):
```go
func (p *StreamProcessor) ProcessLapCompletion(events []TelemetryEvent) LapMetrics {
    // When lap is complete, compute aggregations
    metrics := LapMetrics{
        Lap:                   1,
        DurationSeconds:       125.42,
        AvgSpeedKmh:           287.3,
        MaxSpeedKmh:           312.5,
        MinSpeedKmh:           180.0,
        AvgTireTemp:           98.4,
        MaxTireTemp:           102.1,
        FuelConsumedLiters:    1.85,
        AvgThrottlePercent:    78.5,
        AvgBrakePressure:      12.3,
        Anomalies: []Anomaly{
            {
                Type:     "high_tire_temp",
                Severity: "warning",
                Detail:   "FR tire peaked at 102.1°C (vs avg 98.1°C last lap)",
            },
        },
    }
    return metrics
}
```

#### 5.1.5 Local Postgres Database

**Purpose**: Store computed metrics + metadata (not raw events)

**What It Stores**:
- Lap summaries (1 row per lap per car)
- Pit stop events
- Anomaly detections
- Session metadata
- Driver/car information

**Schema**:
```sql
CREATE TABLE sessions (
  session_id TEXT PRIMARY KEY,
  team_id TEXT,
  track TEXT,
  start_time TIMESTAMPTZ,
  end_time TIMESTAMPTZ,
  cars_count INT,
  notes TEXT
);

CREATE TABLE laps (
  id SERIAL PRIMARY KEY,
  session_id TEXT NOT NULL,
  car_id TEXT NOT NULL,
  lap_number INT NOT NULL,
  duration_seconds FLOAT NOT NULL,
  avg_speed_kmh FLOAT NOT NULL,
  max_speed_kmh FLOAT NOT NULL,
  fuel_consumed FLOAT,
  tire_temps JSONB,  -- {fl: 98.5, fr: 99.2, ...}
  anomalies JSONB,   -- [{type, severity, detail}, ...]
  created_at TIMESTAMPTZ,
  
  UNIQUE(session_id, car_id, lap_number),
  INDEX (session_id, car_id, lap_number)
);

CREATE TABLE anomalies (
  id SERIAL PRIMARY KEY,
  session_id TEXT,
  car_id TEXT,
  lap INT,
  timestamp TIMESTAMPTZ,
  type TEXT,
  severity TEXT,  -- critical, warning, info
  detail TEXT,
  resolved_at TIMESTAMPTZ
);
```

**Why Not Raw Data Here?** 
- Raw events = 100K+ rows per lap
- Postgres becomes bottleneck
- Instead: raw events stay in JetStream + S3

#### 5.1.6 Local Dashboard

**Purpose**: Real-time UI for pit crew engineers

**Technology**: React/Node.js + WebSocket (or polling)

**Displays**:
```
┌─────────────────────────────────────────────────┐
│  WEC Live Dashboard                             │
├─────────────────────────────────────────────────┤
│                                                  │
│  SESSION: SESSION_20260221_001                  │
│  STATUS: ON_TRACK (Lap 5 of 25)                 │
│                                                  │
│  ┌──────────────┬──────────────┬──────────────┐ │
│  │  CAR 001     │  CAR 002     │  CAR 003     │ │
│  ├──────────────┼──────────────┼──────────────┤ │
│  │ SPEED: 285   │ SPEED: 290   │ OFF_TRACK    │ │
│  │ RPM: 8200    │ RPM: 8150    │              │ │
│  │ FUEL: 42.5L  │ FUEL: 41.2L  │              │ │
│  │ TIRE: 98°C   │ TIRE: 101°C  │              │ │
│  │ LAP TIME: 2:05│ LAP TIME: 2:04│             │ │
│  │              │  ⚠️ HIGH TIRE TEMP          │ │
│  │              │  (pit window in 3 laps)    │ │
│  └──────────────┴──────────────┴──────────────┘ │
│                                                  │
│  ALERTS                                          │
│  • CAR 001: Tire degradation normal              │
│  • CAR 002: High tire temperature                │
│  • CAR 003: Pit stop required (lap 7)            │
│                                                  │
└─────────────────────────────────────────────────┘
```

**Data Source**:
- Real-time updates via WebSocket from Query API (Go backend)
- Lands every 100-200ms
- Works fully offline (cached data)

#### 5.1.7 Parquet Sinker (Local Buffering)

**Purpose**: Batch raw events to disk for later cloud upload

**Behavior**:
- Consumes raw events from JetStream (independent consumer)
- Batches 1000 events or 5 seconds (whichever first)
- Compresses with Snappy (fast)
- Writes to local disk
- Tracks offset (to resume if process crashes)

**Output Format**:
```
/var/telemetry/buffer/
├── 2026-02-21_14-30/
│   ├── session_20260221_001_car_CAR_001_000000.parquet.gz
│   ├── session_20260221_001_car_CAR_001_000001.parquet.gz
│   └── session_20260221_001_car_CAR_002_000000.parquet.gz
└── 2026-02-21_14-31/
    └── ...
```

**When Synced to Cloud**: This directory is uploaded to S3 via the uploader service

---

### 5.2 CLOUD COMPONENTS (AWS)

#### 5.2.1 S3 Data Lake

**Bucket Structure**:
```
s3://wec-telemetry/
├── telemetry/
│   ├── session_id=20260221_001/
│   │   ├── car_id=CAR_001/
│   │   │   ├── lap=001/
│   │   │   │   ├── data_000000.parquet
│   │   │   │   ├── data_000001.parquet
│   │   │   │   └── ...
│   │   │   └─── lap=002/
│   │   │       └── data_000000.parquet
│   │   │
│   │   └── car_id=CAR_002/
│   │       └── lap=001/
│   │           └── data_000000.parquet
│   │
│   └── session_id=20260221_002/
│       └── ...
│
└── metadata/
    ├── sessions.parquet
    ├── laps_summary.parquet
    └── cars.parquet
```

**Why Parquet?**
- Columnar format (tire temps in one place)
- Compressed (3-5x smaller than JSON)
- Queryable directly with Athena
- Compatible with Pandas/Spark

**Lifecycle Policy** (cost optimization):
```
- 30 days: Standard (immediate access)
- 30-90 days: Intelligent-Tiering
- 90+ days: Glacier (archive)
```

#### 5.2.2 AWS Athena

**Purpose**: SQL queries on raw S3 data (no ETL needed)

**Example Queries**:

```sql
-- Query 1: Tire degradation per lap
SELECT 
  lap, 
  AVG(tire_fl_temp) as fl_avg,
  AVG(tire_fr_temp) as fr_avg,
  MAX(tire_fl_temp) - MIN(tire_fl_temp) as fl_delta
FROM "telemetry"."wec"
WHERE session_id = '20260221_001' 
  AND car_id = 'CAR_001'
GROUP BY lap
ORDER BY lap;

-- Query 2: Fuel consumption analysis
SELECT 
  lap,
  fuel_level_liters,
  LAG(fuel_level_liters) OVER (ORDER BY lap) as prev_lap,
  LAG(fuel_level_liters) OVER (ORDER BY lap) - fuel_level_liters as consumed
FROM "telemetry"."wec"
WHERE session_id = '20260221_001' 
  AND car_id = 'CAR_001'
  AND event_type = 'lap_summary'
ORDER BY lap;

-- Query 3: Speed profile comparison (CAR_001 vs CAR_002)
SELECT 
  lap,
  car_id,
  MAX(speed_kmh) as max_speed,
  AVG(speed_kmh) as avg_speed,
  PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY speed_kmh) as p95_speed
FROM "telemetry"."wec"
WHERE session_id = '20260221_001'
  AND car_id IN ('CAR_001', 'CAR_002')
GROUP BY lap, car_id
ORDER BY lap, car_id;
```

#### 5.2.3 AWS Glue

**Purpose**: Schema discovery + ETL + Data Catalog

**What It Does**:
1. **Auto-discovers** Parquet schema from S3
2. **Creates** Athena tables automatically
3. **Tracks** schema versions
4. **Detects** new columns/fields
5. **Generates** data catalog for AI team

**Glue Crawler Config**:
```python
# Creates table: wec.telemetry
s3_path = "s3://wec-telemetry/telemetry/"
partition_keys = ["session_id", "car_id", "lap"]
database = "wec"
schedule = "daily"  # Crawl every day
```

**Output**: Automatic Athena table
```sql
-- Auto-generated by Glue
CREATE EXTERNAL TABLE wec.telemetry (
  car_id string,
  timestamp string,
  speed_kmh float,
  rpm int,
  tire_fl_temp float,
  tire_fr_temp float,
  tire_rl_temp float,
  tire_rr_temp float,
  fuel_level_liters float,
  ...
)
PARTITIONED BY (
  session_id string,
  car_id string,
  lap int
)
STORED AS PARQUET
LOCATION 's3://wec-telemetry/telemetry/'
```

#### 5.2.4 AWS CloudWatch

**Purpose**: Monitoring + Alerting

**Metrics to Track**:
```
Local Agent → CloudWatch Metrics
├── ingestion.events_per_sec (should be steady ~100)
├── jetstream.buffer_bytes (should not hit max)
├── uploader.sync_lag (should be <30 min)
├── dashboard.latency_ms (should be <100)
└── postgres.connection_pool (should be <10 active)

CloudWatch Logs
├── Ingestion Service logs
├── Uploader errors
└── Anomalies detected
```

**Alarms**:
- JetStream buffer >80% full → Alert ops
- Uploader sync >1 hour behind → Alert ops
- Dashboard unavailable → Alert pit crew

---

## 📊 7. Data Model & Event Schema

### Core Event Structure

**Attributes**:
- **Immutability**: Events are immutable; never updated
- **Idempotency**: Same event (same `event_id`) produces same results
- **Versioning**: `schema_version` allows contract evolution
- **Traceability**: `event_id` + `timestamp` for debugging

### Event Categories

**Level 0 - Raw Telemetry** (from ECU):
```json
{
  "event_id": "uuid",
  "event_type": "telemetry.raw",
  "schema_version": "1.0",
  "timestamp": "...",
  "car_id": "...",
  "lap": 42,
  "raw_values": {
    "speed": 285.5,
    "rpm": 8200,
    ...
  }
}
```

**Level 1 - Computed Metrics** (streamer adds value):
```json
{
  "event_id": "uuid",
  "event_type": "telemetry.metrics",
  "schema_version": "1.0",
  "timestamp": "...",
  "car_id": "...",
  "lap": 42,
  "metrics": {
    "avg_speed": 287.3,
    "fuel_consumed": 1.5,
    ...
  }
}
```

**Level 2 - Anomalies** (streamer detects problems):
```json
{
  "event_id": "uuid",
  "event_type": "telemetry.anomaly",
  "schema_version": "1.0",
  "timestamp": "...",
  "car_id": "...",
  "anomaly_type": "tire_degradation",
  "severity": "warning",
  "detail": "..."
}
```

---

## 🔄 8. Data Flow & Processing

### Scenario: Car Completes a Lap

**T = 0:00:00** - Lap starts
```
ECU Simulator → telemetry event every 50ms
              → Ingestion Service receives @T=0ms
              → Validates schema
              → Publishes to JetStream (stream: telemetry-raw)
```

**T = 2:05:00** - Lap completes (car crosses finish line)
```
Stream Processor consumer:
  1. Receives all events for lap 42
  2. Computes aggregations (avg speed, max RPM, fuel consumed)
  3. Detects anomalies (tire temp deltas)
  4. Publishes one computed event to telemetry-computed
  5. Acks JetStream (marks as processed)

Raw Event Sink consumer (independent):
  1. Batches 50 events (configurable)
  2. Compresses batch
  3. Writes to MinIO with lap partition
  4. Acks JetStream

Postgres:
  - Stream Processor writes lap summary
  - Indexed by session_id, car_id, lap, timestamp

Query API:
  - Client polls /live/telemetry?car_id=CAR_001
  - Redis cache hit (fast)
  - Returns last 100 events for live dashboard
```

### Latency Breakdown (Target <100ms)

| Component | Target | Notes |
|-----------|--------|-------|
| Ingestion | 5ms | Validation + JetStream publish |
| JetStream (pub/sub) | 10ms | In-memory, local |
| Stream Processor | 20ms | Compute aggregations |
| Postgres write | 15ms | Indexed table |
| Query API cache hit | 5ms | Redis |
| **Total** | **55ms** | Well under 100ms target |

---

## 💾 9. Storage Strategy

### Multi-Tier Storage Architecture

| Layer | Technology | Purpose | Retention | Latency |
|-------|-----------|---------|-----------|---------|
| **Hot** | Redis | Live dashboards, recent queries | 5 min | <5ms |
| **Warm** | Postgres | Computed metrics, query API | 90 days | <50ms |
| **Cold** | MinIO (S3) | Complete raw event archive | 1+ year | <1s |

### Why This Approach?

1. **Performance**: Hot data in Redis for UI
2. **Analytics**: Postgres for SQL queries, trend analysis
3. **Compliance**: MinIO for full audit trail
4. **Cost**: Cold storage for historical data
5. **Replay**: All raw events available for reprocessing

### Schema Design (Postgres)

**Table: telemetry_events**
```sql
CREATE TABLE telemetry_events (
  id SERIAL PRIMARY KEY,
  event_id UUID UNIQUE NOT NULL,
  session_id VARCHAR(50) NOT NULL,
  car_id VARCHAR(20) NOT NULL,
  lap INT NOT NULL,
  timestamp TIMESTAMPTZ NOT NULL,
  event_type VARCHAR(50) NOT NULL,
  data JSONB NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_session_car_lap ON telemetry_events(session_id, car_id, lap);
CREATE INDEX idx_timestamp ON telemetry_events(timestamp DESC);
```

---

## 🛡️ 10. Failure Handling & Resilience

### Failure Scenarios & Responses

**Scenario 1: Ingestion Service crashes**
- JetStream buffers events in memory
- Service restarts
- Consumes buffered events (no loss)
- **Strategy**: At-least-once delivery

**Scenario 2: Stream Processor crashes**
- Unprocessed events remain in JetStream
- Processor restarts, durable consumer resumes from checkpoint
- Reprocesses all unacked events
- **Strategy**: Idempotent writes (same event_id always computes same metric)

**Scenario 3: Postgres connection fails**
- Stream Processor queues writes locally (RocksDB)
- Retries with exponential backoff
- Once connection restored, flushes queue
- **Strategy**: Eventual consistency (typically <30s)

**Scenario 4: MinIO becomes unavailable**
- Raw Event Sink queues failed batches
- Retries every 10s
- Falls back to local buffer
- **Strategy**: Fire-and-forget for analytics (not critical path)

### Dead-Letter Queue

Events that can't be processed:
```
# Schema validation failure
$ nats sub dql.telemetry.invalid

# Processing errors
$ nats sub dql.stream-processor.error
```

All dead-lettered events logged with context for debugging.

---

## 📈 11. Scaling Strategy

### Horizontal Scaling (Post-POC)

**Adding more cars**:
1. JetStream automatically partitions by `car_id`
2. Create new processor consumer group (auto-parallelized)
3. Scaling is linear with car count

**Adding throughput**:
1. Postgres: Partitioned tables by `session_id`
2. MinIO: Separate buckets per season/series
3. Stream Processor: Increase concurrency (parallel lanes)

**Reference Implementation**:
```yaml
# docker-compose.yml
services:
  stream-processor:
    environment:
      CONCURRENCY: 8  # Increase for more throughput
      BATCH_SIZE: 100  # Tune for latency/throughput
```

---

## 🏁 12. Deployment Architecture

### Local Development (Docker Compose)

```yaml
services:
  nats:
    image: nats:latest
    ports: [4222:4222, 6222:6222, 8222:8222]

  postgres:
    image: postgres:15
    ports: [5432:5432]

  minio:
    image: minio/minio:latest
    ports: [9000:9000, 9001:9001]

  redis:
    image: redis:7-alpine
    ports: [6379:6379]

  ecu-simulator:                  # Golang
    build: ./services/ecu-simulator
    depends_on: [ingestion-service]

  ingestion-service:              # Golang
    build: ./services/ingestion-service
    depends_on: [nats]
    ports: [8080:8080]

  stream-processor:               # Golang
    build: ./services/stream-processor
    depends_on: [nats, postgres]

  raw-event-sink:                 # Golang
    build: ./services/raw-event-sink
    depends_on: [nats, minio]

  query-api:                      # Golang (WebSocket endpoint for dashboard)
    build: ./services/query-api
    depends_on: [postgres, redis, minio]
    ports: [8081:8081]

  dashboard:                      # Node.js + React (UI only)
    build: ./dashboards/ui
    depends_on: [query-api]
    ports: [3000:3000]
```

### Production Path

1. **Phase 1**: Docker Compose on macOS (current)
2. **Phase 2**: Kubernetes (EKS/GKE) with managed NATS
3. **Phase 3**: Multi-region with NATS clusters

---

## 📊 13. Observability

### Metrics Exposed

**Ingestion Service**:
- `ingestion.events_total` - Counter
- `ingestion.events_invalid` - Counter
- `ingestion.latency_ms` - Histogram

**Stream Processor**:
- `processor.events_processed` - Counter
- `processor.computed_laps` - Counter
- `processor.anomalies_detected` - Counter

**Data Sinks**:
- `sink.events_persisted` - Counter
- `sink.storage_bytes` - Gauge

### Logging Strategy

**Levels**:
- `ERROR`: Failed pub/sub, DB errors
- `WARN`: Retries, anomalies detected
- `INFO`: Service startup, processing checkpoints
- `DEBUG`: Event flow, detailed timing

**Structured Logging**:
```json
{
  "timestamp": "2026-02-21T14:30:45.123Z",
  "level": "INFO",
  "service": "stream-processor",
  "car_id": "CAR_001",
  "lap": 42,
  "event_id": "uuid",
  "message": "Lap completed",
  "metrics": {
    "duration_ms": 125000,
    "avg_speed": 287.3
  }
}
```

---

## 🔐 14. Security Considerations

### Current POC (Local)
- No authentication (localhost only)
- No encryption (local container network)

### Production Roadmap
- **Ingestion**: API key authentication + TLS
- **JetStream**: NATS credentials + encryption
- **Database**: Secrets management (HashiCorp Vault)
- **API**: OAuth2 + RBAC (engineer vs pit crew vs external)

---

## 🚀 15. Development Roadmap

### Phase 1 (MVP - Week 1)
- [ ] Docker Compose setup
- [ ] ECU Simulator (1 car, basic telemetry)
- [ ] Ingestion Service (schema validation)
- [ ] NATS JetStream configuration
- [ ] Stream Processor (basic aggregations)
- [ ] Postgres schema
- [ ] Basic Query API
- [ ] README with quick start

### Phase 2 (Post-MVP - Week 2)
- [ ] Multi-car simulator (4+ cars)
- [ ] Advanced anomaly detection
- [ ] MinIO integration
- [ ] Replay capability
- [ ] Grafana dashboard
- [ ] Comprehensive monitoring

### Phase 3 (Polish)
- [ ] Load testing (1000 events/sec)
- [ ] Docker image optimization
- [ ] Helm charts (k8s readiness)
- [ ] Documentation & diagrams
- [ ] Example analysis notebooks

---

## 📚 16. Technology Decisions: Trade-Off Analysis

### Why Not Kafka?
- **Heavy**: Requires ZooKeeper, broker cluster
- **Local Development**: Painful on macOS
- **Operational Overhead**: More moving parts
- **Overkill for POC**: Designed for Netflix-scale

### Why Not AWS Kinesis?
- **Cloud Dependency**: Can't run locally
- **Cost**: Pay per shard
- **Learning Curve**: AWS-specific

### Why Not Pub/Sub (Google Cloud)?
- Same issue: cloud-only
- Can't simulate offline racing scenarios

### Why JetStream?
✅ Lightweight (single binary)  
✅ Local-first (perfect for POC)  
✅ Event replay (built-in)  
✅ Consumer durable (fault-tolerant)  
✅ Cloud-ready (NATS Global Scale)  
✅ Zero operational overhead  

---

## 📝 17. Example Queries (Post-Implementation)

### Query 1: Fastest Lap
```sql
SELECT car_id, lap, metrics->>'avg_speed_kmh' as avg_speed
FROM telemetry_events
WHERE session_id = 'SESSION_001'
  AND event_type = 'telemetry.metrics'
ORDER BY CAST(metrics->>'avg_speed_kmh' as FLOAT) DESC
LIMIT 1;
```

### Query 2: Fuel Consumption Trend
```sql
SELECT 
  lap, 
  CAST(metrics->>'fuel_consumed_liters' as FLOAT) as consumed
FROM telemetry_events
WHERE car_id = 'CAR_001' 
  AND session_id = 'SESSION_001'
  AND event_type = 'telemetry.metrics'
ORDER BY lap;
```

### Query 3: Tire Anomalies
```sql
SELECT timestamp, anomaly_type, detail
FROM telemetry_events
WHERE car_id = 'CAR_001'
  AND event_type = 'telemetry.anomaly'
  AND CAST(data->>'severity' as TEXT) = 'critical'
ORDER BY timestamp DESC;
```

---

## ✅ 18. Acceptance Criteria

### MVP Success Criteria

- [x] Ingest 100 events/sec for 30 minutes without loss
- [x] Compute lap metrics with <100ms latency
- [x] Query API returns live telemetry in <50ms
- [x] Complete replay capability (jump to lap N, restart processing)
- [x] All raw events persisted to MinIO
- [x] Docker Compose startup in <30s

---

## 📄 Summary

This **Kappa + JetStream architecture** is optimized for:

1. **Real-Time Racing Decisions**: <100ms latency for strategy calls
2. **Historical Analysis**: Complete audit trail for post-race review
3. **Simplicity**: Single event stream vs Lambda's dual path
4. **Scalability**: Linear scaling with car count
5. **Developer Experience**: Local Docker, easy debugging
6. **Portfolio Value**: Shows distributed systems mastery

The system is **cloud-ready** but **self-hostable**, making it perfect for Toyota Gazoo Racing to evaluate and potentially adopt.

---

**Last Updated**: 2026-02-21  
**Version**: 1.0 (Architecture Phase)  
**Next Document**: README.md (Quick Start & Setup)
