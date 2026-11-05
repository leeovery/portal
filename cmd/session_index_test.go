package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
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

// blockStateDir points the state directory beneath a regular file, so
// sessions.json can be neither found nor read.
func blockStateDir(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	t.Setenv("PORTAL_STATE_DIR", filepath.Join(blocker, "state"))
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
	// Bootstrap-exempt commands are covered too: every row from alias on.
	for _, args := range [][]string{
		{"list"},
		{"open"},
		{"alias", "list"},
		{"init", "zsh"},
		{"__complete", "open", ""},
		{"hook", "list"},
		{"uninstall"},
	} {
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

		if _, _, err := runRootCmd(t, "state", resumeWaitSubcommand, "--session", "proj-a1b2", "--command", "make deploy"); err != nil {
			t.Fatalf("state %s on a version 1 sessions.json: %v", resumeWaitSubcommand, err)
		}
		if !ran {
			t.Error("the state command's body did not run")
		}
	})

	t.Run("it passes a missing sessions.json", func(t *testing.T) {
		isolateStateDir(t)
		fake := backendtest.New()
		withBootstrapDeps(t, BootstrapDeps{})
		withBackendDeps(t, BackendDeps{Backend: fake})

		if _, _, err := runRootCmd(t, "list"); err != nil {
			t.Fatalf("list with no sessions.json: %v", err)
		}
		assertListed(t, fake)
	})

	t.Run("it passes a sessions.json that cannot be read", func(t *testing.T) {
		blockStateDir(t)
		fake := backendtest.New()
		withBootstrapDeps(t, BootstrapDeps{})
		withBackendDeps(t, BackendDeps{Backend: fake})

		if _, _, err := runRootCmd(t, "list"); err != nil {
			t.Fatalf("list under an unreadable state dir: %v", err)
		}
		assertListed(t, fake)
	})

	t.Run("it passes a version 2 sessions.json", func(t *testing.T) {
		seedSessionsJSON(t, `{"version": 2, "saved_at": "2026-11-01T00:00:00Z", "sessions": []}`)
		fake := backendtest.New()
		withBootstrapDeps(t, BootstrapDeps{})
		withBackendDeps(t, BackendDeps{Backend: fake})

		if _, _, err := runRootCmd(t, "list"); err != nil {
			t.Fatalf("list on a version 2 sessions.json: %v", err)
		}
		assertListed(t, fake)
	})

	for name, tc := range map[string]struct{ body, want string }{
		"corrupt sessions.json":             {body: `{"version": 2,`, want: "decode sessions.json"},
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

func TestReadShownIndex(t *testing.T) {
	t.Run("it reads a missing sessions.json as an empty index", func(t *testing.T) {
		dir := isolateStateDir(t)

		shown, err := readShownIndex()
		if err != nil {
			t.Fatalf("readShownIndex: %v", err)
		}
		if shown.idx.Len() != 0 {
			t.Errorf("sessions = %+v, want none", shown.idx.Entries())
		}
		if _, err := os.Stat(sessionindex.Path(dir)); !os.IsNotExist(err) {
			t.Errorf("sessions.json stat = %v, want it still absent", err)
		}
	})

	t.Run("it reads the index in the state directory", func(t *testing.T) {
		dir := isolateStateDir(t)
		storedLabel(t, dir, "api-x7Kd9a", "api")

		shown, err := readShownIndex()
		if err != nil {
			t.Fatalf("readShownIndex: %v", err)
		}
		if got, _ := shown.idx.Lookup("api-x7Kd9a"); got.Label != "api" {
			t.Errorf("label = %q, want %q", got.Label, "api")
		}
	})

	t.Run("it judges entries by the claim rule", func(t *testing.T) {
		thisBoot := sessionindex.Entry{Name: "api-x7Kd9a", Label: "api", CreatedAt: testBoot.Add(time.Hour), PID: 1717}
		dir := seedIndex(t, thisBoot)
		withFuncSeam(t, &claimRule, func(got string, _ func(string) error) sessionindex.Rule {
			if got != dir {
				t.Errorf("claimRule(%q), want the state directory %q", got, dir)
			}
			return sessionindex.Rule{}
		})

		shown, err := readShownIndex()
		if err != nil {
			t.Fatalf("readShownIndex: %v", err)
		}
		live := backend.Session{Name: "api-x7Kd9a", Created: testBoot.Add(2 * time.Hour), PID: 4242, Answered: true}
		if got := shown.show(live); got.Label != "api" {
			t.Errorf("show = %+v, want the label the zero rule's adoption keeps", got)
		}
	})
}

func TestUpdateSessionIndexLanding(t *testing.T) {
	t.Run("it writes a change to the index in the state directory", func(t *testing.T) {
		dir := isolateStateDir(t)

		err := updateSessionIndexLanding(dir, "cli", func(idx *sessionindex.Index) error {
			return idx.SetLabel("api-x7Kd9a", "api")
		}, landedNothing)
		if err != nil {
			t.Fatalf("updateSessionIndexLanding: %v", err)
		}
		idx, err := sessionindex.Read(dir)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if got, _ := idx.Lookup("api-x7Kd9a"); got.Label != "api" {
			t.Errorf("label = %q, want %q", got.Label, "api")
		}
	})

	t.Run("it refuses a version 1 index with the migrate line and writes nothing", func(t *testing.T) {
		dir := isolateStateDir(t)
		path := sessionindex.Path(dir)
		if err := os.WriteFile(path, []byte(legacyIndexBody), 0o600); err != nil {
			t.Fatalf("seed legacy index: %v", err)
		}

		err := updateSessionIndexLanding(dir, "cli", func(idx *sessionindex.Index) error {
			return idx.SetLabel("api-x7Kd9a", "api")
		}, landedNothing)
		if !errors.Is(err, sessionindex.ErrLegacyIndex) || !strings.Contains(err.Error(), "portal migrate") {
			t.Fatalf("err = %v, want the legacy-index refusal naming portal migrate", err)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read legacy index: %v", readErr)
		}
		if string(data) != legacyIndexBody {
			t.Errorf("legacy index rewritten to %s", data)
		}
	})
}

func landedNothing(sessionindex.Outcome) error { return nil }

// acceptName is a name rule refusing no name, for a fixture claiming names it
// knows Create takes.
func acceptName(string) error { return nil }

func TestIndexWritesMakeTheStateDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	t.Setenv("PORTAL_STATE_DIR", dir)

	err := claimSession("cli", acceptName, claimed{Name: "api-x7Kd9a", Created: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC), PID: 4242}, nil, nil)
	if err != nil {
		t.Fatalf("claimSession with no state dir yet: %v", err)
	}
	idx, err := sessionindex.Read(dir)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if _, ok := idx.Lookup("api-x7Kd9a"); !ok {
		t.Errorf("index %+v holds no entry for api-x7Kd9a", idx)
	}
}
