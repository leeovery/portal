package state_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

func TestZmxDirRecord(t *testing.T) {
	t.Run("a state directory with no record names none", func(t *testing.T) {
		if got, err := state.ReadZmxDir(t.TempDir()); got != "" || err != nil {
			t.Errorf("ReadZmxDir = %q, %v; want none and no error", got, err)
		}
	})

	t.Run("a record reads back as written, and is logged once under the bootstrap's component", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		sink := logtest.Install(t)

		if err := state.RecordZmxDir(dir, "/srv/zmx"); err != nil {
			t.Fatalf("RecordZmxDir = %v", err)
		}

		if got, err := state.ReadZmxDir(dir); got != "/srv/zmx" || err != nil {
			t.Errorf("ReadZmxDir = %q, %v; want /srv/zmx", got, err)
		}
		rec := sink.Records().Matching("bootstrap", "zmx dir recorded").AtExactLevel(slog.LevelInfo).Only(t, "the record")
		if got := rec.AttrString(t, "path"); got != "/srv/zmx" {
			t.Errorf("path = %q, want /srv/zmx", got)
		}
		if info, err := os.Stat(state.ZmxDirRecordPath(dir)); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("stat the record = %v, %v; want it 0600", info, err)
		}
	})

	t.Run("it refuses to record a relative directory, writing nothing", func(t *testing.T) {
		dir := t.TempDir()

		if err := state.RecordZmxDir(dir, "zmx"); err == nil {
			t.Error("RecordZmxDir(zmx) = nil, want a refusal")
		}
		if _, err := os.Stat(state.ZmxDirRecordPath(dir)); !os.IsNotExist(err) {
			t.Errorf("stat the record = %v, want none written", err)
		}
	})

	for name, body := range map[string]string{
		"doesn't decode":                  `{`,
		"names no directory":              `{}`,
		"names a relative directory":      `{"zmx_dir":"zmx"}`,
		"names something not a directory": `{"zmx_dir":7}`,
	} {
		t.Run("a record that "+name+" is an error", func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(state.ZmxDirRecordPath(dir), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}

			if got, err := state.ReadZmxDir(dir); err == nil {
				t.Errorf("ReadZmxDir = %q, nil; want an error", got)
			}
		})
	}
}
