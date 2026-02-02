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
	"benchmark-client/internal/orchestrator"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, resolvedServers, err := config.Load(config.DefaultConfigFile)
	if err != nil {
		cli.Failf("Failed to load configuration: %v", err)
		return
	}

	opts, err := getRuntimeOptions(config.GetServerNames(resolvedServers))
	if err != nil {
		cli.Failf("Failed to get options: %v", err)
		return
	}

	var invalidServers []string
	resolvedServers, invalidServers = config.ApplyRuntimeOptions(cfg, resolvedServers, opts)
	if len(invalidServers) > 0 {
		cli.Warnf("Unknown servers ignored: %s", strings.Join(invalidServers, ", "))
	}
	if len(resolvedServers) == 0 {
		cli.Failf("No valid servers selected")
		return
	}

	cfg.Print()

	repoRoot := ".."
	resultsDir := filepath.Join(repoRoot, "results", time.Now().UTC().Format("20060102-150405"))
	orch := orchestrator.New(cfg, resolvedServers, repoRoot, resultsDir)

	if err := orch.Run(ctx); err != nil {
		cli.Failf("Benchmark failed: %v", err)
	}
}

func getRuntimeOptions(availableServers []string) (*config.RuntimeOptions, error) {
	cliOpts, err := cli.ParseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, cli.ErrHelp) {
			os.Exit(0)
		}
		return nil, err
	}

	if cliOpts != nil {
		return &config.RuntimeOptions{
			Warmup:    cliOpts.Warmup,
			Resources: cliOpts.Resources,
			Capacity:  cliOpts.Capacity,
			Servers:   cliOpts.Servers,
		}, nil
	}

	cli.PrintBanner()
	opts, err := cli.PromptOptions(availableServers)
	if err != nil {
		return nil, err
	}

	cli.PrintSummary(opts, len(availableServers))

	return &config.RuntimeOptions{
		Warmup:    opts.Warmup,
		Resources: opts.Resources,
		Capacity:  opts.Capacity,
		Servers:   opts.Servers,
	}, nil
}
