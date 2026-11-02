-- Applied by the stream processor on startup. Every statement is idempotent.

CREATE TABLE IF NOT EXISTS lap_metrics (
    session_id        TEXT        NOT NULL,
    car_id            TEXT        NOT NULL,
    lap               INTEGER     NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL,
    ended_at          TIMESTAMPTZ NOT NULL,
    lap_time_s        DOUBLE PRECISION NOT NULL,
    samples           INTEGER     NOT NULL,
    avg_speed_kmh     DOUBLE PRECISION NOT NULL,
    max_speed_kmh     DOUBLE PRECISION NOT NULL,
    max_rpm           DOUBLE PRECISION NOT NULL,
    fuel_start_l      DOUBLE PRECISION NOT NULL,
    fuel_end_l        DOUBLE PRECISION NOT NULL,
    fuel_used_l       DOUBLE PRECISION NOT NULL,
    avg_tire_fl_c     DOUBLE PRECISION NOT NULL,
    avg_tire_fr_c     DOUBLE PRECISION NOT NULL,
    avg_tire_rl_c     DOUBLE PRECISION NOT NULL,
    avg_tire_rr_c     DOUBLE PRECISION NOT NULL,
    max_tire_c        DOUBLE PRECISION NOT NULL,
    max_brake_c       DOUBLE PRECISION NOT NULL,
    pitted            BOOLEAN     NOT NULL,
    PRIMARY KEY (session_id, car_id, lap)
);

CREATE TABLE IF NOT EXISTS alerts (
    id          BIGSERIAL   PRIMARY KEY,
    session_id  TEXT        NOT NULL,
    car_id      TEXT        NOT NULL,
    lap         INTEGER     NOT NULL,
    kind        TEXT        NOT NULL,   -- tire_overheat | brake_overheat | fuel_low
    value       DOUBLE PRECISION NOT NULL,
    threshold   DOUBLE PRECISION NOT NULL,
    detail      TEXT        NOT NULL,
    raised_at   TIMESTAMPTZ NOT NULL,
    -- one alert of each kind per car per lap; redelivered events don't duplicate it
    UNIQUE (session_id, car_id, lap, kind)
);

CREATE INDEX IF NOT EXISTS alerts_car_time ON alerts (car_id, raised_at DESC);
