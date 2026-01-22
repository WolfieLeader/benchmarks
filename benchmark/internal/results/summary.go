package results

import (
	"fmt"
	"slices"
	"time"
)

// PrintServerSummary outputs a summary for a single server result.
func PrintServerSummary(result *ServerResult) {
	if result.Error != "" {
		fmt.Printf("  FAILED: %s\n\n", result.Error)
		return
	}

	fmt.Printf("  Duration: %s\n", result.Duration)

	if result.OverallStats != nil {
		stats := result.OverallStats
		fmt.Printf("  Endpoints tested: %d\n", stats.EndpointCount)
		fmt.Printf("  Total requests: %d (success: %.2f%%)\n",
			stats.TotalRequests, stats.SuccessRate*100)
		fmt.Printf("  Latency - Avg: %s, Min: %s, Max: %s\n",
			stats.AvgLatency, stats.MinLatency, stats.MaxLatency)
		fmt.Printf("  Latency - P50: %s, P95: %s, P99: %s\n",
			stats.P50Latency, stats.P95Latency, stats.P99Latency)
	}

	fmt.Println("  Endpoints:")
	for _, ep := range result.Endpoints {
		status := "OK"
		if ep.Error != "" {
			status = "FAILED"
		} else if ep.Stats != nil && ep.Stats.SuccessRate < 1.0 {
			status = fmt.Sprintf("%.0f%%", ep.Stats.SuccessRate*100)
		}
		fmt.Printf("    %s %s [%s]\n", ep.Method, ep.Path, status)
		if ep.Stats != nil {
			fmt.Printf("      Avg: %s | P50: %s | P95: %s | P99: %s | Min: %s | Max: %s\n",
				ep.Stats.Avg, ep.Stats.P50, ep.Stats.P95, ep.Stats.P99, ep.Stats.Low, ep.Stats.High)
		}
	}
	fmt.Println()
}

// PrintFinalSummary outputs the overall benchmark summary.
func PrintFinalSummary(results *BenchmarkResults) {
	fmt.Println("\n=== Benchmark Summary ===")
	fmt.Printf("Total servers: %d\n", results.Summary.TotalServers)
	fmt.Printf("Successful: %d\n", results.Summary.SuccessfulServers)
	fmt.Printf("Failed: %d\n", results.Summary.FailedServers)
	fmt.Printf("Total duration: %s\n", results.Summary.TotalDuration)

	type rankedServer struct {
		Name       string
		AvgLatency int64
		P50Latency int64
		P95Latency int64
		P99Latency int64
	}

	ranked := make([]rankedServer, 0)
	for _, s := range results.Servers {
		if s.Error == "" && s.OverallStats != nil {
			ranked = append(ranked, rankedServer{
				Name:       s.Name,
				AvgLatency: s.OverallStats.AvgLatencyNs,
				P50Latency: s.OverallStats.P50LatencyNs,
				P95Latency: s.OverallStats.P95LatencyNs,
				P99Latency: s.OverallStats.P99LatencyNs,
			})
		}
	}

	if len(ranked) > 0 {
		slices.SortFunc(ranked, func(a, b rankedServer) int {
			if a.AvgLatency < b.AvgLatency {
				return -1
			}
			if a.AvgLatency > b.AvgLatency {
				return 1
			}
			return 0
		})

		fmt.Println("\n=== Server Rankings (by avg latency) ===")
		for i, s := range ranked {
			fmt.Printf("  %d. %s - Avg: %s | P50: %s | P95: %s | P99: %s\n",
				i+1,
				s.Name,
				time.Duration(s.AvgLatency).String(),
				time.Duration(s.P50Latency).String(),
				time.Duration(s.P95Latency).String(),
				time.Duration(s.P99Latency).String(),
			)
		}
	}
}
