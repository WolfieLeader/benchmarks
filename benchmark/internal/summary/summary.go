package summary

import (
	"benchmark-client/internal/client"
	"fmt"
	"slices"
	"strings"
	"time"
)

func PrintServerSummary(result *ServerResult) {
	if result.Error != "" {
		fmt.Printf("  Status: FAILED\n")
		fmt.Printf("  Error: %s\n\n", result.Error)
		return
	}

	fmt.Printf("  Duration: %s | Endpoints: %d\n", formatDuration(result.Duration), len(result.Endpoints))

	if result.Resources != nil && result.Resources.Samples > 0 {
		memStr := formatMemory(result.Resources.Memory.AvgBytes)
		cpuStr := formatCPU(result.Resources.CPU.AvgPercent, result.Resources.Samples)
		warning := ""
		if len(result.Resources.Warnings) > 0 {
			warning = fmt.Sprintf(" (%s)", result.Resources.Warnings[0])
		}
		fmt.Printf("  Resources: %s mem, %s cpu%s\n", memStr, cpuStr, warning)
	}

	fmt.Println()
	fmt.Println("  Method  Path                              Status      Avg")
	fmt.Println("  ------  --------------------------------  ------  -------")

	if result.Capacity != nil {
		fmt.Printf("  Capacity: %d max workers, %.0f rps, %.2fms p99, %.1f%% success\n",
			result.Capacity.MaxWorkersPassed,
			result.Capacity.AchievedRPS,
			result.Capacity.P99Ms,
			result.Capacity.SuccessRate*100)
	}

	for _, ep := range result.Endpoints {
		status := formatStatus(ep.Error, ep.Stats)
		avg := "      -"
		if ep.Stats != nil {
			avg = formatLatencyFixed(ep.Stats.Avg)
		}

		path := truncatePath(ep.Path, 32)
		fmt.Printf("  %-6s  %-32s  %-6s  %s", ep.Method, path, status, avg)

		if ep.FailureCount > 0 {
			fmt.Printf("  (%d failed)", ep.FailureCount)
		}
		fmt.Println()

		if ep.Error != "" {
			fmt.Printf("          error: %s\n", truncate(ep.Error, 70))
		} else if ep.LastError != "" {
			fmt.Printf("          last error: %s\n", truncate(ep.LastError, 70))
		}
	}
	fmt.Println()
}

func PrintFinalSummary(meta *MetaResults, servers []ServerSummary) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                      BENCHMARK SUMMARY                      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	duration := time.Duration(meta.Summary.TotalDurationMs) * time.Millisecond
	fmt.Printf("  Servers: %d total, %d passed, %d failed\n",
		meta.Summary.TotalServers,
		meta.Summary.SuccessfulServers,
		meta.Summary.FailedServers)
	fmt.Printf("  Duration: %s\n", formatDuration(duration))
	fmt.Println()

	type rankedServer struct {
		name        string
		avg         int64
		p50         int64
		p95         int64
		mem         float64
		hasMem      bool
		capWorkers  int
		hasCapacity bool
	}

	ranked := make([]rankedServer, 0)
	hasAnyCapacity := false
	for _, s := range servers {
		if s.Error == "" && s.Stats != nil {
			rs := rankedServer{
				name: s.Name,
				avg:  s.Stats.AvgNs,
				p50:  s.Stats.P50Ns,
				p95:  s.Stats.P95Ns,
			}
			if s.Resources != nil && s.Resources.Samples >= 1 {
				rs.mem = s.Resources.Memory.AvgBytes
				rs.hasMem = true
			}
			if s.Capacity != nil {
				rs.capWorkers = s.Capacity.MaxWorkersPassed
				rs.hasCapacity = true
				hasAnyCapacity = true
			}
			ranked = append(ranked, rs)
		}
	}

	if len(ranked) == 0 {
		fmt.Println("  No successful benchmarks to rank.")
		return
	}

	slices.SortFunc(ranked, func(a, b rankedServer) int {
		if a.avg < b.avg {
			return -1
		}
		if a.avg > b.avg {
			return 1
		}
		return 0
	})

	fmt.Println("  Rankings (by avg latency)")
	fmt.Println()

	if hasAnyCapacity {
		fmt.Println("   #  Server              Avg      P50      P95      Mem  Capacity")
		fmt.Println("  ──  ────────────────  ───────  ───────  ───────  ───────  ────────")
	} else {
		fmt.Println("   #  Server              Avg      P50      P95      Mem")
		fmt.Println("  ──  ────────────────  ───────  ───────  ───────  ───────")
	}

	for i, s := range ranked {
		memStr := "      -"
		if s.hasMem {
			memStr = formatMemoryFixed(s.mem)
		}

		if hasAnyCapacity {
			capStr := "       -"
			if s.hasCapacity {
				capStr = fmt.Sprintf("%5d w", s.capWorkers)
			}
			fmt.Printf("  %2d  %-16s  %s  %s  %s  %s  %s\n",
				i+1,
				s.name,
				formatLatencyFixed(s.avg),
				formatLatencyFixed(s.p50),
				formatLatencyFixed(s.p95),
				memStr,
				capStr)
		} else {
			fmt.Printf("  %2d  %-16s  %s  %s  %s  %s\n",
				i+1,
				s.name,
				formatLatencyFixed(s.avg),
				formatLatencyFixed(s.p50),
				formatLatencyFixed(s.p95),
				memStr)
		}
	}
	fmt.Println()
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func formatLatencyFixed[T int64 | time.Duration](t T) string {
	ns := int64(t)
	if ns < 1000 {
		return fmt.Sprintf("%5dns", ns)
	}
	if ns < 1_000_000 {
		us := float64(ns) / 1000
		return fmt.Sprintf("%5.1fµs", us)
	}
	ms := float64(ns) / 1_000_000
	return fmt.Sprintf("%5.2fms", ms)
}

func formatMemory(bytes float64) string {
	mb := bytes / 1024 / 1024
	if mb < 1 {
		return fmt.Sprintf("%.0fKB", bytes/1024)
	}
	if mb < 100 {
		return fmt.Sprintf("%.1fMB", mb)
	}
	return fmt.Sprintf("%.0fMB", mb)
}

func formatMemoryFixed(bytes float64) string {
	mb := bytes / 1024 / 1024
	if mb < 1 {
		return fmt.Sprintf("%4.0fKB", bytes/1024)
	}
	return fmt.Sprintf("%5.1fMB", mb)
}

func formatCPU(percent float64, samples int) string {
	if samples < 2 || percent < 0.1 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", percent)
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

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path[:maxLen-3] + "..."
	}
	return ".../" + parts[len(parts)-1]
}
