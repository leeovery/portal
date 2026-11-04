//go:build integration

package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/bootid"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

const (
	restoredSession = "proj-r3st"
	restoredLabel   = "proj · restored"
	// savedLine is a line of the session's saved transcript.
	savedLine = "saved before the reboot"
	// altScreenEnter is the sequence a program sends to switch the terminal to
	// its alternate screen. zmx's history of a session on that screen holds
	// it, among the modes it leads with.
	altScreenEnter = "\x1b[?1049h"
)

// restoreWorld is a world part way through a restore: the sessions it makes
// run the hydrate helper, as restore's do, each with its hydrating flag raised
// before it is created.
type restoreWorld struct {
	*portalWorld
	work string
}

func newRestoreWorld(t *testing.T, binary string) *restoreWorld {
	t.Helper()
	return &restoreWorld{portalWorld: newPortalWorld(t, binary), work: realPath(t, t.TempDir())}
}

// register stages hooks.json holding seed, a value for each hook key.
func (w *restoreWorld) register(t *testing.T, seed map[string]any) {
	t.Helper()
	body, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	_, path := hookstest.StageStore(t, hookstest.Staging{SidecarAbsent: true, Seed: string(body)})
	w.env = append(w.env, "PORTAL_HOOKS_FILE="+path)
}

// saveEntry gives the session the entry an earlier save left it: a creation
// of this boot, which only its hydrating flag keeps a claim from replacing,
// and a label.
func (w *restoreWorld) saveEntry(t *testing.T) {
	t.Helper()
	saved := time.Now().Add(-time.Minute).Truncate(time.Second)
	booted, err := bootid.BootTime()
	if err != nil || !saved.After(booted.Add(time.Minute)) {
		t.Fatalf("the saved creation %v must fall well after this boot (%v, %v), or the claim rule adopts it for that alone", saved, booted, err)
	}
	if _, err := sessionindex.Update(w.stateDir, "cli", func(idx *sessionindex.Index) error {
		idx.Claim(restoredSession, w.work, saved, 0, state.ClaimRule(w.stateDir))
		return idx.SetLabel(restoredSession, restoredLabel)
	}); err != nil {
		t.Fatalf("save the session's entry: %v", err)
	}
}

// restore creates the session name running the helper on transcript, as
// restore does: its hydrating flag raised first.
func (w *restoreWorld) restore(t *testing.T, name string, transcript []byte) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "scrollback")
	if err := os.WriteFile(file, transcript, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := state.Hydrating.Set(w.stateDir, name); err != nil {
		t.Fatalf("raise the session's hydrating flag: %v", err)
	}
	// env -i keeps the helper to the world's environment, and would clear the
	// ZMX_SESSION zmx gives the session, so the session's shell passes that on.
	command := []string{"/bin/sh", "-c", `exec /usr/bin/env -i ZMX_SESSION="$ZMX_SESSION" "$@"`, "sh"}
	command = append(command, w.env...)
	command = append(command, w.binary, "state", "hydrate", "--session", name, "--file", file)
	if _, err := w.b.Create(t.Context(), name, w.work, command); err != nil {
		t.Fatalf("create the session: %v", err)
	}
}

func (w *restoreWorld) awaitHydrated(t *testing.T, name string) {
	t.Helper()
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		up, err := state.Hydrating.Exists(w.stateDir, name)
		return err == nil && !up
	}) {
		t.Fatal("the session's hydrating flag was never lowered")
	}
}

// awaitHistory waits for the session's history to satisfy shows, and returns
// it as the backend gave it.
func (w *restoreWorld) awaitHistory(t *testing.T, name, what string, shows func(history string) bool) string {
	t.Helper()
	var history string
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		raw, err := w.b.History(t.Context(), name, 0)
		history = string(raw)
		return err == nil && shows(history)
	}) {
		t.Fatalf("session %s never showed %s; its history is %q", name, what, history)
	}
	return history
}

// awaitLoginShell waits for the session's process to be its login shell, as
// zmx starts one: $SHELL with a dash before its name.
func (w *restoreWorld) awaitLoginShell(t *testing.T, name string) {
	t.Helper()
	var argv []string
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		s, err := w.b.Session(t.Context(), name)
		if err != nil {
			return false
		}
		args, err := proctable.ReadArgs(s.PID)
		argv = args.Argv
		return err == nil && len(argv) == 1 && argv[0] == "-sh"
	}) {
		t.Fatalf("session %s runs %q, want its login shell [-sh]", name, argv)
	}
}

// creationRecorded checks the session's entry records the live session's
// creation and kept what it was saved with.
func (w *restoreWorld) creationRecorded(t *testing.T) {
	t.Helper()
	live, err := w.b.Session(t.Context(), restoredSession)
	if err != nil {
		t.Fatalf("read the session: %v", err)
	}
	idx, err := sessionindex.Read(w.stateDir)
	if err != nil {
		t.Fatalf("read the index: %v", err)
	}
	e, ok := idx.Lookup(restoredSession)
	switch {
	case !ok:
		t.Fatal("the session's entry is gone")
	case !e.SameCreation(live.Created, live.PID):
		t.Errorf("the entry records creation %v pid %d, want the live session's, %v pid %d", e.CreatedAt, e.PID, live.Created, live.PID)
	case e.Label != restoredLabel:
		t.Errorf("label = %q, want %q kept: the claim replaced the entry rather than adopting it", e.Label, restoredLabel)
	}
}

func (w *restoreWorld) hookRan() bool {
	_, err := os.Stat(filepath.Join(w.work, hookMark))
	return err == nil
}

func showsSavedLine(history string) bool { return strings.Contains(ansi.Strip(history), savedLine) }

func TestHydrateAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)
	transcript := []byte("$ make\r\n" + savedLine + "\r\n")

	t.Run("a session with no resume command shows its transcript at its login shell", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		w.saveEntry(t)

		w.restore(t, restoredSession, transcript)

		w.awaitHydrated(t, restoredSession)
		w.awaitHistory(t, restoredSession, "its transcript", showsSavedLine)
		w.awaitLoginShell(t, restoredSession)
		w.creationRecorded(t)
	})

	t.Run("an eager resume command runs after the transcript", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		w.register(t, map[string]any{restoredSession: map[string]any{
			hooks.EventOnResume.String(): map[string]string{"command": "touch " + hookMark, "resume": "eager"},
		}})
		w.saveEntry(t)

		w.restore(t, restoredSession, transcript)

		if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, w.hookRan) {
			t.Fatal("the resume command never ran")
		}
		w.awaitHydrated(t, restoredSession)
		w.awaitHistory(t, restoredSession, "its transcript", showsSavedLine)
		w.creationRecorded(t)
	})

	t.Run("a lazy resume command parks the session on its panel", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		w.register(t, map[string]any{restoredSession: map[string]string{hooks.EventOnResume.String(): "touch " + hookMark}})
		w.saveEntry(t)

		w.restore(t, restoredSession, transcript)

		panel := w.awaitHistory(t, restoredSession, "its panel", func(history string) bool {
			return strings.Contains(ansi.Strip(history), hookMark)
		})
		w.awaitHydrated(t, restoredSession)
		if up, err := state.ResumePending.Exists(w.stateDir, restoredSession); err != nil || !up {
			t.Errorf("resume-pending raised = %v, %v; want raised while the panel waits", up, err)
		}
		if w.hookRan() {
			t.Error("the resume command ran before Enter")
		}
		// The panel sits on the alternate screen, which is all a capture of the
		// session would hold, so the saver skips waiting sessions.
		if !strings.Contains(panel, altScreenEnter) || showsSavedLine(panel) {
			t.Errorf("history = %q, want the panel alone, on the alternate screen", panel)
		}
		w.creationRecorded(t)
	})
}

// A session whose program has switched to the alternate screen, as an editor
// or a pager does, saves that screen alone. Replayed, it is drawn on the
// alternate screen and gone when the helper leaves it, and the restored
// session is at a usable shell.
func TestHydrateAltScreenTranscriptAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)
	w := newRestoreWorld(t, binary)
	const (
		mainLine = "on the main screen"
		altLine  = "on the alternate screen"
	)
	program := `printf '` + mainLine + `\n'; printf '\033[?1049h\033[H\033[2J` + altLine + `'; exec sleep 600`
	if _, err := w.b.Create(t.Context(), "editing", w.work, []string{"/bin/sh", "-c", program}); err != nil {
		t.Fatalf("create the editing session: %v", err)
	}

	saved := w.awaitHistory(t, "editing", "its alternate screen", func(history string) bool {
		return strings.Contains(history, altLine)
	})
	if !strings.Contains(saved, altScreenEnter) || strings.Contains(saved, mainLine) {
		t.Fatalf("history = %q, want the alternate screen alone, led by its switch to it", saved)
	}

	w.restore(t, restoredSession, []byte(saved))
	w.awaitHydrated(t, restoredSession)
	w.awaitLoginShell(t, restoredSession)

	restored := w.awaitHistory(t, restoredSession, "its main screen", func(string) bool { return true })
	if strings.Contains(restored, altScreenEnter) || strings.Contains(restored, altLine) {
		t.Errorf("history = %q, want the main screen, the replayed alternate screen left", restored)
	}
	c := zmxtest.Attach(t, w.b, restoredSession)
	w.awaitClients(t, restoredSession, 1)
	c.Type(t, "echo usable-$((40+2))\r")
	w.awaitHistory(t, restoredSession, "the shell's answer", func(history string) bool {
		return strings.Contains(ansi.Strip(history), "usable-42")
	})
}
