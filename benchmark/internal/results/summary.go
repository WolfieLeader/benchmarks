package results

import (
	"benchmark-client/internal/client"
	"fmt"
	"slices"
	"time"
)

func PrintServerSummary(result *ServerResult) {
	if result.Error != "" {
		fmt.Printf("  FAILED: %s\n\n", result.Error)
		return
	}

	fmt.Printf("  Duration: %s\n", result.Duration)
	fmt.Printf("  Endpoints tested: %d\n", len(result.Endpoints))
	fmt.Println("  Endpoints:")
	for _, ep := range result.Endpoints {
		status := formatStatus(ep.Error, ep.Stats)
		avg := "-"
		note := ""
		lastErr := ""
		if ep.Stats != nil {
			avg = formatDurationMs(ep.Stats.Avg)
			if ep.FailureCount > 0 {
				note = fmt.Sprintf("fail %d", ep.FailureCount)
				lastErr = ep.LastError
			}
		} else if ep.Error != "" {
			note = "failed"
			lastErr = ep.Error
		} else {
			note = "no stats"
		}

		fmt.Printf("    %-6s %-30s %6s  %8s", ep.Method, ep.Path, status, avg)
		if note != "" {
			fmt.Printf("  %s", note)
		}
		fmt.Println()
		if lastErr != "" {
			fmt.Printf("      last error: %s\n", truncate(lastErr, 160))
		}
	}
	fmt.Println()
}

func PrintFinalSummary(results *BenchmarkResults) {
	fmt.Println("\n=== Benchmark Summary ===")
	fmt.Printf("Total servers: %d\n", results.Summary.TotalServers)
	fmt.Printf("Successful: %d\n", results.Summary.SuccessfulServers)
	fmt.Printf("Failed: %d\n", results.Summary.FailedServers)
	fmt.Printf("Total duration: %s\n", time.Duration(results.Summary.TotalDurationMs)*time.Millisecond)

	type rankedServer struct {
		name string
		avg  int64
		p50  int64
		p95  int64
		p99  int64
	}

	ranked := make([]rankedServer, 0)
	for _, s := range results.Servers {
		if s.Error == "" && s.Stats != nil {
			ranked = append(ranked, rankedServer{name: s.Name, avg: s.Stats.AvgNs, p50: s.Stats.P50Ns, p95: s.Stats.P95Ns, p99: s.Stats.P99Ns})
		}
	}

	if len(ranked) > 0 {
		slices.SortFunc(ranked, func(a, b rankedServer) int {
			if a.avg < b.avg {
				return -1
			}
			if a.avg > b.avg {
				return 1
			}
			return 0
		})

		fmt.Println("\n========= Server Rankings (by avg latency) =========")
		fmt.Println("  #  Server          Avg      P50      P95      P99")
		fmt.Println("  -- -------------- ------- ------- ------- -------")
		for i, s := range ranked {
			fmt.Printf("  %-2d %-14s %7s %7s %7s %7s\n",
				i+1,
				s.name,
				formatDurationMs(s.avg),
				formatDurationMs(s.p50),
				formatDurationMs(s.p95),
				formatDurationMs(s.p99))
		}
		fmt.Println("====================================================")
	}
}

func formatDurationMs[T int64 | time.Duration](t T) string {
	ms := float64(t) / float64(time.Millisecond)
	return fmt.Sprintf("%.2fms", ms)
}

func formatStatus(errMsg string, stats *client.Stats) string {
	if errMsg != "" {
		return "FAIL"
	}
	if stats != nil && stats.SuccessRate < 1.0 {
		return fmt.Sprintf("%.0f%%", stats.SuccessRate*100)
	}
	return "OK"
}

func truncate(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}
