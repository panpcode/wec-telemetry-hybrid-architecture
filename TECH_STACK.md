# Technology Stack & Implementation Guide

## 🎯 Technology Choices

### Backend Services: Golang

**All backend services are implemented in Golang**:
- ✅ ECU Simulator
- ✅ Ingestion Service
- ✅ Stream Processor
- ✅ Raw Event Sink
- ✅ Cloud Uploader
- ✅ Query API (WebSocket backend)

**Why Golang?**

| Criterion | Value | Why |
|-----------|-------|-----|
| **Latency** | <100ms guaranteed | No GC pauses (concurrent mark-sweep) |
| **Throughput** | 1000s goroutines | Lightweight threads (1KB stack vs threads) |
| **Binary Size** | <50MB total | Statically linked, single executable |
| **Deployment** | 1 Docker image | No runtime dependencies |
| **NATS Support** | Native, best-in-class | Go client maintained by NATS team |
| **Concurrency** | Async-ready | Built-in channels, select statements |
| **Development Speed** | Fast iteration | Simple syntax, fast compilation |
| **Production Ready** | Battle-tested | Used by Docker, Kubernetes, Uber, etc. |

---

### Frontend: React + Node.js

**Dashboard for race engineers**:
- ✅ React (UI components, state management)
- ✅ Node.js (dev server, build tooling)
- ✅ WebSocket client (real-time updates)

**Why Node.js for UI?**

| Criterion | Value |
|-----------|-------|
| **Rich Ecosystem** | Charting (Chart.js, Recharts), real-time (Socket.io) |
| **Developer Experience** | Hot reload, rich debugging, quick iteration |
| **Accessibility** | Non-backend engineers can contribute |
| **Performance** | Sufficient for UI (WebSocket bottleneck is backend) |
| **Separation of Concerns** | UI development independent of backend |

---

### Message Broker: NATS JetStream

**Local event streaming with cloud readiness**:

**Why JetStream (not Kafka)?**

| Factor | NATS JetStream | Kafka | Winner |
|--------|---|---|---|
| **Offline Capability** | Disk-backed (48h buffer) | Yes, but complex | JetStream ✓ |
| **Operational Overhead** | Single binary | ZooKeeper + brokers | JetStream ✓ |
| **Startup Time** | <1 second | 30+ seconds | JetStream ✓ |
| **Latency** | <10ms pub/sub | <50ms | JetStream ✓ |
| **Event Replay** | Native | Supported | Tie |
| **Learning Curve** | Gentle | Steep | JetStream ✓ |
| **Go Integration** | Native support | Sarama (less optimized) | JetStream ✓ |
| **Clustering** | Good | Excellent | Kafka |
| **Production Ready** | Yes (CNCF) | Yes | Tie |

**Result**: JetStream is optimal for on-track racing system

---

### Database: PostgreSQL

**For computed metrics + metadata** (NOT raw events):

```sql
-- 1-3 cars, ~50 laps/session
-- ~5000 rows per session = acceptable
-- Raw events (100K+) go to S3, not Postgres
```

**Why Postgres?**

- ✅ ACID guarantees (important for metrics)
- ✅ Efficient indexing (fast lap queries)
- ✅ Moderate data volume (compute metrics offline)
- ✅ Time-series friendly (TIMESTAMPTZ, window functions)
- ✅ JSON support (JSONB for anomalies)

**What NOT to use**:
- ❌ MongoDB - No ACID, harder to index
- ❌ TimescaleDB - Overkill for 5K rows/session
- ❌ Cassandra - Complexity > benefit for small scale

---

### Cache: Redis

**Live dashboard performance**:

- ✅ <5ms latency for hot data
- ✅ 5-minute TTL for computed metrics
- ✅ Pub/Sub for WebSocket updates
- ✅ Simple key-value store

**What NOT to use**:
- ❌ Memcached - No pub/sub (need updates)
- ❌ In-memory NATS KV - Latency + no persistence

---

### Cloud Storage: AWS S3 + Parquet

**Long-term data lake**:

**Parquet Format**:
- ✅ Columnar (tire temps in one place)
- ✅ Compressed 3-5x
- ✅ Native Athena support
- ✅ Glue-compatible schema

**Alternative Formats** (rejected):
- ❌ JSON - 10x larger, slower to scan
- ❌ CSV - No nested structures, no compression
- ❌ Avro - Good, but Parquet better for analytics

**S3 Partitioning**:
```
s3://wec-telemetry/
├── telemetry/
│   ├── session_id=20260221_001/
│   │   ├── car_id=CAR_001/
│   │   │   ├── lap=001/data_000000.parquet
│   │   │   ├── lap=002/data_000000.parquet
```

Allows Athena partition pruning = faster/cheaper queries

---

### Cloud Analytics: AWS Athena + Glue

**SQL on S3 (no ETL needed)**:

**Athena**:
```sql
-- Direct query on Parquet in S3
SELECT 
  lap,
  MAX(tire_fl_temp) as max_tire_temp,
  AVG(speed_kmh) as avg_speed
FROM "wec"."telemetry"
WHERE session_id = '20260221_001'
  AND car_id = 'CAR_001'
GROUP BY lap
ORDER BY lap;

-- Cost: $0.05/GB scanned ~$0.01 per race worth
```

**Glue**:
- Auto-discovers Parquet schema
- Creates Athena tables automatically
- Tracks schema versions
- Generates data catalog

**Alternative Approaches** (rejected):
- ❌ Manual ETL - Requires ongoing maintenance
- ❌ Redshift - Overkill, requires data loading
- ❌ BigQuery - Non-AWS, different auth model

---

## 🏗️ Project Structure

```
wec-telemetry-hybrid-architecture/
├── services/                           # Golang services
│   ├── ecu-simulator/
│   │   ├── main.go
│   │   ├── simulator.go
│   │   ├── physics.go
│   │   ├── go.mod
│   │   └── Dockerfile
│   ├── ingestion-service/
│   │   ├── main.go
│   │   ├── handlers.go
│   │   ├── jetstream.go
│   │   ├── go.mod
│   │   └── Dockerfile
│   ├── stream-processor/
│   │   ├── main.go
│   │   ├── processor.go
│   │   ├── metrics.go
│   │   ├── go.mod
│   │   └── Dockerfile
│   ├── raw-event-sink/
│   │   ├── main.go
│   │   ├── sinker.go
│   │   ├── parquet.go
│   │   ├── go.mod
│   │   └── Dockerfile
│   ├── query-api/
│   │   ├── main.go
│   │   ├── handlers.go
│   │   ├── websocket.go
│   │   ├── go.mod
│   │   └── Dockerfile
│   └── cloud-uploader/
│       ├── main.go
│       ├── uploader.go
│       ├── s3.go
│       ├── go.mod
│       └── Dockerfile
│
├── dashboards/                         # React UI
│   ├── ui/
│   │   ├── src/
│   │   │   ├── components/
│   │   │   │   ├── Dashboard.jsx
│   │   │   │   ├── LiveTelemetry.jsx
│   │   │   │   ├── LapMetrics.jsx
│   │   │   │   └── Alerts.jsx
│   │   │   ├── hooks/
│   │   │   │   └── useWebSocket.js
│   │   │   ├── App.jsx
│   │   │   └── main.jsx
│   │   ├── package.json
│   │   ├── Dockerfile
│   │   └── vite.config.js
│
├── infrastructure/
│   ├── docker-compose.yml
│   ├── nats/
│   │   └── jetstream.conf
│   ├── postgres/
│   │   ├── schema.sql
│   │   └── init.sh
│   └── minio/
│       └── config
│
├── architecture/
│   ├── Architecture.md
│   └── diagrams/
│       └── *.mmd (Mermaid diagrams)
│
├── ECU_DATA_GENERATION.md
├── TECH_STACK.md                    # This file
├── README.md
├── Makefile
├── docker-compose.yml
└── .github/
    └── workflows/
        ├── build.yml
        └── test.yml
```

---

## 🛠️ Development Workflow

### Phase 1: Backend Services (Golang)

#### 1.1 ECU Simulator
```bash
cd services/ecu-simulator
go mod init github.com/panppcode/wec-telemetry/ecu-simulator
go get github.com/nats-io/nats.go

# Core files needed:
# - main.go (orchestration loop)
# - simulator.go (CarSimulator struct + Step method)
# - physics.go (realistic tire/fuel/speed models)
# - models.go (TelemetryEvent struct)
# - config.go (SIMULATION_RATE, TRACK_LENGTH, etc.)
```

**Key Implementation**:
```go
// Run at 20 Hz (50ms per POST)
for range time.Ticker(50 * time.Millisecond).C {
    for _, car := range simulators {
        event := car.Step(0.05)  // 50ms timestep
        http.Post("http://ingestion:8080/telemetry/ingest", 
                  "application/json", 
                  json.Marshal(event))
    }
}
```

#### 1.2 Ingestion Service
```bash
cd services/ingestion-service
go mod init github.com/panpcode/wec-telemetry/ingestion-service
go get github.com/nats-io/nats.go

# Core files:
# - main.go (HTTP server setup)
# - handlers.go (POST /telemetry/ingest)
# - jetstream.go (JetStream publish logic)
# - models.go (TelemetryEvent validation schema)
# - middleware.go (metrics, logging)
```

**Key Implementation**:
```go
func (s *Server) IngestTelemetry(w http.ResponseWriter, r *http.Request) {
    // 1. Parse + validate
    var event TelemetryEvent
    json.NewDecoder(r.Body).Decode(&event)
    
    // 2. Publish async to JetStream
    s.jetstream.PublishAsync(
        fmt.Sprintf("telemetry.%s.raw", event.CarID),
        event.MarshalJSON())
    
    // 3. Return 202 Accepted (don't wait)
    w.WriteHeader(http.StatusAccepted)
}
```

#### 1.3 Stream Processor
```bash
cd services/stream-processor
go mod init github.com/panpcode/wec-telemetry/stream-processor
go get github.com/nats-io/nats.go
go get github.com/lib/pq

# Core files:
# - main.go (consumer loop)
# - processor.go (metric computation)
# - anomaly.go (anomaly detection)
# - postgres.go (database writes)
# - models.go (LapMetrics struct)
```

**Key Implementation**:
```go
// Subscribe to JetStream
sub, _ := jetstream.Subscribe("telemetry.*.raw", 
    func(msg jetstream.Msg) {
        // 1. Buffer events by lap
        // 2. When lap complete, compute aggregations
        // 3. Write to Postgres
        // 4. Ack JetStream
        metrics := processor.ProcessLap(events)
        postgres.InsertLap(metrics)
        msg.Ack()
    })
```

#### 1.4 Query API
```bash
cd services/query-api
go mod init github.com/panpcode/wec-telemetry/query-api
go get github.com/lib/pq
go get github.com/redis/go-redis/v9
go get nhooyr.io/websocket

# Core files:
# - main.go (HTTP + WebSocket server)
# - handlers.go (REST endpoints)
# - websocket.go (live updates)
# - queries.go (Postgres queries)
# - cache.go (Redis integration)
```

**Key Implementation**:
```go
// REST endpoint
func (s *Server) GetLiveTelemetry(w http.ResponseWriter, r *http.Request) {
    carID := r.URL.Query().Get("car_id")
    
    // Try Redis first (TTL 5min)
    cached := s.redis.Get(context.Background(), 
        fmt.Sprintf("telemetry:%s", carID))
    if cached != nil {
        w.Write(cached.Val())
        return
    }
    
    // Fall back to Postgres
    metrics := s.db.GetLatestMetrics(carID)
    s.redis.Set(..., metrics, 5*time.Minute)
    json.NewEncoder(w).Encode(metrics)
}

// WebSocket for real-time updates
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, _ := websocket.Accept(w, r)
    
    // Subscribe to JetStream computed events
    jetstream.Subscribe("telemetry-computed", func(msg jetstream.Msg) {
        conn.Write(context.Background(), 
            websocket.MessageText, msg.Data)
    })
}
```

### Phase 2: Frontend (React + Node.js)

```bash
cd dashboards/ui
npm init -y
npm install react vite recharts ws

# Core files:
# - src/components/Dashboard.jsx (main layout)
# - src/hooks/useWebSocket.js (real-time updates)
# - src/components/LiveTelemetry.jsx (speed, RPM, fuel)
# - src/components/LapMetrics.jsx (lap times, averages)
# - src/components/Alerts.jsx (anomalies)
```

**Key Implementation**:
```jsx
// Real-time WebSocket hook
function useWebSocket(url) {
  const [data, setData] = useState(null);
  
  useEffect(() => {
    const ws = new WebSocket(url);
    ws.onmessage = (e) => setData(JSON.parse(e.data));
    return () => ws.close();
  }, [url]);
  
  return data;
}

// Dashboard component
export function Dashboard() {
  const telemetry = useWebSocket('ws://localhost:8081/ws/telemetry');
  
  return (
    <div>
      <LiveTelemetry data={telemetry} />
      <LapMetrics data={telemetry} />
      <Alerts data={telemetry} />
    </div>
  );
}
```

### Phase 3: Cloud Integration

```bash
cd services/cloud-uploader
go mod init github.com/panpcode/wec-telemetry/cloud-uploader
go get github.com/aws/aws-sdk-go-v2/s3
go get github.com/nats-io/nats.go

# Core files:
# - main.go (background poller)
# - uploader.go (S3 upload logic)
# - s3.go (AWS SDK wrapper)
# - retry.go (exponential backoff)
# - models.go (upload state tracking)
```

---

## 📦 Golang Dependencies

### Core Dependencies for All Services

```go
// go.mod
require (
    github.com/nats-io/nats.go v1.32.0          // NATS client
    github.com/nats-io/nats-server/v2 v2.10.0   // For local testing
)

// Service-specific
// Ingestion Service:
    github.com/gorilla/mux v1.8.1               // HTTP routing
    github.com/prometheus/client_golang         // Metrics
    
// Stream Processor:
    github.com/lib/pq v1.10.9                   // PostgreSQL
    github.com/jmoiron/sqlc                     // Type-safe SQL
    
// Query API:
    nhooyr.io/websocket v1.8.10                 // WebSocket
    github.com/redis/go-redis/v9                // Redis client
    
// Cloud Uploader:
    github.com/aws/aws-sdk-go-v2/s3            // AWS S3
    github.com/aws/aws-sdk-go-v2/config        // AWS config
    github.com/google/uuid                      // ID generation
```

---

## 🚀 Getting Started

### Step 1: Setup Golang Project

```bash
git init wec-telemetry-hybrid-architecture
cd wec-telemetry-hybrid-architecture

# Create service structure
mkdir -p services/{ecu-simulator,ingestion-service,stream-processor,query-api}
mkdir -p dashboards/ui infrastructure

# Initialize each service
cd services/ecu-simulator
go mod init github.com/panpcode/wec-telemetry/ecu-simulator
```

### Step 2: Start Local Stack

```bash
docker-compose up nats postgres redis
```

### Step 3: Run Backends (each in separate terminal)

```bash
# Terminal 1: Ingestion
cd services/ingestion-service && go run main.go

# Terminal 2: Stream Processor
cd services/stream-processor && go run main.go

# Terminal 3: ECU Simulator
cd services/ecu-simulator && go run main.go

# Terminal 4: Query API
cd services/query-api && go run main.go
```

### Step 4: Run Frontend

```bash
cd dashboards/ui
npm run dev
# Visit http://localhost:5173
```

---

## ✅ Success Criteria

### Phase 1 Complete
- [ ] All Golang services compile and run
- [ ] ECU simulator sends 100 events/sec to ingestion
- [ ] JetStream streams show events in real-time
- [ ] Postgres stores lap metrics (<100ms latency)
- [ ] React dashboard displays live telemetry

### Phase 2 Complete
- [ ] Cloud uploader syncs Parquet to S3
- [ ] Athena queries return results in <5 seconds (48 hour data)
- [ ] Glue crawler detects schema automatically

### Portfolio Grade
- ✅ Shows Golang mastery (concurrency, async patterns)
- ✅ Shows distributed systems (event sourcing, NATS)
- ✅ Shows cloud integration (AWS data lake)
- ✅ Shows full-stack (backend + frontend)

---

## 📚 Resources

### Golang
- [Go Official Docs](https://golang.org/doc/)
- [NATS Go Client](https://github.com/nats-io/nats.go)
- [pq PostgreSQL Driver](https://github.com/lib/pq)
- [WebSocket Library](https://nhooyr.io/websocket)

### NATS JetStream
- [NATS Docs](https://docs.nats.io/)
- [JetStream Guide](https://docs.nats.io/nats-concepts/jetstream)
- [Examples](https://github.com/nats-io/nats.go/tree/main/examples)

### React
- [React Docs](https://react.dev/)
- [Vite Guide](https://vitejs.dev/)
- [Recharts Library](https://recharts.org/)

### AWS
- [S3 Documentation](https://docs.aws.amazon.com/s3/)
- [Athena Query Syntax](https://docs.aws.amazon.com/athena/latest/ug/SELECT.html)
- [Glue Crawler Setup](https://docs.aws.amazon.com/glue/latest/dg/crawler-configuration.html)

