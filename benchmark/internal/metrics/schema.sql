-- Metrics schema (PLAN §9.1 decision 3), applied idempotently by the client at
-- startup. Former Influx tags are plain indexed columns; run_id is the primary
-- selector everywhere. Timestamps are real wall-clock times (server run start +
-- request offset), not the old synthetic baseTime+index·µs hack.
--
-- Aggregate tables (endpoint_stats, sequence_stats, resource_samples) carry
-- exact numbers computed from the FULL in-memory result set before sampling;
-- request_events holds sampled raw drilldown ONLY and is never the source of
-- truth for percentiles (§9.1 decision 4, §9.2 query contract).

CREATE TABLE IF NOT EXISTS runs (
    run_id             text PRIMARY KEY,
    started_at         timestamptz NOT NULL,
    finished_at        timestamptz NOT NULL,
    sample_rate        double precision NOT NULL,
    points_written     bigint NOT NULL,
    points_dropped     bigint NOT NULL,
    points_sampled_out bigint NOT NULL
);

-- Sampled raw drilldown only. source: 'endpoint' | 'sequence' | 'sequence_step'
-- ('sequence' rows carry the full-sequence duration; endpoint = sequence id).
CREATE TABLE IF NOT EXISTS request_events (
    time               timestamptz NOT NULL,
    run_id             text NOT NULL,
    server             text NOT NULL,
    endpoint           text NOT NULL,
    method             text NOT NULL DEFAULT '',
    source             text NOT NULL,
    database           text NOT NULL DEFAULT '',
    server_offset_ms   bigint NOT NULL,
    endpoint_offset_ms bigint NOT NULL,
    latency_ns         bigint NOT NULL
);

CREATE INDEX IF NOT EXISTS request_events_run_idx ON request_events (run_id, server, endpoint);

-- Exact per-endpoint aggregates from the full result set. source: 'endpoint' |
-- 'sequence_step'. Open-mode columns are NULL for closed-mode rows.
CREATE TABLE IF NOT EXISTS endpoint_stats (
    time                timestamptz NOT NULL,
    run_id              text NOT NULL,
    server              text NOT NULL,
    endpoint            text NOT NULL,
    method              text NOT NULL DEFAULT '',
    source              text NOT NULL,
    database            text NOT NULL DEFAULT '',
    count               bigint NOT NULL,
    rps                 double precision NOT NULL,
    avg_ns              bigint NOT NULL,
    p50_ns              bigint NOT NULL,
    p95_ns              bigint NOT NULL,
    p99_ns              bigint NOT NULL,
    p999_ns             bigint NOT NULL,
    min_ns              bigint NOT NULL,
    max_ns              bigint NOT NULL,
    success_rate        double precision NOT NULL,
    target_rate         double precision,
    offered_rate        double precision,
    attempted           bigint,
    dropped_iterations  bigint,
    max_backlog         bigint,
    schedule_lag_p50_ns bigint,
    schedule_lag_p99_ns bigint,
    schedule_lag_max_ns bigint
);

CREATE INDEX IF NOT EXISTS endpoint_stats_run_idx ON endpoint_stats (run_id, server);

-- Exact per-sequence aggregates (full-sequence durations) from the full result set.
CREATE TABLE IF NOT EXISTS sequence_stats (
    time         timestamptz NOT NULL,
    run_id       text NOT NULL,
    server       text NOT NULL,
    sequence_id  text NOT NULL,
    database     text NOT NULL DEFAULT '',
    total_runs   bigint NOT NULL,
    successes    bigint NOT NULL,
    failures     bigint NOT NULL,
    success_rate double precision NOT NULL,
    avg_ns       bigint NOT NULL,
    p50_ns       bigint NOT NULL,
    p95_ns       bigint NOT NULL,
    p99_ns       bigint NOT NULL
);

CREATE INDEX IF NOT EXISTS sequence_stats_run_idx ON sequence_stats (run_id, server);

-- One summary row per sampled container per server run (min/avg/max over the
-- sampler window). source: 'server' | 'database' (database column names the DB).
CREATE TABLE IF NOT EXISTS resource_samples (
    time             timestamptz NOT NULL,
    run_id           text NOT NULL,
    server           text NOT NULL,
    source           text NOT NULL,
    database         text NOT NULL DEFAULT '',
    memory_min_bytes double precision NOT NULL,
    memory_avg_bytes double precision NOT NULL,
    memory_max_bytes double precision NOT NULL,
    cpu_min_percent  double precision NOT NULL,
    cpu_avg_percent  double precision NOT NULL,
    cpu_max_percent  double precision NOT NULL,
    samples          bigint NOT NULL
);

CREATE INDEX IF NOT EXISTS resource_samples_run_idx ON resource_samples (run_id, server);
