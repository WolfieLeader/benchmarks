package client

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"benchmark-client/internal/config"
)

func TestArrivalScheduleConstant(t *testing.T) {
	t.Parallel()
	sched := newArrivalSchedule(config.LoadConfig{Rate: 100}, time.Second)

	cases := []struct {
		n    int
		want time.Duration
	}{
		{0, 0},
		{1, 10 * time.Millisecond},
		{50, 500 * time.Millisecond},
		{99, 990 * time.Millisecond},
	}
	for _, tc := range cases {
		got, ok := sched.arrivalTime(tc.n)
		if !ok {
			t.Fatalf("arrival %d: schedule ended early", tc.n)
		}
		if diff := (got - tc.want).Abs(); diff > time.Millisecond {
			t.Errorf("arrival %d: got %v, want %v", tc.n, got, tc.want)
		}
	}

	if _, ok := sched.arrivalTime(100); ok {
		t.Error("arrival 100 should be past the 1s window (exactly 100 arrivals fit)")
	}
	if got := sched.totalArrivals(); math.Abs(got-100) > 1e-9 {
		t.Errorf("totalArrivals: got %v, want 100", got)
	}
}

func TestArrivalScheduleRampFromZero(t *testing.T) {
	t.Parallel()
	sched := newArrivalSchedule(config.LoadConfig{
		Rate:   0,
		Stages: []config.StageConfig{{Target: 100, Duration: time.Second}},
	}, 5*time.Second) // window must be ignored when stages exist

	if sched.total != time.Second {
		t.Fatalf("schedule total: got %v, want 1s (stages override the window)", sched.total)
	}
	if got := sched.totalArrivals(); math.Abs(got-50) > 1e-9 {
		t.Fatalf("totalArrivals: got %v, want 50 (triangle area)", got)
	}

	// N(t) = 50·t² on a 0→100 ramp over 1s, so arrival k is at sqrt(k/50).
	prev := time.Duration(-1)
	for n := range 50 {
		got, ok := sched.arrivalTime(n)
		if !ok {
			t.Fatalf("arrival %d: schedule ended early", n)
		}
		want := time.Duration(math.Sqrt(float64(n)/50) * float64(time.Second))
		if diff := (got - want).Abs(); diff > time.Millisecond {
			t.Errorf("arrival %d: got %v, want %v", n, got, want)
		}
		if got <= prev && n > 0 {
			t.Errorf("arrival %d: %v not after previous %v", n, got, prev)
		}
		prev = got
	}
	if _, ok := sched.arrivalTime(50); ok {
		t.Error("arrival 50 should be past the ramp (only 50 arrivals fit)")
	}
}

func TestArrivalScheduleSilentThenRampAndDownRamp(t *testing.T) {
	t.Parallel()
	sched := newArrivalSchedule(config.LoadConfig{
		Rate: 0,
		Stages: []config.StageConfig{
			{Target: 0, Duration: time.Second},   // silent: 0→0, no arrivals
			{Target: 100, Duration: time.Second}, // ramp up: 50 arrivals
			{Target: 0, Duration: time.Second},   // ramp down: 50 arrivals
		},
	}, 0)

	if got := sched.totalArrivals(); math.Abs(got-100) > 1e-9 {
		t.Fatalf("totalArrivals: got %v, want 100", got)
	}

	first, ok := sched.arrivalTime(0)
	if !ok {
		t.Fatal("arrival 0 missing")
	}
	if first < time.Second {
		t.Errorf("arrival 0 at %v, want >= 1s (first stage is silent)", first)
	}

	prev := time.Duration(-1)
	for n := range 100 {
		got, ok := sched.arrivalTime(n)
		if !ok {
			t.Fatalf("arrival %d: schedule ended early", n)
		}
		if got <= prev && n > 0 {
			t.Errorf("arrival %d: %v not after previous %v", n, got, prev)
		}
		if got > 3*time.Second {
			t.Errorf("arrival %d at %v, past schedule end", n, got)
		}
		prev = got
	}
	if _, ok := sched.arrivalTime(100); ok {
		t.Error("arrival 100 should not exist")
	}
}

func newTestSuite(t *testing.T, handler http.HandlerFunc, load config.LoadConfig, window time.Duration) (*Suite, []*config.Testcase) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	server := &config.ResolvedServer{
		Name:                "test",
		RequestTimeout:      2 * time.Second,
		Concurrency:         4,
		Load:                load,
		DurationPerEndpoint: window,
	}
	testcases := []*config.Testcase{{
		EndpointName:   "root",
		Name:           "root",
		Path:           "/",
		RequestURI:     "/",
		Method:         "GET",
		ExpectedStatus: 200,
	}}
	suite := NewSuite(context.Background(), server, srv.URL, nil)
	suite.serverStartTime = time.Now()
	t.Cleanup(suite.Close)
	return suite, testcases
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestOpenLoopHitsTargetRate(t *testing.T) {
	t.Parallel()
	suite, testcases := newTestSuite(t, okHandler,
		config.LoadConfig{Mode: config.LoadModeOpen, Rate: 400, MaxInFlight: 128},
		250*time.Millisecond)

	outcome := suite.runTestcases(testcases)

	if outcome.open == nil {
		t.Fatal("open stats missing in open mode")
	}
	total := outcome.stats.TotalCount
	// 400 req/s over 250ms schedules 100 arrivals; allow generous timing slack.
	if total < 70 || total > 130 {
		t.Errorf("completed %d requests, want ~100", total)
	}
	if outcome.failureCount != 0 {
		t.Errorf("unexpected failures: %d (last: %s)", outcome.failureCount, outcome.lastError)
	}
	if outcome.open.DroppedIterations != 0 {
		t.Errorf("unexpected drops against an instant handler: %d", outcome.open.DroppedIterations)
	}
	if math.Abs(outcome.open.TargetRate-400) > 1 {
		t.Errorf("target rate: got %v, want 400", outcome.open.TargetRate)
	}
	if outcome.stats.Rps <= 0 {
		t.Error("rps not computed")
	}
	if outcome.open.Response == nil || outcome.open.Response.P99 < outcome.stats.P99 {
		t.Error("CO-corrected response percentiles should be >= service-latency percentiles")
	}
}

func TestOpenLoopCountsDropsWhenSaturated(t *testing.T) {
	t.Parallel()
	slow := func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}
	// 1000 req/s for 200ms = ~200 arrivals; 2 workers each stuck for the whole
	// window plus a 2-deep queue means nearly everything must be dropped loudly.
	suite, testcases := newTestSuite(t, slow,
		config.LoadConfig{Mode: config.LoadModeOpen, Rate: 1000, MaxInFlight: 2},
		200*time.Millisecond)

	outcome := suite.runTestcases(testcases)

	if outcome.open == nil {
		t.Fatal("open stats missing in open mode")
	}
	if outcome.open.DroppedIterations < 100 {
		t.Errorf("expected heavy drops under saturation, got %d (attempted %d)",
			outcome.open.DroppedIterations, outcome.open.Attempted)
	}
	if outcome.open.Attempted < 150 {
		t.Errorf("arrival clock stalled: attempted %d, want ~200 — the clock must not block on workers",
			outcome.open.Attempted)
	}
	if outcome.open.MaxBacklog > 2 {
		t.Errorf("backlog %d exceeds queue capacity 2", outcome.open.MaxBacklog)
	}
}

func TestOpenLoopCanceledPickupsKeepScheduleLag(t *testing.T) {
	t.Parallel()
	slow := func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}
	// Window 100ms + RequestTimeout 500ms grace = hard stop at 600ms. One
	// worker: request A runs 0→400ms; B (queued since ~20ms) is picked up at
	// ~400ms with ~380ms schedule lag and needs 400ms more — the 600ms window
	// deadline cancels it mid-flight. Its lag must still reach the lag stats.
	suite, testcases := newTestSuite(t, slow,
		config.LoadConfig{Mode: config.LoadModeOpen, Rate: 50, MaxInFlight: 1},
		100*time.Millisecond)
	suite.server.RequestTimeout = 500 * time.Millisecond

	outcome := suite.runTestcases(testcases)

	if outcome.open == nil {
		t.Fatal("open stats missing in open mode")
	}
	if outcome.canceledCount == 0 {
		t.Fatalf("expected a window-canceled request (count=%d failures=%d last=%q)",
			outcome.stats.Count, outcome.failureCount, outcome.lastError)
	}
	// B's ~380ms queue wait dwarfs any completed request's lag — if canceled
	// pickups were dropped, ScheduleLagMax would be near zero.
	if outcome.open.ScheduleLagMax < 200*time.Millisecond {
		t.Errorf("canceled pickup's schedule lag missing from lag stats: max=%v", outcome.open.ScheduleLagMax)
	}
}

func TestClosedLoopUnchanged(t *testing.T) {
	t.Parallel()
	suite, testcases := newTestSuite(t, okHandler, config.LoadConfig{Mode: config.LoadModeClosed}, 200*time.Millisecond)

	outcome := suite.runTestcases(testcases)

	if outcome.open != nil {
		t.Error("closed mode must not produce open stats")
	}
	if outcome.stats.TotalCount == 0 {
		t.Error("closed loop completed no requests")
	}
	if outcome.stats.Rps <= 0 {
		t.Error("rps not computed in closed mode")
	}
}
