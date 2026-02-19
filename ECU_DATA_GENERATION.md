# ECU Data Generation & System Design Guide

## 📡 Your Question Answered: How to Generate Real ECU Data

You asked: **"What is the design for generating real-time data from ECUs to develop an app which collects them and stores them in JetStream?"**

### The Reality

I don't have real ECUs (sensor hardware). My solution: **Simulate realistic car telemetry**.

There are two approaches:

---

## Approach 1: Direct Simulation (Recommended for POC)

### Architecture

```
┌──────────────────────────┐
│  ECU Simulator Service   │
│  (Single Python process) │
│                          │
│  - Math-based physics    │
│  - 1-3 car loops         │
│  - HTTP POST every 50ms  │
└────────┬─────────────────┘
         │
         │ POST /telemetry/ingest
         │ Content-Type: application/json
         │
         ▼
   Ingestion Service
         │
         ▼
   NATS JetStream
   (buffered stream)
```

### How It Works: Step by Step

#### Step 1: Create ECU Simulator Service

**File Structure**:
```
services/ecu-simulator/
├── main.go
├── simulator.go
├── physics.go
├── config.go
└── go.mod
```

**main.go** - Orchestrates simulation (Golang):
```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
    "time"
)

func main() {
    cars := map[string]*CarSimulator{
        "CAR_001": NewCarSimulator("CAR_001"),
        "CAR_002": NewCarSimulator("CAR_002"),
    }
    
    ticker := time.NewTicker(50 * time.Millisecond)  // 50ms = 20 Hz
    defer ticker.Stop()
    
    for range ticker.C {
        for _, simulator := range cars {
            event := simulator.Step(0.05)  // 50ms timestep
            
            // Send to ingestion via HTTP
            data, _ := json.Marshal(event)
            http.Post(
                "http://localhost:8080/telemetry/ingest",
                "application/json",
                bytes.NewReader(data),
            )
        }
    }
}
```

**simulator.go** - Physics-based car simulation (Golang):
```go
package main

import (
    "math"
    "math/rand"
    "time"
)

type CarSimulator struct {
    CarID           string
    SessionID       string
    Lap             int
    LapDistance     float64  // km
    Speed           float64  // km/h
    RPM             float64
    FuelLevel       float64  // liters
    TireTemps       map[string]float64
    BrakeTemp       float64
    Throttle        float64  // 0-1
    BrakePressure   float64
}

func NewCarSimulator(carID string) *CarSimulator {
    return &CarSimulator{
        CarID: carID,
        SessionID: "SESSION_" + time.Now().Format("20060102_150405"),
        Lap: 0,
        FuelLevel: 75.0,
        TireTemps: map[string]float64{"fl": 70, "fr": 70, "rl": 70, "rr": 70},
        BrakeTemp: 100,
    }
}

func (cs *CarSimulator) Step(dt float64) TelemetryEvent {
    // Simulate acceleration/braking
    lapProgress := math.Mod(cs.LapDistance / 13.6, 1.0)
    
    if lapProgress < 0.3 || (0.5 < lapProgress && lapProgress < 0.7) {
        cs.Throttle = math.Max(0, cs.Throttle - 0.15)
        cs.BrakePressure = math.Min(20, cs.BrakePressure + 0.2)
    } else {
        cs.Throttle = math.Min(1.0, cs.Throttle + 0.1)
        cs.BrakePressure = math.Max(0, cs.BrakePressure - 0.25)
    }
    
    // Physics: Throttle → RPM → Speed
    targetRPM := 3000 + (cs.Throttle * 6500)
    cs.RPM += (targetRPM - cs.RPM) * 0.1 * dt * 1000
    cs.Speed = (cs.RPM / 8000) * 300
    cs.Speed *= (1 - cs.BrakePressure * 0.01)
    
    // Distance traveled
    distanceKm := (cs.Speed / 3600) * dt
    cs.LapDistance += distanceKm
    
    // Lap complete?
    if cs.LapDistance >= 13.6 {
        cs.Lap++
        cs.LapDistance = 0
    }
    
    // Tire temperature
    thermalLoad := (cs.Speed / 300) * 100 + cs.BrakePressure * 3
    for tire := range cs.TireTemps {
        targetTemp := 70 + thermalLoad
        cs.TireTemps[tire] += (targetTemp - cs.TireTemps[tire]) * 0.05
        cs.TireTemps[tire] += rand.NormFloat64() * 0.3
    }
    
    // Fuel consumption
    fuelBurnRate := (cs.Throttle * 150) + (cs.RPM / 9000 * 50)  // L/h
    cs.FuelLevel -= (fuelBurnRate / 3600) * dt
    
    // Return event
    return TelemetryEvent{
        EventID: generateUUID(),
        Timestamp: time.Now(),
        CarID: cs.CarID,
        SessionID: cs.SessionID,
        Lap: cs.Lap,
        SpeedKmh: cs.Speed,
        RPM: cs.RPM,
        ThrottlePercent: cs.Throttle * 100,
        BrakePressureBar: cs.BrakePressure,
        TireFlTemp: cs.TireTemps["fl"],
        TireFrTemp: cs.TireTemps["fr"],
        TireRlTemp: cs.TireTemps["rl"],
        TireRrTemp: cs.TireTemps["rr"],
        FuelLevelLiters: cs.FuelLevel,
        SchemaVersion: "1.0",
    }
}
```

#### Step 2: Data Flow to JetStream

```
ECU Simulator
    ↓
    | HTTP POST
    ↓
Ingestion Service
    │
    ├─ Validates JSON schema
    ├─ Adds server timestamp
    ├─ Publishes to JetStream
    │   Subject: telemetry.{car_id}.raw
    │
    ↓
NATS JetStream
(stored on disk for offline operation)
    │
    ├─→ Stream Processor (real-time lap metrics)
    │        ↓ → Postgres (for dashboard)
    │
    └─→ Parquet Sinker (batch to disk)
         ↓ → Cloud Uploader → S3
```

#### Step 3: Store in JetStream

**JetStream Configuration** (automatic):

```nats
# Stream configuration
stream telemetry-raw {
  subjects: telemetry.*.raw
  
  # Disk storage (survive offline)
  storage: file
  
  # Retention policy
  max_bytes: 10GB      # Use ~5GB on-track, keep 10GB for margin
  max_age: 7d         # Auto-purge after 7 days
  max_msgs: 50000000  # Max 50M messages
  
  # Replicate for reliability (single node for local)
  num_replicas: 1
}

# Consumers
consumer_group stream-processor for telemetry-raw {
  delivery_policy: at_least_once
  ack_policy: explicit
  durable: true  # Survive process restarts
}

consumer_group s3-uploader for telemetry-raw {
  delivery_policy: at_least_once
  ack_policy: explicit
  durable: true
}
```

---

## Approach 2: Hardware Integration (Future)

Once you have real ECUs, replace the simulator with a real sensor gateway:

```python
# Instead of:
simulator = CarSimulator("CAR_001")
event = simulator.step(dt=0.05)

# You'd do:
gateway = SensorGateway("TRACK_NODE_001")
event = gateway.read_sensors()  # Actual ECU data via CAN-bus

# Same HTTP POST to ingestion!
```

No code changes needed - same architecture.

---

## 🏗️ Full System Flow for Your Use Case

### Scenario: Race Day (Saturday)

```
┌─────────────────────────────────────────────────────────┐
│  ON-TRACK (Pit Garage - Local Laptop)                   │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  09:00 - SESSION STARTS                                │
│  ┌────────────────┐                                     │
│  │ ECU Simulator  │ (Python) - 1 to 3 cars              │
│  │ HTTP POST      │ every 50ms to Ingestion             │
│  └────┬───────────┘                                     │
│       │                                                 │
│       ├──→ Ingestion Service (validates)               │
│           ├──→ NATS JetStream (disk-backed)            │
│           │   └─ Survives WiFi loss                    │
│           │                                            │
│           ├─→ Stream Processor (sub-100ms)             │
│           │   └─ Computes lap metrics                  │
│           │   └─ Updates Postgres                      │
│           │                                            │
│           └─→ Local Dashboard                          │
│               (Poll Postgres every 500ms)              │
│               Shows: Speed, RPM, Tire Temps,           │
│               Fuel Level, Pit Strategy                 │
│                                                        │
│  Parallel: Parquet Sinker batches events locally        │
│  └─ Every 5 sec: Write batch to /var/telemetry/buffer/ │
│     (gzip compressed Parquet files)                    │
│                                                        │
│  15:30 - SESSION ENDS                                  │
│  ┌────────────────────────────────┐                    │
│  │ Cloud Uploader Service         │                    │
│  │ (when WiFi available)          │                    │
│  │                                │                    │
│  │ Scans /var/telemetry/buffer/  │                    │
│  │ For each gzip file:            │                    │
│  │  1. Read from JetStream offset │                    │
│  │  2. Compress + upload to S3    │                    │
│  │  3. Delete local file          │                    │
│  │  4. Ack JetStream consumer     │                    │
│  └────┬─────────────────────────┘                      │
│       │                                                 │
└───────┼────────────────────────────────────────────────┘
        │
        │ HTTPS Upload
        │ (15MB gzipped Parquet)
        │
        ▼
┌─────────────────────────────────────────────────────────┐
│  CLOUD (AWS - Remote HQ)                                │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  S3 Bucket: wec-telemetry/                             │
│  └─ telemetry/                                          │
│     └─ session_id=20260221_001/                        │
│        ├─ car_id=CAR_001/                              │
│        │  └─ lap=001/data_000000.parquet               │
│        │  └─ lap=002/data_000000.parquet               │
│        │  └─ lap=003/data_000000.parquet               │
│        │                                               │
│        └─ car_id=CAR_002/                              │
│           └─ lap=001/data_000000.parquet               │
│                                                        │
│  Sunday Morning: AI Team Analyzes                      │
│  ────────────────────────────────────                 │
│                                                        │
│  1. AWS Glue (discovery)                              │
│     Creates tables in Data Catalog                    │
│     Schemas auto-detected                            │
│                                                        │
│  2. Athena (SQL queries)                              │
│     SELECT lap, MAX(tire_temp) FROM telemetry        │
│     WHERE car_id = 'CAR_001'                          │
│                                                        │
│  3. SageMaker (optional ML)                           │
│     Train tire wear prediction model                  │
│     on historical data                               │
│                                                        │
│  4. Tableau/QuickSight (optional dashboards)          │
│     Visualize trends across multiple races            │
│                                                        │
│  Output: Strategy insights for next race              │
│                                                        │
└─────────────────────────────────────────────────────────┘
```

---

## 🔧 Implementation Steps (Order)

### Phase 1: Local Simulation (Week 1)

1. **Create ECU Simulator** (Golang)
   - Car physics loop
   - HTTP POST every 50ms
   - 1 car initially

2. **Create Ingestion Service** (Golang)
   - HTTP endpoint
   - Schema validation
   - JetStream publish

3. **Setup NATS JetStream** (Docker)
   - Stream configuration
   - Consumer groups
   - Stream storage

4. **Create Stream Processor** (Golang)
   - Consumes from JetStream
   - Computes lap metrics
   - Writes to Postgres

5. **Setup Postgres** (Docker)
   - Metrics schema
   - Indexes

6. **Create Local Dashboard** (React/Node.js)
   - Real-time WebSocket/polling
   - Display telemetry
   - Show anomalies

### Phase 2: Cloud Integration (Week 2)

7. **Create Parquet Sinker** (Golang)
   - Batches events to disk
   - Gzip compression

8. **Create Cloud Uploader** (Golang)
   - Scans local buffer
   - Uploads to S3
   - Handles retries/dedup
   - Tracks offsets

9. **Setup AWS Infrastructure**
   - S3 bucket + lifecycle policy
   - Glue crawler
   - Athena table

10. **Test End-to-End**
    - Generate data locally
    - Upload to S3
    - Query with Athena

---

## 📊 Data Generation Key Parameters

### Update `config.py`:

```python
# Which cars to simulate
CARS = ["CAR_001", "CAR_002"]  # Can add CAR_003

# Simulation rate
SIMULATION_RATE_HZ = 20  # 50ms per event (20 events/sec per car)
# Total: 2 cars × 20 = 40 events/sec

# Track length (Lemans ~13.6 km)
TRACK_LENGTH_KM = 13.6

# Realistic ranges
SPEED_MAX_KMH = 330
RPM_MAX = 9500
TIRE_TEMP_MAX = 120  # °C
FUEL_CONSUMPTION_LPH = 140  # liters per hour avg

# Ingestion endpoint
INGESTION_URL = "http://localhost:8080"
INGESTION_TIMEOUT = 5  # seconds

# Optional: simulate stops (pit stops)
ENABLE_PIT_STOPS = False
PIT_STOP_INTERVAL_LAPS = 5
PIT_STOP_DURATION_SEC = 30
```

---

## 🎯 Key Design Decisions Explained

### 1. Why HTTP POST (not direct JetStream)?

**Simulator → Ingestion (HTTP POST)**  
✅ Decoupled (simulator doesn't need JetStream library)  
✅ Easy to debug (can curl endpoints)  
✅ Can replay events (Ingestion is stateless)  
✅ Standard (easier to replace with real sensors)
✅ Language-agnostic (Golang simulator can post to any backend)  

### 2. Why Golang?  

**For backend services** (Simulator, Ingestion, Processor, Uploader):
✅ Fast (compiled, not interpreted)  
✅ Concurrent (goroutines handle 1000s of async tasks)  
✅ Small binary (single executable, easy Docker deployment)  
✅ Zero GC pauses (predictable latency <100ms)  
✅ Battle-tested (used by Docker, Kubernetes, Uber, etc.)  
✅ Great stdlib (HTTP, JSON, sync primitives)  
✅ NATS Go client (native, high performance)  

**For UI** (Dashboard):
✅ Node.js + React (standard for web UIs)  
✅ Rich ecosystem (charting, real-time libraries)  
✅ Easy frontend iteration for race engineers  
✅ Separate from backend (can develop independently)
✅ Simple schema evolution

### 4. Why batched S3 upload (not real-time)?

**Local + Cloud split:**  
✅ Doesn't block local dashboard  
✅ Can work offline (buffer locally)  
✅ Reduces AWS cost (batch uploads cheaper)  
✅ Reliable (retry logic if upload fails)

---

### What You'll Have After This Setup

```bash
# Terminal 1: Start the stack
docker-compose up

# Terminal 2: Start ECU simulator (Golang binary)
./bin/ecu-simulator

# Terminal 3: Watch JetStream stream
docker-compose exec nats nats sub telemetry.>

# Terminal 4: Check Postgres
docker-compose exec postgres psql -d telemetry -c \
  "SELECT car_id, lap, avg_speed_kmh FROM laps ORDER BY created_at DESC LIMIT 5;"

# Browser: Open dashboard (React)
open http://localhost:3000
```

**Result**: 
- Data flowing: Simulator (Go) → Ingestion (Go) → JetStream → Processor (Go) → Dashboard (React)
- Shows: Real-time lap metrics on local dashboard
- All events buffered in JetStream (48hr offline capacity)
- Ready to upload to S3 when WiFi available

---

## Summary: Your Architecture

**Local (On-Track)**:
```
Simulated ECU Sensors
    ↓ (HTTP POST, 50ms)
Ingestion Service
    ↓ (JetStream publish)
NATS JetStream Stream
    ├→ Stream Processor → Postgres → Dashboard
    └→ Parquet Sinker → Local FS Buffer
```

**Cloud (At HQ)**:
```
S3 Data Lake
    ├→ Athena (SQL)
    ├→ Glue (Discovery + ETL)
    └→ SageMaker (ML)
    
AI Team: Analyze + Build Models
```

**Connection**: Async Cloud Uploader (when WiFi available)

Is this clearer? Want me to create the actual Python/config files next?

