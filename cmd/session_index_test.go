package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/sessionindex"
)

// isolateStateDir points the state directory at a fresh one of the test's own.
func isolateStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	return dir
}

// seedSessionsJSON writes body as sessions.json in a state directory of the
// test's own, and returns the directory.
func seedSessionsJSON(t *testing.T, body string) string {
	t.Helper()
	dir := isolateStateDir(t)
	if err := os.WriteFile(sessionindex.Path(dir), []byte(body), 0o600); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	return dir
}

const legacyIndexBody = `{"version": 1, "saved_at": "2026-10-01T00:00:00Z", "sessions": []}`

func TestLegacyIndexStop(t *testing.T) {
	// Exempt commands are covered too: alias and init are bootstrap-exempt.
	for _, args := range [][]string{{"list"}, {"open"}, {"alias", "list"}, {"init", "zsh"}} {
		t.Run("it stops "+strings.Join(args, " ")+" with one line pointing at portal migrate", func(t *testing.T) {
			dir := seedSessionsJSON(t, legacyIndexBody)
			fake := backendtest.New()
			withBackendDeps(t, BackendDeps{Backend: fake})
			t.Setenv("PORTAL_ALIASES_FILE", t.TempDir()+"/aliases")
			reached := stubPicker(t)

			stdout, _, err := runRootCmd(t, args...)
			if err == nil {
				t.Fatal("the command ran on a version 1 sessions.json")
			}
			want := sessionindex.Path(dir) + " is from the tmux version of Portal; run portal migrate to move it aside"
			if err.Error() != want || !errors.Is(err, sessionindex.ErrLegacyIndex) {
				t.Errorf("error = %q, want %q wrapping ErrLegacyIndex", err, want)
			}
			if stdout.Len() != 0 || *reached || len(fake.Calls()) != 0 {
				t.Errorf("stdout = %q, picker reached = %v, backend calls = %+v; want nothing run", stdout, *reached, fake.Calls())
			}
		})
	}

	t.Run("it lets version run", func(t *testing.T) {
		seedSessionsJSON(t, legacyIndexBody)

		if _, _, err := runRootCmd(t, "version"); err != nil {
			t.Errorf("version on a version 1 sessions.json: %v", err)
		}
	})

	t.Run("it lets the tmux version's state commands run on its index", func(t *testing.T) {
		seedSessionsJSON(t, legacyIndexBody)
		ran := false
		withFuncSeam(t, &resumeWaitRunFunc, func(resumeWaitConfig) error {
			ran = true
			return nil
		})

		if _, _, err := runRootCmd(t, "state", resumeWaitSubcommand, "--pane", "%7", "--command", "make deploy"); err != nil {
			t.Fatalf("state %s on a version 1 sessions.json: %v", resumeWaitSubcommand, err)
		}
		if !ran {
			t.Error("the state command's body did not run")
		}
	})

	t.Run("it passes a missing sessions.json", func(t *testing.T) {
		isolateStateDir(t)
		fake := backendtest.New()
		withBackendDeps(t, BackendDeps{Backend: fake})

		if _, _, err := runRootCmd(t, "list"); err != nil {
			t.Fatalf("list with no sessions.json: %v", err)
		}
		assertListed(t, fake)
	})

	t.Run("it passes a sessions.json that cannot be read", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write blocker file: %v", err)
		}
		t.Setenv("PORTAL_STATE_DIR", filepath.Join(blocker, "state"))
		fake := backendtest.New()
		withBackendDeps(t, BackendDeps{Backend: fake})

		if _, _, err := runRootCmd(t, "list"); err != nil {
			t.Fatalf("list under an unreadable state dir: %v", err)
		}
		assertListed(t, fake)
	})

	t.Run("it passes a version 2 sessions.json", func(t *testing.T) {
		seedSessionsJSON(t, `{"version": 2, "saved_at": "2026-11-01T00:00:00Z", "sessions": []}`)
		fake := backendtest.New()
		withBackendDeps(t, BackendDeps{Backend: fake})

		if _, _, err := runRootCmd(t, "list"); err != nil {
			t.Fatalf("list on a version 2 sessions.json: %v", err)
		}
		assertListed(t, fake)
	})

	for name, tc := range map[string]struct{ body, want string }{
		"corrupt sessions.json": {body: `{"version": 2,`, want: "decode sessions.json"},
		"sessions.json from a newer Portal": {body: `{"version": 3, "sessions": []}`, want: "unsupported sessions.json version: 3"},
	} {
		t.Run("it stops on a "+name+", saying what is wrong", func(t *testing.T) {
			dir := seedSessionsJSON(t, tc.body)
			fake := backendtest.New()
			withBackendDeps(t, BackendDeps{Backend: fake})

			_, _, err := runRootCmd(t, "list")
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), dir) {
				t.Errorf("error = %v, want it to name %s and say %q", err, dir, tc.want)
			}
			if len(fake.Calls()) != 0 {
				t.Errorf("backend calls = %+v, want none", fake.Calls())
			}
		})
	}
}

func TestReadSessionIndex(t *testing.T) {
	t.Run("it reads a missing sessions.json as an empty index", func(t *testing.T) {
		dir := isolateStateDir(t)

		idx, err := readSessionIndex()
		if err != nil {
			t.Fatalf("readSessionIndex: %v", err)
		}
		if len(idx.Sessions) != 0 {
			t.Errorf("sessions = %+v, want none", idx.Sessions)
		}
		if _, err := os.Stat(sessionindex.Path(dir)); !os.IsNotExist(err) {
			t.Errorf("sessions.json stat = %v, want it still absent", err)
		}
	})

	t.Run("it reads the index in the state directory", func(t *testing.T) {
		dir := isolateStateDir(t)
		setLabel(t, dir, "api-x7Kd9a", "api")

		idx, err := readSessionIndex()
		if err != nil {
			t.Fatalf("readSessionIndex: %v", err)
		}
		if got := idx.Label("api-x7Kd9a"); got != "api" {
			t.Errorf("label = %q, want %q", got, "api")
		}
	})
}
