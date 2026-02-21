# Ingestion Service

Receives telemetry events from ECU simulators, validates them, and publishes to NATS JetStream for downstream processing.

## What It Does

```
ECU Simulator (20 Hz events)
    ↓ HTTP POST /telemetry/ingest
Ingestion Service (port 8080)
    ↓ Validate structure + ranges
    ↓
NATS JetStream
    ↓ Stream: TELEMETRY.{CAR_ID}
Stream Processor (computes metrics)
```

## Endpoints

### POST /telemetry/ingest
Receives and validates telemetry events.

**Request:**
```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "car_id": "CAR_001",
  "timestamp": "2026-02-21T15:22:46Z",
  "lap": 3,
  "speed_kmh": 285.5,
  "rpm": 8240,
  ...
}
```

**Responses:**
- `202 Accepted` - Event valid, published to NATS
- `400 Bad Request` - Validation failed (invalid JSON or invalid ranges)
- `500 Internal Server Error` - NATS publish failed

### GET /health
Returns service health and stats.

**Response:**
```json
{
  "status": "healthy",
  "nats_connected": true,
  "events_ingested": 1200,
  "timestamp": "2026-02-21T15:25:00Z"
}
```

## Validation Rules

| Field | Min | Max | Notes |
|-------|-----|-----|-------|
| speed_kmh | 0 | 400 | km/h |
| rpm | 0 | 10,000 | revolutions/min |
| throttle_percent | 0 | 100 | % |
| brake_pressure_bar | 0 | 25 | bar |
| tire_*_temp | 0 | 200 | °C |
| brake_temp | 0 | 600 | °C |
| fuel_level_liters | 0 | 100 | liters |
| fuel_flow_lph | 0 | 250 | L/h |

All fields are checked for NaN/Infinity values.

## Building

```bash
cd /Users/panp/PersonalCode/wec-telemetry-hybrid-architecture/services/ingestion-service
go build -o ingestion-service .
```

## Running

### Prerequisites
- NATS JetStream running on localhost:4222 (or set `-nats` flag)

### Start the service

```bash
./ingestion-service
```

### With custom addresses

```bash
./ingestion-service -http :9000 -nats nats://nats-server:4222
```

### Output
```
🏎️  WEC Ingestion Service Starting...
✅ Connected to NATS at nats://localhost:4222
🚀 Ingestion Service listening on :8080
```

## Testing

### Health check
```bash
curl http://localhost:8080/health | jq
```

### Send test event
```bash
curl -X POST http://localhost:8080/telemetry/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "event_id": "test-001",
    "car_id": "CAR_001",
    "session_id": "SESSION_20260221",
    "lap": 1,
    "sector": 0,
    "speed_kmh": 200,
    "rpm": 5000,
    "throttle_percent": 75,
    "brake_pressure_bar": 0,
    "tire_fl_temp": 95,
    "tire_fr_temp": 98,
    "tire_rl_temp": 92,
    "tire_rr_temp": 94,
    "fuel_level_liters": 70,
    "fuel_flow_lph": 150,
    "brake_temp": 200,
    "gps_lat": 48.27,
    "gps_long": 11.63,
    "on_track": true,
    "schema_version": "1.0"
  }' | jq
```

## NATS JetStream Streams

Events are published to streams named `TELEMETRY.{CAR_ID}`:

- `TELEMETRY.CAR_001` - Events from CAR_001
- `TELEMETRY.CAR_002` - Events from CAR_002
- etc.

Subscribers can listen to:
- Individual car: `TELEMETRY.CAR_001`
- All cars: `TELEMETRY.>` (wildcard)

## Architecture Integration

In the full system:

1. **ECU Simulator** - Generates raw telemetry (20 Hz × N cars)
2. **Ingestion Service** (this) - Validates + routes to NATS
3. **Stream Processor** - Computes lap metrics (<100ms latency)
4. **PostgreSQL** - Stores computed metrics only
5. **NATS JetStream** - 48-hour offline buffer
6. **Query API** - WebSocket for real-time dashboard
7. **React Dashboard** - Race engineer UI

## Performance

- Throughput: ~1,200 events/min per car
- Latency: <10ms ingest + validate + publish
- Memory: ~50MB baseline + buffer
- Connections: 1 persistent NATS connection
