-- Runs at the start of every DuckDB session (make query / make query-examples).
-- Credentials are the local dev ones from docker-compose.yml.
.bail on
.output /dev/null
LOAD httpfs;
LOAD postgres;

CREATE SECRET seaweedfs (
    TYPE S3,
    KEY_ID 'wec-dev',
    SECRET 'wec-dev-secret',
    ENDPOINT 'seaweedfs:8333',
    URL_STYLE 'path',
    USE_SSL false,
    REGION 'us-east-1'
);

-- Every raw 20 Hz sample the cloud uploader has archived.
-- session_id and car_id come from the folder names (Hive partitioning).
CREATE VIEW telemetry AS
SELECT * FROM read_parquet('s3://wec-telemetry/*/*/*.parquet', hive_partitioning = true);

-- Lap summaries and alerts computed live by the stream processor.
ATTACH 'host=postgres user=postgres password=postgres dbname=telemetry' AS pg (TYPE postgres, READ_ONLY);
.output
.bail off
