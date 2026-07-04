package influx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/InfluxCommunity/influxdb3-go/influxdb3"

	"benchmark-client/internal/cli"
)

type Client struct {
	client        *influxdb3.Client
	database      string
	ctx           context.Context
	timeout       time.Duration
	flushDeadline time.Duration
	sampleRate    float64
	wg            sync.WaitGroup

	// Run-level accounting (§7.2). Written into run_meta and printed in the
	// final summary; low-frequency increments, so atomics are cheap here.
	pointsWritten atomic.Int64 // points confirmed written to the metrics DB
	pointsDropped atomic.Int64 // points lost to write failures after retries
	pointsSampled atomic.Int64 // points intentionally skipped by 10% sampling
}

const (
	DefaultUrl          = "http://localhost:8181"
	DefaultDatabase     = "benchmarks"
	DefaultToken        = "benchmark-token"
	DefaultSampleRate   = 0.1
	defaultWriteTimeout = 15 * time.Second

	// A write is retried on transient failure before its points are counted as
	// dropped; Wait bounds the final drain so a wedged metrics DB can't hang the run.
	maxWriteAttempts     = 3
	writeBackoff         = 200 * time.Millisecond
	defaultFlushDeadline = 60 * time.Second
)

// Overridable in tests; the health poll must stay slow enough for a real
// metrics container to come up in production.
var (
	healthPollTimeout  = 30 * time.Second
	healthPollInterval = 2 * time.Second
)

func waitToBeReady(ctx context.Context, host string) error {
	healthUrl := host + "/health"
	deadline := time.Now().Add(healthPollTimeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, healthUrl, http.NoBody)
		if err != nil {
			cancel()
			return fmt.Errorf("build metrics health request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("metrics health check returned status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthPollInterval):
		}
	}

	if lastErr == nil {
		lastErr = errors.New("timed out")
	}
	return fmt.Errorf("metrics DB not ready at %s after %s: %w", host, healthPollTimeout, lastErr)
}

// NewClient connects to the metrics DB, returning an error when it is
// unreachable so the caller can fail the run (§7.2 — no silent metrics loss).
func NewClient(ctx context.Context, sampleRate float64) (*Client, error) {
	return newClient(ctx, DefaultUrl, DefaultToken, DefaultDatabase, sampleRate)
}

//nolint:contextcheck // context is stored in Client for use in async write operations
func newClient(ctx context.Context, host, token, database string, sampleRate float64) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := waitToBeReady(ctx, host); err != nil {
		return nil, err
	}

	client, err := influxdb3.New(influxdb3.ClientConfig{
		Host:     host,
		Token:    token,
		Database: database,
	})
	if err != nil {
		return nil, fmt.Errorf("create metrics client: %w", err)
	}

	if sampleRate <= 0 || sampleRate > 1 {
		sampleRate = DefaultSampleRate
	}

	return &Client{
		client:        client,
		database:      database,
		ctx:           ctx,
		timeout:       defaultWriteTimeout,
		flushDeadline: defaultFlushDeadline,
		sampleRate:    sampleRate,
	}, nil
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	if err := c.client.Close(); err != nil {
		cli.Warnf("Failed to close metrics client: %v", err)
	}
}

func (c *Client) WritePoint(measurement string, tags map[string]string, fields map[string]any, ts time.Time) {
	if c == nil {
		return
	}

	p := influxdb3.NewPoint(measurement, tags, fields, ts)
	_ = c.writePoints([]*influxdb3.Point{p})
}

// writePoints writes a batch with bounded retry. Parent-context cancellation is a
// clean stop (not counted); exhausting the retries counts the batch as dropped and
// returns an error so the failure is surfaced by the final flush.
func (c *Client) writePoints(points []*influxdb3.Point) error {
	if c == nil || len(points) == 0 {
		return nil
	}

	if c.ctx.Err() != nil {
		return c.ctx.Err()
	}

	var lastErr error
	for attempt := 1; attempt <= maxWriteAttempts; attempt++ {
		writeCtx := c.ctx
		var cancel context.CancelFunc
		if c.timeout > 0 {
			writeCtx, cancel = context.WithTimeout(c.ctx, c.timeout)
		}
		err := c.client.WritePoints(writeCtx, points)
		if cancel != nil {
			cancel()
		}
		if err == nil {
			c.pointsWritten.Add(int64(len(points)))
			return nil
		}

		// Parent cancellation (interrupt/shutdown) is a clean stop, not a drop.
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}

		lastErr = err
		if attempt < maxWriteAttempts {
			select {
			case <-c.ctx.Done():
				return c.ctx.Err()
			case <-time.After(writeBackoff * time.Duration(attempt)):
			}
		}
	}

	c.pointsDropped.Add(int64(len(points)))
	cli.Failf("Metrics write failed after %d attempts (%d points dropped): %v", maxWriteAttempts, len(points), lastErr)
	return fmt.Errorf("metrics write failed after %d attempts: %w", maxWriteAttempts, lastErr)
}

func (c *Client) writePointsAsync(points []*influxdb3.Point) {
	if c == nil || len(points) == 0 {
		return
	}

	pointsCopy := make([]*influxdb3.Point, len(points))
	copy(pointsCopy, points)

	c.wg.Go(func() {
		// Error is recorded via the drop counter and surfaced by Wait.
		_ = c.writePoints(pointsCopy)
	})
}

// Wait is the deadline-bounded final flush: it drains outstanding async writes and
// returns an error if the drain overruns the deadline or any point was dropped, so
// the orchestrator can fail the run instead of silently losing metrics (§7.2).
func (c *Client) Wait() error {
	if c == nil {
		return nil
	}

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	deadline := c.flushDeadline
	if deadline <= 0 {
		deadline = defaultFlushDeadline
	}

	select {
	case <-done:
	case <-time.After(deadline):
		return fmt.Errorf("metrics final flush exceeded %s deadline", deadline)
	}

	if n := c.pointsDropped.Load(); n > 0 {
		return fmt.Errorf("metrics flush dropped %d point(s) after retries", n)
	}
	return nil
}

func RunId(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}
