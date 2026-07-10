-- Cross-run diff (PLAN §9.2 query contract): compare two run_ids side by side,
-- e.g. Go 1.26 vs 1.27rc, or Node vs Bun for the same framework.
-- Reads ONLY the aggregate endpoint_stats table.
--
-- ${run_a} and ${run_b} are Grafana dashboard variables (swap for literals in
-- psql). Rows appear for every server+endpoint present in either run; deltas
-- are NULL when one side is missing.
WITH a AS (
    SELECT server, endpoint, method, source, rps, p50_ns, p95_ns, p99_ns
    FROM endpoint_stats
    WHERE run_id = '${run_a}'
), b AS (
    SELECT server, endpoint, method, source, rps, p50_ns, p95_ns, p99_ns
    FROM endpoint_stats
    WHERE run_id = '${run_b}'
)
SELECT
    COALESCE(a.server, b.server)     AS server,
    COALESCE(a.endpoint, b.endpoint) AS endpoint,
    COALESCE(a.source, b.source)     AS source,
    a.rps    AS rps_a,
    b.rps    AS rps_b,
    (b.rps - a.rps) / NULLIF(a.rps, 0) * 100                                    AS rps_delta_pct,
    a.p50_ns AS p50_ns_a,
    b.p50_ns AS p50_ns_b,
    (b.p50_ns - a.p50_ns)::double precision / NULLIF(a.p50_ns, 0) * 100         AS p50_delta_pct,
    a.p99_ns AS p99_ns_a,
    b.p99_ns AS p99_ns_b,
    (b.p99_ns - a.p99_ns)::double precision / NULLIF(a.p99_ns, 0) * 100         AS p99_delta_pct
FROM a
FULL OUTER JOIN b
    ON a.server = b.server AND a.endpoint = b.endpoint AND a.method = b.method AND a.source = b.source
ORDER BY server, endpoint;
