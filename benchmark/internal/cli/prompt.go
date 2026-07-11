package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type Options struct {
	Servers      []string // empty means all servers
	Conformance  bool     // run the contract suite instead of the benchmark
	NoMetrics    bool     // run without the metrics DB (results JSON still written)
	BaseURL      string   // base URL for conformance runs
	ContractDir  string   // contract cases directory for conformance runs
	TestFilesDir string   // upload fixtures directory for conformance runs
	SkipSuites   []string // conformance suites to load but not execute (per-server gating)
	JWTSecret    string   // shared HS256 secret backing the web suite's $jwt matcher
	Target       string   // benchmark one externally-managed server at this base URL (no containers, no metrics)
	ConfigFile   string   // config file path override (default ../config/config.json)
	ResultsDir   string   // results output directory override (default ../results/<timestamp>)
}

var bannerLines = []string{
	"██████╗ ███████╗███╗   ██╗ ██████╗██╗  ██╗",
	"██╔══██╗██╔════╝████╗  ██║██╔════╝██║  ██║",
	"██████╔╝█████╗  ██╔██╗ ██║██║     ███████║",
	"██╔══██╗██╔══╝  ██║╚██╗██║██║     ██╔══██║",
	"██████╔╝███████╗██║ ╚████║╚██████╗██║  ██║",
	"╚═════╝ ╚══════╝╚═╝  ╚═══╝ ╚═════╝╚═╝  ╚═╝",
}

var gradientStops = [][3]float64{
	{79, 70, 229},   // indigo #4F46E5
	{129, 92, 246},  // violet #8B5CF6
	{168, 85, 247},  // purple #A855F7
	{217, 70, 239},  // fuchsia #D946EF
	{236, 72, 153},  // pink #EC4899
	{251, 113, 133}, // rose #FB7185
}

func lerpColor(c1, c2 [3]float64, t float64) [3]float64 {
	return [3]float64{
		c1[0] + (c2[0]-c1[0])*t,
		c1[1] + (c2[1]-c1[1])*t,
		c1[2] + (c2[2]-c1[2])*t,
	}
}

func getGradientColor(t float64) [3]float64 {
	if t <= 0 {
		return gradientStops[0]
	}
	if t >= 1 {
		return gradientStops[len(gradientStops)-1]
	}

	// Find which segment we're in
	segments := float64(len(gradientStops) - 1)
	scaled := t * segments
	index := int(scaled)
	if index >= len(gradientStops)-1 {
		index = len(gradientStops) - 2
	}
	localT := scaled - float64(index)

	return lerpColor(gradientStops[index], gradientStops[index+1], localT)
}

func PrintBanner() {
	fmt.Println()

	height := len(bannerLines)
	width := 0
	for _, line := range bannerLines {
		if w := len([]rune(line)); w > width {
			width = w
		}
	}

	for y, line := range bannerLines {
		runes := []rune(line)
		var result strings.Builder

		for x, r := range runes {
			diagonal := (float64(x)/float64(width))*0.5 + (float64(y)/float64(height))*0.5
			color := getGradientColor(diagonal)

			style := lipgloss.NewStyle().Foreground(lipgloss.Color(
				fmt.Sprintf("#%02X%02X%02X", int(color[0]), int(color[1]), int(color[2])),
			))
			result.WriteString(style.Render(string(r)))
		}
		fmt.Println(result.String())
	}
	fmt.Println()
}

func PromptOptions(availableServers []string) (*Options, error) {
	opts := Options{}

	var serverMode string
	var selectedServers []string
	serverOptions := make([]huh.Option[string], len(availableServers))
	for i, s := range availableServers {
		serverOptions[i] = huh.NewOption(s, s)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select servers to benchmark").
				Options(
					huh.NewOption("All servers (recommended)", "all"),
					huh.NewOption("Select specific servers", "select"),
				).Value(&serverMode),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select servers").
				Description("Select the servers you want to benchmark").
				Options(serverOptions...).
				Value(&selectedServers),
		).WithHideFunc(func() bool { return serverMode != "select" }),
	).WithTheme(huh.ThemeCatppuccin()).WithKeyMap(huh.NewDefaultKeyMap())

	if err := form.Run(); err != nil {
		return nil, err
	}

	if serverMode == "select" {
		if len(selectedServers) == 0 {
			return nil, errors.New("no servers selected - please select at least one server")
		}
		opts.Servers = selectedServers
	}

	return &opts, nil
}

var ErrHelp = errors.New("help requested")

func ParseFlags(args []string) (*Options, error) {
	if len(args) == 0 {
		return nil, nil
	}

	opts := Options{}
	hasExplicitFlags := false
	var unknownFlags []string

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--servers="):
			serverList := strings.TrimPrefix(arg, "--servers=")
			parts := strings.Split(serverList, ",")
			opts.Servers = make([]string, 0, len(parts))
			for _, s := range parts {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					opts.Servers = append(opts.Servers, trimmed)
				}
			}
			hasExplicitFlags = true
		case arg == "--conformance":
			opts.Conformance = true
			hasExplicitFlags = true
		case arg == "--no-metrics":
			opts.NoMetrics = true
			hasExplicitFlags = true
		case strings.HasPrefix(arg, "--base-url="):
			opts.BaseURL = strings.TrimSpace(strings.TrimPrefix(arg, "--base-url="))
			hasExplicitFlags = true
		case strings.HasPrefix(arg, "--contract-dir="):
			opts.ContractDir = strings.TrimSpace(strings.TrimPrefix(arg, "--contract-dir="))
			hasExplicitFlags = true
		case strings.HasPrefix(arg, "--test-files-dir="):
			opts.TestFilesDir = strings.TrimSpace(strings.TrimPrefix(arg, "--test-files-dir="))
			hasExplicitFlags = true
		case strings.HasPrefix(arg, "--skip-suite="):
			for s := range strings.SplitSeq(strings.TrimPrefix(arg, "--skip-suite="), ",") {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					opts.SkipSuites = append(opts.SkipSuites, trimmed)
				}
			}
			hasExplicitFlags = true
		case strings.HasPrefix(arg, "--jwt-secret="):
			opts.JWTSecret = strings.TrimSpace(strings.TrimPrefix(arg, "--jwt-secret="))
			hasExplicitFlags = true
		case strings.HasPrefix(arg, "--target="):
			opts.Target = strings.TrimSpace(strings.TrimPrefix(arg, "--target="))
			if opts.Target == "" {
				return nil, errors.New("--target requires a URL")
			}
			hasExplicitFlags = true
		case strings.HasPrefix(arg, "--config="):
			opts.ConfigFile = strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
			hasExplicitFlags = true
		case strings.HasPrefix(arg, "--results-dir="):
			opts.ResultsDir = strings.TrimSpace(strings.TrimPrefix(arg, "--results-dir="))
			hasExplicitFlags = true
		case arg == "--help" || arg == "-h":
			printHelp()
			return nil, ErrHelp
		case strings.HasPrefix(arg, "-"):
			unknownFlags = append(unknownFlags, arg)
		}
	}

	if len(unknownFlags) > 0 {
		return nil, fmt.Errorf("unknown flags: %s", strings.Join(unknownFlags, ", "))
	}

	if opts.Target != "" {
		if opts.Conformance || len(opts.Servers) > 0 {
			return nil, errors.New("--target cannot be combined with --servers or --conformance")
		}
		if !strings.HasPrefix(opts.Target, "http://") && !strings.HasPrefix(opts.Target, "https://") {
			return nil, fmt.Errorf("--target must be an http(s) URL, got %q", opts.Target)
		}
	}

	if !hasExplicitFlags {
		return nil, nil
	}

	return &opts, nil
}

func printHelp() {
	fmt.Println(`Usage: benchmark [options]

Options:
  --servers=a,b,c    Only benchmark specific servers (comma-separated)
  --conformance      Run the contract conformance suite instead of the benchmark
  --no-metrics       Run without the metrics DB (results JSON still written)
  --base-url=URL     Base URL for --conformance (default http://localhost:8080)
  --contract-dir=DIR Contract cases directory for --conformance (default ../contract)
  --test-files-dir=DIR Upload fixtures directory for --conformance (default ../contract/test-files)
  --skip-suite=a,b   Contract suites to load but not run (per-server gating, e.g. web)
  --jwt-secret=SECRET Shared HS256 secret for the web suite's $jwt matcher (default dev secret)
  --target=URL       Benchmark one externally-managed server at URL (no containers, no metrics DB)
  --config=PATH      Config file override (default ../config/config.json)
  --results-dir=DIR  Results output directory override (default ../results/<timestamp>)
  --help, -h         Show this help message

Interactive mode:
  Run without flags to use interactive selection.

Examples:
  benchmark                                            # Interactive mode
  benchmark --servers=go-chi,go-gin                    # Benchmark specific servers
  benchmark --conformance --base-url=http://localhost:8080  # Run the contract gate
  benchmark --target=http://localhost:8080 --config=../config/calibration.json  # External target`)
}
