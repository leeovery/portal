package cmd

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// unrestorable is a name zmx attach makes a session under and Create refuses,
// so restore could never make it again.
const unrestorable = "notes*"

// decision 74: no writer of sessions.json records an entry under a name Create
// refuses, and each says why.
func TestClaim_RefusesANameCreateRefuses(t *testing.T) {
	assertRefused := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, sessionindex.ErrNameRefused) || !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("err = %v, want the name refused for the rule Create refuses it by", err)
		}
		if idx := readIndex(t, os.Getenv("PORTAL_STATE_DIR")); idx.Len() != 0 {
			t.Errorf("index = %+v, want nothing recorded under %s", idx.Entries(), unrestorable)
		}
	}

	t.Run("portal rename", func(t *testing.T) {
		inSession(t, unrestorable)

		_, _, err := runRootCmd(t, "rename", "api work")

		assertRefused(t, err)
	})

	t.Run("the picker's r", func(t *testing.T) {
		fake := inSession(t, unrestorable)

		err := pickerLabeller{ctx: context.Background(), b: fake}.SetLabel(unrestorable, "api work")

		assertRefused(t, err)
		if !strings.Contains(err.Error(), "its name is one Portal could never restore") {
			t.Errorf("err = %q, want words the picker can flash", err)
		}
	})

	t.Run("portal hook set, which registers no hook", func(t *testing.T) {
		inSession(t, unrestorable)
		_, hooksFile := hooksFileInTempDir(t, map[string]map[string]string{"api": {"on-resume": "vim"}})
		before, err := os.ReadFile(hooksFile)
		if err != nil {
			t.Fatal(err)
		}

		_, err = runHookSet(t, "claude --resume")

		assertRefused(t, err)
		assertHooksFileUnchanged(t, hooksFile, before, "a hook set refused its claim")
	})

	t.Run("the hydrate helper, which still hands the session on", func(t *testing.T) {
		w := newHydrateWorld(t)
		w.backend.Add(backend.Session{Name: unrestorable, Created: w.live.Created, PID: w.live.PID})
		if err := state.Hydrating.Set(w.stateDir, unrestorable); err != nil {
			t.Fatal(err)
		}
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		exec := &stubExecShell{}
		cfg := w.config(exec)
		cfg.Session, cfg.Current, cfg.Logger = unrestorable, unrestorable, logger

		runHydrate(t.Context(), cfg)

		warn := execLogLine(t, sink.Body(), "WARN", "record session creation failed")
		if !strings.Contains(warn, "its name is one Portal could never restore") {
			t.Errorf("WARN %q, want it to say the name is refused", warn)
		}
		if idx := readIndex(t, w.stateDir); idx.Len() != 0 {
			t.Errorf("index = %+v, want nothing recorded under %s", idx.Entries(), unrestorable)
		}
		if !exec.called {
			t.Error("the session was never handed on")
		}
	})
}
