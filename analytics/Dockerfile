# DuckDB CLI with the S3 (httpfs) and Postgres extensions pre-installed,
# so queries work without downloading anything at run time.
FROM debian:bookworm-slim

ARG DUCKDB_VERSION=1.1.3
ARG TARGETARCH

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl unzip \
 && rm -rf /var/lib/apt/lists/*

RUN arch=$([ "$TARGETARCH" = "arm64" ] && echo aarch64 || echo amd64) \
 && curl -fsSL -o /tmp/duckdb.zip \
      "https://github.com/duckdb/duckdb/releases/download/v${DUCKDB_VERSION}/duckdb_cli-linux-${arch}.zip" \
 && unzip /tmp/duckdb.zip -d /usr/local/bin \
 && rm /tmp/duckdb.zip \
 && duckdb -c "INSTALL httpfs; INSTALL postgres;"

WORKDIR /analytics
ENTRYPOINT ["duckdb", "-init", "/analytics/init.sql"]
