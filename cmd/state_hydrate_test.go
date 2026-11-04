package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// hydrateSession is the session every hydrate case restores, as restore names
// it in the helper's argv and zmx names it in the environment.
const hydrateSession = "proj-a1b2"

// hydrateWorld is what a hydrate case runs in, as restore leaves it: a state
// directory of the case's own holding the session's hydrating flag, raised,
// and a backend holding the session, live under a creation of this boot.
type hydrateWorld struct {
	stateDir string
	backend  *backendtest.Fake
	live     backend.Session
}

func newHydrateWorld(t *testing.T) *hydrateWorld {
	t.Helper()
	w := &hydrateWorld{
		stateDir: resumeStateDir(t),
		live:     backend.Session{Name: hydrateSession, Created: testBoot.Add(time.Hour), PID: 4242},
	}
	w.backend = backendtest.New(w.live)
	withBackendDeps(t, BackendDeps{Backend: w.backend})
	if err := state.Hydrating.Set(w.stateDir, hydrateSession); err != nil {
		t.Fatalf("raise the session's hydrating flag: %v", err)
	}
	return w
}

// config is the helper's config for the world's session, run in it, with
// nothing to replay: a case names the transcript, the registration and
// whatever else it is about. Its settle is instant, so no case waits on it
// but the one about it.
func (w *hydrateWorld) config(exec *stubExecShell) hydrateConfig {
	return hydrateConfig{
		Session:           hydrateSession,
		Current:           hydrateSession,
		Stdout:            io.Discard,
		ExecShell:         exec.fn(),
		DisableTTYSignals: func() error { return nil },
		Settle:            func() {},
	}
}

func (w *hydrateWorld) hydrating(t *testing.T) bool {
	t.Helper()
	raised, err := state.Hydrating.Exists(w.stateDir, hydrateSession)
	if err != nil {
		t.Fatalf("look for the session's hydrating flag: %v", err)
	}
	return raised
}

func (w *hydrateWorld) seedIndex(t *testing.T, entries ...sessionindex.Entry) {
	t.Helper()
	if err := os.WriteFile(sessionindex.Path(w.stateDir), indexJSON(t, entries), 0o600); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
}

// entry is the session's index entry as sessions.json holds it now.
func (w *hydrateWorld) entry(t *testing.T) (sessionindex.Entry, bool) {
	t.Helper()
	idx, err := sessionindex.Read(w.stateDir)
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	return idx.Lookup(hydrateSession)
}

// recordsLiveCreation reports whether the session's entry records the
// creation its backend reports.
func (w *hydrateWorld) recordsLiveCreation(t *testing.T) bool {
	t.Helper()
	e, ok := w.entry(t)
	return ok && e.CreatedAt.Equal(w.live.Created) && e.PID == w.live.PID
}

// transcript writes body as a saved scrollback file and returns its path.
func transcript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scrollback.bin")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("seed the scrollback file: %v", err)
	}
	return path
}

// unreadableTranscript is a saved scrollback file the helper can't open.
func unreadableTranscript(t *testing.T, body string) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses 0o000 mode bits")
	}
	path := transcript(t, body)
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod 0: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	return path
}

type stubExecShell struct {
	mu     sync.Mutex
	called bool
	target string
	args   []string
}

func (s *stubExecShell) fn() func(string, []string) {
	return func(prog string, args []string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.called = true
		s.target = prog
		s.args = args
	}
}

// hydrateStoreWithMode stages a hooks.json holding one on-resume registration
// under the named resume mode. resumemode.Unset stages the bare-command shape,
// which names no mode of its own and so follows the install.
func hydrateStoreWithMode(t *testing.T, key, command string, mode resumemode.Mode) *hooks.Store {
	t.Helper()
	var value any = command
	if mode != resumemode.Unset {
		value = map[string]string{"command": command, "resume": mode.String()}
	}
	seed, err := json.Marshal(map[string]map[string]any{key: {hooks.EventOnResume.String(): value}})
	if err != nil {
		t.Fatalf("marshal seed hooks.json: %v", err)
	}
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Seed: string(seed)})
	return store
}

func emptyHookStore(t *testing.T) *hooks.Store {
	t.Helper()
	store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Body: map[string]map[string]string{}})
	return store
}

func TestHydrate_Replay(t *testing.T) {
	t.Run("it writes the transcript verbatim between the reset sequences", func(t *testing.T) {
		w := newHydrateWorld(t)
		body := "line1\nline2\r\nline3\x00\xff\x1b[31mred\x1b[0m\x1b[1mbold\x1b[0m"
		stdout := new(bytes.Buffer)
		cfg := w.config(&stubExecShell{})
		cfg.File, cfg.Stdout = transcript(t, body), stdout

		runHydrate(t.Context(), cfg)

		if want := hydrateResetPreamble + body + hydrateResetPostamble; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
		if !strings.HasSuffix(hydrateResetPostamble, "\r\n") {
			t.Errorf("postamble %q does not end on CRLF, so the prompt would not land at column 0", hydrateResetPostamble)
		}
	})

	// A reset written on the main screen restores its saved cursor; one saved
	// first leaves the cursor where it stands.
	t.Run("each reset saves the cursor before it leaves the alternate screen", func(t *testing.T) {
		for name, reset := range map[string]string{"preamble": hydrateResetPreamble, "postamble": hydrateResetPostamble} {
			saved, left := strings.Index(reset, "\x1b7"), strings.Index(reset, "\x1b[?1049l")
			if saved < 0 || saved > left {
				t.Errorf("%s %q leaves the alternate screen with no cursor saved first", name, reset)
			}
		}
	})

	t.Run("it replays a capture's rows alone, without the modes its program left on or the state zmx closed it with", func(t *testing.T) {
		w := newHydrateWorld(t)
		const rows = "line-1\r\n\x1b[1mline-2\x1b[0m\r\n$ ls\r\n$"
		stdout := new(bytes.Buffer)
		cfg := w.config(&stubExecShell{})
		cfg.File, cfg.Stdout = transcript(t, "\x1b[?1000h\x1b[?1004h\x1b[?2004h"+rows+"\x1b[3;3H\x1b[0m\x1b]7;file://host/work\x1b\\"), stdout

		runHydrate(t.Context(), cfg)

		if want := hydrateResetPreamble + rows + hydrateResetPostamble; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	})

	t.Run("it streams a large transcript whole, but for the modes opening it and the state closing it", func(t *testing.T) {
		w := newHydrateWorld(t)
		body := make([]byte, 5*1024*1024)
		for i := range body {
			body[i] = byte(i % 251)
		}
		stdout := new(bytes.Buffer)
		cfg := w.config(&stubExecShell{})
		cfg.File, cfg.Stdout = transcript(t, "\x1b[?1004h"+string(body)+"\x1b[24;120H\x1b[0m"), stdout

		runHydrate(t.Context(), cfg)

		got := stdout.Bytes()
		if want := len(hydrateResetPreamble) + len(body) + len(hydrateResetPostamble); len(got) != want {
			t.Fatalf("stdout length = %d, want %d", len(got), want)
		}
		if !bytes.Equal(got[len(hydrateResetPreamble):len(hydrateResetPreamble)+len(body)], body) {
			t.Error("the streamed transcript does not match the file")
		}
	})

	for name, body := range map[string]string{"an empty transcript": "", "a capture of a blank screen": "\x1b[?1004h\x1b[1;1H\x1b[0m"} {
		t.Run("it writes nothing for "+name+", not even the reset sequences", func(t *testing.T) {
			w := newHydrateWorld(t)
			stdout := new(bytes.Buffer)
			settled := false
			exec := &stubExecShell{}
			cfg := w.config(exec)
			cfg.File, cfg.Stdout = transcript(t, body), stdout
			cfg.Settle = func() { settled = true }

			runHydrate(t.Context(), cfg)

			if stdout.Len() != 0 || settled {
				t.Errorf("stdout = %q, settled = %v; want nothing written and nothing to settle", stdout.String(), settled)
			}
			if !exec.called {
				t.Error("the session was never handed on")
			}
		})
	}

	t.Run("it replays nothing for a session that saved no transcript", func(t *testing.T) {
		w := newHydrateWorld(t)
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		stdout := new(bytes.Buffer)
		exec := &stubExecShell{}
		cfg := w.config(exec)
		cfg.Stdout, cfg.Logger = stdout, logger

		runHydrate(t.Context(), cfg)

		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing written", stdout.String())
		}
		if body := sink.Body(); strings.Contains(body, "scrollback") {
			t.Errorf("a session with no transcript logged about one: %q", body)
		}
		if !exec.called || w.hydrating(t) {
			t.Errorf("handed on = %v, hydrating still raised = %v; want the session handed on, its restore ended", exec.called, w.hydrating(t))
		}
	})

	t.Run("it lets the replay settle before its restore ends", func(t *testing.T) {
		w := newHydrateWorld(t)
		var replayedAt, loweredAt time.Time
		cfg := w.config(&stubExecShell{})
		cfg.File = transcript(t, "CONTENT")
		cfg.Settle = nil
		cfg.Stdout = writerFunc(func(p []byte) {
			if bytes.Equal(p, []byte(hydrateResetPostamble)) {
				replayedAt = time.Now()
			}
		})
		cfg.ClearHydrating = func(dir, name string) (bool, error) {
			loweredAt = time.Now()
			return state.Hydrating.Clear(dir, name)
		}

		runHydrate(t.Context(), cfg)

		if replayedAt.IsZero() || loweredAt.IsZero() {
			t.Fatalf("replayed at %v, lowered at %v; want both", replayedAt, loweredAt)
		}
		if settled := loweredAt.Sub(replayedAt); settled < hydrateSettleSleep {
			t.Errorf("the hydrating flag came down %v after the replay, want at least %v", settled, hydrateSettleSleep)
		}
	})
}

// writerFunc is an io.Writer that hands each write to its function.
type writerFunc func(p []byte)

func (f writerFunc) Write(p []byte) (int, error) {
	f(p)
	return len(p), nil
}

func TestHydrate_TranscriptUnreadable(t *testing.T) {
	cases := []struct {
		name string
		file func(t *testing.T) string
		warn string
	}{
		{name: "missing", file: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.bin") }, warn: "scrollback file not found"},
		{name: "unreadable", file: func(t *testing.T) string { return unreadableTranscript(t, "HIDDEN") }, warn: "scrollback file unreadable (permission denied)"},
	}
	for _, tc := range cases {
		t.Run("a "+tc.name+" transcript writes nothing and hands the session on as any other", func(t *testing.T) {
			w := newHydrateWorld(t)
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			stdout := new(bytes.Buffer)
			exec := &stubExecShell{}
			cfg := w.config(exec)
			cfg.File, cfg.Stdout, cfg.Logger = tc.file(t), stdout, logger
			cfg.HookStore = hydrateStoreWithMode(t, hydrateSession, "echo hi", resumemode.Eager)
			t.Setenv("SHELL", "/bin/zsh")

			runHydrate(t.Context(), cfg)

			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing written", stdout.String())
			}
			if want := []string{"sh", "-c", "trap : TERM; echo hi; exec '/bin/zsh' -l"}; !reflect.DeepEqual(exec.args, want) {
				t.Errorf("exec args = %q, want the eager hook %q", exec.args, want)
			}
			warn := execLogLine(t, sink.Body(), "WARN", tc.warn)
			for _, attr := range []string{"session=" + hydrateSession, "path=" + cfg.File} {
				if !strings.Contains(warn, attr) {
					t.Errorf("WARN %q does not carry %s", warn, attr)
				}
			}
			if w.hydrating(t) || !w.recordsLiveCreation(t) {
				t.Errorf("hydrating still raised = %v, creation recorded = %v; want the restore ended as any other", w.hydrating(t), w.recordsLiveCreation(t))
			}
		})
	}

	t.Run("a transcript failing at its first read writes nothing and logs the cause", func(t *testing.T) {
		w := newHydrateWorld(t)
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		stdout := new(bytes.Buffer)
		exec := &stubExecShell{}
		cfg := w.config(exec)
		// A directory opens cleanly and fails the copy's first read.
		cfg.File, cfg.Stdout, cfg.Logger = t.TempDir(), stdout, logger

		runHydrate(t.Context(), cfg)

		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing written", stdout.String())
		}
		warn := execLogLine(t, sink.Body(), "WARN", "scrollback file I/O error")
		if !strings.Contains(warn, "error=") {
			t.Errorf("WARN %q carries no cause", warn)
		}
		if !exec.called {
			t.Error("the session was never handed on")
		}
	})

	// The modes a part-read opened with are cut as a whole one's are; what its
	// rows set, a pen or the alternate screen among it, must not reach the
	// shell, and a state that looks like zmx's closing one is no end the read
	// reached.
	t.Run("a transcript failing part way is closed and settled as a whole one is, and logged as cut short", func(t *testing.T) {
		w := newHydrateWorld(t)
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		stdout := new(bytes.Buffer)
		settled := false
		exec := &stubExecShell{}
		cfg := w.config(exec)
		const read = "row-1\x1b[?1049h\x1b[31mrow-2\x1b[2;6H\x1b[0m"
		cfg.File, cfg.Stdout, cfg.Logger = "/saved/proj-a1b2.bin", stdout, logger
		cfg.OpenTranscript = func(string) (io.ReadCloser, error) {
			return io.NopCloser(io.MultiReader(strings.NewReader("\x1b[?1004h"+read), failingReader{errors.New("device went away")})), nil
		}
		cfg.Settle = func() { settled = true }

		runHydrate(t.Context(), cfg)

		if want := hydrateResetPreamble + read + hydrateResetPostamble; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
		if !settled {
			t.Error("the part replayed was never let settle")
		}
		warn := execLogLine(t, sink.Body(), "WARN", "scrollback replay cut short")
		for _, attr := range []string{"session=" + hydrateSession, "path=" + cfg.File, "bytes=" + strconv.Itoa(len(read)), "error=device went away"} {
			if !strings.Contains(warn, attr) {
				t.Errorf("WARN %q does not carry %s", warn, attr)
			}
		}
		if body := sink.Body(); strings.Contains(body, "scrollback missing") || strings.Contains(body, "scrollback replayed") {
			t.Errorf("a part-read was recorded as a missing or a whole replay: %q", body)
		}
		if !exec.called {
			t.Error("the session was never handed on")
		}
	})
}

// failingReader fails every read with its error.
type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

// The causes arrive as the open and the copy return them, so the record keys
// off the error they wrap, never its text.
func TestLogTranscriptUnreadable_ClassifiesTheCause(t *testing.T) {
	cases := []struct {
		name  string
		cause error
		warn  string
	}{
		{name: "ENOENT", cause: &os.PathError{Op: "open", Path: "/x/sb.bin", Err: syscall.ENOENT}, warn: "scrollback file not found"},
		{name: "permission", cause: &os.PathError{Op: "open", Path: "/x/sb.bin", Err: syscall.EACCES}, warn: "scrollback file unreadable (permission denied)"},
		{name: "generic", cause: errors.New("synthetic mid-stream failure"), warn: "scrollback file I/O error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, sink := newCaptureLoggerForComponent(t, "hydrate")
			start := time.Now()

			logTranscriptUnreadable(hydrateConfig{Session: hydrateSession, File: "/x/sb.bin", Logger: logger}, tc.cause)

			if n := countLogLines(sink.Body(), "WARN", tc.warn); n != 1 {
				t.Errorf("got %d %q WARNs, want 1: %q", n, tc.warn, sink.Body())
			}
			if n := countLogLines(sink.Body(), "INFO", "scrollback missing"); n != 1 {
				t.Errorf("got %d scrollback missing INFOs, want 1: %q", n, sink.Body())
			}
			if took := time.Since(start); took >= hydrateSettleSleep {
				t.Errorf("recording the cause took %v; nothing was replayed, so nothing settles", took)
			}
		})
	}
}

func TestHydrate_HandOff(t *testing.T) {
	t.Run("a session with no registration is handed its login shell, as zmx starts a fresh one's", func(t *testing.T) {
		for _, store := range []struct {
			name  string
			store func(t *testing.T) *hooks.Store
		}{
			{name: "no hook store", store: func(*testing.T) *hooks.Store { return nil }},
			{name: "no registration", store: emptyHookStore},
		} {
			t.Run(store.name, func(t *testing.T) {
				w := newHydrateWorld(t)
				shell := stageExecutable(t, t.TempDir(), "myshell", "#!/bin/sh\n")
				t.Setenv("SHELL", shell)
				exec := &stubExecShell{}
				cfg := w.config(exec)
				cfg.File, cfg.HookStore = transcript(t, ""), store.store(t)

				runHydrate(t.Context(), cfg)

				if exec.target != shell || !reflect.DeepEqual(exec.args, []string{"-myshell"}) {
					t.Errorf("exec = %q %q, want %s run as -myshell", exec.target, exec.args, shell)
				}
			})
		}
	})

	t.Run("it falls back to /bin/sh when SHELL is unset", func(t *testing.T) {
		w := newHydrateWorld(t)
		t.Setenv("SHELL", "")
		exec := &stubExecShell{}

		runHydrate(t.Context(), w.config(exec))

		if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, []string{"-sh"}) {
			t.Errorf("exec = %q %q, want /bin/sh run as -sh", exec.target, exec.args)
		}
	})

	t.Run("an eager registration runs its command under sh, then the login shell", func(t *testing.T) {
		w := newHydrateWorld(t)
		t.Setenv("SHELL", "/bin/zsh")
		exec := &stubExecShell{}
		cfg := w.config(exec)
		cfg.File = transcript(t, "OLD")
		cfg.HookStore = hydrateStoreWithMode(t, hydrateSession, "claude --resume abc", resumemode.Eager)

		runHydrate(t.Context(), cfg)

		want := []string{"sh", "-c", "trap : TERM; claude --resume abc; exec '/bin/zsh' -l"}
		if exec.target != "/bin/sh" || !reflect.DeepEqual(exec.args, want) {
			t.Errorf("exec = %q %q, want /bin/sh %q", exec.target, exec.args, want)
		}
	})

	t.Run("it passes the command verbatim in one argv slot, so sh parses its quotes", func(t *testing.T) {
		w := newHydrateWorld(t)
		t.Setenv("SHELL", "/bin/zsh")
		raw := "echo 'it works' && echo \"\\$x\""
		exec := &stubExecShell{}
		cfg := w.config(exec)
		cfg.HookStore = hydrateStoreWithMode(t, hydrateSession, raw, resumemode.Eager)

		runHydrate(t.Context(), cfg)

		if len(exec.args) != 3 || exec.args[2] != "trap : TERM; "+raw+"; exec '/bin/zsh' -l" {
			t.Errorf("exec args = %q, want the command verbatim in the -c script", exec.args)
		}
	})

	t.Run("it looks the hook up by the session's name", func(t *testing.T) {
		w := newHydrateWorld(t)
		t.Setenv("SHELL", "/bin/zsh")
		store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Body: map[string]map[string]string{
			hydrateSession: {"on-resume": "echo mine"},
			"proj-other":   {"on-resume": "echo theirs"},
		}})
		exec := &stubExecShell{}
		cfg := w.config(exec)
		cfg.HookStore = store

		runHydrate(t.Context(), cfg)

		if len(exec.args) != 3 || !strings.Contains(exec.args[2], "echo mine") {
			t.Errorf("exec args = %q, want the session's own hook", exec.args)
		}
	})

	t.Run("an unreadable hook store hands the session its login shell, with one WARN", func(t *testing.T) {
		w := newHydrateWorld(t)
		t.Setenv("SHELL", "/bin/zsh")
		logger, sink := newCaptureLoggerForComponent(t, "hydrate")
		store, _ := hookstest.StageStore(t, hookstest.Staging{Dir: t.TempDir(), SidecarAbsent: true, Unreadable: true})
		exec := &stubExecShell{}
		cfg := w.config(exec)
		cfg.HookStore, cfg.Logger = store, logger

		runHydrate(t.Context(), cfg)

		if !reflect.DeepEqual(exec.args, []string{"-zsh"}) {
			t.Errorf("exec args = %q, want the login shell", exec.args)
		}
		warn := execLogLine(t, sink.Body(), "WARN", "lookup on-resume hook failed")
		if !strings.Contains(warn, "hook_key="+hydrateSession) {
			t.Errorf("WARN %q does not name the hook key", warn)
		}
		if n := strings.Count(sink.Body(), "WARN"); n != 1 {
			t.Errorf("got %d WARNs, want the lookup's alone: %q", n, sink.Body())
		}
	})
}
