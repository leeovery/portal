package cmd

import (
	"bytes"
	"errors"
	"go/ast"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// resumeWaitProbe records what the wait hands to its seams, so a test asserts on
// the hand-off and the terminal restore without a process being replaced or a
// real tty being switched into raw mode.
type resumeWaitProbe struct {
	stdout    bytes.Buffer
	sink      *logtest.Sink
	rawCalls  int
	restores  int
	execProg  string
	execArgs  []string
	execCalls int

	// Restores already run when the exec happened, so a restore that moved
	// below the hand-off is visible as a zero reading.
	restoredAtExec int

	// order names each seam in the sequence it was reached, so an answer whose
	// steps ran out of order fails on the sequence rather than on three
	// independent call counts.
	order      []string
	clearErr   error
	clearCalls int
	lookup     func(hookKey string) (hooks.OnResume, error)
	lookupKeys []string
}

// orderedWriter records that the pane was written to before it passes the bytes
// on, so the leave sequence takes its place in the same sequence as the seams.
type orderedWriter struct {
	probe *resumeWaitProbe
}

func (w orderedWriter) Write(p []byte) (int, error) {
	w.probe.order = append(w.probe.order, "stdout")
	return w.probe.stdout.Write(p)
}

func newResumeWaitConfig(t *testing.T, p *resumeWaitProbe, payload resumeChainPayload, in io.Reader) resumeWaitConfig {
	t.Helper()
	logger, sink := logtest.NewCaptureLogger(t)
	p.sink = sink
	return resumeWaitConfig{
		resumeChainPayload: payload,
		Stdout:             orderedWriter{probe: p},
		In:                 in,
		Logger:             logger,
		IsTerminal:         func() bool { return true },
		MakeRaw: func() (func(), error) {
			p.rawCalls++
			return func() { p.restores++ }, nil
		},
		ClearMarker: func() error {
			p.order = append(p.order, "clear")
			p.clearCalls++
			return p.clearErr
		},
		LookupResume: func(hookKey string) (hooks.OnResume, error) {
			p.order = append(p.order, "lookup")
			p.lookupKeys = append(p.lookupKeys, hookKey)
			if p.lookup != nil {
				return p.lookup(hookKey)
			}
			return hooks.OnResume{}, nil
		},
		ExecSelf: func(prog string, args []string) {
			p.order = append(p.order, "exec")
			p.execProg = prog
			p.execArgs = args
			p.execCalls++
			p.restoredAtExec = p.restores
		},
	}
}

// oneByteReader fails the test if the wait ever reads ahead: a byte pulled into
// a buffer the loop did not dispatch is a byte the next process image loses.
type oneByteReader struct {
	t     *testing.T
	inner io.Reader
	reads int
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	r.t.Helper()
	if len(p) != 1 {
		r.t.Errorf("wait read into a %d-byte buffer, want 1: reading ahead strands input the hand-off should inherit", len(p))
	}
	r.reads++
	return r.inner.Read(p)
}

func keystrokes(t *testing.T, s string) *oneByteReader {
	t.Helper()
	return &oneByteReader{t: t, inner: strings.NewReader(s)}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func assertHandOff(t *testing.T, p *resumeWaitProbe, payload resumeChainPayload) {
	t.Helper()
	exe, err := resumeChainExe()
	if err != nil {
		t.Fatalf("resumeChainExe() error = %v", err)
	}
	want := resumeChainArgv(exe, resumeDrawSubcommand, payload)

	if p.execCalls != 1 {
		t.Fatalf("ExecSelf called %d times, want exactly 1", p.execCalls)
	}
	if p.execProg != exe {
		t.Errorf("exec target = %q, want %q", p.execProg, exe)
	}
	if !slices.Equal(p.execArgs, want) {
		t.Errorf("exec argv = %q, want %q", p.execArgs, want)
	}

	rec := p.sink.Records().WithMessage("exec").Only(t, "the hand-off's exec marker")
	if got := rec.AttrOrEmpty("target"); got != exe {
		t.Errorf("exec marker target = %q, want %q", got, exe)
	}
	if got := rec.AttrOrEmpty("args"); got != strings.Join(want, " ") {
		t.Errorf("exec marker args = %q, want %q", got, strings.Join(want, " "))
	}
	if rec.Level != slog.LevelInfo {
		t.Errorf("exec marker level = %v, want INFO", rec.Level)
	}
}

func assertNoHandOff(t *testing.T, p *resumeWaitProbe) {
	t.Helper()
	if p.execCalls != 0 {
		t.Errorf("ExecSelf called %d times, want 0: the key must be swallowed", p.execCalls)
	}
	if p.stdout.Len() != 0 {
		t.Errorf("the wait wrote %q to stdout, want nothing", p.stdout.String())
	}
}

func TestRunResumeWait_ActingKeys(t *testing.T) {
	t.Run("it resumes the pane on Enter", func(t *testing.T) {
		for _, key := range []string{"\r", "\n"} {
			t.Run(keyName(key), func(t *testing.T) {
				var probe resumeWaitProbe
				payload := samplePayload()
				payload.Report = "could not clear the pending marker"
				payload.Width, payload.Height = 100, 30
				probe.lookup = foundHook("make deploy")

				if err := runResumeWait(newResumeWaitConfig(t, &probe, payload, keystrokes(t, key))); err != nil {
					t.Fatalf("runResumeWait() error = %v", err)
				}
				assertHookHandOff(t, &probe, "make deploy")
			})
		}
	})

	t.Run("it hands the pane over on d", func(t *testing.T) {
		var probe resumeWaitProbe
		payload := samplePayload()
		payload.Width, payload.Height = 100, 30

		if err := runResumeWait(newResumeWaitConfig(t, &probe, payload, keystrokes(t, "d"))); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, &probe, payload)
	})

	t.Run("it acts on both keys at a size below the card's", func(t *testing.T) {
		sizes := []struct {
			name string
			w, h int
		}{
			{"a single cell", 1, 1},
			{"narrower than the card", 20, 6},
			{"unmeasured", 0, 0},
			{"negative", -4, -2},
		}
		for _, size := range sizes {
			for _, key := range []string{"\r", "d"} {
				t.Run(size.name+"/"+keyName(key), func(t *testing.T) {
					var probe resumeWaitProbe
					payload := samplePayload()
					payload.Width, payload.Height = size.w, size.h
					probe.lookup = foundHook(payload.Command)

					if err := runResumeWait(newResumeWaitConfig(t, &probe, payload, keystrokes(t, key))); err != nil {
						t.Fatalf("runResumeWait() error = %v", err)
					}
					if key == "d" {
						assertHandOff(t, &probe, payload)
						return
					}
					assertHookHandOff(t, &probe, payload.Command)
				})
			}
		}
	})
}

func TestRunResumeWait_SwallowedKeys(t *testing.T) {
	t.Run("it swallows every other key", func(t *testing.T) {
		keys := []string{"\x03", "\x04", "\x1a", "\x1b", "D", "y", "q", "a", " "}
		for _, key := range keys {
			t.Run(keyName(key), func(t *testing.T) {
				var probe resumeWaitProbe
				reader := keystrokes(t, key)

				err := runResumeWait(newResumeWaitConfig(t, &probe, samplePayload(), reader))

				if !errors.Is(err, io.EOF) {
					t.Fatalf("runResumeWait() error = %v, want the EOF that ended the wait after the swallowed key", err)
				}
				if reader.reads < 2 {
					t.Errorf("the wait read %d times, want it to read on past the swallowed key", reader.reads)
				}
				assertNoHandOff(t, &probe)
			})
		}
	})

	t.Run("it swallows the bytes of an escape sequence without assembling an action", func(t *testing.T) {
		var probe resumeWaitProbe

		err := runResumeWait(newResumeWaitConfig(t, &probe, samplePayload(), keystrokes(t, "\x1b[A")))

		if !errors.Is(err, io.EOF) {
			t.Fatalf("runResumeWait() error = %v, want EOF", err)
		}
		assertNoHandOff(t, &probe)
	})

	t.Run("it acts on the first acting key in a burst and leaves the rest unread", func(t *testing.T) {
		var probe resumeWaitProbe
		payload := samplePayload()
		reader := keystrokes(t, "dy")

		if err := runResumeWait(newResumeWaitConfig(t, &probe, payload, reader)); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}
		assertHandOff(t, &probe, payload)

		rest, err := io.ReadAll(reader.inner)
		if err != nil {
			t.Fatalf("reading what the wait left behind: %v", err)
		}
		if string(rest) != "y" {
			t.Errorf("input left for the next process image = %q, want %q", rest, "y")
		}
	})
}

func TestRunResumeWait_TerminalMode(t *testing.T) {
	t.Run("it restores the terminal on every exit path", func(t *testing.T) {
		cases := []struct {
			name string
			in   io.Reader
		}{
			{"answered", strings.NewReader("d")},
			{"read error", errReader{err: errors.New("input/output error")}},
			{"EOF", strings.NewReader("")},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var probe resumeWaitProbe

				_ = runResumeWait(newResumeWaitConfig(t, &probe, samplePayload(), tc.in))

				if probe.restores != 1 {
					t.Errorf("terminal restored %d times, want exactly 1", probe.restores)
				}
			})
		}
	})

	t.Run("it restores the terminal before the hand-off exec", func(t *testing.T) {
		var probe resumeWaitProbe

		if err := runResumeWait(newResumeWaitConfig(t, &probe, samplePayload(), keystrokes(t, "\r"))); err != nil {
			t.Fatalf("runResumeWait() error = %v", err)
		}

		if probe.restoredAtExec != 1 {
			t.Errorf("%d restores had run when the hand-off exec'd, want 1: the next process image must inherit a cooked tty", probe.restoredAtExec)
		}
	})

	t.Run("it refuses to wait on a stdin that is not a terminal", func(t *testing.T) {
		var probe resumeWaitProbe
		reader := keystrokes(t, "d")
		cfg := newResumeWaitConfig(t, &probe, samplePayload(), reader)
		cfg.IsTerminal = func() bool { return false }

		err := runResumeWait(cfg)

		if err == nil {
			t.Fatal("runResumeWait() on a non-terminal stdin returned nil; spinning on a non-tty would burn a core for the life of the pane")
		}
		if reader.reads != 0 {
			t.Errorf("the wait read %d times before refusing, want 0", reader.reads)
		}
		if probe.rawCalls != 0 {
			t.Errorf("MakeRaw called %d times on a non-terminal stdin, want 0", probe.rawCalls)
		}
		assertNoHandOff(t, &probe)
	})

	t.Run("it refuses to wait when raw mode cannot be entered", func(t *testing.T) {
		var probe resumeWaitProbe
		reader := keystrokes(t, "d")
		cfg := newResumeWaitConfig(t, &probe, samplePayload(), reader)
		rawErr := errors.New("inappropriate ioctl for device")
		cfg.MakeRaw = func() (func(), error) { return nil, rawErr }

		err := runResumeWait(cfg)

		if !errors.Is(err, rawErr) {
			t.Fatalf("runResumeWait() error = %v, want the MakeRaw failure", err)
		}
		if reader.reads != 0 {
			t.Errorf("the wait read %d times after raw mode failed, want 0", reader.reads)
		}
		assertNoHandOff(t, &probe)
	})
}

func TestRunResumeWait_Waiting(t *testing.T) {
	t.Run("it ends the wait on a read error rather than spinning", func(t *testing.T) {
		var probe resumeWaitProbe
		readErr := errors.New("input/output error")

		err := runResumeWait(newResumeWaitConfig(t, &probe, samplePayload(), errReader{err: readErr}))

		if !errors.Is(err, readErr) {
			t.Fatalf("runResumeWait() error = %v, want the read failure", err)
		}
		assertNoHandOff(t, &probe)
	})

	t.Run("it writes nothing and resolves no theme while waiting", func(t *testing.T) {
		var probe resumeWaitProbe

		_ = runResumeWait(newResumeWaitConfig(t, &probe, samplePayload(), keystrokes(t, "\x1b[Ayq ")))

		assertNoHandOff(t, &probe)

		source := sourceguardtest.PackageSource(t, ".", "state_resume_wait.go")
		forbidden := []string{
			"github.com/leeovery/portal/internal/theme",
			"github.com/leeovery/portal/internal/tui",
			"github.com/leeovery/portal/internal/prefs",
		}
		for _, imp := range source.File.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if slices.Contains(forbidden, path) {
				t.Errorf("%s imports %s; the pane between screens carries the wait alone", source.Path, path)
			}
		}
	})

	t.Run("it holds its report and goes on waiting with no input at all", func(t *testing.T) {
		reader, writer := io.Pipe()
		t.Cleanup(func() { _ = writer.Close() })

		var probe resumeWaitProbe
		payload := samplePayload()
		payload.Report = "could not clear the pending marker"
		cfg := newResumeWaitConfig(t, &probe, payload, reader)

		done := make(chan error, 1)
		go func() { done <- runResumeWait(cfg) }()

		select {
		case err := <-done:
			t.Fatalf("the wait ended on its own with no input: %v", err)
		case <-time.After(150 * time.Millisecond):
		}

		if _, err := writer.Write([]byte("d")); err != nil {
			t.Fatalf("sending the discard key: %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("runResumeWait() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("the wait did not act on the key it was sent")
		}

		assertHandOff(t, &probe, payload)
		if !slices.Contains(probe.execArgs, payload.Report) {
			t.Errorf("exec argv %q does not carry the report forward", probe.execArgs)
		}
	})
}

// The waiter is the pane's only process: declining a hangup would outlive the
// destruction of its own pane, leaving a Portal process per culled session.
// SIGWINCH is the one signal the wait path may watch, so a resize seam leaves
// this green while a declined hangup fails it.
func TestRunResumeWait_InstallsNoHangupTerminateOrInterruptHandler(t *testing.T) {
	source := sourceguardtest.PackageSource(t, ".", "state_resume_wait.go")

	const onlyWatchable = "only syscall.SIGWINCH may be watched — the default disposition must end the waiter when tmux tears the pane down"

	sourceguardtest.ForEachFuncCall(source.File, func(_ string, call *ast.CallExpr) bool {
		switch sourceguardtest.CalleeName(call) {
		case "Ignore", "Reset":
			t.Errorf("%s: the wait path takes a signal off its default disposition without a Notify; %s",
				source.Position(call.Pos()), onlyWatchable)
			return true
		case "Notify":
		default:
			return true
		}

		// A Notify naming no signal relays every signal, which is the refusal
		// this guard exists to forbid.
		if len(call.Args) < 2 {
			t.Errorf("%s: the wait path notifies on every signal; %s",
				source.Position(call.Pos()), onlyWatchable)
			return true
		}

		for _, arg := range call.Args[1:] {
			if got := signalName(arg); got != "syscall.SIGWINCH" {
				t.Errorf("%s: the wait path notifies on %s; %s",
					source.Position(call.Pos()), got, onlyWatchable)
			}
		}
		return true
	})
}

func signalName(arg ast.Expr) string {
	switch e := arg.(type) {
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok {
			return pkg.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.Ident:
		return e.Name
	}
	return "an unrecognised expression"
}

// keyName names a subtest after the byte it sends, so a failure reads as the
// key a user pressed rather than an escape.
func keyName(key string) string {
	switch key {
	case "\r":
		return "CR"
	case "\n":
		return "LF"
	case " ":
		return "space"
	}
	if b := key[0]; b < 0x20 {
		return "ctrl-" + string(rune('a'+b-1))
	}
	return key
}

func TestStateResumeWaitCommand(t *testing.T) {
	t.Run("it parses the chain argv into the payload it was composed from", func(t *testing.T) {
		payload := resumeChainPayload{
			Command: "make deploy",
			Report:  "could not clear the pending marker",
			HookKey: "tok123",
			Pane:    "%7",
			PaneKey: "proj-a1b2:0.1",
			Width:   120,
			Height:  40,
		}

		var got resumeWaitConfig
		withFuncSeam(t, &resumeWaitRunFunc, func(cfg resumeWaitConfig) error {
			got = cfg
			return nil
		})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		errBuf := new(bytes.Buffer)
		rootCmd.SetErr(errBuf)
		rootCmd.SetArgs(resumeChainArgv("portal", resumeWaitSubcommand, payload)[1:])
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("executing the composed argv: %v\nstderr: %s", err, errBuf)
		}

		if got.resumeChainPayload != payload {
			t.Errorf("payload = %+v, want %+v", got.resumeChainPayload, payload)
		}
	})

	t.Run("it refuses an invocation naming no command", func(t *testing.T) {
		withFuncSeam(t, &resumeWaitRunFunc, func(resumeWaitConfig) error { return nil })

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"state", resumeWaitSubcommand, "--pane", "%7"})
		if err := rootCmd.Execute(); err == nil {
			t.Error("executing resume-wait with no --command succeeded; the flag is required")
		}
	})
}
