package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	SymbolPass    = "✓"
	SymbolFail    = "✗"
	SymbolArrow   = "→"
	SymbolDot     = "•"
	SymbolWarning = "⚠"
	SymbolInfo    = "ℹ"

	Indent = "  "
)

func Header(title string) {
	width := 60
	padding := (width - len(title) - 2) / 2
	border := strings.Repeat("═", width)

	fmt.Println()
	fmt.Printf("╔%s╗\n", border)
	fmt.Printf("║%s %s %s║\n", strings.Repeat(" ", padding), title, strings.Repeat(" ", width-padding-len(title)-2))
	fmt.Printf("╚%s╝\n", border)
	fmt.Println()
}

func Section(title string) {
	fmt.Printf("\n━━ %s ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n", title)
}

func ServerHeader(name string) {
	fmt.Printf("\n┌─ %s %s\n", name, strings.Repeat("─", 58-len(name)))
}

func ServerFooter() {
	fmt.Println("└" + strings.Repeat("─", 60))
}

func Infof(format string, args ...any) {
	fmt.Printf("%s%s %s\n", Indent, SymbolInfo, fmt.Sprintf(format, args...))
}

func Successf(format string, args ...any) {
	fmt.Printf("%s%s %s\n", Indent, SymbolPass, fmt.Sprintf(format, args...))
}

func Failf(format string, args ...any) {
	fmt.Printf("%s%s %s\n", Indent, SymbolFail, fmt.Sprintf(format, args...))
}

func Warnf(format string, args ...any) {
	fmt.Printf("%s%s %s\n", Indent, SymbolWarning, fmt.Sprintf(format, args...))
}

func Linef(format string, args ...any) {
	fmt.Printf("%s%s\n", Indent, fmt.Sprintf(format, args...))
}

func KeyValue(key, value string) {
	fmt.Printf("%s%-20s %s\n", Indent, key+":", value)
}

func KeyValuePairs(pairs ...string) {
	if len(pairs)%2 != 0 {
		return
	}
	var parts []string
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, fmt.Sprintf("%s: %s", pairs[i], pairs[i+1]))
	}
	fmt.Printf("%s%s\n", Indent, strings.Join(parts, "  │  "))
}

func StatusLinef(status bool, format string, args ...any) {
	symbol := SymbolPass
	if !status {
		symbol = SymbolFail
	}
	fmt.Printf("%s%s %s\n", Indent, symbol, fmt.Sprintf(format, args...))
}

func Progress(current, label string, details string) {
	fmt.Printf("%s[%s] %-12s %s\n", Indent, current, label, details)
}

func TableHeader(columns ...string) {
	header := make([]string, 0, len(columns))
	separator := make([]string, 0, len(columns))
	for _, col := range columns {
		header = append(header, col)
		separator = append(separator, strings.Repeat("─", len(col)))
	}
	fmt.Printf("%s%s\n", Indent, strings.Join(header, "  "))
	fmt.Printf("%s%s\n", Indent, strings.Join(separator, "──"))
}

func Blank() {
	fmt.Println()
}

func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func FormatLatency[T int64 | time.Duration](t T) string {
	ns := int64(t)
	if ns < 1000 {
		return fmt.Sprintf("%5dns", ns)
	}
	if ns < 1_000_000 {
		us := float64(ns) / 1000
		return fmt.Sprintf("%5.1fµs", us)
	}
	ms := float64(ns) / 1_000_000
	return fmt.Sprintf("%5.2fms", ms)
}

func FormatMemory(bytes float64) string {
	mb := bytes / 1024 / 1024
	if mb < 1 {
		return fmt.Sprintf("%.0fKB", bytes/1024)
	}
	if mb < 100 {
		return fmt.Sprintf("%.1fMB", mb)
	}
	return fmt.Sprintf("%.0fMB", mb)
}

func FormatMemoryFixed(bytes float64) string {
	mb := bytes / 1024 / 1024
	if mb < 1 {
		return fmt.Sprintf("%4.0fKB", bytes/1024)
	}
	return fmt.Sprintf("%5.1fMB", mb)
}

func FormatCpu(percent float64, samples int) string {
	if samples < 2 || percent < 0.1 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", percent)
}

func FormatPercent(value float64) string {
	return fmt.Sprintf("%.1f%%", value*100)
}

func Truncate(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

func TruncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path[:maxLen-3] + "..."
	}
	return ".../" + parts[len(parts)-1]
}

func FormatReqs(count int) string {
	if count < 1000 {
		return strconv.Itoa(count)
	}
	if count < 1_000_000 {
		return fmt.Sprintf("%.2fk", float64(count)/1000)
	}
	return fmt.Sprintf("%.2fM", float64(count)/1_000_000)
}

func FormatRate(rate float64) string {
	pct := rate * 100
	if pct >= 99.95 {
		return "100%"
	}
	if pct >= 9.95 {
		return fmt.Sprintf("%.1f%%", pct)
	}
	return fmt.Sprintf("%.2f%%", pct)
}
