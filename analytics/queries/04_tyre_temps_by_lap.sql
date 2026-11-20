-- Average tyre temperature per lap. The fronts run hotter under braking,
-- and every tyre creeps up as the set wears over a stint.
SELECT car_id,
       lap,
       round(avg(tire_fl_temp), 1) AS fl,
       round(avg(tire_fr_temp), 1) AS fr,
       round(avg(tire_rl_temp), 1) AS rl,
       round(avg(tire_rr_temp), 1) AS rr
FROM telemetry
WHERE on_track
GROUP BY car_id, lap
ORDER BY car_id, lap;
