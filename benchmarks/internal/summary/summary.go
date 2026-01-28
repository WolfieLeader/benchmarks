package summary

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"benchmark-client/internal/client"
	"benchmark-client/internal/printer"
)

func PrintServerSummary(result *ServerResult) {
	if result.Error != "" {
		printer.Failf("Status: FAILED")
		printer.Linef("Error: %s", result.Error)
		printer.Blank()
		return
	}

	printer.KeyValuePairs(
		"Duration", printer.FormatDuration(result.Duration),
		"Endpoints", strconv.Itoa(len(result.Endpoints)),
	)

	if result.Resources != nil && result.Resources.Samples > 0 {
		memStr := printer.FormatMemory(result.Resources.Memory.AvgBytes)
		cpuStr := printer.FormatCPU(result.Resources.CPU.AvgPercent, result.Resources.Samples)
		warning := ""
		if len(result.Resources.Warnings) > 0 {
			warning = fmt.Sprintf(" (%s)", result.Resources.Warnings[0])
		}
		printer.KeyValuePairs("Memory", memStr, "CPU", cpuStr+warning)
	}

	printer.Blank()

	if result.Capacity != nil {
		printer.Linef("Capacity: %d max workers │ %.0f rps │ %.2fms p99 │ %.1f%% success",
			result.Capacity.MaxWorkersPassed,
			result.Capacity.AchievedRPS,
			result.Capacity.P99Ms,
			result.Capacity.SuccessRate*100)
		printer.Blank()
	}

	fmt.Printf("  %-6s  %-32s  %-6s  %s\n", "Method", "Path", "Status", "Avg")
	fmt.Printf("  %-6s  %-32s  %-6s  %s\n", "──────", "────────────────────────────────", "──────", "───────")

	for _, ep := range result.Endpoints {
		status := formatStatus(ep.Error, ep.Stats)
		avg := "      -"
		if ep.Stats != nil {
			avg = printer.FormatLatency(ep.Stats.Avg)
		}

		path := printer.TruncatePath(ep.Path, 32)
		statusSymbol := printer.SymbolPass
		if ep.Error != "" || (ep.Stats != nil && ep.Stats.SuccessRate < 1.0) {
			statusSymbol = printer.SymbolFail
		}
		fmt.Printf("  %-6s  %-32s  %s %-4s  %s", ep.Method, path, statusSymbol, status, avg)

		if ep.FailureCount > 0 {
			fmt.Printf("  (%d failed)", ep.FailureCount)
		}
		fmt.Println()

		if ep.Error != "" {
			printer.Linef("       └─ error: %s", printer.Truncate(ep.Error, 60))
		} else if ep.LastError != "" {
			printer.Linef("       └─ last error: %s", printer.Truncate(ep.LastError, 60))
		}
	}
	printer.Blank()
}

func PrintFinalSummary(meta *MetaResults, servers []ServerSummary) {
	printer.Header("BENCHMARK SUMMARY")

	duration := time.Duration(meta.Summary.TotalDurationMs) * time.Millisecond

	if meta.Summary.FailedServers > 0 {
		printer.Linef("Servers: %d total │ %s %d passed │ %s %d failed",
			meta.Summary.TotalServers,
			printer.SymbolPass, meta.Summary.SuccessfulServers,
			printer.SymbolFail, meta.Summary.FailedServers)
	} else {
		printer.Linef("Servers: %d total │ %s %d passed",
			meta.Summary.TotalServers,
			printer.SymbolPass, meta.Summary.SuccessfulServers)
	}
	printer.Linef("Duration: %s", printer.FormatDuration(duration))
	printer.Blank()

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
		printer.Linef("No successful benchmarks to rank.")
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

	printer.Linef("Rankings (by avg latency)")
	printer.Blank()

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
			memStr = printer.FormatMemoryFixed(s.mem)
		}

		rank := fmt.Sprintf("%2d", i+1)
		if i == 0 {
			rank = printer.SymbolPass + strconv.Itoa(i+1)
		}

		if hasAnyCapacity {
			capStr := "       -"
			if s.hasCapacity {
				capStr = fmt.Sprintf("%5d w", s.capWorkers)
			}
			fmt.Printf("  %s  %-16s  %s  %s  %s  %s  %s\n",
				rank,
				s.name,
				printer.FormatLatency(s.avg),
				printer.FormatLatency(s.p50),
				printer.FormatLatency(s.p95),
				memStr,
				capStr)
		} else {
			fmt.Printf("  %s  %-16s  %s  %s  %s  %s\n",
				rank,
				s.name,
				printer.FormatLatency(s.avg),
				printer.FormatLatency(s.p50),
				printer.FormatLatency(s.p95),
				memStr)
		}
	}
	printer.Blank()
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
