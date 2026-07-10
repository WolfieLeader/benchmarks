package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"benchmark-client/internal/cli"
	"benchmark-client/internal/config"
	"benchmark-client/internal/conformance"
	"benchmark-client/internal/orchestrator"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cliOpts, err := cli.ParseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, cli.ErrHelp) {
			return 0
		}
		cli.Failf("Failed to parse flags: %v", err)
		return 1
	}

	// Conformance mode runs plain HTTP against a base URL — no config, docker, or metrics.
	if cliOpts != nil && cliOpts.Conformance {
		return conformance.Run(ctx, cliOpts.BaseURL, cliOpts.ContractDir, cliOpts.TestFilesDir, cliOpts.SkipSuites, cliOpts.JWTSecret)
	}

	configFile := config.DefaultConfigFile
	if cliOpts != nil && cliOpts.ConfigFile != "" {
		configFile = cliOpts.ConfigFile
	}

	// Target mode benchmarks one externally-managed server: no roster, no
	// containers, no compose stacks, no metrics DB (calibration gate, PLAN §7.6).
	if cliOpts != nil && cliOpts.Target != "" {
		cfg, target, loadErr := config.LoadTarget(configFile, cliOpts.Target)
		if loadErr != nil {
			cli.Failf("Failed to load configuration: %v", loadErr)
			return 1
		}
		cfg.Print(1)
		if runErr := orchestrator.RunTarget(ctx, cfg, target, cliOpts.Target, resultsDir(cliOpts)); runErr != nil {
			cli.Failf("Benchmark failed: %v", runErr)
			return 1
		}
		return 0
	}

	// Roster is discovered from servers/*/bench.json relative to the repo root
	// (the client runs from benchmark/, so the repo root is one level up).
	serversDir := filepath.Join("..", "servers")
	cfg, resolvedServers, err := config.Load(configFile, serversDir)
	if err != nil {
		cli.Failf("Failed to load configuration: %v", err)
		return 1
	}

	opts, err := getRuntimeOptions(cliOpts, config.GetServerNames(resolvedServers))
	if err != nil {
		cli.Failf("Failed to get options: %v", err)
		return 1
	}

	var invalidServers []string
	resolvedServers, invalidServers = config.ApplyRuntimeOptions(resolvedServers, opts)
	if len(invalidServers) > 0 {
		cli.Warnf("Unknown servers ignored: %s", strings.Join(invalidServers, ", "))
	}
	if len(resolvedServers) == 0 {
		cli.Failf("No valid servers selected")
		return 1
	}

	cfg.Print(len(resolvedServers))

	repoRoot := ".."
	noMetrics := cliOpts != nil && cliOpts.NoMetrics
	orch := orchestrator.New(cfg, resolvedServers, repoRoot, resultsDir(cliOpts), noMetrics)

	if err := orch.Run(ctx); err != nil {
		cli.Failf("Benchmark failed: %v", err)
		return 1
	}
	return 0
}

func resultsDir(cliOpts *cli.Options) string {
	if cliOpts != nil && cliOpts.ResultsDir != "" {
		return cliOpts.ResultsDir
	}
	return filepath.Join("..", "results", time.Now().UTC().Format("20060102-150405"))
}

func getRuntimeOptions(cliOpts *cli.Options, availableServers []string) (*config.RuntimeOptions, error) {
	if cliOpts != nil {
		return &config.RuntimeOptions{
			Servers: cliOpts.Servers,
		}, nil
	}

	cli.PrintBanner()
	opts, err := cli.PromptOptions(availableServers)
	if err != nil {
		return nil, err
	}

	return &config.RuntimeOptions{
		Servers: opts.Servers,
	}, nil
}
