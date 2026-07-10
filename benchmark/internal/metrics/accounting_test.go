package metrics

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeDb stands in for the metrics DB: Ping gates newClient readiness, Exec
// carries the schema apply + runs upsert, CopyFrom is the batched row path.
type fakeDb struct {
	pingErr  error
	copyErr  error        // returned by CopyFrom
	copies   atomic.Int64 // count of CopyFrom calls
	lastSql  atomic.Pointer[string]
	lastArgs atomic.Pointer[[]any]
	block    chan struct{} // if non-nil, CopyFrom blocks until closed
}

func (f *fakeDb) Ping(_ context.Context) error { return f.pingErr }

func (f *fakeDb) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.lastSql.Store(&sql)
	argsCopy := slices.Clone(args)
	f.lastArgs.Store(&argsCopy)
	return pgconn.CommandTag{}, nil
}

func (f *fakeDb) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, src pgx.CopyFromSource) (int64, error) {
	f.copies.Add(1)
	if f.block != nil {
		<-f.block
	}
	if f.copyErr != nil {
		return 0, f.copyErr
	}
	var n int64
	for src.Next() {
		n++
	}
	return n, nil
}

func (f *fakeDb) Close() {}

func testClient(t *testing.T, f *fakeDb, sampleRate float64) *Client {
	t.Helper()
	// Ping succeeds immediately, so the readiness poll passes on the first try.
	c, err := newClient(context.Background(), f, sampleRate)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func oneRow() [][]any {
	return [][]any{{time.Now(), "r", "srv", "ep", "", "endpoint", "", int64(0), int64(0), int64(1)}}
}

func TestNewClientAppliesSchema(t *testing.T) {
	f := &fakeDb{}
	testClient(t, f, 0)

	sql := ""
	if p := f.lastSql.Load(); p != nil {
		sql = *p
	}
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS runs") {
		t.Fatalf("newClient did not apply the schema, last SQL: %q", sql)
	}
}

func TestWriteSuccessCounted(t *testing.T) {
	f := &fakeDb{}
	c := testClient(t, f, 0)

	if err := c.writeRows("request_events", requestEventColumns, oneRow()); err != nil {
		t.Fatalf("writeRows: %v", err)
	}

	if got := c.Accounting().PointsWritten; got != 1 {
		t.Fatalf("PointsWritten = %d, want 1", got)
	}
	if got := c.Accounting().PointsDropped; got != 0 {
		t.Fatalf("PointsDropped = %d, want 0", got)
	}
	if err := c.Wait(); err != nil {
		t.Fatalf("Wait after success = %v, want nil", err)
	}
}

func TestWriteRetriesThenDrops(t *testing.T) {
	f := &fakeDb{copyErr: errors.New("connection refused")}
	c := testClient(t, f, 0)

	err := c.writeRows("request_events", requestEventColumns, oneRow())
	if err == nil {
		t.Fatal("writeRows against failing DB = nil error, want failure")
	}

	if got := f.copies.Load(); got != int64(maxWriteAttempts) {
		t.Fatalf("write attempts = %d, want %d", got, maxWriteAttempts)
	}
	if got := c.Accounting().PointsDropped; got != 1 {
		t.Fatalf("PointsDropped = %d, want 1", got)
	}
	if got := c.Accounting().PointsWritten; got != 0 {
		t.Fatalf("PointsWritten = %d, want 0", got)
	}
	err = c.Wait()
	if err == nil || !strings.Contains(err.Error(), "dropped") {
		t.Fatalf("Wait after drop = %v, want dropped error", err)
	}
}

func TestCanceledWriteNotCountedAsDrop(t *testing.T) {
	f := &fakeDb{copyErr: errors.New("connection refused")}
	ctx, cancel := context.WithCancel(context.Background())
	c, err := newClient(ctx, f, 0)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	t.Cleanup(c.Close)
	cancel() // parent canceled before any write

	if writeErr := c.writeRows("request_events", requestEventColumns, oneRow()); writeErr == nil {
		t.Fatal("writeRows after cancel = nil error, want context error")
	}

	if got := f.copies.Load(); got != 0 {
		t.Fatalf("write attempts after cancel = %d, want 0", got)
	}
	if got := c.Accounting().PointsDropped; got != 0 {
		t.Fatalf("PointsDropped after cancel = %d, want 0 (cancel is not a drop)", got)
	}
	if waitErr := c.Wait(); waitErr != nil {
		t.Fatalf("Wait after cancel = %v, want nil", waitErr)
	}
}

func TestFinalFlushDeadline(t *testing.T) {
	f := &fakeDb{block: make(chan struct{})}
	t.Cleanup(func() { close(f.block) })

	c := testClient(t, f, 0)
	c.flushDeadline = 100 * time.Millisecond

	c.writeEventRowsAsync(oneRow())

	start := time.Now()
	err := c.Wait()
	if err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("Wait with wedged write = %v, want deadline error", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Wait blocked %s, deadline not enforced", elapsed)
	}
}

func TestWriteRunMetaCarriesAccounting(t *testing.T) {
	f := &fakeDb{}
	c := testClient(t, f, 0)

	// Force counters so the runs row reports non-zero accounting.
	c.pointsDropped.Store(3)
	c.pointsWritten.Store(42)
	c.pointsSampled.Store(7)

	startedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	if err := c.WriteRunMeta("run-x", startedAt, 0.1); err != nil {
		t.Fatalf("WriteRunMeta: %v", err)
	}

	sql := ""
	if p := f.lastSql.Load(); p != nil {
		sql = *p
	}
	if !strings.Contains(sql, "INSERT INTO runs") {
		t.Fatalf("WriteRunMeta SQL %q does not target runs", sql)
	}

	var args []any
	if p := f.lastArgs.Load(); p != nil {
		args = *p
	}
	want := []any{"run-x", startedAt, 0.1, int64(42), int64(3), int64(7)}
	if len(args) != len(want) {
		t.Fatalf("runs upsert args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("runs upsert arg %d = %v, want %v", i, args[i], want[i])
		}
	}
}

func TestSampledOutCounting(t *testing.T) {
	// 10% sampling: every skip must bump the counter exactly once.
	c := &Client{sampleRate: 0.1}
	skipped := int64(0)
	for range 2000 {
		if c.shouldSkipSample() {
			skipped++
		}
	}
	if got := c.pointsSampled.Load(); got != skipped {
		t.Fatalf("pointsSampled = %d, observed %d skips", got, skipped)
	}
	if skipped == 0 {
		t.Fatal("expected some points to be sampled out at rate 0.1")
	}

	// Full sampling (rate 1) never skips and never counts.
	full := &Client{sampleRate: 1}
	for range 100 {
		if full.shouldSkipSample() {
			t.Fatal("rate 1 must never skip")
		}
	}
	if got := full.pointsSampled.Load(); got != 0 {
		t.Fatalf("pointsSampled at rate 1 = %d, want 0", got)
	}
}

func TestNewClientUnreachable(t *testing.T) {
	f := &fakeDb{pingErr: errors.New("connection refused")}

	savedTimeout, savedInterval := healthPollTimeout, healthPollInterval
	healthPollTimeout, healthPollInterval = 150*time.Millisecond, 30*time.Millisecond
	t.Cleanup(func() { healthPollTimeout, healthPollInterval = savedTimeout, savedInterval })

	_, err := newClient(context.Background(), f, 0)
	if err == nil {
		t.Fatal("newClient against unreachable metrics DB = nil error, want failure")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("error = %v, want 'not ready'", err)
	}
}

func TestNilClientIsSafe(t *testing.T) {
	var c *Client
	c.WriteEndpointLatencies("r", "srv", time.Now(), nil)
	c.WriteSequenceLatencies("r", "srv", time.Now(), nil)
	c.WriteEndpointStats("r", "srv", nil)
	c.WriteSequenceStats("r", "srv", nil)
	c.WriteResourceStats("r", "srv", nil)
	if err := c.Wait(); err != nil {
		t.Fatalf("nil Wait = %v, want nil", err)
	}
	if err := c.WriteRunMeta("r", time.Now(), 0.1); err != nil {
		t.Fatalf("nil WriteRunMeta = %v, want nil", err)
	}
	if got := (c.Accounting()); got != (Accounting{}) {
		t.Fatalf("nil Accounting = %+v, want zero", got)
	}
	c.Close()
}
