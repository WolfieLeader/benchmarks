package client

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"benchmark-client/internal/config"
)

// Open-model load generation (PLAN §7.1): requests are dispatched on a fixed
// arrival timetable derived from the configured rate/stages, and the headline
// latency is measured from the *intended* send time (wrk2 semantics), so a
// stalling server cannot suppress the samples that would have recorded the
// stall (coordinated omission). The arrival clock never blocks on workers:
// when all MaxInFlight workers are busy and the backlog queue is full, the
// arrival is counted as a dropped iteration (k6 semantics) — saturation is
// reported as data (schedule lag, backlog, drops), never silently absorbed.

// arrivalSegment is one piece of the piecewise-linear arrival-rate curve.
type arrivalSegment struct {
	startOffset time.Duration // segment start, offset from schedule start
	duration    time.Duration
	startRate   float64 // arrivals/sec at segment start
	endRate     float64 // arrivals/sec at segment end
	startCount  float64 // cumulative arrivals at segment start
}

// arrivals returns the total arrivals over the segment (trapezoid area).
func (seg arrivalSegment) arrivals() float64 {
	return (seg.startRate + seg.endRate) / 2 * seg.duration.Seconds()
}

type arrivalSchedule struct {
	segments []arrivalSegment
	total    time.Duration
}

// newArrivalSchedule builds the rate curve. With stages, each stage ramps
// linearly from the previous rate (initially load.Rate) to its target over its
// duration (k6 ramping-arrival-rate semantics) and the schedule length is the
// stages' total duration; without stages the rate is constant for window.
func newArrivalSchedule(load config.LoadConfig, window time.Duration) *arrivalSchedule {
	if len(load.Stages) == 0 {
		return &arrivalSchedule{
			segments: []arrivalSegment{{duration: window, startRate: load.Rate, endRate: load.Rate}},
			total:    window,
		}
	}

	segments := make([]arrivalSegment, 0, len(load.Stages))
	rate := load.Rate
	var offset time.Duration
	var count float64
	for _, stage := range load.Stages {
		seg := arrivalSegment{
			startOffset: offset,
			duration:    stage.Duration,
			startRate:   rate,
			endRate:     stage.Target,
			startCount:  count,
		}
		segments = append(segments, seg)
		count += seg.arrivals()
		offset += stage.Duration
		rate = stage.Target
	}
	return &arrivalSchedule{segments: segments, total: offset}
}

func (s *arrivalSchedule) totalArrivals() float64 {
	last := s.segments[len(s.segments)-1]
	return last.startCount + last.arrivals()
}

// arrivalTime returns the offset of the n-th (0-indexed) arrival from schedule
// start, or false when the schedule ends before that arrival. The n-th arrival
// is the instant the cumulative arrival curve N(t) reaches n, so a constant
// rate r yields arrivals at 0, 1/r, 2/r, …
func (s *arrivalSchedule) arrivalTime(n int) (time.Duration, bool) {
	k := float64(n)
	for _, seg := range s.segments {
		if k >= seg.startCount+seg.arrivals() {
			continue
		}
		local := k - seg.startCount // arrivals into this segment, in [0, seg.arrivals())
		durSec := seg.duration.Seconds()

		var t float64 // seconds into the segment
		if seg.startRate == seg.endRate {
			// constant rate; seg.arrivals() > local >= 0 implies startRate > 0
			t = local / seg.startRate
		} else {
			// N(t) = r0·t + (r1−r0)·t²/(2D) = local  →  a·t² + r0·t − local = 0
			a := (seg.endRate - seg.startRate) / (2 * durSec)
			disc := seg.startRate*seg.startRate + 4*a*local
			if disc < 0 {
				disc = 0 // float slack near the segment end
			}
			t = (-seg.startRate + math.Sqrt(disc)) / (2 * a)
		}

		if t < 0 {
			t = 0
		}
		if t > durSec {
			t = durSec
		}
		return seg.startOffset + time.Duration(t*float64(time.Second)), true
	}
	return 0, false
}

// OpenStats reports the open-model measurement extras for one endpoint run.
// Response carries the coordinated-omission-corrected percentiles: schedule
// lag (worker pickup − intended send) + service latency. The endpoint's main
// Stats keep the service-latency boundary (Do + body read + close) identical
// to closed mode so numbers stay comparable across modes. Two documented
// boundaries: BuildRequest runs between pickup and the service timer and is
// counted in neither (in-memory, and excluded identically in closed mode);
// requests canceled by the run window before completing have no response
// latency — they are reported via CanceledCount and their schedule lag IS
// included in the lag percentiles, so saturation's worst tail stays visible.
type OpenStats struct {
	TargetRate        float64       `json:"target_rate"`  // scheduled average arrivals/sec
	OfferedRate       float64       `json:"offered_rate"` // arrivals the generator actually produced/sec
	Attempted         int           `json:"attempted"`    // arrivals produced (dispatched + dropped)
	DroppedIterations int           `json:"dropped_iterations"`
	MaxBacklog        int           `json:"max_backlog"` // deepest queued backlog observed
	ScheduleLagP50    time.Duration `json:"schedule_lag_p50"`
	ScheduleLagP99    time.Duration `json:"schedule_lag_p99"`
	ScheduleLagMax    time.Duration `json:"schedule_lag_max"`
	Response          *Stats        `json:"response"`
}

type openItem struct {
	tc         *config.Testcase
	intendedAt time.Time
}

type openResult struct {
	scheduleLag    time.Duration // worker pickup − intended send time
	latency        time.Duration // service latency, same boundary as closed mode
	serverOffset   time.Duration
	endpointOffset time.Duration
	err            error
}

type dispatchOutcome struct {
	attempted       int
	dropped         int
	maxBacklog      int
	scheduleElapsed time.Duration
}

// runOpenTestcases is the open-model counterpart of the closed loop in
// runTestcases. A dispatcher walks the arrival timetable and hands work to a
// bounded queue consumed by MaxInFlight workers; a full queue converts the
// arrival into a dropped iteration instead of delaying the clock.
func (s *Suite) runOpenTestcases(testcases []*config.Testcase) *runOutcome {
	load := s.server.Load
	sched := newArrivalSchedule(load, s.server.DurationPerEndpoint)
	start := time.Now()

	// The window extends past the schedule by one request timeout so requests
	// dispatched near the end can complete: the late tail is exactly what
	// coordinated-omission correction must not throw away.
	ctx, cancel := context.WithTimeout(s.ctx, sched.total+s.server.RequestTimeout)
	defer cancel()

	queueCh := make(chan openItem, load.MaxInFlight)
	resultsCh := make(chan openResult, load.MaxInFlight)

	var wg sync.WaitGroup
	wg.Add(load.MaxInFlight)
	for range load.MaxInFlight {
		go func() {
			defer wg.Done()
			for item := range queueCh {
				pickup := time.Now()
				latency, err := s.executeTestcase(ctx, item.tc)
				resultsCh <- openResult{
					scheduleLag:    pickup.Sub(item.intendedAt),
					latency:        latency,
					serverOffset:   item.intendedAt.Sub(s.serverStartTime),
					endpointOffset: item.intendedAt.Sub(start),
					err:            err,
				}
			}
		}()
	}

	dispatchDone := make(chan dispatchOutcome, 1)
	go func() {
		defer close(queueCh)
		var out dispatchOutcome
		defer func() {
			out.scheduleElapsed = time.Since(start)
			dispatchDone <- out
		}()

		timer := time.NewTimer(time.Hour)
		defer timer.Stop()
		for n := 0; ; n++ {
			at, ok := sched.arrivalTime(n)
			if !ok {
				return
			}
			intendedAt := start.Add(at)
			if wait := time.Until(intendedAt); wait > 0 {
				timer.Reset(wait)
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
				}
			} else if ctx.Err() != nil {
				return
			}

			out.attempted++
			select {
			case queueCh <- openItem{tc: testcases[n%len(testcases)], intendedAt: intendedAt}:
				if backlog := len(queueCh); backlog > out.maxBacklog {
					out.maxBacklog = backlog
				}
			default:
				out.dropped++
			}
		}
	}()

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	outcome := &runOutcome{}
	latencies := make([]time.Duration, 0, 10000)
	responses := make([]time.Duration, 0, 10000)
	lags := make([]time.Duration, 0, 10000)
	outcome.timedLatencies = make([]TimedLatency, 0, 10000)

	var count int
	for r := range resultsCh {
		if r.err != nil {
			if isBenchmarkContextCancellation(ctx, r.err) {
				// The pickup happened, so the schedule lag is real — window-
				// canceled requests are the longest-waiting tail and dropping
				// their lag would reintroduce coordinated omission.
				outcome.canceledCount++
				lags = append(lags, r.scheduleLag)
				continue
			}
			outcome.failureCount++
			outcome.lastError = r.err.Error()
			lags = append(lags, r.scheduleLag)
			continue
		}

		count++
		lags = append(lags, r.scheduleLag)
		latencies = append(latencies, r.latency)
		responses = append(responses, r.scheduleLag+r.latency)
		outcome.timedLatencies = append(outcome.timedLatencies, TimedLatency{
			ServerOffset:   r.serverOffset,
			EndpointOffset: r.endpointOffset,
			Duration:       r.latency,
		})
	}

	dispatch := <-dispatchDone
	elapsed := time.Since(start)
	totalRequests := count + outcome.failureCount

	outcome.stats = CalculateStats(latencies, count, totalRequests, elapsed)

	open := &OpenStats{
		TargetRate:        sched.totalArrivals() / sched.total.Seconds(),
		Attempted:         dispatch.attempted,
		DroppedIterations: dispatch.dropped,
		MaxBacklog:        dispatch.maxBacklog,
		Response:          CalculateStats(responses, count, totalRequests, elapsed),
	}
	if sec := dispatch.scheduleElapsed.Seconds(); sec > 0 {
		open.OfferedRate = float64(dispatch.attempted) / sec
	}
	if len(lags) > 0 {
		slices.Sort(lags)
		open.ScheduleLagP50 = Percentile(lags, 50)
		open.ScheduleLagP99 = Percentile(lags, 99)
		open.ScheduleLagMax = lags[len(lags)-1]
	}
	outcome.open = open

	return outcome
}
