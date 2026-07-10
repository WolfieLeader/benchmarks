// Package metrics writes benchmark results to the dedicated metrics-postgres
// container (PLAN §9.1) — never the benchmarked postgres instance. Aggregate
// tables are written from full in-memory result sets; raw request events are
// sampled drilldown only. All failure modes are loud (§7.2): an unreachable
// metrics DB fails the run, dropped writes are counted and fail the final flush.
package metrics

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"benchmark-client/internal/cli"
)

//go:embed schema.sql
var schemaSql string

// pgDb is the consumer-side seam over *pgxpool.Pool: exactly the calls the
// writer makes, fakeable in tests without a live Postgres.
type pgDb interface {
	Ping(ctx context.Context) error
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
	Close()
}

type Client struct {
	db            pgDb
	ctx           context.Context
	timeout       time.Duration
	flushDeadline time.Duration
	sampleRate    float64
	wg            sync.WaitGroup

	// Run-level accounting (§7.2). Written into the runs row and printed in the
	// final summary; low-frequency increments, so atomics are cheap here.
	pointsWritten atomic.Int64 // rows confirmed written to the metrics DB
	pointsDropped atomic.Int64 // rows lost to write failures after retries
	pointsSampled atomic.Int64 // rows intentionally skipped by sampling
}

const (
	// Dedicated metrics-postgres from the grafana compose stack, host port 20091
	// (PLAN §6) — never the benchmarked postgres on 5432.
	DefaultDsn          = "postgres://benchmark:benchmark@localhost:20091/benchmarks?sslmode=disable" //nolint:gosec // local metrics container credentials, not a secret
	DefaultSampleRate   = 0.1
	defaultWriteTimeout = 15 * time.Second

	// A write is retried on transient failure before its rows are counted as
	// dropped; Wait bounds the final drain so a wedged metrics DB can't hang the run.
	maxWriteAttempts     = 3
	writeBackoff         = 200 * time.Millisecond
	defaultFlushDeadline = 60 * time.Second
)

// Overridable in tests; the readiness poll must stay slow enough for a real
// metrics container to come up in production.
var (
	healthPollTimeout  = 30 * time.Second
	healthPollInterval = 2 * time.Second
)

func waitToBeReady(ctx context.Context, db pgDb) error {
	deadline := time.Now().Add(healthPollTimeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := db.Ping(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthPollInterval):
		}
	}

	if lastErr == nil {
		lastErr = errors.New("timed out")
	}
	return fmt.Errorf("metrics DB not ready after %s: %w", healthPollTimeout, lastErr)
}

// NewClient connects to the metrics DB and applies the schema, returning an
// error when it is unreachable so the caller can fail the run (§7.2 — no
// silent metrics loss).
//
//nolint:contextcheck // context is stored in Client for use in async write operations
func NewClient(ctx context.Context, sampleRate float64) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	pool, err := pgxpool.New(ctx, DefaultDsn)
	if err != nil {
		return nil, fmt.Errorf("create metrics pool: %w", err)
	}

	client, err := newClient(ctx, pool, sampleRate)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return client, nil
}

func newClient(ctx context.Context, db pgDb, sampleRate float64) (*Client, error) {
	if err := waitToBeReady(ctx, db); err != nil {
		return nil, err
	}

	if _, err := db.Exec(ctx, schemaSql); err != nil {
		return nil, fmt.Errorf("apply metrics schema: %w", err)
	}

	if sampleRate <= 0 || sampleRate > 1 {
		sampleRate = DefaultSampleRate
	}

	return &Client{
		db:            db,
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
	c.db.Close()
}

// retryWrite runs op with bounded retry and per-attempt timeout, keeping the
// accounting honest: success counts rowCount as written, exhausting the retries
// counts it as dropped and returns an error so the failure is surfaced by the
// final flush. Parent-context cancellation is a clean stop (not counted).
func (c *Client) retryWrite(label string, rowCount int64, op func(ctx context.Context) error) error {
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
		err := op(writeCtx)
		if cancel != nil {
			cancel()
		}
		if err == nil {
			c.pointsWritten.Add(rowCount)
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

	c.pointsDropped.Add(rowCount)
	cli.Failf("Metrics write to %s failed after %d attempts (%d rows dropped): %v", label, maxWriteAttempts, rowCount, lastErr)
	return fmt.Errorf("metrics write to %s failed after %d attempts: %w", label, maxWriteAttempts, lastErr)
}

// writeRows COPYs a batch into table with bounded retry. COPY is transactional,
// so a batch either lands whole or not at all.
func (c *Client) writeRows(table string, columns []string, rows [][]any) error {
	if c == nil || len(rows) == 0 {
		return nil
	}

	return c.retryWrite(table, int64(len(rows)), func(ctx context.Context) error {
		_, err := c.db.CopyFrom(ctx, pgx.Identifier{table}, columns, pgx.CopyFromRows(rows))
		return err
	})
}

// writeEventRowsAsync queues a request_events batch for a background COPY; the
// raw-event stream is the only high-volume write, so it is the only async path.
func (c *Client) writeEventRowsAsync(rows [][]any) {
	if c == nil || len(rows) == 0 {
		return
	}

	rowsCopy := make([][]any, len(rows))
	copy(rowsCopy, rows)

	c.wg.Go(func() {
		// Error is recorded via the drop counter and surfaced by Wait.
		_ = c.writeRows("request_events", requestEventColumns, rowsCopy)
	})
}

// Wait is the deadline-bounded final flush: it drains outstanding async writes and
// returns an error if the drain overruns the deadline or any row was dropped, so
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
		return fmt.Errorf("metrics flush dropped %d row(s) after retries", n)
	}
	return nil
}

func RunId(t time.Time) string {
	return t.UTC().Format("20060102-150405")
}
