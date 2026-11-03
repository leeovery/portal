//go:build integration

package cmd_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/zmxtest"
)

func TestPortalKillAgainstZmx(t *testing.T) {
	env, stateDir := portaltest.IsolateStateForTest(t)
	b := zmxtest.New(t)
	binary := buildPortalBinary(t)

	workDir := t.TempDir()
	for _, name := range []string{"api-x7Kd9a", "web-Qm3pLz"} {
		if err := b.Create(t.Context(), name, workDir, []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
		if err := sessionindex.Update(stateDir, "cli", func(idx *sessionindex.Index) error {
			return idx.SetLabel(name, name+" · work")
		}); err != nil {
			t.Fatalf("label %s: %v", name, err)
		}
	}

	env = append(env,
		"ZMX_DIR="+b.Dir(),
		"PATH="+filepath.Dir(zmxtest.Binary(t))+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	kill := func(name string) (string, error) {
		t.Helper()
		cmd := exec.Command(binary, "kill", name)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	t.Run("it kills the session and removes its index entry", func(t *testing.T) {
		if out, err := kill("api-x7Kd9a"); err != nil {
			t.Fatalf("portal kill: %v\n%s", err, out)
		}

		if held, err := b.Exists(t.Context(), "api-x7Kd9a"); err != nil || held {
			t.Errorf("Exists(api-x7Kd9a) = %v, %v; want gone", held, err)
		}
		if held, err := b.Exists(t.Context(), "web-Qm3pLz"); err != nil || !held {
			t.Errorf("Exists(web-Qm3pLz) = %v, %v; want it left running", held, err)
		}
		idx, err := sessionindex.Read(stateDir)
		if err != nil {
			t.Fatalf("read sessions.json: %v", err)
		}
		if _, ok := idx.Lookup("api-x7Kd9a"); ok {
			t.Error("the killed session's index entry survived")
		}
		if _, ok := idx.Lookup("web-Qm3pLz"); !ok {
			t.Error("the other session's index entry went with the kill")
		}
	})

	t.Run("it exits 1 with one line for a session it does not hold", func(t *testing.T) {
		out, err := kill("gone-Zx81Qa")

		if exitErr, ok := errors.AsType[*exec.ExitError](err); !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("portal kill of a missing session = %v, want exit 1\n%s", err, out)
		}
		if got := strings.TrimSpace(out); got != "No session found: gone-Zx81Qa" {
			t.Errorf("output = %q, want %q", got, "No session found: gone-Zx81Qa")
		}
	})

	t.Run("it names the spelling a live session is held under", func(t *testing.T) {
		out, err := kill("WEB-qm3PLZ")

		if exitErr, ok := errors.AsType[*exec.ExitError](err); !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("portal kill under another spelling = %v, want exit 1\n%s", err, out)
		}
		if got, want := strings.TrimSpace(out), "No session found: WEB-qm3PLZ; did you mean web-Qm3pLz?"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
		if held, err := b.Exists(t.Context(), "web-Qm3pLz"); err != nil || !held {
			t.Errorf("Exists(web-Qm3pLz) = %v, %v; want it left running", held, err)
		}
	})
}
