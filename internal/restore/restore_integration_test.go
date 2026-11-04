//go:build integration

package restore_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

const (
	restoredSession = "proj-reboot"
	label           = "proj · before the reboot"
	// savedLine is a row of the session's transcript before the reboot.
	savedLine = "saved before the reboot"
	// hook is the session's resume hook, run in the directory it starts in.
	hook     = "touch resumed"
	hookMark = "resumed"
	// saveSettle bounds the wait for the saver's first capture: it polls every
	// 5 s.
	saveSettle = 30 * time.Second
	// helperSettle bounds each wait on the hydrate helper and what it hands
	// the session to.
	helperSettle = 15 * time.Second
)

// rebootWorld is a session a real saver saved, with a label and a resume hook,
// and the boot after a reboot that ended it.
type rebootWorld struct {
	binary string
	saver  *portaltest.SaverDaemon
	// work is where the session ran before the reboot.
	work string
	// saved is the session's entry as the saver left it.
	saved sessionindex.Entry
	// booted is the boot time the processes after the reboot judge by.
	booted string
}

// saveSession runs a session whose transcript holds savedLine, labels it and
// gives it a resume hook of mode as a user would, from inside it, and waits for
// the saver to save its transcript.
func saveSession(t *testing.T, binary, mode string) *rebootWorld {
	t.Helper()
	w := &rebootWorld{binary: binary, saver: portaltest.StartSaverDaemon(t, binary), work: t.TempDir()}
	b := w.saver.Backend
	script := `printf '$ make\r\n` + savedLine + `\r\n$ '; exec sleep 600`
	if _, err := b.Create(t.Context(), restoredSession, w.work, []string{"/bin/sh", "-c", script}); err != nil {
		t.Fatalf("create the session: %v", err)
	}
	w.portal(t, w.inSession(b), "rename", label)
	w.portal(t, w.inSession(b), "hook", "set", "--on-resume", hook, "--resume-mode", mode)
	w.awaitSaved(t)
	return w
}

// inSession is the environment of a command typed in the session, in the zmx
// directory b binds, isolated as the saver is.
func (w *rebootWorld) inSession(b *zmx.Backend, extra ...string) []string {
	return slices.Concat(w.saver.Env, b.ReachEnv(), []string{"ZMX_SESSION=" + restoredSession}, extra)
}

// portal runs the built portal with args in env, and returns what it printed.
func (w *rebootWorld) portal(t *testing.T, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command(w.binary, args...)
	cmd.Env = env
	out, err := cmd.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		t.Fatalf("portal %s: %v\n%s", strings.Join(args, " "), err, exit.Stderr)
	}
	if err != nil {
		t.Fatalf("portal %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

func (w *rebootWorld) awaitSaved(t *testing.T) {
	t.Helper()
	if !harnesstest.PollUntil(t, saveSettle, 50*time.Millisecond, func() bool {
		idx, err := sessionindex.Read(w.saver.StateDir)
		if err != nil {
			return false
		}
		w.saved, _ = idx.Lookup(restoredSession)
		path, err := state.ScrollbackPath(w.saver.StateDir, w.saved)
		if err != nil {
			return false
		}
		transcript, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(transcript), savedLine)
	}) {
		t.Fatalf("the saver never saved the session's transcript; its entry is %+v\n--- portal.log ---\n%s", w.saved, portaltest.ReadPortalLogSafe(w.saver.StateDir))
	}
	if w.saved.Label != label {
		t.Fatalf("the saved entry's label is %q, want %q", w.saved.Label, label)
	}
}

// reboot ends every session, the saver's first so it drops nothing, and
// returns the zmx directory of the boot that follows: empty, with every
// session it makes judging the entries saved as from before the boot.
func (w *rebootWorld) reboot(t *testing.T) *zmx.Backend {
	t.Helper()
	b := w.saver.Backend
	if err := b.Kill(t.Context(), saver.SessionName); err != nil {
		t.Fatalf("end the saver: %v", err)
	}
	if !w.saver.Exited(t, helperSettle) {
		t.Fatal("the daemon outlived its session")
	}
	if err := b.Kill(t.Context(), restoredSession); err != nil {
		t.Fatalf("end the session: %v", err)
	}
	w.booted = state.BootTimeEnv + "=" + time.Now().Format(time.RFC3339Nano)
	return zmxtest.New(t, zmxtest.WithEnv(append(slices.Clone(w.saver.Env), w.booted)...))
}

func (w *rebootWorld) restore(t *testing.T, b *zmx.Backend) {
	t.Helper()
	logger, sink := logtest.NewCaptureLogger(t)
	r, err := restore.New(restore.Config{Backend: b, StateDir: w.saver.StateDir, Exe: w.binary, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	if res, want := r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
		t.Fatalf("Restore = %+v, want %+v\n%s", res, want, sink.Body())
	}
}

// awaitHydrated waits for the helper to lower the session's hydrating flag in
// the test's own state directory, which it can only have found through the
// isolated environment the session was given.
func (w *rebootWorld) awaitHydrated(t *testing.T) {
	t.Helper()
	if !harnesstest.PollUntil(t, helperSettle, 20*time.Millisecond, func() bool {
		up, err := state.Hydrating.Exists(w.saver.StateDir, restoredSession)
		return err == nil && !up
	}) {
		t.Fatalf("the session's hydrating flag was never lowered\n--- portal.log ---\n%s", portaltest.ReadPortalLogSafe(w.saver.StateDir))
	}
}

// checkRestored checks the session is back under its name, where it last
// worked, with its entry recording the new creation and keeping its label,
// which a command typed in it shows, and that the helper replayed the
// transcript.
func (w *rebootWorld) checkRestored(t *testing.T, b *zmx.Backend) {
	t.Helper()
	live, err := b.Session(t.Context(), restoredSession)
	if err != nil {
		t.Fatalf("the session is not back: %v", err)
	}
	if live.Dir != w.saved.Cwd {
		t.Errorf("the session started in %q, want where it last worked, %q", live.Dir, w.saved.Cwd)
	}
	idx, err := sessionindex.Read(w.saver.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := idx.Lookup(restoredSession)
	if !e.SameCreation(live.Created, live.PID) || e.SameCreation(w.saved.CreatedAt, w.saved.PID) {
		t.Errorf("entry records the creation %v pid %d, want the restored session's, %v pid %d", e.CreatedAt, e.PID, live.Created, live.PID)
	}
	if e.Label != label {
		t.Errorf("entry's label = %q, want %q", e.Label, label)
	}
	if shown := strings.TrimSpace(w.portal(t, w.inSession(b, w.booted), "current")); shown != label {
		t.Errorf("portal current = %q, want the label %q", shown, label)
	}
	if log := portaltest.ReadPortalLogSafe(w.saver.StateDir); !strings.Contains(log, "hydrate: scrollback replayed") {
		t.Errorf("the helper logged no replay to the test's state directory\n--- portal.log ---\n%s", log)
	}
}

func (w *rebootWorld) history(t *testing.T, b *zmx.Backend, what string, shows func(string) bool) {
	t.Helper()
	var shown string
	if !harnesstest.PollUntil(t, helperSettle, 20*time.Millisecond, func() bool {
		raw, err := b.History(t.Context(), restoredSession, 0)
		shown = ansi.Strip(string(raw))
		return err == nil && shows(shown)
	}) {
		t.Fatalf("the session never showed %s; it shows %q", what, shown)
	}
}

func (w *rebootWorld) hookRan() bool {
	_, err := os.Stat(filepath.Join(w.work, hookMark))
	return err == nil
}

func TestRestoreAgainstZmx(t *testing.T) {
	binary := portalbintest.Build(t)

	t.Run("an eager hook runs once the transcript is back", func(t *testing.T) {
		w := saveSession(t, binary, "eager")
		b := w.reboot(t)

		w.restore(t, b)

		if !harnesstest.PollUntil(t, helperSettle, 20*time.Millisecond, w.hookRan) {
			t.Fatalf("the resume hook never ran\n--- portal.log ---\n%s", portaltest.ReadPortalLogSafe(w.saver.StateDir))
		}
		w.awaitHydrated(t)
		w.history(t, b, "its transcript", func(shown string) bool { return strings.Contains(shown, savedLine) })
		w.checkRestored(t, b)
	})

	t.Run("a lazy hook parks the session on its panel", func(t *testing.T) {
		w := saveSession(t, binary, "lazy")
		b := w.reboot(t)

		w.restore(t, b)

		w.history(t, b, "its panel", func(shown string) bool { return strings.Contains(shown, hook) })
		w.awaitHydrated(t)
		if up, err := state.ResumePending.Exists(w.saver.StateDir, restoredSession); err != nil || !up {
			t.Errorf("resume-pending raised = %v, %v; want raised while the panel waits", up, err)
		}
		if w.hookRan() {
			t.Error("the resume hook ran before it was asked to")
		}
		w.checkRestored(t, b)
	})
}
