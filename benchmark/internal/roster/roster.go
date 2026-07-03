// Package roster discovers the benchmarked server roster from the filesystem.
//
// The roster is NOT configured centrally: each server app carries a bench.json
// manifest next to its Dockerfile (PLAN §7.4) and the client learns the roster by
// scanning servers/*/bench.json. Adding a server = adding a folder with a manifest,
// zero edits to config or client code. This mirrors scripts/lib.mts's discovery so
// the Go client and the TS orchestrators agree on one source of truth.
package roster

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Entry is the subset of a manifest the benchmark client needs: which image to
// run and which container port it listens on. Language/runtime/databases/etc. in
// the manifest are consumed by other tools (scripts, later client slices) and are
// deliberately ignored here to keep this focused.
type Entry struct {
	Name  string
	Image string
	Port  int
}

// manifest mirrors config/bench.schema.json. Unknown members are ignored by
// json/v2's default, so listing only the fields the client uses is safe.
type manifest struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Port  int    `json:"port"`
}

// Discover scans serversDir with a fixed one-level walk (serversDir/<entry>/bench.json,
// flat layout PLAN §2.1) — never a recursive scan, so installed dependency trees
// (node_modules/.venv) can't inject a stray bench.json. It fails loud on a broken or
// missing manifest and on duplicate names/images; full schema + config cross-checks
// live in scripts/check-config.mts.
func Discover(serversDir string) ([]Entry, error) {
	dirEntries, err := os.ReadDir(serversDir)
	if err != nil {
		return nil, fmt.Errorf("read servers directory %q: %w", serversDir, err)
	}

	entries := make([]Entry, 0, len(dirEntries))
	seenName := make(map[string]string) // name -> manifest that declared it
	seenImage := make(map[string]string)

	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		path := filepath.Join(serversDir, de.Name(), "bench.json")
		if _, statErr := os.Stat(path); statErr != nil {
			continue // not every servers/ subdir is a server
		}

		data, readErr := os.ReadFile(path) //nolint:gosec // fixed one-level path under serversDir
		if readErr != nil {
			return nil, fmt.Errorf("read manifest %q: %w", path, readErr)
		}
		var m manifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parse manifest %q: %w", path, err)
		}

		// Keep this acceptance predicate in sync with scripts/lib.mts — the two
		// discoverers must agree on what a valid manifest is (runtime/eco fields
		// are validated only by the scripts, which are their sole consumer).
		if strings.TrimSpace(m.Name) == "" {
			return nil, fmt.Errorf("manifest %q: missing required field \"name\"", path)
		}
		if strings.TrimSpace(m.Image) == "" {
			return nil, fmt.Errorf("manifest %q: missing required field \"image\"", path)
		}
		if m.Port < 1 || m.Port > 65535 {
			return nil, fmt.Errorf("manifest %q: port must be between 1 and 65535, got %d", path, m.Port)
		}
		if prior, ok := seenName[m.Name]; ok {
			return nil, fmt.Errorf("duplicate server name %q in %q and %q", m.Name, path, prior)
		}
		if prior, ok := seenImage[m.Image]; ok {
			return nil, fmt.Errorf("duplicate image %q in %q and %q", m.Image, path, prior)
		}
		seenName[m.Name] = path
		seenImage[m.Image] = path

		entries = append(entries, Entry(m))
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no bench.json manifests found under %q", serversDir)
	}

	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	return entries, nil
}
