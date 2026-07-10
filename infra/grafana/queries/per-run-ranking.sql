-- Per-run server ranking (PLAN §9.2 query contract).
-- Reads ONLY the aggregate endpoint_stats table — the exact numbers computed
-- from the full in-memory result set. Never derive percentiles from the
-- sampled request_events table.
--
-- ${run_id} is the Grafana dashboard variable (swap for a literal in psql).
-- Per-endpoint p50/p95/p99 are exact; the cross-endpoint roll-up is the
-- request-count-weighted mean of those exact per-endpoint percentiles.
WITH per_server AS (
    SELECT
        server,
        SUM(count)                                        AS requests,
        SUM(rps * count) / SUM(count)                     AS rps,
        SUM(p50_ns::double precision * count) / SUM(count) AS p50_ns,
        SUM(p95_ns::double precision * count) / SUM(count) AS p95_ns,
        SUM(p99_ns::double precision * count) / SUM(count) AS p99_ns,
        SUM(success_rate * count) / SUM(count)            AS success_rate
    FROM endpoint_stats
    WHERE run_id = '${run_id}'
    GROUP BY server
)
SELECT
    ROW_NUMBER() OVER (ORDER BY p50_ns) AS rank,
    server,
    requests,
    rps,
    p50_ns,
    p95_ns,
    p99_ns,
    success_rate
FROM per_server
ORDER BY rank;
