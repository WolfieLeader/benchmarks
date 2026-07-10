-- Saturation curve (PLAN §9.2 query contract): open-model backpressure per
-- server across target rates — where offered rate falls off target, drops
-- appear, and tail latency climbs (PLAN §7.1 max-throughput search).
-- Reads ONLY the aggregate endpoint_stats table; open-mode columns are NULL
-- for closed-mode rows and are filtered out here.
--
-- ${run_id} and ${endpoint} are Grafana dashboard variables (swap for
-- literals in psql).
SELECT
    server,
    target_rate,
    offered_rate,
    rps                                                          AS completed_rate,
    dropped_iterations,
    dropped_iterations::double precision / NULLIF(attempted, 0)  AS drop_rate,
    max_backlog,
    schedule_lag_p99_ns,
    p99_ns,
    success_rate
FROM endpoint_stats
WHERE run_id = '${run_id}'
  AND endpoint = '${endpoint}'
  AND target_rate IS NOT NULL
ORDER BY server, target_rate;
