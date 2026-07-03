package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"benchmark-client/internal/cli"
)

const (
	// DefaultContractDir is resolved relative to the client's working directory
	// (benchmarks/), matching how config and test-files are located.
	DefaultContractDir = "../contract"
	DefaultTestFiles   = "../test-files"
	DefaultBaseURL     = "http://localhost:8080"
	requestTimeout     = 15 * time.Second
)

type failure struct {
	suite string
	name  string
	err   error
}

// Run loads the contract cases from contractDir and executes them once,
// sequentially, against baseURL with strict assertions. It prints a concise
// report and returns a process exit code (0 = all passed, 1 = any failure or
// setup error). It performs plain HTTP only — no docker, orchestrator, or metrics.
func Run(ctx context.Context, baseURL, contractDir string) int {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if contractDir == "" {
		contractDir = DefaultContractDir
	}

	suites, err := loadSuites(contractDir)
	if err != nil {
		cli.Failf("%v", err)
		return 1
	}

	cli.Header("Contract Conformance")
	cli.KeyValue("Base URL", baseURL)
	cli.KeyValue("Contract dir", contractDir)

	httpClient := &http.Client{Timeout: requestTimeout}

	var passed, failed int
	var failures []failure

	for _, suite := range suites {
		cli.Section(suite.Name)
		for i := range suite.Cases {
			c := &suite.Cases[i]
			if len(c.Flow) > 0 {
				p, f, fails := runFlow(ctx, httpClient, baseURL, suite.Name, c)
				passed += p
				failed += f
				failures = append(failures, fails...)
				continue
			}
			if err := runCase(ctx, httpClient, baseURL, nil, c); err != nil {
				failed++
				failures = append(failures, failure{suite.Name, c.Name, err})
				cli.Failf("%s", c.Name)
			} else {
				passed++
				cli.Successf("%s", c.Name)
			}
		}
	}

	printSummary(passed, failed, failures)
	if failed > 0 {
		return 1
	}
	return 0
}

// runFlow executes an ordered set of steps sharing one capture map. A failed
// step aborts the remaining steps in the flow (they depend on it).
func runFlow(ctx context.Context, hc *http.Client, baseURL, suite string, group *Case) (passed, failed int, failures []failure) {
	captured := make(map[string]string)
	cli.Linef("%s %s", cli.SymbolDot, group.Name)
	for i := range group.Flow {
		step := &group.Flow[i]
		label := group.Name + "/" + step.Name
		if err := runCase(ctx, hc, baseURL, captured, step); err != nil {
			failed++
			failures = append(failures, failure{suite, label, err})
			cli.Failf("  %s", label)
			// Abort the rest of this flow: subsequent steps depend on this one.
			for j := i + 1; j < len(group.Flow); j++ {
				failed++
				skipErr := fmt.Errorf("skipped (earlier step %q failed)", step.Name)
				failures = append(failures, failure{suite, group.Name + "/" + group.Flow[j].Name, skipErr})
			}
			return passed, failed, failures
		}
		passed++
		cli.Successf("  %s", label)
	}
	return passed, failed, failures
}

func runCase(ctx context.Context, hc *http.Client, baseURL string, captured map[string]string, c *Case) error {
	resolved := resolveCase(c, captured)

	req, err := buildRequest(ctx, baseURL, DefaultTestFiles, &resolved)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	closeErr := resp.Body.Close()
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close response: %w", closeErr)
	}

	if err := validate(&resolved.Expect, resp, body); err != nil {
		return err
	}

	if captured != nil {
		if err := capture(c.Capture, body, captured); err != nil {
			return err
		}
	}
	return nil
}

func loadSuites(dir string) ([]Suite, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read contract dir %q: %w", dir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no .json contract files found in %q", dir)
	}

	suites := make([]Suite, 0, len(files))
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // contract dir is trusted repo content
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var suite Suite
		if err := json.Unmarshal(data, &suite); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if suite.Name == "" {
			suite.Name = strings.TrimSuffix(name, ".json")
		}
		suites = append(suites, suite)
	}
	return suites, nil
}

func printSummary(passed, failed int, failures []failure) {
	cli.Section("Summary")
	if failed == 0 {
		cli.Successf("%d passed, 0 failed", passed)
		return
	}

	cli.Failf("%d passed, %d failed", passed, failed)
	cli.Blank()
	for _, f := range failures {
		cli.Linef("%s %s/%s", cli.SymbolFail, f.suite, f.name)
		cli.Linef("    %s", f.err.Error())
	}
}
