package roster

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, name, body string) {
	t.Helper()
	sub := filepath.Join(dir, name)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "bench.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverSortsAndParses(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "go-chi", `{"name":"go-chi","language":"go","runtime":"go","image":"bench/go-chi","port":8080,"databases":["postgres"],"experimental":false,"dev_port":21002,"web":true}`)
	writeManifest(t, dir, "ts-express", `{"name":"ts-express","runtime":"node","image":"bench/ts-express","port":8080}`)
	// A non-server dir without a manifest must be skipped, not error.
	if err := os.MkdirAll(filepath.Join(dir, "shared"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(entries), entries)
	}
	// Sorted by name: go-chi before ts-express.
	if entries[0].Name != "go-chi" || entries[1].Name != "ts-express" {
		t.Fatalf("wrong order/parse: %+v", entries)
	}
	if entries[0].Image != "bench/go-chi" || entries[0].Port != 8080 || !entries[0].Web {
		t.Fatalf("wrong fields: %+v", entries[0])
	}
	// web defaults to false when the manifest omits it (ts-express here).
	if entries[1].Web {
		t.Fatalf("expected ts-express web=false, got %+v", entries[1])
	}
}

func TestDiscoverErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{"empty", func(_ *testing.T, _ string) {}},
		{"bad json", func(t *testing.T, dir string) { writeManifest(t, dir, "a", `{not json`) }},
		{"missing name", func(t *testing.T, dir string) { writeManifest(t, dir, "a", `{"image":"i","port":1}`) }},
		{"missing image", func(t *testing.T, dir string) { writeManifest(t, dir, "a", `{"name":"a","port":1}`) }},
		{"bad port", func(t *testing.T, dir string) { writeManifest(t, dir, "a", `{"name":"a","image":"i","port":0}`) }},
		{"dup name", func(t *testing.T, dir string) {
			writeManifest(t, dir, "a", `{"name":"x","image":"bench/a","port":1}`)
			writeManifest(t, dir, "b", `{"name":"x","image":"bench/b","port":2}`)
		}},
		{"dup image", func(t *testing.T, dir string) {
			writeManifest(t, dir, "a", `{"name":"a","image":"bench/x","port":1}`)
			writeManifest(t, dir, "b", `{"name":"b","image":"bench/x","port":2}`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			if _, err := Discover(dir); err == nil {
				t.Fatalf("expected error for %q, got nil", tt.name)
			}
		})
	}
}

func TestDiscoverMissingDir(t *testing.T) {
	if _, err := Discover(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected error for missing servers dir")
	}
}
