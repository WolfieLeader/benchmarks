package conformance

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"benchmark-client/internal/cli"
)

const (
	// DefaultContractDir is resolved relative to the client's working directory
	// (benchmark/), matching how config and test-files are located.
	DefaultContractDir = "../contract"
	DefaultTestFiles   = "../contract/test-files"
	DefaultBaseURL     = "http://localhost:8080"
	requestTimeout     = 15 * time.Second

	// DefaultJWTSecret backs the web suite's $jwt matcher when --jwt-secret is
	// not passed. It MUST equal the shared env modules' JWT_SECRET dev default
	// (shared/{typescript,python,rust,go}) so a raw `--conformance` run agrees
	// with a server started on defaults. The scripts/contract.mts harness passes
	// this same value to both the container (JWT_SECRET) and the runner.
	DefaultJWTSecret = "benchmarks-shared-jwt-secret-dev-default"
)

type failure struct {
	suite string
	name  string
	err   error
}

// Run loads the contract cases from contractDir and executes them once,
// sequentially, against baseURL with strict assertions. Suites whose name is in
// skipSuites are loaded (and structurally validated) but not executed — this is
// how per-server suite gating works: the harness passes --skip-suite=web for
// servers whose manifest does not declare web support. jwtSecret (default when
// empty) backs the $jwt matcher. It prints a concise report and returns a
// process exit code (0 = all passed, 1 = any failure or setup error). It
// performs plain HTTP only — no docker, orchestrator, or metrics.
func Run(ctx context.Context, baseURL, contractDir, testFilesDir string, skipSuites []string, jwtSecret string) int {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if contractDir == "" {
		contractDir = DefaultContractDir
	}
	if testFilesDir == "" {
		testFilesDir = DefaultTestFiles
	}
	if jwtSecret == "" {
		jwtSecret = DefaultJWTSecret
	}

	suites, err := loadSuites(contractDir)
	if err != nil {
		cli.Failf("%v", err)
		return 1
	}

	cli.Header("Contract Conformance")
	cli.KeyValue("Base URL", baseURL)
	cli.KeyValue("Contract dir", contractDir)
	if len(skipSuites) > 0 {
		cli.KeyValue("Skipped suites", strings.Join(skipSuites, ", "))
	}

	httpClient := &http.Client{Timeout: requestTimeout}
	passed, failed, failures := runSuites(ctx, httpClient, baseURL, testFilesDir, suites, skipSuites, []byte(jwtSecret))

	// Guard against a vacuous green: zero executed cases is a setup error
	// (empty suites, wrong --contract-dir, schema drift, or every suite skipped),
	// never a pass.
	if passed+failed == 0 {
		cli.Failf("no contract cases were executed — check --contract-dir, --skip-suite, and suite contents")
		return 1
	}

	printSummary(passed, failed, failures)
	if failed > 0 {
		return 1
	}
	return 0
}

// runSuites executes every non-skipped suite's cases in order and tallies the
// results. Kept separate from Run so tests can drive a single suite against an
// in-process stub (web_test.go) without the CLI/exit-code shell.
func runSuites(
	ctx context.Context, hc *http.Client, baseURL, testFilesDir string,
	suites []Suite, skipSuites []string, jwtSecret []byte,
) (passed, failed int, failures []failure) {
	for _, suite := range suites {
		if slices.Contains(skipSuites, suite.Name) {
			continue
		}
		cli.Section(suite.Name)
		for i := range suite.Cases {
			c := &suite.Cases[i]
			if len(c.Flow) > 0 {
				p, f, fails := runFlow(ctx, hc, baseURL, testFilesDir, suite.Name, c, jwtSecret)
				passed += p
				failed += f
				failures = append(failures, fails...)
				continue
			}
			if err := runCase(ctx, hc, baseURL, testFilesDir, nil, c, jwtSecret); err != nil {
				failed++
				failures = append(failures, failure{suite.Name, c.Name, err})
				cli.Failf("%s", c.Name)
			} else {
				passed++
				cli.Successf("%s", c.Name)
			}
		}
	}
	return passed, failed, failures
}

// runFlow executes an ordered set of steps sharing one capture map. A failed
// step aborts the remaining steps in the flow (they depend on it).
func runFlow(ctx context.Context, hc *http.Client, baseURL, testFilesDir, suite string, group *Case, jwtSecret []byte) (passed, failed int, failures []failure) {
	captured := make(map[string]string)
	cli.Linef("%s %s", cli.SymbolDot, group.Name)
	for i := range group.Flow {
		step := &group.Flow[i]
		label := group.Name + "/" + step.Name
		if err := runCase(ctx, hc, baseURL, testFilesDir, captured, step, jwtSecret); err != nil {
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

func runCase(ctx context.Context, hc *http.Client, baseURL, testFilesDir string, captured map[string]string, c *Case, jwtSecret []byte) error {
	resolved := resolveCase(c, captured)

	req, err := buildRequest(ctx, baseURL, testFilesDir, &resolved)
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

	if err := validate(&resolved.Expect, resp, body, jwtSecret); err != nil {
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
		if err := validateSuite(name, &suite); err != nil {
			return nil, err
		}
		suites = append(suites, suite)
	}
	return suites, nil
}

func validateSuite(file string, suite *Suite) error {
	if len(suite.Cases) == 0 {
		return fmt.Errorf("%s: suite contains zero cases", file)
	}
	for i := range suite.Cases {
		c := &suite.Cases[i]
		if len(c.Flow) == 0 && len(c.Capture) > 0 {
			return fmt.Errorf("%s: case %q uses capture outside a flow — captures only carry across flow steps", file, c.Name)
		}
		if err := validateExpect(file, c.Name, &c.Expect); err != nil {
			return err
		}
		for j := range c.Flow {
			step := &c.Flow[j]
			if err := validateExpect(file, c.Name+"/"+step.Name, &step.Expect); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateExpect(file, name string, exp *Expect) error {
	if len(exp.StatusAnyOf) > 0 && (exp.Body != nil || exp.Text != nil) {
		return fmt.Errorf("%s: case %q combines statusAnyOf with a body/text assertion — bodies differ per status, assert one or the other", file, name)
	}
	// text, htmlContains, and body are mutually exclusive body-assertion modes;
	// validate (validate.go) checks them in that order and would silently ignore
	// the later ones — an authoring error must fail loud at load time instead.
	modes := 0
	for _, set := range []bool{exp.Text != nil, len(exp.HTMLContains) > 0, exp.Body != nil} {
		if set {
			modes++
		}
	}
	if modes > 1 {
		return fmt.Errorf("%s: case %q sets more than one of text/htmlContains/body — they are mutually exclusive body-assertion modes", file, name)
	}
	return nil
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
