# Ingestion Service

HTTP entry point for telemetry. Each event is checked against basic range limits and then published to NATS JetStream on `TELEMETRY.<car_id>`.

## Endpoints

`POST /telemetry/ingest` takes one event as JSON (see the main README for the full schema).

| Status | Meaning |
|---|---|
| `202` | Valid event, published to NATS |
| `400` | Invalid JSON, or a value outside the limits below |
| `500` | NATS publish failed |

`GET /health` returns `status`, `nats_connected` and `events_ingested`.

## Validation limits

| Field | Range |
|---|---|
| `speed_kmh` | 0–400 |
| `rpm` | 0–10,000 |
| `throttle_percent` | 0–100 |
| `brake_pressure_bar` | 0–25 |
| `tire_*_temp` | 0–200 °C |
| `brake_temp` | 0–600 °C |
| `fuel_level_liters` | 0–100 |
| `fuel_flow_lph` | 0–250 |

NaN and infinite values are rejected.

## Stream

On startup the service creates the `TELEMETRY` stream if it doesn't already exist. The stream uses file storage and keeps 7 days of data, capped at 10 GB. The stream processor and the cloud uploader create it the same way, so start-up order doesn't matter.

## Configuration

| Env var | Flag | Default |
|---|---|---|
| `HTTP_ADDR` | `-http` | `:8080` |
| `NATS_URL` | `-nats` | `nats://localhost:4222` |

If both are set, the flag wins.

## Try it

```bash
go run .                                   # needs NATS on localhost:4222
curl localhost:8080/health

curl -X POST localhost:8080/telemetry/ingest -H 'Content-Type: application/json' -d '{
  "event_id": "test-001", "timestamp": "2026-06-14T15:00:00Z",
  "car_id": "CAR_001", "session_id": "TEST", "lap": 1, "sector": 0,
  "speed_kmh": 200, "rpm": 7000, "throttle_percent": 80, "brake_pressure_bar": 0,
  "tire_fl_temp": 95, "tire_fr_temp": 97, "tire_rl_temp": 92, "tire_rr_temp": 94,
  "fuel_level_liters": 70, "fuel_flow_lph": 120, "brake_temp": 250,
  "gps_lat": 47.95, "gps_long": 0.21, "on_track": true, "in_pit": false,
  "schema_version": "1.0"
}'
```
