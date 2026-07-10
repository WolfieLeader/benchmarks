package metrics

import (
	"context"
	"time"
)

// Accounting is a snapshot of the run-level metrics counters (§7.2): how many
// rows reached the metrics DB, how many were lost to write failures after
// retries, and how many were intentionally dropped by sampling.
type Accounting struct {
	PointsWritten    int64
	PointsDropped    int64
	PointsSampledOut int64
}

// Accounting returns the current counter snapshot. Safe to call any time; the
// orchestrator reads it after the final flush.
func (c *Client) Accounting() Accounting {
	if c == nil {
		return Accounting{}
	}
	return Accounting{
		PointsWritten:    c.pointsWritten.Load(),
		PointsDropped:    c.pointsDropped.Load(),
		PointsSampledOut: c.pointsSampled.Load(),
	}
}

const upsertRunSql = `
INSERT INTO runs (run_id, started_at, finished_at, sample_rate, points_written, points_dropped, points_sampled_out)
VALUES ($1, $2, now(), $3, $4, $5, $6)
ON CONFLICT (run_id) DO UPDATE SET
    finished_at        = EXCLUDED.finished_at,
    sample_rate        = EXCLUDED.sample_rate,
    points_written     = EXCLUDED.points_written,
    points_dropped     = EXCLUDED.points_dropped,
    points_sampled_out = EXCLUDED.points_sampled_out`

// WriteRunMeta upserts the runs row at the end of the run, carrying the
// accounting counters alongside the sample rate. It goes through the retrying
// writer and returns an error so a failed runs write also fails the run.
func (c *Client) WriteRunMeta(runId string, startedAt time.Time, sampleRate float64) error {
	if c == nil {
		return nil
	}

	// Snapshot before this write so the stored counts describe the run's data
	// rows, not the runs row itself.
	acct := c.Accounting()
	return c.retryWrite("runs", 1, func(ctx context.Context) error {
		_, err := c.db.Exec(ctx, upsertRunSql,
			runId, startedAt, sampleRate,
			acct.PointsWritten, acct.PointsDropped, acct.PointsSampledOut)
		return err
	})
}
