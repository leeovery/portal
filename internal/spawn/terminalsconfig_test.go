package spawn

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
)

// installSpawnCapture swaps a fresh logtest.Sink into the process-wide log
// indirection for the duration of the test so the package-level spawn-component
// spawnLogger (log.For("spawn")) routes its WARN records into the sink.
func installSpawnCapture(t *testing.T) *logtest.Sink {
	t.Helper()
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)
	return sink
}

// warnRecords returns only the captured WARN-level records — Load emits nothing
// on the happy paths, so an assertion of "exactly one spawn WARN" is a count of
// these.
func warnRecords(sink *logtest.Sink) []logtest.Record {
	var out []logtest.Record
	for _, r := range sink.Records() {
		if r.Level == slog.LevelWarn {
			out = append(out, r)
		}
	}
	return out
}

func TestTerminalsStoreLoad(t *testing.T) {
	t.Run("it returns an empty config with no WARN for a missing file", func(t *testing.T) {
		sink := installSpawnCapture(t)
		path := filepath.Join(t.TempDir(), "terminals.json")

		cfg := NewTerminalsStore(path).Load()

		if cfg == nil {
			t.Fatal("Load returned a nil config for a missing file")
		}
		if len(cfg) != 0 {
			t.Errorf("config len = %d, want 0 for a missing file", len(cfg))
		}
		if got := warnRecords(sink); len(got) != 0 {
			t.Errorf("emitted %d WARN records for a missing file, want 0: %+v", len(got), got)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("Load created a file at a previously-absent path (stat err = %v)", err)
		}
	})

	t.Run("it ignores an unreadable file and emits a spawn WARN", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: chmod 0000 does not deny reads")
		}
		sink := installSpawnCapture(t)
		path := filepath.Join(t.TempDir(), "terminals.json")
		writeFile(t, path, `{"com.example.MyTerm":{"commands":{"open":{"argv":["kitty","{command}"]}}}}`)
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatalf("chmod 0000 failed: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

		cfg := NewTerminalsStore(path).Load()

		if cfg == nil || len(cfg) != 0 {
			t.Errorf("config = %+v, want an empty non-nil config for an unreadable file", cfg)
		}
		got := warnRecords(sink)
		if len(got) != 1 {
			t.Fatalf("emitted %d WARN records for an unreadable file, want exactly 1: %+v", len(got), got)
		}
		rec := got[0]
		if v := rec.AttrString(t, "component"); v != "spawn" {
			t.Errorf("WARN component = %q, want %q", v, "spawn")
		}
		if !rec.HasAttr("detail") || rec.AttrString(t, "detail") == "" {
			t.Errorf("unreadable WARN missing a non-empty detail attr: %+v", rec.Attrs)
		}
	})

	t.Run("it ignores a malformed file and emits a spawn WARN", func(t *testing.T) {
		sink := installSpawnCapture(t)
		path := filepath.Join(t.TempDir(), "terminals.json")
		writeFile(t, path, `{ not valid json`)

		cfg := NewTerminalsStore(path).Load()

		if cfg == nil || len(cfg) != 0 {
			t.Errorf("config = %+v, want an empty non-nil config for malformed JSON", cfg)
		}
		got := warnRecords(sink)
		if len(got) != 1 {
			t.Fatalf("emitted %d WARN records for malformed JSON, want exactly 1: %+v", len(got), got)
		}
		rec := got[0]
		if v := rec.AttrString(t, "component"); v != "spawn" {
			t.Errorf("WARN component = %q, want %q", v, "spawn")
		}
		if !rec.HasAttr("detail") || rec.AttrString(t, "detail") == "" {
			t.Errorf("malformed WARN missing a non-empty detail attr: %+v", rec.Attrs)
		}
	})

	t.Run("it parses a valid entry and ignores unknown capability sub-keys", func(t *testing.T) {
		sink := installSpawnCapture(t)
		path := filepath.Join(t.TempDir(), "terminals.json")
		writeFile(t, path, `{"com.example.MyTerm":{"commands":{"open":{"argv":["kitty","{command}"]},"introspect":{"foo":1},"place":{"bar":2}}}}`)

		cfg := NewTerminalsStore(path).Load()

		entry, ok := cfg["com.example.MyTerm"]
		if !ok {
			t.Fatalf("config missing the com.example.MyTerm entry: %+v", cfg)
		}
		if entry.Commands.Open == nil {
			t.Fatal("Commands.Open is nil, want the parsed open recipe")
		}
		wantArgv := []string{"kitty", "{command}"}
		if got := entry.Commands.Open.Argv; !slices.Equal(got, wantArgv) {
			t.Errorf("open argv = %v, want %v", got, wantArgv)
		}
		if got := warnRecords(sink); len(got) != 0 {
			t.Errorf("emitted %d WARN records for a valid entry, want 0: %+v", len(got), got)
		}
	})

	t.Run("it never writes the file (read-only load)", func(t *testing.T) {
		installSpawnCapture(t)
		path := filepath.Join(t.TempDir(), "terminals.json")
		writeFile(t, path, `{"com.example.MyTerm":{"commands":{"open":{"argv":["kitty","{command}"]}}}}`)
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading seeded file: %v", err)
		}
		beforeInfo, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat seeded file: %v", err)
		}

		_ = NewTerminalsStore(path).Load()

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading file after Load: %v", err)
		}
		if string(after) != string(before) {
			t.Errorf("Load mutated the file bytes:\nbefore = %q\nafter  = %q", before, after)
		}
		afterInfo, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat file after Load: %v", err)
		}
		if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
			t.Errorf("Load changed the file mtime: before = %v, after = %v", beforeInfo.ModTime(), afterInfo.ModTime())
		}
	})

	t.Run("it normalises a JSON null to an empty config", func(t *testing.T) {
		installSpawnCapture(t)
		path := filepath.Join(t.TempDir(), "terminals.json")
		writeFile(t, path, `null`)

		cfg := NewTerminalsStore(path).Load()

		if cfg == nil {
			t.Fatal("Load returned a nil config for a JSON null (callers would nil-panic ranging over it)")
		}
		n := 0
		for range cfg {
			n++
		}
		if n != 0 {
			t.Errorf("config has %d entries, want 0 for a JSON null", n)
		}
	})
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
