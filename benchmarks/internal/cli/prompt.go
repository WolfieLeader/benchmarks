package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type Options struct {
	Warmup    bool
	Resources bool
	Capacity  bool
	Servers   []string // empty means all servers
}

func DefaultOptions() Options {
	return Options{Warmup: true, Resources: true, Capacity: false, Servers: nil}
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
	idx := int(scaled)
	if idx >= len(gradientStops)-1 {
		idx = len(gradientStops) - 2
	}
	localT := scaled - float64(idx)

	return lerpColor(gradientStops[idx], gradientStops[idx+1], localT)
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
	opts := DefaultOptions()

	var phases []string
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
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select benchmark phases").
				Description("Choose which benchmark phases to run").
				Options(
					huh.NewOption("Warmup (recommended)", "warmup").Selected(true),
					huh.NewOption("Resource monitoring", "resources").Selected(true),
					huh.NewOption("Capacity test (slow)", "capacity"),
				).Value(&phases),
		),
	).WithTheme(huh.ThemeCatppuccin()).WithKeyMap(huh.NewDefaultKeyMap())

	if err := form.Run(); err != nil {
		return nil, err
	}

	opts.Warmup = slices.Contains(phases, "warmup")
	opts.Resources = slices.Contains(phases, "resources")
	opts.Capacity = slices.Contains(phases, "capacity")

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

	opts := DefaultOptions()
	hasExplicitFlags := false
	var unknownFlags []string

	for _, arg := range args {
		switch {
		case arg == "--all":
			opts.Warmup = true
			opts.Resources = true
			opts.Capacity = true
			hasExplicitFlags = true
		case arg == "--warmup":
			opts.Warmup = true
			hasExplicitFlags = true
		case arg == "--resources":
			opts.Resources = true
			hasExplicitFlags = true
		case arg == "--capacity":
			opts.Capacity = true
			hasExplicitFlags = true
		case arg == "--no-warmup":
			opts.Warmup = false
			hasExplicitFlags = true
		case arg == "--no-resources":
			opts.Resources = false
			hasExplicitFlags = true
		case arg == "--no-capacity":
			opts.Capacity = false
			hasExplicitFlags = true
		case arg == "--quick":
			opts.Warmup = false
			opts.Resources = true
			opts.Capacity = false
			hasExplicitFlags = true
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

	if !hasExplicitFlags {
		return nil, nil
	}

	return &opts, nil
}

func printHelp() {
	fmt.Println(`Usage: benchmark [options]

Options:
  --all              Enable all benchmark phases (warmup, resources, capacity)
  --warmup           Enable warmup phase
  --resources        Enable resource monitoring
  --capacity         Enable capacity testing
  --no-warmup        Disable warmup phase
  --no-resources     Disable resource monitoring
  --no-capacity      Disable capacity testing
  --quick            Quick mode: no warmup, resources only
  --servers=a,b,c    Only benchmark specific servers (comma-separated)
  --help, -h         Show this help message

Interactive mode:
  Run without flags to use interactive selection.

Examples:
  benchmark                           # Interactive mode
  benchmark --all                     # Run all phases
  benchmark --warmup --resources      # Warmup and resources only
  benchmark --quick --servers=chi,gin # Quick test on specific servers`)
}

func PrintSummary(opts *Options, serverCount int) {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	enabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	disabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	formatStatus := func(enabled bool) string {
		if enabled {
			return enabledStyle.Render("enabled")
		}
		return disabledStyle.Render("disabled")
	}

	fmt.Println(headerStyle.Render("Configuration"))
	fmt.Println(strings.Repeat("─", 40))

	fmt.Printf("%s %s\n", labelStyle.Render("Warmup:"), formatStatus(opts.Warmup))
	fmt.Printf("%s %s\n", labelStyle.Render("Resources:"), formatStatus(opts.Resources))
	fmt.Printf("%s %s\n", labelStyle.Render("Capacity:"), formatStatus(opts.Capacity))

	if len(opts.Servers) > 0 {
		fmt.Printf("%s %s\n", labelStyle.Render("Servers:"), valueStyle.Render(strings.Join(opts.Servers, ", ")))
	} else {
		fmt.Printf("%s %s\n", labelStyle.Render("Servers:"), valueStyle.Render(fmt.Sprintf("all (%d)", serverCount)))
	}

	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()
}
