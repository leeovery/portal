//go:build integration

package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

func TestPortalListAgainstZmx(t *testing.T) {
	env, stateDir := portaltest.IsolateStateForTest(t)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	b := zmxtest.New(t)
	binary := buildPortalBinary(t)

	workDir := realPath(t, t.TempDir())
	for _, name := range []string{"api-x7Kd9a", "web-Qm3pLz", "wedged-Lp4s"} {
		if _, err := b.Create(t.Context(), name, workDir, []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}
	zmxtest.Attach(t, b, "web-Qm3pLz")
	if !harnesstest.PollUntil(t, 5*time.Second, 20*time.Millisecond, func() bool { return clients(t, b, "web-Qm3pLz") == 1 }) {
		t.Fatal("web-Qm3pLz never showed its client attached")
	}
	zmxtest.StopDaemon(t, b, "wedged-Lp4s")
	// A label shows only for the creation it was set on, so it is claimed for
	// the live session's, as rename claims it.
	web, err := b.Session(t.Context(), "web-Qm3pLz")
	if err != nil {
		t.Fatalf("read web-Qm3pLz: %v", err)
	}
	if _, err := sessionindex.Update(stateDir, "cli", func(idx *sessionindex.Index) error {
		if _, err := idx.Claim(web.Name, web.Dir, web.Created, web.PID, state.ClaimRule(stateDir, b.ValidateName)); err != nil {
			return err
		}
		return idx.SetLabel(web.Name, "web · checkout")
	}); err != nil {
		t.Fatalf("label web-Qm3pLz: %v", err)
	}

	env = append(env,
		"ZMX_DIR="+b.Dir(),
		"PATH="+filepath.Dir(zmxtest.Binary(t))+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	list := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, append([]string{"list"}, args...)...)
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("portal list %v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	t.Run("it prints the user's sessions by name, Portal's own hidden", func(t *testing.T) {
		if got, want := list("--short"), "api-x7Kd9a\nweb-Qm3pLz\nwedged-Lp4s\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
		if held, err := b.Exists(t.Context(), saver.SessionName); err != nil || !held {
			t.Errorf("Exists(%s) = %v, %v; want the saver its bootstrap started, hidden", saver.SessionName, held, err)
		}
	})

	t.Run("it adds label, attached state and dir with --long, and marks one that does not answer", func(t *testing.T) {
		want := "api-x7Kd9a    api-x7Kd9a    detached    " + workDir + "\n" +
			"web-Qm3pLz    web · checkout    attached    " + workDir + "\n" +
			"wedged-Lp4s    not answering\n"
		if got := list("--long"); got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})
}

func clients(t *testing.T, b *zmx.Backend, name string) int {
	t.Helper()
	sessions, err := b.List(t.Context())
	if err != nil {
		t.Fatalf("list %s: %v", b.Dir(), err)
	}
	i := slices.IndexFunc(sessions, func(s backend.Session) bool { return s.Name == name })
	if i < 0 {
		return 0
	}
	return sessions[i].Clients
}

// realPath resolves dir as zmx reports a session's cwd: macOS's temp dirs sit
// behind the /var symlink.
func realPath(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}
