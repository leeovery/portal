//go:build integration

package cmd_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const symptomKillBudget = 1500 * time.Millisecond

// markerSetSettle stands in for a poll: the marker-set path writes
// nothing, so there is no change to wait for. 250ms covers the hook
// fork → portal startup → marker query → exit chain.
const markerSetSettle = 250 * time.Millisecond

func TestCommitNowSymptom(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// PATH-prepend so every portal subprocess resolves to the freshly built
	// binary; sub-tests inherit it.
	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	t.Run("canonical symptom: kill, sync sessions.json update, second bootstrap does not reconstruct", func(t *testing.T) {
		fixture := newSymptomFixture(t, binary, binDir, "ptl-symptom-canon-")

		// The hook's run-shell fork-execs commit-now asynchronously, so the
		// poll below is the load-bearing wait.
		fixture.sock.Run(t, "kill-session", "-t", "B")

		ctx, cancel := context.WithTimeout(context.Background(), symptomKillBudget)
		defer cancel()
		if perr := pollSessionsJSON(ctx, fixture.stateDir, []string{"A"}, []string{"B"}); perr != nil {
			t.Fatalf(
				"sessions.json did not reflect kill of B within %s: %v\n%s",
				symptomKillBudget, perr, fixture.diagnostic(),
			)
		}

		runPortalList(t, binary, fixture)

		live := liveSessionNames(t, fixture.sock)
		if _, present := live["B"]; present {
			t.Fatalf(
				"second bootstrap reconstructed killed session B as a live tmux session; "+
					"live sessions = %v\n%s",
				keysOf(live), fixture.diagnostic(),
			)
		}
		if _, present := live["A"]; !present {
			t.Errorf("second bootstrap dropped surviving session A; live sessions = %v", keysOf(live))
		}

		assertSessionsJSONHas(t, fixture.stateDir, []string{"A"}, []string{"B"})
	})

	t.Run("_portal-saver self-kill with marker clear leaves user sessions intact and omits _portal-saver", func(t *testing.T) {
		fixture := newSymptomFixture(t, binary, binDir, "ptl-symptom-saverkill-")

		if !rawSessionPresent(t, fixture.sock, tmux.PortalSaverName) {
			t.Fatalf("_portal-saver not present after bootstrap; "+
				"sub-test cannot exercise saver self-kill scenario\n%s",
				fixture.diagnostic())
		}

		fixture.sock.Run(t, "kill-session", "-t", tmux.PortalSaverName)

		ctx, cancel := context.WithTimeout(context.Background(), symptomKillBudget)
		defer cancel()
		if perr := pollSessionsJSON(ctx, fixture.stateDir,
			[]string{"A", "B"},
			[]string{tmux.PortalSaverName},
		); perr != nil {
			t.Fatalf(
				"sessions.json did not stabilise after _portal-saver kill within %s: %v\n%s",
				symptomKillBudget, perr, fixture.diagnostic(),
			)
		}

		assertSessionsJSONHas(t, fixture.stateDir, []string{"A", "B"}, []string{tmux.PortalSaverName})
	})

	t.Run("@portal-restoring defence: marker-set saver-kill is byte-identical, marker-clear kill updates correctly", func(t *testing.T) {
		fixture := newSymptomFixture(t, binary, binDir, "ptl-symptom-marker-")

		if !rawSessionPresent(t, fixture.sock, tmux.PortalSaverName) {
			t.Fatalf("_portal-saver not present after bootstrap; "+
				"sub-test cannot exercise marker-set defence\n%s",
				fixture.diagnostic())
		}

		fixture.sock.Run(t, "set-option", "-s", state.RestoringMarkerName, "1")

		t.Cleanup(func() {
			_, _ = fixture.sock.TryRun("set-option", "-su", state.RestoringMarkerName)
		})

		pre, err := os.ReadFile(state.SessionsJSON(fixture.stateDir))
		if err != nil {
			t.Fatalf("read sessions.json pre-kill: %v\n%s", err, fixture.diagnostic())
		}

		fixture.sock.Run(t, "kill-session", "-t", tmux.PortalSaverName)

		time.Sleep(markerSetSettle)

		post, err := os.ReadFile(state.SessionsJSON(fixture.stateDir))
		if err != nil {
			t.Fatalf("read sessions.json post-kill: %v\n%s", err, fixture.diagnostic())
		}
		if string(pre) != string(post) {
			t.Fatalf(
				"sessions.json mutated despite @portal-restoring set\n"+
					"--- pre (%d bytes) ---\n%s\n"+
					"--- post (%d bytes) ---\n%s\n%s",
				len(pre), string(pre),
				len(post), string(post),
				fixture.diagnostic(),
			)
		}

		fixture.sock.Run(t, "set-option", "-su", state.RestoringMarkerName)

		fixture.sock.Run(t, "kill-session", "-t", "B")

		ctx, cancel := context.WithTimeout(context.Background(), symptomKillBudget)
		defer cancel()
		if perr := pollSessionsJSON(ctx, fixture.stateDir, []string{"A"}, []string{"B"}); perr != nil {
			t.Fatalf(
				"sessions.json did not reflect post-marker-clear kill of B within %s: %v\n%s",
				symptomKillBudget, perr, fixture.diagnostic(),
			)
		}
	})
}

type symptomFixture struct {
	sock     *tmuxtest.Socket
	stateDir string
	binary   string
	binDir   string
}

func newSymptomFixture(t *testing.T, binary, binDir, sockPrefix string) symptomFixture {
	t.Helper()

	_, stateDir := portaltest.IsolateStateForTest(t)

	// Set before the tmux server is spawned below, so the server inherits it and
	// propagates it to every session-closed hook subprocess. Without it those
	// subprocesses write to the developer's real state dir and the polls below
	// watch a file nothing updates.
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(stateDir, "hooks.json"))
	t.Setenv("PORTAL_PROJECTS_FILE", filepath.Join(stateDir, "projects.json"))
	t.Setenv("PORTAL_ALIASES_FILE", filepath.Join(stateDir, "aliases"))

	// Registered after IsolateStateForTest and before tmuxtest.New so LIFO runs
	// it between kill-server and the TempDir RemoveAll — the saver's shutdown
	// flush and straggler hook subprocesses otherwise race the removal.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	sock := tmuxtest.New(t, sockPrefix)

	// Keeps the server alive after the user sessions are killed.
	sock.Run(t, "new-session", "-d", "-s", "_anchor", "sh", "-c", "sleep infinity")

	sock.Run(t, "new-session", "-d", "-s", "A", "sh", "-c", "sleep infinity")
	sock.Run(t, "new-session", "-d", "-s", "B", "sh", "-c", "sleep infinity")

	fixture := symptomFixture{
		sock:     sock,
		stateDir: stateDir,
		binary:   binary,
		binDir:   binDir,
	}

	runPortalList(t, binary, fixture)

	// Bootstrap does not write sessions.json and the daemon's first commit can
	// lag on a slow host, so drive commit-now directly for a stable pre-kill
	// baseline.
	runPortalCommitNow(t, binary, fixture)

	assertSessionsJSONHas(t, fixture.stateDir, []string{"A", "B"},
		[]string{tmux.PortalSaverName, "_anchor"})

	return fixture
}

func (f symptomFixture) diagnostic() string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- state directory (%s) ---\n", f.stateDir)
	b.WriteString(dumpStateDir(f.stateDir))
	b.WriteString("\n--- raw tmux sessions (test socket) ---\n")
	if out, err := f.sock.TryRun("list-sessions", "-F", "#{session_name}"); err == nil {
		b.WriteString(out)
	} else {
		fmt.Fprintf(&b, "(list-sessions error: %v)\n%s", err, out)
	}
	b.WriteString("\n--- raw tmux panes (test socket) ---\n")
	if out, err := f.sock.TryRun("list-panes", "-a", "-F",
		"#{session_name}:#{window_index}.#{pane_index} #{pane_current_command}"); err == nil {
		b.WriteString(out)
	} else {
		fmt.Fprintf(&b, "(list-panes error: %v)\n%s", err, out)
	}
	return b.String()
}

func runPortalSubprocess(t *testing.T, binary string, f symptomFixture, args ...string) {
	t.Helper()

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("TMUX=%s,%d,0", f.sock.SocketPath(), liveServerPID(t, f.sock)),
		"PORTAL_STATE_DIR="+f.stateDir,
		"PATH="+f.binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("portal %s subprocess failed: %v\n--- output ---\n%s\n%s",
			strings.Join(args, " "), err, string(out), f.diagnostic())
	}
	if testing.Verbose() && len(out) > 0 {
		t.Logf("portal %s subprocess output:\n%s", strings.Join(args, " "), string(out))
	}
}

func runPortalCommitNow(t *testing.T, binary string, f symptomFixture) {
	t.Helper()
	runPortalSubprocess(t, binary, f, "state", "commit-now")
}

// runPortalList triggers the full bootstrap orchestrator from outside the test
// process. `list` has no TUI path, so it exits deterministically without a
// controlling terminal and touches no session state.
func runPortalList(t *testing.T, binary string, f symptomFixture) {
	t.Helper()
	runPortalSubprocess(t, binary, f, "list")
}

// pollSessionsJSON waits for consecutive reads of sessions.json to agree on the
// requested shape. ENOENT and the skip flag reset the counter: both mark the
// pre-write window before the hook subprocess lands a file.
func pollSessionsJSON(ctx context.Context, stateDir string, mustHave, mustOmit []string) error {
	var consecutive int
	ticker := time.NewTicker(reentrancyPollInterval)
	defer ticker.Stop()

	for {
		idx, skip, err := state.ReadIndex(stateDir)
		switch {
		case err != nil && errors.Is(err, fs.ErrNotExist):
			consecutive = 0
		case err != nil:
			return fmt.Errorf("read sessions.json during poll: %w", err)
		case skip:
			consecutive = 0
		default:
			present := sessionNames(idx)
			if matchesShape(present, mustHave, mustOmit) {
				consecutive++
				if consecutive >= reentrancyConsecutiveReads {
					return nil
				}
			} else {
				consecutive = 0
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func matchesShape(present map[string]struct{}, mustHave, mustOmit []string) bool {
	for _, n := range mustHave {
		if _, ok := present[n]; !ok {
			return false
		}
	}
	for _, n := range mustOmit {
		if _, ok := present[n]; ok {
			return false
		}
	}
	return true
}

func assertSessionsJSONHas(t *testing.T, stateDir string, mustHave, mustOmit []string) {
	t.Helper()
	idx, skip, err := state.ReadIndex(stateDir)
	if err != nil || skip {
		t.Fatalf("assert sessions.json shape: skip=%v err=%v\n%s",
			skip, err, dumpStateDir(stateDir))
	}
	present := sessionNames(idx)
	for _, n := range mustHave {
		if _, ok := present[n]; !ok {
			t.Errorf("sessions.json missing %q; present=%v", n, keysOf(present))
		}
	}
	for _, n := range mustOmit {
		if _, ok := present[n]; ok {
			t.Errorf("sessions.json unexpectedly contains %q; present=%v", n, keysOf(present))
		}
	}
}

func liveSessionNames(t *testing.T, sock *tmuxtest.Socket) map[string]struct{} {
	t.Helper()
	out, err := sock.TryRun("list-sessions", "-F", "#{session_name}")
	if err != nil {
		t.Fatalf("list-sessions on test socket: %v\n%s", err, out)
	}
	set := map[string]struct{}{}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		set[line] = struct{}{}
	}
	return set
}

// rawSessionPresent probes tmux directly: Client.ListSessions filters
// underscore-prefixed names, so the Go client cannot see _portal-saver.
func rawSessionPresent(t *testing.T, sock *tmuxtest.Socket, name string) bool {
	t.Helper()
	_, err := sock.TryRun("has-session", "-t", name)
	return err == nil
}

// keysOf is for failure messages only — map iteration order makes the result
// unsuitable to assert on.
func keysOf(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
