package client

import "time"

type TimedLatency struct {
	ServerOffset   time.Duration // Time since server benchmark started
	EndpointOffset time.Duration // Time since this endpoint/flow started
	Duration       time.Duration // Request latency
}

type TimedResult struct {
	Endpoint  string
	Method    string
	Latencies []TimedLatency
}

type TimedFlowResult struct {
	FlowId    string
	Database  string
	Latencies []TimedLatency            // Total flow durations
	StepStats map[string][]TimedLatency // Per-step latencies keyed by step name
}
