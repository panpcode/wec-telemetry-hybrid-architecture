# ECU Simulator

Generates physics-based telemetry for N WEC Hypercars and POSTs each sample to the ingestion service at `SIM_HZ` (default 20 Hz per car).

## Configuration

All settings are environment variables:

| Variable | Default | Meaning |
|---|---|---|
| `INGESTION_URL` | `http://localhost:8080` | Ingestion service base URL |
| `TRACK` | `le_mans` | `le_mans`, `spa` or `imola` |
| `NUM_CARS` | `2` | Cars to simulate (`CAR_001`, `CAR_002`, …) |
| `SIM_HZ` | `20` | Samples per second per car |
| `SIM_DURATION` | `10m` | How long to run before exiting |
| `PIT_EVERY_LAPS` | `8` | Laps per stint; `0` disables pit stops |

All cars in a run share one `session_id`.

## Physics model

- **Speed**: flat-out acceleration that tapers toward top speed; in each braking zone the driver brakes at up to ~4 g down to apex speed (40 % of top speed), then holds part throttle through the corner.
- **RPM**: derived from speed through a 7-speed gearbox (shifts at 9,200 rpm), so the trace has the usual saw-tooth shape.
- **Tyres**: settle at ~85–105 °C. Braking heats the fronts, traction heats the rears, and worn tyres run about 0.9 °C hotter per lap of the stint. Fresh tyres are fitted at each pit stop.
- **Brakes**: heat input scales with brake pressure × speed, and cooling scales with airflow. Normal laps peak at ~320–380 °C.
- **Fuel**: up to ~150 L/h flat out, which works out to ~5.6 L per Le Mans lap, or about 12 laps on the 75 L tank before the 8 L low-fuel alert.
- **Pit stops**: 30 s stationary, refuelled at ~3 L/s, with new tyres. The first stop comes after `PIT_EVERY_LAPS` laps.
- **Variation**: each car gets its own pace (97–100 % of top speed) and braking style, plus a small random spread from lap to lap, so the cars never produce identical traces.

Reference flying-lap times from the offline test (real Hypercar pole times in brackets):

| Track | Simulated | Real |
|---|---|---|
| Le Mans | ~3:33 | ~3:24 |
| Spa | ~2:08 | ~2:01 |
| Imola | ~1:35 | ~1:29 |

Every value stays inside the ingestion service's validation limits. `simulator_test.go` checks this for every sample it simulates.

## Running

```bash
# Whole stack (recommended)
docker compose up --build

# Standalone, against a local ingestion service
go run .
TRACK=spa NUM_CARS=3 PIT_EVERY_LAPS=12 go run .
```

## Testing

```bash
go test -v .
```

The tests simulate 10 laps per track offline, with no waiting on the clock. They print per-lap time, speed, fuel, tyre and brake figures, and check that:
- lap times are realistic
- normal laps stay under the alert thresholds
- the first pit stop comes on the configured lap
- cars with different seeds don't produce identical laps

## Files

- `main.go`: configuration from env, one simulator per car, HTTP send loop
- `simulator.go`: physics (`CarSimulator.Step`)
- `models.go`: event schema, track definitions, per-car config
- `simulator_test.go`: offline physics tests
