//go:build integration

package restore_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/commitlock"
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
	// dropSettle outlasts the saver's drop of an entry missing from its
	// listings, 10 s apart, and a poll.
	dropSettle = 16 * time.Second
	// heldSettle bounds this process's wait for a commit lock the test holds.
	heldSettle = 30 * time.Second
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
	// booted is the boot time the processes after the reboot judge by, as an
	// environment entry.
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
	return slices.Concat(w.bootEnv(), b.ReachEnv(), []string{"ZMX_SESSION=" + restoredSession}, extra)
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

// reboot ends every session and returns the zmx directory of the boot that
// follows: empty, with every session it makes, and this process, judging the
// entries saved as from before the boot.
func (w *rebootWorld) reboot(t *testing.T) *zmx.Backend {
	t.Helper()
	w.endSessions(t)
	booted := time.Now().Format(time.RFC3339Nano)
	t.Setenv(state.BootTimeEnv, booted)
	w.booted = state.BootTimeEnv + "=" + booted
	return zmxtest.New(t, zmxtest.WithEnv(w.bootEnv()...))
}

// logout ends every session, as a logout or a crash does, and returns the
// empty zmx directory sessions are made in next, within the same boot.
func (w *rebootWorld) logout(t *testing.T) *zmx.Backend {
	t.Helper()
	w.endSessions(t)
	return zmxtest.New(t, zmxtest.WithEnv(w.bootEnv()...))
}

// endSessions ends the saver and the session, the saver first so it drops
// nothing.
func (w *rebootWorld) endSessions(t *testing.T) {
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
}

// bootEnv is what the processes of the boot run with: the saver's
// environment, and the boot a reboot moved, if one did.
func (w *rebootWorld) bootEnv() []string {
	env := slices.Clone(w.saver.Env)
	if w.booted != "" {
		env = append(env, w.booted)
	}
	return env
}

// restorer is a Restorer over b and the saved state, logging to a sink of its
// own.
func (w *rebootWorld) restorer(t *testing.T, b *zmx.Backend) (*restore.Restorer, *logtest.Sink) {
	t.Helper()
	logger, sink := logtest.NewCaptureLogger(t)
	r, err := restore.New(restore.Config{Backend: b, StateDir: w.saver.StateDir, Exe: w.binary, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	return r, sink
}

func (w *rebootWorld) restore(t *testing.T, b *zmx.Backend) {
	t.Helper()
	r, sink := w.restorer(t, b)
	if res, want := r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
		t.Fatalf("Restore = %+v, want %+v\n%s", res, want, sink.Body())
	}
}

// restoreBehind runs the restore while this process holds the commit lock,
// which it releases once release returns, and checks the restore made the
// session.
func (w *rebootWorld) restoreBehind(t *testing.T, b *zmx.Backend, release func()) {
	t.Helper()
	r, sink := w.restorer(t, b)
	commitlock.SetTimeoutForTest(t, heldSettle)
	var running sync.WaitGroup
	// Ahead of the hold's own cleanup, so it waits once the lock is free.
	t.Cleanup(running.Wait)
	held := commitlock.HoldForTest(t, w.saver.StateDir)
	var res restore.Result
	running.Go(func() { res = r.Restore(t.Context()) })
	release()
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	running.Wait()
	if want := (restore.Result{Restored: 1}); res != want {
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
	if shown := strings.TrimSpace(w.portal(t, w.inSession(b), "current")); shown != label {
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

// awaitLive waits for the session to be back, and returns it as it answers.
func (w *rebootWorld) awaitLive(t *testing.T, b *zmx.Backend) backend.Session {
	t.Helper()
	var live backend.Session
	if !harnesstest.PollUntil(t, helperSettle, 20*time.Millisecond, func() bool {
		var err error
		live, err = b.Session(t.Context(), restoredSession)
		return err == nil && !live.Created.IsZero()
	}) {
		t.Fatal("the session never came back")
	}
	return live
}

// transcriptOf is where the saver saves the transcript of the session's
// creation live.
func (w *rebootWorld) transcriptOf(t *testing.T, live backend.Session) string {
	t.Helper()
	file, err := state.SessionScrollbackFile(live.Name, live.Created, live.PID)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(w.saver.StateDir, filepath.FromSlash(file))
}

// awaitHydratedUnsaved waits for the helper to lower the session's hydrating
// flag, failing if a saver wrote the transcript at path, the restored
// creation's, while it was raised. Each look reads the file before the flag,
// so a file found with the flag still up was written before the flag came
// down, which nothing raises again.
func (w *rebootWorld) awaitHydratedUnsaved(t *testing.T, path string) {
	t.Helper()
	early := false
	if !harnesstest.PollUntil(t, helperSettle, 20*time.Millisecond, func() bool {
		_, err := os.Stat(path)
		written := err == nil
		up, err := state.Hydrating.Exists(w.saver.StateDir, restoredSession)
		early = err == nil && up && written
		return early || err == nil && !up
	}) {
		t.Fatalf("the session's hydrating flag was never lowered\n--- portal.log ---\n%s", portaltest.ReadPortalLogSafe(w.saver.StateDir))
	}
	if early {
		t.Fatal("the saver saved the session's screen while the helper was replaying its transcript onto it")
	}
}

// awaitSavedAs waits for the session's entry to name the transcript at path,
// holding savedLine.
func (w *rebootWorld) awaitSavedAs(t *testing.T, path string) {
	t.Helper()
	var e sessionindex.Entry
	if !harnesstest.PollUntil(t, saveSettle, 50*time.Millisecond, func() bool {
		idx, err := sessionindex.Read(w.saver.StateDir)
		if err != nil {
			return false
		}
		e, _ = idx.Lookup(restoredSession)
		named, err := state.ScrollbackPath(w.saver.StateDir, e)
		if err != nil || named != path {
			return false
		}
		transcript, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(transcript), savedLine)
	}) {
		t.Fatalf("the saver never saved the restored session's transcript at %s; its entry is %+v\n--- portal.log ---\n%s", path, e, portaltest.ReadPortalLogSafe(w.saver.StateDir))
	}
}

// checkNeitherDroppedNorReplaced checks no saver logged the session's entry
// dropped or replaced.
func (w *rebootWorld) checkNeitherDroppedNorReplaced(t *testing.T) {
	t.Helper()
	log := portaltest.ReadPortalLogSafe(w.saver.StateDir)
	for _, op := range []string{"rm", "replace"} {
		if strings.Contains(log, "op="+op+" session="+restoredSession+" ") {
			t.Errorf("a saver logged the session's entry %s\n--- portal.log ---\n%s", op, log)
		}
	}
}

// answerPanel attaches a client to the session and answers its resume panel
// with Enter, which runs the hook.
func (w *rebootWorld) answerPanel(t *testing.T, b *zmx.Backend) {
	t.Helper()
	c := zmxtest.Attach(t, b, restoredSession)
	if !harnesstest.PollUntil(t, helperSettle, 20*time.Millisecond, func() bool {
		s, err := b.Session(t.Context(), restoredSession)
		return err == nil && s.Clients == 1
	}) {
		t.Fatal("the client never attached")
	}
	c.Type(t, "\r")
	if !harnesstest.PollUntil(t, helperSettle, 20*time.Millisecond, w.hookRan) {
		t.Fatalf("Enter on the panel never ran the resume hook\n--- portal.log ---\n%s", portaltest.ReadPortalLogSafe(w.saver.StateDir))
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

	// decisions 58 and 62: a logout, every zmx daemon dying or a crash ends
	// the saver with the sessions, so no saver saw them end, and the bootstrap
	// that finds none marks this boot's owed and brings them back with the
	// rest.
	t.Run("with no saver running, a session of this boot marked owed comes back", func(t *testing.T) {
		w := saveSession(t, binary, "eager")
		b := w.logout(t)
		r, sink := w.restorer(t, b)
		if res := r.Restore(t.Context()); res != (restore.Result{}) {
			t.Fatalf("Restore = %+v, want this boot's session left to the saver that ran\n%s", res, sink.Body())
		}

		if err := r.MarkOwed(t.Context()); err != nil {
			t.Fatalf("MarkOwed: %v", err)
		}

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

		w.answerPanel(t, b)

		w.history(t, b, "its transcript, under the panel", func(shown string) bool { return strings.Contains(shown, savedLine) })
		if up, err := state.ResumePending.Exists(w.saver.StateDir, restoredSession); err != nil || up {
			t.Errorf("resume-pending raised = %v, %v; want lowered once the panel is answered", up, err)
		}
		w.checkRestored(t, b)
	})

	// The saver of the boot after the reboot runs before restore does, as a
	// bootstrap starts it, over the state the saver before it kept. It must
	// keep the saved entry, which predates its boot, take the restored
	// session's creation over rather than replace the entry, and save nothing
	// of the session's screen while the helper replays the transcript onto
	// it. The restore runs behind a commit this process holds, as one landing
	// beside it would, which keeps the helper from recording the creation, and
	// so from lowering the flag, for the helper's 5 s lock bound: a poll's
	// span, so one of the saver's polls finds the session hydrating, unless it
	// listed in the moment before restore made the session.
	t.Run("a saver running across the reboot keeps the session and saves it once hydrated", func(t *testing.T) {
		w := saveSession(t, binary, "eager")
		b := w.reboot(t)
		portaltest.StartSaverDaemonIn(t, w.binary, portaltest.SaverDir{Backend: b, Env: w.bootEnv(), StateDir: w.saver.StateDir})
		if harnesstest.PollUntil(t, dropSettle, 100*time.Millisecond, func() bool {
			idx, err := sessionindex.Read(w.saver.StateDir)
			if err != nil {
				return false
			}
			_, saved := idx.Lookup(restoredSession)
			return !saved
		}) {
			t.Fatalf("the saver dropped the saved entry before restore\n--- portal.log ---\n%s", portaltest.ReadPortalLogSafe(w.saver.StateDir))
		}

		var transcript string
		w.restoreBehind(t, b, func() {
			transcript = w.transcriptOf(t, w.awaitLive(t, b))
			w.awaitHydratedUnsaved(t, transcript)
		})

		w.awaitSavedAs(t, transcript)
		w.checkRestored(t, b)
		w.checkNeitherDroppedNorReplaced(t)
	})
}
