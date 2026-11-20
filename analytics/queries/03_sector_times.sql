-- Sector times per lap, pivoted so the cars can be compared side by side.
WITH sectors AS (
    SELECT car_id, lap, sector,
           round(epoch(max(timestamp) - min(timestamp)), 2) AS seconds
    FROM telemetry
    WHERE NOT in_pit
    GROUP BY car_id, lap, sector
)
PIVOT sectors
ON 'S' || (sector + 1)
USING any_value(seconds)
GROUP BY car_id, lap
ORDER BY lap, car_id;
