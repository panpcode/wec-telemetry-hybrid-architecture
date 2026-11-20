-- Lap times rebuilt from raw samples, with top speed and fuel used.
-- Laps that are still in progress, or only partly archived, are left out.
WITH laps AS (
    SELECT car_id,
           lap,
           max(timestamp) - min(timestamp)            AS lap_time,
           max(speed_kmh)                             AS top_speed_kmh,
           max(fuel_level_liters) FILTER (WHERE NOT in_pit)
             - min(fuel_level_liters) FILTER (WHERE NOT in_pit) AS fuel_used_l,
           bool_or(in_pit)                            AS pitted,
           count(*)                                   AS samples
    FROM telemetry
    GROUP BY car_id, lap
)
SELECT car_id, lap, lap_time, round(top_speed_kmh) AS top_speed_kmh,
       round(fuel_used_l, 2) AS fuel_used_l, pitted
FROM laps
WHERE lap < (SELECT max(lap) FROM telemetry t WHERE t.car_id = laps.car_id)
  AND samples > 100
ORDER BY car_id, lap;
