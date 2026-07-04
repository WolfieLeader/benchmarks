package influx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/InfluxCommunity/influxdb3-go/influxdb3"
)

// fakeInflux stands in for the metrics DB: /health gates NewClient and
// /api/v2/write is where influxdb3-go POSTs line protocol.
type fakeInflux struct {
	writeStatus  int          // status returned for /api/v2/write
	healthStatus int          // status returned for /health
	writes       atomic.Int64 // count of write requests received
	lastBody     atomic.Pointer[string]
	block        chan struct{} // if non-nil, /api/v2/write blocks until closed
}

func newFakeInflux(t *testing.T, writeStatus int) (*fakeInflux, *httptest.Server) {
	t.Helper()
	f := &fakeInflux{writeStatus: writeStatus, healthStatus: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(f.healthStatus)
	})
	mux.HandleFunc("/api/v2/write", func(w http.ResponseWriter, r *http.Request) {
		f.writes.Add(1)
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		f.lastBody.Store(&s)
		if f.block != nil {
			<-f.block
		}
		w.WriteHeader(f.writeStatus)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

func testClient(t *testing.T, srv *httptest.Server, sampleRate float64) *Client {
	t.Helper()
	// Health returns 200 immediately, so the poll succeeds on the first try.
	c, err := newClient(context.Background(), srv.URL, DefaultToken, DefaultDatabase, sampleRate)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestWriteSuccessCounted(t *testing.T) {
	_, srv := newFakeInflux(t, http.StatusNoContent)
	c := testClient(t, srv, 0)

	c.WritePoint("m", map[string]string{"run_id": "r"}, map[string]any{"v": int64(1)}, time.Now())

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
	f, srv := newFakeInflux(t, http.StatusInternalServerError)
	c := testClient(t, srv, 0)
	c.timeout = 2 * time.Second

	c.WritePoint("m", map[string]string{"run_id": "r"}, map[string]any{"v": int64(1)}, time.Now())

	if got := f.writes.Load(); got != int64(maxWriteAttempts) {
		t.Fatalf("write attempts = %d, want %d", got, maxWriteAttempts)
	}
	if got := c.Accounting().PointsDropped; got != 1 {
		t.Fatalf("PointsDropped = %d, want 1", got)
	}
	if got := c.Accounting().PointsWritten; got != 0 {
		t.Fatalf("PointsWritten = %d, want 0", got)
	}
	err := c.Wait()
	if err == nil || !strings.Contains(err.Error(), "dropped") {
		t.Fatalf("Wait after drop = %v, want dropped error", err)
	}
}

func TestCanceledWriteNotCountedAsDrop(t *testing.T) {
	f, srv := newFakeInflux(t, http.StatusInternalServerError)
	ctx, cancel := context.WithCancel(context.Background())
	c, err := newClient(ctx, srv.URL, DefaultToken, DefaultDatabase, 0)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	t.Cleanup(c.Close)
	cancel() // parent canceled before any write

	c.WritePoint("m", map[string]string{"run_id": "r"}, map[string]any{"v": int64(1)}, time.Now())

	if got := f.writes.Load(); got != 0 {
		t.Fatalf("write attempts after cancel = %d, want 0", got)
	}
	if got := c.Accounting().PointsDropped; got != 0 {
		t.Fatalf("PointsDropped after cancel = %d, want 0 (cancel is not a drop)", got)
	}
	if err := c.Wait(); err != nil {
		t.Fatalf("Wait after cancel = %v, want nil", err)
	}
}

func TestFinalFlushDeadline(t *testing.T) {
	f, srv := newFakeInflux(t, http.StatusNoContent)
	f.block = make(chan struct{})
	t.Cleanup(func() { close(f.block) })

	c := testClient(t, srv, 0)
	c.timeout = 10 * time.Second
	c.flushDeadline = 100 * time.Millisecond

	pt := influxdb3.NewPoint("m", map[string]string{"run_id": "r"}, map[string]any{"v": int64(1)}, time.Now())
	c.writePointsAsync([]*influxdb3.Point{pt})

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
	f, srv := newFakeInflux(t, http.StatusNoContent)
	c := testClient(t, srv, 0)

	// Force one drop so run_meta reports a non-zero points_dropped.
	c.pointsDropped.Store(3)
	c.pointsWritten.Store(42)
	c.pointsSampled.Store(7)

	if err := c.WriteRunMeta("run-x", 0.1); err != nil {
		t.Fatalf("WriteRunMeta: %v", err)
	}

	body := ""
	if p := f.lastBody.Load(); p != nil {
		body = *p
	}
	for _, want := range []string{"run_meta", "points_written=42", "points_dropped=3", "points_sampled_out=7"} {
		if !strings.Contains(body, want) {
			t.Fatalf("run_meta line protocol %q missing %q", body, want)
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
	f, srv := newFakeInflux(t, http.StatusNoContent)
	f.healthStatus = http.StatusServiceUnavailable

	savedTimeout, savedInterval := healthPollTimeout, healthPollInterval
	healthPollTimeout, healthPollInterval = 150*time.Millisecond, 30*time.Millisecond
	t.Cleanup(func() { healthPollTimeout, healthPollInterval = savedTimeout, savedInterval })

	_, err := newClient(context.Background(), srv.URL, DefaultToken, DefaultDatabase, 0)
	if err == nil {
		t.Fatal("newClient against unhealthy metrics DB = nil error, want failure")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("error = %v, want 'not ready'", err)
	}
}

func TestNilClientIsSafe(t *testing.T) {
	var c *Client
	c.WritePoint("m", nil, nil, time.Now())
	if err := c.Wait(); err != nil {
		t.Fatalf("nil Wait = %v, want nil", err)
	}
	if err := c.WriteRunMeta("r", 0.1); err != nil {
		t.Fatalf("nil WriteRunMeta = %v, want nil", err)
	}
	if got := (c.Accounting()); got != (Accounting{}) {
		t.Fatalf("nil Accounting = %+v, want zero", got)
	}
	c.Close()
}
