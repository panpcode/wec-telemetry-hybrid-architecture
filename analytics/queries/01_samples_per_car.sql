-- How much raw data is archived per session and car.
SELECT session_id,
       car_id,
       count(*)                       AS samples,
       min(timestamp)                 AS first_sample,
       max(timestamp)                 AS last_sample,
       max(lap)                       AS last_lap
FROM telemetry
GROUP BY ALL
ORDER BY session_id, car_id;
