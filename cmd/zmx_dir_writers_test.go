package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// stateWriter is a bootstrap-exempt command that writes state, staged to
// succeed, with the hooks.json it would write.
type stateWriter struct {
	args  []string
	stage func(t *testing.T) (hooksFile string)
}

func stateWriters() map[string]stateWriter {
	return map[string]stateWriter{
		"hook set": {args: []string{"hook", "set", "--on-resume", "npm start"}, stage: func(t *testing.T) string {
			_, hooksFile := hooksFileInTempDir(t, nil)
			inSession(t, hookSessionA)
			return hooksFile
		}},
		"hooks set": {args: []string{"hooks", "set", "--on-resume", "npm start"}, stage: func(t *testing.T) string {
			_, hooksFile := hooksFileInTempDir(t, nil)
			inSession(t, hookSessionA)
			return hooksFile
		}},
		"hook rm": {args: []string{"hook", "rm", "--on-resume"}, stage: func(t *testing.T) string {
			_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{hookSessionA: {"on-resume": "npm start"}})
			inSession(t, hookSessionA)
			return hooksFile
		}},
		"rename": {args: []string{"rename", "api work"}, stage: func(t *testing.T) string {
			_, hooksFile := hooksFileInTempDir(t, nil)
			stageRename(t)
			return hooksFile
		}},
	}
}

// decision 76: a command that skips the bootstrap yet writes state makes the
// bootstrap gate's zmx dir check itself, recording nothing.
func TestExemptStateWritersJudgeTheZmxDir(t *testing.T) {
	for name, writer := range stateWriters() {
		t.Run(name+" refuses a state dir of another zmx dir, writing nothing", func(t *testing.T) {
			hooksFile := writer.stage(t)
			zmxDir, recorded := t.TempDir(), zmx.CanonicalDir(t.TempDir())
			t.Setenv("ZMX_DIR", zmxDir)
			stateDir := os.Getenv("PORTAL_STATE_DIR")
			if err := state.RecordZmxDir(stateDir, recorded); err != nil {
				t.Fatal(err)
			}
			hooksBefore := readFileBytes(t, hooksFile)

			_, _, err := runRootCmd(t, writer.args...)

			want := fmt.Sprintf(bootstrap.OtherZmxDirMessage, stateDir, recorded, zmxDir)
			if fatal, ok := errors.AsType[*bootstrap.FatalError](err); !ok || fatal.UserMessage != want {
				t.Errorf("%s = %v, want the fatal %q", name, err, want)
			}
			assertHooksFileUnchanged(t, hooksFile, hooksBefore)
			if got := dirListing(t, stateDir); !slices.Equal(got, []string{"zmx-dir"}) {
				t.Errorf("state dir = %v, want the record alone", got)
			}
		})

		t.Run(name+" refuses a record it can't read, writing nothing", func(t *testing.T) {
			hooksFile := writer.stage(t)
			stateDir := os.Getenv("PORTAL_STATE_DIR")
			if err := os.WriteFile(state.ZmxDirRecordPath(stateDir), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
			hooksBefore := readFileBytes(t, hooksFile)

			_, _, err := runRootCmd(t, writer.args...)

			if _, ok := errors.AsType[*bootstrap.ZmxDirRecordError](err); !ok {
				t.Errorf("%s = %v, want the unreadable record's fatal", name, err)
			}
			assertHooksFileUnchanged(t, hooksFile, hooksBefore)
			if _, err := os.Stat(sessionindex.Path(stateDir)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("stat sessions.json = %v, want nothing claimed", err)
			}
		})

		t.Run(name+" runs in a state dir that records no zmx dir, and records none", func(t *testing.T) {
			hooksFile := writer.stage(t)
			t.Setenv("ZMX_DIR", t.TempDir())
			stateDir := os.Getenv("PORTAL_STATE_DIR")
			hooksBefore := readFileBytes(t, hooksFile)

			if _, _, err := runRootCmd(t, writer.args...); err != nil {
				t.Fatalf("%s = %v, want it to run", name, err)
			}
			if _, err := os.Stat(state.ZmxDirRecordPath(stateDir)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("stat the record = %v, want none written", err)
			}
			_, indexErr := os.Stat(sessionindex.Path(stateDir))
			if bytes.Equal(readFileBytes(t, hooksFile), hooksBefore) && indexErr != nil {
				t.Errorf("hooks.json unchanged and no sessions.json (%v), want %s to have written state", indexErr, name)
			}
		})
	}
}
