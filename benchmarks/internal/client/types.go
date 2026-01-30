package client

import "time"

// TimedLatency captures a request latency with timing context.
type TimedLatency struct {
	ServerOffset   time.Duration // Time since server benchmark started
	EndpointOffset time.Duration // Time since this endpoint/flow started
	Duration       time.Duration // Request latency
}

// TimedResult contains latencies with timing for InfluxDB export.
type TimedResult struct {
	Endpoint  string
	Method    string
	Latencies []TimedLatency
}

// TimedFlowResult contains flow latencies with timing.
type TimedFlowResult struct {
	FlowId    string
	Database  string
	Latencies []TimedLatency            // Total flow durations
	StepStats map[string][]TimedLatency // Per-step latencies keyed by step name
}
