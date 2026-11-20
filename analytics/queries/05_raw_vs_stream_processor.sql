-- Cross-check: lap times rebuilt from the S3 archive against the lap times
-- the stream processor computed live and stored in Postgres.
WITH raw AS (
    SELECT session_id, car_id, lap,
           epoch(max(timestamp) - min(timestamp)) AS raw_lap_s
    FROM telemetry
    GROUP BY ALL
)
SELECT m.car_id,
       m.lap,
       round(m.lap_time_s, 2)                 AS live_lap_s,
       round(raw.raw_lap_s, 2)                AS archived_lap_s,
       CAST(round((raw.raw_lap_s - m.lap_time_s) * 1000) AS INTEGER) AS diff_ms
FROM pg.public.lap_metrics m
JOIN raw USING (session_id, car_id, lap)
ORDER BY m.car_id, m.lap;
