package influx

import (
	"time"

	"github.com/InfluxCommunity/influxdb3-go/influxdb3"
)

// Accounting is a snapshot of the run-level metrics counters (§7.2): how many
// points reached the metrics DB, how many were lost to write failures after
// retries, and how many were intentionally dropped by the 10% sampling.
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

// WriteRunMeta writes the run_meta row at the end of the run, carrying the
// accounting counters alongside the sample rate. It goes through the retrying
// writer and returns an error so a failed run_meta write also fails the run.
func (c *Client) WriteRunMeta(runId string, sampleRate float64) error {
	if c == nil {
		return nil
	}

	// Snapshot before this write so the stored counts describe the run's data
	// points, not the run_meta row itself.
	acct := c.Accounting()
	point := influxdb3.NewPoint(
		"run_meta",
		map[string]string{tagRunId: runId},
		map[string]any{
			"sample_rate":        sampleRate,
			"points_written":     acct.PointsWritten,
			"points_dropped":     acct.PointsDropped,
			"points_sampled_out": acct.PointsSampledOut,
		},
		time.Now(),
	)
	return c.writePoints([]*influxdb3.Point{point})
}
