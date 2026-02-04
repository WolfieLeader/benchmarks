package summary

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"benchmark-client/internal/cli"
	"benchmark-client/internal/client"
)

func PrintServerSummary(result *ServerResult) {
	if result.Error != "" {
		cli.Failf("Status: FAILED")
		cli.Linef("Error: %s", result.Error)
		cli.Blank()
		return
	}

	cli.KeyValuePairs(
		"Duration", cli.FormatDuration(result.Duration),
		"Endpoints", strconv.Itoa(len(result.Endpoints)),
	)

	if result.Resources != nil && result.Resources.Samples > 0 {
		memStr := cli.FormatMemory(result.Resources.Memory.AvgBytes)
		cpuStr := cli.FormatCpu(result.Resources.Cpu.AvgPercent, result.Resources.Samples)
		warning := ""
		if len(result.Resources.Warnings) > 0 {
			warning = fmt.Sprintf(" (%s)", result.Resources.Warnings[0])
		}
		cli.KeyValuePairs("Memory", memStr, "CPU", cpuStr+warning)
	}

	cli.Blank()

	fmt.Printf("  %-6s  %-32s  %-6s  %s\n", "Method", "Path", "Status", "Avg")
	fmt.Printf("  %-6s  %-32s  %-6s  %s\n", "──────", "────────────────────────────────", "──────", "───────")

	for _, ep := range result.Endpoints {
		status := formatStatus(ep.Error, ep.Stats)
		avg := "      -"
		if ep.Stats != nil {
			avg = cli.FormatLatency(ep.Stats.Avg)
		}

		path := cli.TruncatePath(ep.Path, 32)
		statusSymbol := cli.SymbolPass
		if ep.Error != "" || (ep.Stats != nil && ep.Stats.SuccessRate < 1.0) {
			statusSymbol = cli.SymbolFail
		}
		fmt.Printf("  %-6s  %-32s  %s %-4s  %s", ep.Method, path, statusSymbol, status, avg)

		if ep.FailureCount > 0 {
			fmt.Printf("  (%d failed)", ep.FailureCount)
		}
		fmt.Println()

		if ep.Error != "" {
			cli.Linef("       └─ error: %s", cli.Truncate(ep.Error, 60))
		} else if ep.LastError != "" {
			cli.Linef("       └─ last error: %s", cli.Truncate(ep.LastError, 60))
		}
	}
	cli.Blank()

	// Print sequence results if any
	if len(result.Sequences) > 0 {
		cli.Blank()
		cli.Linef("Sequences")
		fmt.Printf("  %-6s  %-32s  %-6s  %s\n", "Method", "Path", "Status", "Avg")
		fmt.Printf("  %-6s  %-32s  %-6s  %s\n", "──────", "────────────────────────────────", "──────", "───────")

		for i := range result.Sequences {
			seq := &result.Sequences[i]
			seqName := seq.SequenceId
			if seq.Database != "" {
				seqName = fmt.Sprintf("%s/%s", seq.SequenceId, seq.Database)
			}
			statusSymbol := cli.SymbolPass
			status := "OK"
			if seq.SuccessRate < 1.0 {
				statusSymbol = cli.SymbolFail
				status = fmt.Sprintf("%.0f%%", seq.SuccessRate*100)
			}
			fmt.Printf("  %-6s  %-32s  %s %-4s  %s\n",
				"SEQ",
				cli.TruncatePath(seqName, 32),
				statusSymbol,
				status,
				cli.FormatLatency(seq.AvgDuration))

			// Print per-step stats
			for j := range seq.Steps {
				step := &seq.Steps[j]
				path := cli.TruncatePath(step.Path, 26)
				fmt.Printf("    %-6s  %-26s          %s\n",
					step.Method,
					path,
					cli.FormatLatency(step.Avg))
			}

			if seq.LastError != "" {
				cli.Linef("       └─ last error (step %d): %s", seq.FailedStep, cli.Truncate(seq.LastError, 50))
			}
		}
		cli.Blank()
	}
}

func PrintFinalSummary(meta *MetaResults, servers []ServerSummary) {
	cli.Header("BENCHMARK SUMMARY")

	duration := time.Duration(meta.Summary.TotalDurationMs) * time.Millisecond

	if meta.Summary.FailedServers > 0 {
		cli.Linef("Servers: %d total │ %s %d passed │ %s %d failed",
			meta.Summary.TotalServers,
			cli.SymbolPass, meta.Summary.SuccessfulServers,
			cli.SymbolFail, meta.Summary.FailedServers)
	} else {
		cli.Linef("Servers: %d total │ %s %d passed",
			meta.Summary.TotalServers,
			cli.SymbolPass, meta.Summary.SuccessfulServers)
	}
	cli.Linef("Duration: %s", cli.FormatDuration(duration))
	cli.Blank()

	type rankedServer struct {
		name   string
		avg    int64
		mem    float64
		hasMem bool
	}

	ranked := make([]rankedServer, 0)
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
			ranked = append(ranked, rs)
		}
	}

	if len(ranked) == 0 {
		cli.Linef("No successful benchmarks to rank.")
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

	cli.Linef("Rankings (by avg latency)")
	cli.Blank()

	fmt.Println("   #  Server              Avg      Mem")
	fmt.Println("  ──  ────────────────  ───────  ───────")

	for i, s := range ranked {
		memStr := "      -"
		if s.hasMem {
			memStr = cli.FormatMemoryFixed(s.mem)
		}

		rank := fmt.Sprintf("%2d", i+1)
		fmt.Printf("  %s  %-16s  %s  %s\n",
			rank,
			s.name,
			cli.FormatLatency(s.avg),
			memStr)
	}
	cli.Blank()

	// Print sequence rankings by server+database combination
	printSequenceRankings(servers)
}

func printSequenceRankings(servers []ServerSummary) {
	type rankedSequence struct {
		name        string
		sequenceId  string
		avgDuration int64
		successRate float64
	}

	sequences := make([]rankedSequence, 0)
	for i := range servers {
		s := &servers[i]
		if s.Error != "" || len(s.Sequences) == 0 {
			continue
		}
		for j := range s.Sequences {
			seq := &s.Sequences[j]
			name := s.Name
			if seq.Database != "" {
				name = fmt.Sprintf("%s-%s", s.Name, seq.Database)
			}
			sequences = append(sequences, rankedSequence{
				name:        name,
				sequenceId:  seq.SequenceId,
				avgDuration: seq.AvgDuration.Nanoseconds(),
				successRate: seq.SuccessRate,
			})
		}
	}

	if len(sequences) == 0 {
		return
	}

	// Sort by success rate (failed sequences last), then by avg duration
	slices.SortFunc(sequences, func(a, b rankedSequence) int {
		// Failed sequences (0% success) go to the bottom
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

	cli.Linef("Sequence Rankings (by avg duration)")
	cli.Blank()

	fmt.Println("   #  Server+Database          Seq       Avg     Success")
	fmt.Println("  ──  ──────────────────────  ──────  ───────  ─────────")

	for i, seq := range sequences {
		rank := fmt.Sprintf("%2d", i+1)
		successStr := "100%"
		if seq.successRate < 1.0 {
			successStr = fmt.Sprintf("%.0f%%", seq.successRate*100)
		}
		fmt.Printf("  %s  %-22s  %-6s  %s  %7s\n",
			rank,
			cli.Truncate(seq.name, 22),
			seq.sequenceId,
			cli.FormatLatency(seq.avgDuration),
			successStr)
	}
	cli.Blank()
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
