package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func validateCpu(limit string) error {
	limit = strings.TrimSpace(limit)
	if limit == "" {
		return errors.New("cpu_limit is empty")
	}

	if value, ok := strings.CutSuffix(limit, "%"); ok {
		if value == "" {
			return errors.New("cpu_limit must be a number or percentage")
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed <= 0 {
			return errors.New("cpu_limit must be a number or percentage")
		}
		return nil
	}

	parsed, err := strconv.ParseFloat(limit, 64)
	if err != nil || parsed <= 0 {
		return errors.New("cpu_limit must be a number or percentage")
	}

	return nil
}

func normalizeMemoryLimit(limit string) (string, error) {
	limit = strings.TrimSpace(limit)
	if limit == "" {
		return "", errors.New("memory_limit is empty")
	}

	lower := strings.ToLower(limit)
	idx := len(lower)
	for idx > 0 {
		ch := lower[idx-1]
		if ch >= 'a' && ch <= 'z' {
			idx--
			continue
		}
		break
	}

	value := strings.TrimSpace(lower[:idx])
	unit := strings.TrimSpace(lower[idx:])
	if value == "" {
		return "", errors.New("memory_limit must be a number with optional unit (k, m, g, kb, mb, gb)")
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return "", errors.New("memory_limit must be a number with optional unit (k, m, g, kb, mb, gb)")
	}

	switch unit {
	case "":
		return value, nil
	case "k", "kb":
		return value + "kb", nil
	case "m", "mb":
		return value + "mb", nil
	case "g", "gb":
		return value + "gb", nil
	default:
		return "", errors.New("memory_limit must be a number with optional unit (k, m, g, kb, mb, gb)")
	}
}

func parsePercent(value, defaultValue string) (float64, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultValue
	}
	value = strings.TrimSpace(value)
	if !strings.HasSuffix(value, "%") {
		return 0, fmt.Errorf("must be a percentage (e.g. %q)", defaultValue)
	}
	numStr := strings.TrimSuffix(value, "%")
	parsed, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid percentage %q", value)
	}
	return parsed, nil
}

func parseDuration(value, defaultValue string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		value = defaultValue
	}
	return time.ParseDuration(strings.TrimSpace(value))
}
