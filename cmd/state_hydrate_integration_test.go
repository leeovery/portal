//go:build integration

package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"github.com/leeovery/portal/cmd"
	"github.com/leeovery/portal/internal/bootid"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/vtcapture"
	"github.com/leeovery/portal/internal/zmxtest"
)

const (
	restoredSession = "proj-r3st"
	restoredLabel   = "proj · restored"
	// savedLine is a row of the session's saved transcript.
	savedLine = "saved before the reboot"
	// promptMark is the row a world's login shell draws its prompt as.
	promptMark = "prompt-mark$"
	// altScreenEnter is the sequence a program sends to switch the terminal to
	// its alternate screen. zmx's history of a session on that screen holds
	// it, among the modes it leads with.
	altScreenEnter = "\x1b[?1049h"
)

// restoreWorld is a world part way through a restore: the sessions it makes
// run the hydrate helper, as restore's do, each with its hydrating flag raised
// before it is created. Its login shell's prompt is promptMark.
type restoreWorld struct {
	*portalWorld
	work string
}

func newRestoreWorld(t *testing.T, binary string) *restoreWorld {
	t.Helper()
	w := &restoreWorld{portalWorld: newPortalWorld(t, binary), work: realPath(t, t.TempDir())}
	// An interactive sh reads $ENV last, after the login files that set the
	// system's prompt.
	env := filepath.Join(t.TempDir(), "env.sh")
	if err := os.WriteFile(env, []byte("PS1='"+promptMark+" '\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w.env = append(w.env, "ENV="+env)
	return w
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
		if _, err := idx.Claim(restoredSession, w.work, saved, 0, state.ClaimRule(w.stateDir, w.b.ValidateName)); err != nil {
			return err
		}
		return idx.SetLabel(restoredSession, restoredLabel)
	}); err != nil {
		t.Fatalf("save the session's entry: %v", err)
	}
}

// capture runs script in a session of its own until its screen shows last,
// and returns the session's History: the transcript a save of it writes.
func (w *restoreWorld) capture(t *testing.T, name, script, last string) []byte {
	t.Helper()
	if _, err := w.b.Create(t.Context(), name, w.work, []string{"/bin/sh", "-c", script + "; exec sleep 600"}); err != nil {
		t.Fatalf("create the session to save: %v", err)
	}
	return []byte(w.awaitHistory(t, name, "what it saves", func(history string) bool {
		return strings.Contains(ansi.Strip(history), last)
	}))
}

// run creates the session name running prelude, then argv in the world's
// environment, isolated. env -i would clear the ZMX_SESSION zmx gives the
// session, so the session's shell passes that on.
func (w *restoreWorld) run(t *testing.T, name, prelude string, argv ...string) {
	t.Helper()
	script := `exec /usr/bin/env -i ZMX_SESSION="$ZMX_SESSION" "$@"`
	if prelude != "" {
		script = prelude + "; " + script
	}
	command := append([]string{"/bin/sh", "-c", script, "sh"}, w.env...)
	command = append(command, argv...)
	if _, err := w.b.Create(t.Context(), name, w.work, command); err != nil {
		t.Fatalf("create the session: %v", err)
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
	w.run(t, name, "", w.binary, "state", "hydrate", "--session", name, "--file", file)
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

// screenRows is a history's rows as its session shows them, escapes and
// trailing blanks gone.
func screenRows(history string) []string {
	rows := strings.Split(ansi.Strip(history), "\r\n")
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, " ")
	}
	return rows
}

// setModes is the DEC private modes a capture's lead has set: those its
// terminal had on.
func setModes(capture []byte) []string {
	modes, _ := vtcapture.Modes(capture)
	var set []string
	for _, m := range modes {
		if m.Private && m.Set {
			set = append(set, m.Param)
		}
	}
	return set
}

// awaitPromptBelow waits for the session's shell to draw its prompt, then
// checks it drew it on the row below the last of rows, every one of them
// intact above it.
func (w *restoreWorld) awaitPromptBelow(t *testing.T, name string, rows []string) {
	t.Helper()
	shown := screenRows(w.awaitHistory(t, name, "its prompt", func(history string) bool {
		return slices.Contains(screenRows(history), promptMark)
	}))
	if want := append(slices.Clone(rows), promptMark); !slices.Equal(shown, want) {
		t.Errorf("session %s shows %q, want %q: every row intact, the prompt below the last", name, shown, want)
	}
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

// shownRows is what a saved transcript showed, row by row.
func shownRows(transcript []byte) []string { return screenRows(string(transcript)) }

func showsSavedLine(history string) bool { return strings.Contains(ansi.Strip(history), savedLine) }

func TestHydrateAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)
	const saveScript = `printf '$ make\r\n` + savedLine + `\r\n$ '`

	t.Run("a session with no resume command shows its transcript at its login shell", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		transcript := w.capture(t, "saved", saveScript, savedLine)
		w.saveEntry(t)

		w.restore(t, restoredSession, transcript)

		w.awaitHydrated(t, restoredSession)
		w.awaitPromptBelow(t, restoredSession, shownRows(transcript))
		w.awaitLoginShell(t, restoredSession)
		w.creationRecorded(t)
	})

	t.Run("an eager resume command runs after the transcript", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		transcript := w.capture(t, "saved", saveScript, savedLine)
		w.register(t, map[string]any{restoredSession: map[string]any{
			hooks.EventOnResume.String(): map[string]string{"command": "touch " + hookMark, "resume": "eager"},
		}})
		w.saveEntry(t)

		w.restore(t, restoredSession, transcript)

		if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, w.hookRan) {
			t.Fatal("the resume command never ran")
		}
		w.awaitHydrated(t, restoredSession)
		w.awaitPromptBelow(t, restoredSession, shownRows(transcript))
		w.creationRecorded(t)
	})

	t.Run("a lazy resume command parks the session on its panel", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		transcript := w.capture(t, "saved", saveScript, savedLine)
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

// A save places the cursor on the screen it captured, so a replay that kept
// that placement would leave the prompt among the rows whenever they land
// elsewhere: after a clear that kept the scrollback, or in a taller terminal.
func TestHydrateReplaysWhateverTheScreenAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)

	t.Run("the prompt lands below a transcript saved after a clear that kept the scrollback", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		// Ctrl-L in bash or zsh: home, then erase the screen.
		script := `i=1; while [ $i -le 40 ]; do printf 'line-%d\r\n' $i; i=$((i+1)); done; printf '\033[H\033[2J$ ls\r\nafter-clear-output\r\n$ '`
		transcript := w.capture(t, "cleared", script, "after-clear-output")
		if !strings.HasPrefix(ansi.Strip(string(transcript)), "line-1\r\n") || !strings.HasSuffix(string(transcript), "\x1b[3;3H\x1b[0m") {
			t.Fatalf("the saved transcript %q is not scrollback above a cleared screen, its cursor on that screen's third row", transcript)
		}

		w.restore(t, restoredSession, transcript)

		w.awaitPromptBelow(t, restoredSession, shownRows(transcript))
	})

	// A program such as Claude Code leaves focus reporting on, and with it a
	// shell that read it would type ^[[I and ^[[O at its prompt on every
	// focus change.
	t.Run("the modes the saved program left on are off in the restored session", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		script := `printf '\033[?1004h\033[?1000h$ claude\r\nworking on it\r\n'`
		transcript := w.capture(t, "focused", script, "working on it")
		if left := setModes(transcript); !slices.Contains(left, "1004") || !slices.Contains(left, "1000") {
			t.Fatalf("the saved transcript %q leads with %q, want focus and mouse reporting set", transcript, left)
		}

		w.restore(t, restoredSession, transcript)

		w.awaitPromptBelow(t, restoredSession, shownRows(transcript))
		restored, err := w.b.History(t.Context(), restoredSession, 0)
		if err != nil {
			t.Fatal(err)
		}
		if on := setModes(restored); slices.Contains(on, "1004") || slices.Contains(on, "1000") {
			t.Errorf("the restored session leads with %q: the saved program's modes are on in its shell", on)
		}
	})

	t.Run("the prompt lands below a transcript replayed into a taller terminal", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		script := `until [ "$(stty size)" = "10 120" ]; do sleep 0.05; done; i=1; while [ $i -le 30 ]; do printf 'line-%d\r\n' $i; i=$((i+1)); done; printf '$ '`
		if _, err := w.b.Create(t.Context(), "short", w.work, []string{"/bin/sh", "-c", script + "; exec sleep 600"}); err != nil {
			t.Fatalf("create the session to save: %v", err)
		}
		attach, err := w.b.AttachCommand(t.Context(), "short")
		if err != nil {
			t.Fatal(err)
		}
		client := w.startIn(t, attach.Env, attach.Argv...)
		w.awaitClients(t, "short", 1)
		if err := pty.Setsize(client.tty, &pty.Winsize{Rows: 10, Cols: 120}); err != nil {
			t.Fatalf("shrink the saved session's terminal: %v", err)
		}
		transcript := []byte(w.awaitHistory(t, "short", "its rows", func(history string) bool {
			return strings.Contains(history, "line-30")
		}))
		if !strings.HasSuffix(string(transcript), "\x1b[10;3H\x1b[0m") {
			t.Fatalf("the saved transcript %q does not leave its cursor on the tenth row of its terminal", transcript)
		}

		w.restore(t, restoredSession, transcript)

		w.awaitPromptBelow(t, restoredSession, shownRows(transcript))
	})
}

// The resume chain's reset leaves the panel's screen, but runs on the main
// screen too when the panel was never drawn. There it must leave the cursor
// where the rows above left it.
func TestResumeResetOnTheMainScreenAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)
	const rows = `printf 'row-1\r\nrow-2\r\n'`

	t.Run("the tail's prompt lands below the rows before it", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		if err := state.ResumePending.Set(w.stateDir, restoredSession); err != nil {
			t.Fatal(err)
		}

		w.run(t, restoredSession, rows, binary, "state", "resume-recover", "--session", restoredSession)

		w.awaitPromptBelow(t, restoredSession, []string{"row-1", "row-2"})
	})

	t.Run("the backstop's prompt lands below the errors of a chain whose binary is gone", func(t *testing.T) {
		w := newRestoreWorld(t, binary)
		if err := state.ResumePending.Set(w.stateDir, restoredSession); err != nil {
			t.Fatal(err)
		}
		flag, err := state.ResumePending.Path(w.stateDir, restoredSession)
		if err != nil {
			t.Fatal(err)
		}
		// A short path, so each step's error fits a row.
		chain := cmd.ParkedResumeChain("/nonexistent-portal", flag, "touch "+hookMark, restoredSession)

		w.run(t, restoredSession, rows, "/bin/sh", "-c", chain)

		shown := screenRows(w.awaitHistory(t, restoredSession, "its prompt", func(history string) bool {
			return slices.Contains(screenRows(history), promptMark)
		}))
		errors := shown[2 : len(shown)-1]
		if !slices.Equal(shown[:2], []string{"row-1", "row-2"}) || len(errors) != 2 || shown[len(shown)-1] != promptMark {
			t.Fatalf("session shows %q, want the rows, the draw's and the tail's errors, then the prompt", shown)
		}
		for _, line := range errors {
			if !strings.Contains(line, "No such file or directory") {
				t.Errorf("row %q, want a chain step's error intact", line)
			}
		}
	})
}

// A session whose program has switched to the alternate screen, as an editor
// or a pager does, saves that screen alone. The saver skips such a capture,
// but one saved before it is replayed as any capture is: its rows alone, on
// the main screen, with the restored session at a usable shell below them.
func TestHydrateAltScreenTranscriptAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)
	w := newRestoreWorld(t, binary)
	const (
		mainLine = "on the main screen"
		altLine  = "on the alternate screen"
	)
	program := `printf '` + mainLine + `\n'; printf '\033[?1049h\033[H\033[2J` + altLine + `'`
	saved := w.capture(t, "editing", program, altLine)
	if !strings.Contains(string(saved), altScreenEnter) || strings.Contains(string(saved), mainLine) {
		t.Fatalf("history = %q, want the alternate screen alone, led by its switch to it", saved)
	}

	w.restore(t, restoredSession, saved)
	w.awaitHydrated(t, restoredSession)
	w.awaitLoginShell(t, restoredSession)

	restored := w.awaitHistory(t, restoredSession, "its prompt", func(history string) bool {
		return slices.Contains(screenRows(history), promptMark)
	})
	if strings.Contains(restored, altScreenEnter) {
		t.Errorf("history = %q, want the main screen", restored)
	}
	w.awaitPromptBelow(t, restoredSession, []string{altLine})
	c := zmxtest.Attach(t, w.b, restoredSession)
	w.awaitClients(t, restoredSession, 1)
	c.Type(t, "echo usable-$((40+2))\r")
	w.awaitHistory(t, restoredSession, "the shell's answer", func(history string) bool {
		return strings.Contains(ansi.Strip(history), "usable-42")
	})
}
