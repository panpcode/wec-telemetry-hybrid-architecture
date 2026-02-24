# ECU Simulator

Physics-based racing telemetry data generator for WEC (World Endurance Championship) on-track telemetry systems.

## What It Does

The ECU Simulator generates **realistic racing telemetry events** by simulating a race car's physics, sensor readings, and performance characteristics. It outputs 20 events per second (~1,200 per minute) in JSON format, designed to mimic real Electronic Control Unit (ECU) data from a competition car.

## How It Works

### Physics Simulation
The simulator models realistic car behavior on a Le Mans-style 13.6 km circuit:

- **Speed & Throttle**: Throttle input (0-100%) → RPM (3,000-9,500) → Speed (0-330 km/h)
- **Braking Zones**: Automatically brakes in corners (30% of lap), accelerates on straights
- **Tire Temperature**: Realistic thermal behavior (70°C baseline → 110°C+ under load)
- **Fuel Consumption**: ~150-200 L/h at full throttle, based on throttle + RPM
- **Brake Temperature**: Builds up during braking, cools at ~2% per step
- **Lap Tracking**: Completes 13.6 km laps, tracks lap number and sector position

### Data Validation
All float values are clamped to physical ranges and validated before JSON serialization to prevent NaN/Infinity errors:

- Speed: 0-396 km/h (MaxSpeed × 1.2)
- RPM: 0-10,450 (MaxRPM × 1.1)
- Tire Temps: 30-150°C
- Brake Temps: 80-500°C
- Fuel: 0-75.75L (capacity × 1.01)

## Event Structure

Each telemetry event includes:

```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-02-21T14:55:44Z",
  "car_id": "CAR_001",
  "session_id": "SESSION_20260221_145544",
  "lap": 3,
  "sector": 1,
  "speed_kmh": 285.5,
  "rpm": 8240,
  "throttle_percent": 95.2,
  "brake_pressure_bar": 0.50,
  "tire_fl_temp": 108.3,
  "tire_fr_temp": 112.1,
  "tire_rl_temp": 106.8,
  "tire_rr_temp": 110.2,
  "fuel_level_liters": 52.34,
  "fuel_flow_lph": 185.6,
  "brake_temp": 380.5,
  "gps_lat": 48.2712,
  "gps_long": 11.6298,
  "on_track": true,
  "schema_version": "1.0"
}
```

## Running the Simulator

### Build

```bash
cd /Users/panp/PersonalCode/wec-telemetry-hybrid-architecture/services/ecu-simulator
go build -o simulator .
```

### Run

```bash
./simulator
```

### Output Example

```
2026/02/21 14:55:44 🏎️  WEC ECU Simulator Starting...
2026/02/21 14:55:44 ✅ Created simulator for CAR_001
2026/02/21 14:55:44 🚀 Starting simulation loop (20.0 Hz, duration: 10m0s)
2026/02/21 14:55:44 ❌ Failed to send event to http://localhost:8080: dial tcp 127.0.0.1:8080: connect: connection refused
2026/02/21 14:55:45 📡 Events sent: 100 (elapsed: 1s)
2026/02/21 14:56:00 📡 Events sent: 1200 (elapsed: 16s)
```

## Configuration

Edit constants in `main.go`:

```go
const (
    SIMULATION_RATE_HZ  = 20              // Events per second
    SIMULATION_INTERVAL = 50 * time.Millisecond
    INGESTION_URL       = "http://localhost:8080"  // Target endpoint
    NUM_CARS            = 2               // Number of cars to simulate
    SIMULATION_DURATION = 10 * time.Minute
)
```

## What to Expect

### Success Indicators
- ✅ Events generated at **20 Hz** (one every 50ms)
- ✅ Physics values in realistic ranges
- ✅ Lap completes every ~20 seconds (13.6 km at 330 km/h cruise)
- ✅ Tire temps gradually rise from 70°C to 110°C
- ✅ Fuel depletes ~1% per lap

### Without Ingestion Service
- ⚠️ Connection refused errors (expected if no server on port 8080)
- ✅ Simulator still generates data in memory
- ✅ Events are validated before JSON serialization

### With Ingestion Service Running
- ✅ Events POST to `/telemetry/ingest` endpoint
- ✅ Status 200/202 responses logged
- ✅ Data flows through NATS JetStream → PostgreSQL → S3

## Testing

Generate data and pipe to a local HTTP mock server:

```bash
# Terminal 1: Start a simple echo server
nc -l 8080

# Terminal 2: Run simulator
./simulator
```

Monitor raw events:

```bash
# Terminal 3: Simulation creates structured logs
tail -f simulator.log
```

## Files

- **main.go** - Event loop orchestration, HTTP client
- **simulator.go** - Physics engine, CarSimulator struct, Step() method
- **models.go** - TelemetryEvent, CarConfig structures
- **go.mod** - Dependencies (uuid generation)

## Next Steps

1. **Ingestion Service** - Receives events, validates, publishes to NATS
2. **Stream Processor** - Computes lap metrics from event stream
3. **Query API** - WebSocket server for real-time dashboard
4. **Dashboard** - React UI displaying live telemetry

## Dependencies

- Go 1.21+
- `github.com/google/uuid` - Event ID generation
- Standard library only (no external dependencies beyond UUID)
