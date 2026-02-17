# WEC Real-Time Telemetry Platform - Architecture Design

## 📋 Executive Summary

This document describes a **hybrid event-driven architecture** for real-time and historical telemetry processing in endurance racing (WEC context). The system simultaneously handles high-frequency ECU data streams (10-50ms intervals) requiring sub-100ms latency for live analytics, while maintaining complete event replay and historical analysis capabilities.

**Architecture Style**: Kappa Architecture with Event Sourcing  
**Core Technology**: NATS JetStream (event streaming broker)  
**Deployment**: Docker Compose (local POC → cloud-ready)

---

## 🎯 1. Problem Statement

### Racing Telemetry Requirements

**Real-Time Challenges**:
- ECU generates ~100-1000 telemetry points per second per vehicle
- Racing engineers need **<100ms latency** decision-making (fuel strategy, tire decisions)
- Simultaneous multi-car tracking for team strategy
- Live event detection (tire degradation, fuel anomalies, mechanical issues)

**Historical Requirements**:
- Post-race performance analysis (lap-by-lap comparison)
- Predictive modeling (tire wear curves, fuel consumption patterns)
- Strategy simulation for future races
- Complete audit trail of all decisions and changes

**Traditional Approach Limitations**:
- Lambda architecture = complex (separate batch + stream)
- Kafka = overkill for POC, harder to self-host
- MQTT = not built for event replay
- REST polling = not event-driven

### Our Solution

**Single unified event stream** (Kappa architecture) with:
- Real-time processing through stream consumers
- Complete event persistence for historical analysis
- Native replay capabilities without code changes
- Lightweight footprint suitable for local development

---

## 📋 2. Functional Requirements

### Must Have (MVP)

1. **ECU Telemetry Ingestion**
   - Accept telemetry from simulated ECUs
   - Support multiple vehicles (1-50)
   - Schema validation on ingest

2. **Real-Time Stream Processing**
   - Compute lap metrics in <100ms
   - Detect anomalies (sensor failures, tire degradation)
   - Publish computed events downstream

3. **Historical Event Storage**
   - Store every raw telemetry event
   - Indexed by session, lap, timestamp
   - Support event replay from any point

4. **Query API**
   - Live telemetry feed for dashboard
   - Lap analytics (avg speed, tire temps, fuel burn)
   - Historical data retrieval

### Nice to Have (Post-MVP)

- Predictive tire wear modeling
- Fuel strategy optimization engine
- Multi-session comparison
- Grafana dasboard integration

---

## ⚙️ 3. Non-Functional Requirements

| Requirement | Target | Rationale |
|-----------|--------|-----------|
| **Latency (P99)** | <100ms end-to-end | Racing decisions are time-critical |
| **Throughput** | 10K events/sec per car | 4 cars × 2500 Hz = 10K events/sec |
| **Availability** | 99.5% (POC), 99.9% (prod) | Live race depends on it |
| **Data Retention** | 90-day rolling window | Regulatory + analytics window |
| **Replay Capability** | Full event log replay | Essential for post-race analysis |
| **Scalability** | Horizontal to 50+ cars | Multi-team scenarios |

---

## 🏗️ 4. Architectural Style: Kappa Architecture

### Why Kappa over Lambda?

| Aspect | Lambda | Kappa (Our Choice) | Racing Context |
|--------|--------|----------|---|
| **Complexity** | High | Low | POC needs speed |
| **Operational Burden** | Batch + Stream | Stream only | Single source of truth |
| **Latency** | Hours (batch) + ms (stream) | Consistent ms | Need uniform latency |
| **Replay** | Requires reprocessing | Native | Critical for analysis |
| **Cost** | 2x infra | 1x infra | Budget conscious |

### Why NATS JetStream over Kafka?

| Factor | Kafka | NATS JetStream | Winner |
|--------|-------|---|---|
| **Self-hosting on Mac** | Docker, heavy | Docker, lightweight | JetStream ✓ |
| **Operational Complexity** | High (ZK, brokers) | Simple | JetStream ✓ |
| **Event Replay** | Yes, but complex | Native, built-in | JetStream ✓ |
| **Learning Curve** | Steep | Gentle | JetStream ✓ |
| **Production Ready** | Yes | Yes | Tie |
| **Clustering (future)** | Excellent | Good | Kafka |

**Decision**: Kappa + NATS JetStream is optimal for racing analytics POC.

---

## 🧩 5. System Components

### High-Level Data Flow

```
┌─────────────────┐
│  ECU Simulator  │ (generates telemetry @ 100Hz per car)
└────────┬────────┘
         │
         ▼
┌─────────────────────────┐
│ Ingestion Service       │ (validates schema, publishes to stream)
└────────┬────────────────┘
         │
         ▼
    NATS JetStream
    (event_stream)
    /              \
   ▼                ▼
Stream Processor   Raw Event Sink
(real-time)       (persistence)
   │                  │
   ▼                  ▼
Postgres          MinIO (S3)
(computed)        (raw archive)
   │
   ▼
┌──────────────────────┐
│  Query API Service   │
└─────────┬────────────┘
          │
          ▼
    ┌─────────────┐
    │  Dashboard  │
    └─────────────┘
```

---

## 🔧 6. Component Specifications

### 6.1 ECU Simulator

**Purpose**: Generate realistic telemetry from simulated race car(s)

**Responsibilities**:
- Emit telemetry every 10-50ms (configurable)
- Simulate tire degradation curves
- Simulate fuel consumption
- Add realistic sensor noise
- Respect lap structure (start/finish detection)

**Key Outputs**:
```json
{
  "event_id": "uuid",
  "timestamp": "2026-02-21T14:30:45.123Z",
  "car_id": "CAR_001",
  "lap": 42,
  "sector": 1,
  "speed_kmh": 285.5,
  "rpm": 8200,
  "throttle_percent": 95.2,
  "brake_pressure_bar": 0.0,
  "tire_temp": {
    "fl": 98.5,
    "fr": 99.2,
    "rl": 97.8,
    "rr": 98.1
  },
  "fuel_level_liters": 42.5,
  "gps_lat": 48.2645,
  "gps_long": 11.6265,
  "session_id": "SESSION_001",
  "schema_version": "1.0"
}
```

### 6.2 Ingestion Service

**Purpose**: Entry point for all telemetry events

**Responsibilities**:
- Accept HTTP POST from simulators
- Validate against schema
- Enrich with metadata (received_at)
- Publish to JetStream stream
- Log and meter all ingestions

**Endpoints**:
```
POST /telemetry/ingest
POST /health
GET /metrics
```

**Failure Handling**:
- Schema validation failure → 400, log to dead-letter queue
- Publish failure → 503, retry with backoff
- Metrics collection → never fails (async)

### 6.3 NATS JetStream (Message Broker)

**Configuration**:

**Stream**: `telemetry-raw`
- Subjects: `telemetry.{car_id}.*`
- Retention: 30 days (rolling)
- Replication: 1 (local POC)
- Storage: File-based

**Consumers**:

1. **Real-Time Processor Consumer**
   - Group: `stream-processor`
   - Delivery policy: At-least-once
   - Durable (survives restarts)

2. **Persistence Consumer**
   - Group: `event-sink`
   - Delivery policy: At-least-once
   - Separate for independent scaling

### 6.4 Stream Processor

**Purpose**: Real-time computation on events

**Responsibilities**:
- Consume from JetStream
- Window operations (lap-based aggregations)
- Anomaly detection
- Compute derived metrics
- Publish results to `telemetry-computed` stream

**Computed Events Example**:
```json
{
  "event_id": "uuid",
  "timestamp": "2026-02-21T14:31:15.000Z",
  "car_id": "CAR_001",
  "lap": 42,
  "metrics": {
    "avg_speed_kmh": 287.3,
    "max_speed_kmh": 310.2,
    "min_speed_kmh": 180.5,
    "avg_rpm": 7850,
    "tire_temps_avg": 98.4,
    "time_delta_from_last_lap": 105.2,
    "fuel_consumed_liters": 1.5,
    "estimated_fuel_at_finish": 5.2
  },
  "anomalies": [
    {
      "type": "tire_degradation",
      "severity": "warning",
      "detail": "Front-left tire temp increased 12C vs lap 41"
    }
  ]
}
```

**Processing Strategy**:
- Lap-based windows (event source: cross-start/finish line)
- Sliding 5-second windows for real-time aggregations
- Stateful processing using RocksDB
- Idempotent computation (same event → same result)

### 6.5 Raw Event Sink

**Purpose**: Archive all raw telemetry for historical analysis

**Responsibilities**:
- Consume from `telemetry-raw` stream
- Batch events for efficiency
- Write to MinIO with partitioned structure
- Track write state

**Storage Structure**:
```
s3://telemetry-archive/
├── session_id={SESSION_001}/
│   ├── lap={001}/
│   │   ├── 2026-02-21-14-30-00.parquet
│   │   ├── 2026-02-21-14-31-00.parquet
│   └── lap={002}/
└── session_id={SESSION_002}/
```

**Format**: Parquet (columnar, compressible, queryable)

### 6.6 Query API Service

**Purpose**: Expose telemetry for dashboards and analysis tools

**Endpoints**:

```
# Live telemetry
GET /api/v1/live/telemetry?car_id=CAR_001&limit=100

# Lap metrics
GET /api/v1/lap/{lap_id}
GET /api/v1/car/{car_id}/laps?limit=10

# Analytics
GET /api/v1/analytics/fuel?car_id=CAR_001&session_id=SESSION_001
GET /api/v1/analytics/tire-wear?car_id=CAR_001

# Replay
GET /api/v1/replay?session_id=SESSION_001&start_lap=1&end_lap=10

# Health
GET /health
GET /metrics
```

**Data Sources**:
- Cache layer: Redis for live data (5-min TTL)
- Query DB: Postgres for computed metrics
- Archive: MinIO for detailed historical analysis

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

  ecu-simulator:
    build: ./services/ecu-simulator
    depends_on: [ingestion-service]

  ingestion-service:
    build: ./services/ingestion-service
    depends_on: [nats]
    ports: [8080:8080]

  stream-processor:
    build: ./services/stream-processor
    depends_on: [nats, postgres]

  raw-event-sink:
    build: ./services/raw-event-sink
    depends_on: [nats, minio]

  query-api:
    build: ./services/query-api
    depends_on: [postgres, redis, minio]
    ports: [8081:8081]
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
