package summary

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
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

	memStr := "n/a"
	cpuStr := "n/a"
	if result.Resources != nil && result.Resources.Samples > 0 {
		memStr = cli.FormatMemory(result.Resources.Memory.AvgBytes)
		cpuStr = cli.FormatCpu(result.Resources.Cpu.AvgPercent, result.Resources.Samples)
	}
	cli.Linef("Duration: %s  Memory: %s  CPU: %s", cli.FormatDuration(result.Duration), memStr, cpuStr)
	cli.Blank()

	var endpointIdx, seqStepIdx []int
	for i := range result.Results {
		if result.Results[i].Database != "" {
			seqStepIdx = append(seqStepIdx, i)
		} else {
			endpointIdx = append(endpointIdx, i)
		}
	}

	cli.Linef("Endpoints")
	fmt.Println("  ───────────────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("  %-6s  %-27s  %8s  %8s  %8s  %8s  %5s  %s\n",
		"Method", "Path", "Reqs", "Avg", "P50", "P95", "Rate", "Status")

	var totalReqs, totalSuccesses int
	for _, i := range endpointIdx {
		totalReqs, totalSuccesses = printResultRow(&result.Results[i], totalReqs, totalSuccesses)
	}

	if len(seqStepIdx) > 0 {
		cli.Blank()
		cli.Linef("Sequence Steps")
		fmt.Println("  ───────────────────────────────────────────────────────────────────────────────────────")
		fmt.Printf("  %-6s  %-27s  %8s  %8s  %8s  %8s  %5s  %s\n",
			"Method", "Path", "Reqs", "Avg", "P50", "P95", "Rate", "Status")

		for _, i := range seqStepIdx {
			totalReqs, totalSuccesses = printResultRow(&result.Results[i], totalReqs, totalSuccesses)
		}
	}

	var totalSeqRuns, totalSeqSuccesses int
	if len(result.Sequences) > 0 {
		cli.Blank()
		cli.Linef("Sequences")
		fmt.Println("  ───────────────────────────────────────────────────────────────────────────────────────")
		fmt.Printf("  %-18s  %8s  %10s  %10s  %10s  %5s  %s\n",
			"Name", "Runs", "Avg", "P50", "P95", "Rate", "Status")

		for i := range result.Sequences {
			seq := &result.Sequences[i]
			seqName := seq.SequenceId
			if seq.Database != "" {
				seqName = fmt.Sprintf("%s/%s", seq.SequenceId, seq.Database)
			}

			totalSeqRuns += seq.TotalRuns
			totalSeqSuccesses += seq.Successes

			statusSymbol := cli.SymbolPass
			status := "OK"
			if seq.SuccessRate < 1.0 {
				statusSymbol = cli.SymbolFail
				status = fmt.Sprintf("FAIL (%d)", seq.Failures)
			}

			fmt.Printf("  %-18s  %8s  %10s  %10s  %10s  %5s  %s %s\n",
				cli.Truncate(seqName, 18),
				cli.FormatReqs(seq.TotalRuns),
				cli.FormatLatency(seq.AvgDuration),
				cli.FormatLatency(seq.P50Duration),
				cli.FormatLatency(seq.P95Duration),
				cli.FormatRate(seq.SuccessRate),
				statusSymbol, status)

			for j := range seq.Steps {
				step := &seq.Steps[j]
				fmt.Printf("    %-6s %-27s  %10s\n",
					step.Method,
					cli.TruncatePath(step.Path, 27),
					cli.FormatLatency(step.Avg))
			}

			if seq.LastError != "" {
				fmt.Printf("    └─ last (step %d): %s\n", seq.FailedStep, cli.Truncate(seq.LastError, 65))
			}
		}
	}

	cli.Blank()
	fmt.Println("  ───────────────────────────────────────────────────────────────────────────────────────")

	var successRate float64
	if totalReqs > 0 {
		successRate = float64(totalSuccesses) / float64(totalReqs)
	}

	if len(result.Sequences) > 0 {
		fmt.Printf("  Total: %s reqs │ %s seqs │ Success: %s\n",
			cli.FormatReqs(totalReqs),
			cli.FormatReqs(totalSeqRuns),
			cli.FormatRate(successRate))
	} else {
		fmt.Printf("  Total: %s reqs │ Success: %s\n",
			cli.FormatReqs(totalReqs),
			cli.FormatRate(successRate))
	}
	cli.Blank()
}

func printResultRow(ep *client.EndpointResult, totalReqs, totalSuccesses int) (updatedReqs, updatedSuccesses int) {
	path := ep.Path
	if ep.Database != "" {
		path = fmt.Sprintf("[%s] %s", ep.Database, ep.Path)
	}
	path = cli.TruncatePath(path, 27)
	reqs := "-"
	avg := "-"
	p50 := "-"
	p95 := "-"
	rate := "-"
	status := "OK"
	statusSymbol := cli.SymbolPass

	if ep.Error != "" {
		status = "FAIL"
		statusSymbol = cli.SymbolFail
	} else if ep.Stats != nil {
		totalCount := ep.Stats.Count + ep.FailureCount
		totalReqs += totalCount
		totalSuccesses += ep.Stats.Count
		reqs = cli.FormatReqs(totalCount)
		avg = cli.FormatLatency(ep.Stats.Avg)
		p50 = cli.FormatLatency(ep.Stats.P50)
		p95 = cli.FormatLatency(ep.Stats.P95)
		rate = cli.FormatRate(ep.Stats.SuccessRate)
		if ep.Stats.SuccessRate < 1.0 {
			statusSymbol = cli.SymbolFail
			if ep.FailureCount > 0 {
				status = fmt.Sprintf("FAIL (%d)", ep.FailureCount)
			} else {
				status = "FAIL"
			}
		}
	}

	fmt.Printf("  %-6s  %-27s  %8s  %8s  %8s  %8s  %5s  %s %s\n",
		ep.Method, path, reqs, avg, p50, p95, rate, statusSymbol, status)

	if ep.Error != "" {
		fmt.Printf("    └─ %s\n", cli.Truncate(ep.Error, 75))
	} else if ep.LastError != "" {
		fmt.Printf("    └─ last: %s\n", cli.Truncate(ep.LastError, 70))
	}

	return totalReqs, totalSuccesses
}

func PrintFinalSummary(meta *MetaResults, servers []ServerSummary) {
	cli.Header("BENCHMARK SUMMARY")

	duration := time.Duration(meta.Summary.TotalDurationMs) * time.Millisecond

	cli.Linef("Config")
	fmt.Println("  ───────────────────────────────────────────────────────────────────────────────────────")
	cli.Linef("Base: %s  Concurrency: %d  Duration: %s  Timeout: %s",
		meta.Meta.Config.BaseUrl,
		meta.Meta.Config.Concurrency,
		meta.Meta.Config.DurationPerEndpoint,
		meta.Meta.Config.RequestTimeout)
	cli.Blank()

	type rankedServer struct {
		name        string
		avg         int64
		min         int64
		max         int64
		mem         float64
		cpu         float64
		hasMem      bool
		totalReqs   int
		successRate float64
		failed      bool
	}

	var ranked []rankedServer
	var totalReqs int
	var issues []struct {
		server    string
		endpoint  string
		failures  int
		lastError string
	}

	for i := range servers {
		s := &servers[i]
		if s.Error != "" {
			ranked = append(ranked, rankedServer{name: s.Name, failed: true})
			continue
		}
		if s.Stats == nil {
			continue
		}

		rs := rankedServer{
			name:        s.Name,
			avg:         s.Stats.AvgNs,
			min:         s.Stats.MinNs,
			max:         s.Stats.MaxNs,
			totalReqs:   s.Stats.TotalCount,
			successRate: s.Stats.SuccessRate,
		}
		totalReqs += s.Stats.TotalCount

		if s.Resources != nil && s.Resources.Samples >= 1 {
			rs.mem = s.Resources.Memory.AvgBytes
			rs.cpu = s.Resources.Cpu.AvgPercent
			rs.hasMem = true
		}
		ranked = append(ranked, rs)

		// Collect issues
		for j := range s.Results {
			ep := &s.Results[j]
			if ep.FailureCount > 0 || ep.Error != "" {
				errMsg := ep.LastError
				if ep.Error != "" {
					errMsg = ep.Error
				}
				issues = append(issues, struct {
					server    string
					endpoint  string
					failures  int
					lastError string
				}{
					server:    s.Name,
					endpoint:  fmt.Sprintf("%s %s", ep.Method, ep.Path),
					failures:  ep.FailureCount,
					lastError: errMsg,
				})
			}
		}
	}

	if len(ranked) == 0 {
		cli.Linef("No benchmarks to display.")
		return
	}

	slices.SortFunc(ranked, func(a, b rankedServer) int {
		if a.failed != b.failed {
			if a.failed {
				return 1
			}
			return -1
		}
		return cmp.Compare(a.avg, b.avg)
	})

	cli.Linef("Server Rankings (by avg latency, all requests)")
	fmt.Println("  ───────────────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("  %2s  %-10s  %8s  %8s  %8s  %6s  %5s  %9s  %5s  %s\n",
		"#", "Server", "Avg", "Min", "Max", "Mem", "CPU", "Reqs", "Rate", "Status")

	for i, s := range ranked {
		rank := fmt.Sprintf("%2d", i+1)

		if s.failed {
			fmt.Printf("  %s  %-10s  %8s  %8s  %8s  %6s  %5s  %9s  %5s  %s FAIL\n",
				rank, s.name, "-", "-", "-", "-", "-", "-", "-", cli.SymbolFail)
			continue
		}

		memStr := "-"
		cpuStr := "-"
		if s.hasMem {
			memStr = cli.FormatMemory(s.mem)
			cpuStr = fmt.Sprintf("%.0f%%", s.cpu)
		}

		status := cli.SymbolPass + " OK"
		if s.successRate < 1.0 {
			status = cli.SymbolFail + " FAIL"
		}

		fmt.Printf("  %s  %-10s  %8s  %8s  %8s  %6s  %5s  %9s  %5s  %s\n",
			rank, s.name,
			cli.FormatLatency(s.avg),
			cli.FormatLatency(s.min),
			cli.FormatLatency(s.max),
			memStr, cpuStr,
			cli.FormatReqs(s.totalReqs),
			cli.FormatRate(s.successRate),
			status)
	}
	cli.Blank()

	printSequenceRankings(servers)

	if len(issues) > 0 {
		cli.Linef("Issues")
		fmt.Println("  ───────────────────────────────────────────────────────────────────────────────────────")
		for _, issue := range issues {
			fmt.Printf("  %-10s  %-30s  %d failed  last: %s\n",
				issue.server,
				cli.Truncate(issue.endpoint, 30),
				issue.failures,
				cli.Truncate(issue.lastError, 35))
		}
		cli.Blank()
	}

	fmt.Println("  ───────────────────────────────────────────────────────────────────────────────────────")
	statusStr := fmt.Sprintf("%s %d passed", cli.SymbolPass, meta.Summary.SuccessfulServers)
	if meta.Summary.FailedServers > 0 {
		statusStr += fmt.Sprintf("  %s %d failed", cli.SymbolFail, meta.Summary.FailedServers)
	}
	fmt.Printf("  %d servers │ %s │ %s │ Total: %s reqs\n",
		meta.Summary.TotalServers,
		cli.FormatDuration(duration),
		statusStr,
		cli.FormatReqs(totalReqs))
	cli.Linef("Results: %s", meta.Meta.Timestamp.Format("results/20060102-150405/"))
	cli.Blank()

	fmt.Printf("# servers=%d passed=%d failed=%d duration_ms=%d total_reqs=%d\n",
		meta.Summary.TotalServers,
		meta.Summary.SuccessfulServers,
		meta.Summary.FailedServers,
		meta.Summary.TotalDurationMs,
		totalReqs)
}

type seqRankingData struct {
	name        string
	dbDurations map[string]time.Duration
	avgDuration time.Duration
	successRate float64
	failed      bool
}

func printSequenceRankings(servers []ServerSummary) {
	seqIds, dbList := collectSequenceInfo(servers)
	if len(seqIds) == 0 {
		return
	}

	for _, seqId := range seqIds {
		serverData := collectServerSeqData(servers, seqId)
		if len(serverData) == 0 {
			continue
		}
		sortSeqRankingData(serverData)
		printSeqRankingTable(seqId, dbList, serverData)
	}
}

func collectSequenceInfo(servers []ServerSummary) (seqList, dbList []string) {
	seqIds := make(map[string]struct{})
	databases := make(map[string]struct{})
	for i := range servers {
		s := &servers[i]
		if s.Error != "" {
			continue
		}
		for j := range s.Sequences {
			seq := &s.Sequences[j]
			seqIds[seq.SequenceId] = struct{}{}
			if seq.Database != "" {
				databases[seq.Database] = struct{}{}
			}
		}
	}

	return slices.Sorted(maps.Keys(seqIds)), slices.Sorted(maps.Keys(databases))
}

func collectServerSeqData(servers []ServerSummary, seqId string) []seqRankingData {
	var result []seqRankingData
	for i := range servers {
		s := &servers[i]
		if s.Error != "" {
			continue
		}

		data := seqRankingData{
			name:        s.Name,
			dbDurations: make(map[string]time.Duration),
			successRate: 1.0,
		}

		var totalDuration time.Duration
		var count int
		for j := range s.Sequences {
			seq := &s.Sequences[j]
			if seq.SequenceId != seqId {
				continue
			}
			if seq.Database != "" {
				data.dbDurations[seq.Database] = seq.AvgDuration
			}
			totalDuration += seq.AvgDuration
			count++
			if seq.SuccessRate < data.successRate {
				data.successRate = seq.SuccessRate
			}
			if seq.SuccessRate < 1.0 {
				data.failed = true
			}
		}

		if count > 0 {
			data.avgDuration = totalDuration / time.Duration(count)
			result = append(result, data)
		}
	}
	return result
}

func sortSeqRankingData(data []seqRankingData) {
	slices.SortFunc(data, func(a, b seqRankingData) int {
		if a.failed != b.failed {
			if a.failed {
				return 1
			}
			return -1
		}
		return cmp.Compare(a.avgDuration, b.avgDuration)
	})
}

func printSeqRankingTable(seqId string, dbList []string, serverData []seqRankingData) {
	cli.Linef("Sequence Rankings (%s - by avg duration)", seqId)
	fmt.Println("  ───────────────────────────────────────────────────────────────────────────────────────")

	printSeqRankingHeader(dbList)

	for i, data := range serverData {
		printSeqRankingRow(i+1, dbList, data)
	}
	cli.Blank()
}

func printSeqRankingHeader(dbList []string) {
	if len(dbList) == 0 {
		fmt.Printf("  %2s  %-10s  %10s  %5s\n", "#", "Server", "Avg", "Rate")
		return
	}

	fmt.Printf("  %2s  %-10s", "#", "Server")
	for _, db := range dbList {
		fmt.Printf("  %10s", db)
	}
	fmt.Printf("  %10s  %5s\n", "avg", "Rate")
}

func printSeqRankingRow(rank int, dbList []string, data seqRankingData) {
	if len(dbList) == 0 {
		fmt.Printf("  %2d  %-10s  %10s  %5s\n",
			rank, data.name,
			cli.FormatLatency(data.avgDuration),
			cli.FormatRate(data.successRate))
		return
	}

	fmt.Printf("  %2d  %-10s", rank, data.name)
	for _, db := range dbList {
		dur, ok := data.dbDurations[db]
		if ok {
			fmt.Printf("  %10s", cli.FormatLatency(dur))
		} else {
			fmt.Printf("  %10s", "-")
		}
	}
	fmt.Printf("  %10s  %5s\n", cli.FormatLatency(data.avgDuration), cli.FormatRate(data.successRate))
}
