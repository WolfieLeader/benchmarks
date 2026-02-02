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

	// Print flow results if any
	if len(result.Flows) > 0 {
		printer.Blank()
		printer.Linef("Flows")
		fmt.Printf("  %-6s  %-32s  %-6s  %s\n", "Method", "Path", "Status", "Avg")
		fmt.Printf("  %-6s  %-32s  %-6s  %s\n", "──────", "────────────────────────────────", "──────", "───────")

		for i := range result.Flows {
			flow := &result.Flows[i]
			flowName := flow.FlowId
			if flow.Database != "" {
				flowName = fmt.Sprintf("%s/%s", flow.FlowId, flow.Database)
			}
			statusSymbol := printer.SymbolPass
			status := "OK"
			if flow.SuccessRate < 1.0 {
				statusSymbol = printer.SymbolFail
				status = fmt.Sprintf("%.0f%%", flow.SuccessRate*100)
			}
			fmt.Printf("  %-6s  %-32s  %s %-4s  %s\n",
				"FLOW",
				printer.TruncatePath(flowName, 32),
				statusSymbol,
				status,
				printer.FormatLatency(flow.AvgDuration))

			// Print per-step stats
			for j := range flow.Steps {
				step := &flow.Steps[j]
				path := printer.TruncatePath(step.Path, 26)
				fmt.Printf("    %-6s  %-26s          %s\n",
					step.Method,
					path,
					printer.FormatLatency(step.Avg))
			}

			if flow.LastError != "" {
				printer.Linef("       └─ last error (step %d): %s", flow.FailedStep, printer.Truncate(flow.LastError, 50))
			}
		}
		printer.Blank()
	}
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
		fmt.Println("   #  Server              Avg      Mem  Capacity")
		fmt.Println("  ──  ────────────────  ───────  ───────  ────────")
	} else {
		fmt.Println("   #  Server              Avg      Mem")
		fmt.Println("  ──  ────────────────  ───────  ───────")
	}

	for i, s := range ranked {
		memStr := "      -"
		if s.hasMem {
			memStr = printer.FormatMemoryFixed(s.mem)
		}

		rank := fmt.Sprintf("%2d", i+1)

		if hasAnyCapacity {
			capStr := "       -"
			if s.hasCapacity {
				capStr = fmt.Sprintf("%5d w", s.capWorkers)
			}
			fmt.Printf("  %s  %-16s  %s  %s  %s\n",
				rank,
				s.name,
				printer.FormatLatency(s.avg),
				memStr,
				capStr)
		} else {
			fmt.Printf("  %s  %-16s  %s  %s\n",
				rank,
				s.name,
				printer.FormatLatency(s.avg),
				memStr)
		}
	}
	printer.Blank()

	// Print flow rankings by server+database combination
	printFlowRankings(servers)
}

func printFlowRankings(servers []ServerSummary) {
	type rankedFlow struct {
		name        string
		flowId      string
		avgDuration int64
		successRate float64
	}

	flows := make([]rankedFlow, 0)
	for i := range servers {
		s := &servers[i]
		if s.Error != "" || len(s.Flows) == 0 {
			continue
		}
		for j := range s.Flows {
			f := &s.Flows[j]
			name := s.Name
			if f.Database != "" {
				name = fmt.Sprintf("%s-%s", s.Name, f.Database)
			}
			flows = append(flows, rankedFlow{
				name:        name,
				flowId:      f.FlowId,
				avgDuration: f.AvgDuration.Nanoseconds(),
				successRate: f.SuccessRate,
			})
		}
	}

	if len(flows) == 0 {
		return
	}

	// Sort by success rate (failed flows last), then by avg duration
	slices.SortFunc(flows, func(a, b rankedFlow) int {
		// Failed flows (0% success) go to the bottom
		aFailed := a.successRate == 0
		bFailed := b.successRate == 0
		if aFailed != bFailed {
			if aFailed {
				return 1
			}
			return -1
		}
		// Sort by avg duration
		if a.avgDuration < b.avgDuration {
			return -1
		}
		if a.avgDuration > b.avgDuration {
			return 1
		}
		return 0
	})

	printer.Linef("Flow Rankings (by avg duration)")
	printer.Blank()

	fmt.Println("   #  Server+Database          Flow      Avg     Success")
	fmt.Println("  ──  ──────────────────────  ──────  ───────  ─────────")

	for i, f := range flows {
		rank := fmt.Sprintf("%2d", i+1)
		successStr := "100%"
		if f.successRate < 1.0 {
			successStr = fmt.Sprintf("%.0f%%", f.successRate*100)
		}
		fmt.Printf("  %s  %-22s  %-6s  %s  %7s\n",
			rank,
			printer.Truncate(f.name, 22),
			f.flowId,
			printer.FormatLatency(f.avgDuration),
			successStr)
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
