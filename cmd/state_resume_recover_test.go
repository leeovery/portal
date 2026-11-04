package cmd

import (
	"bytes"
	"errors"
	"go/ast"
	"go/token"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/state"
)

const recoverSession = "proj-a1b2"

// resumeRecoverProbe records what the tail hands to its seams, so a test asserts
// on the recovery without a flag being read or a process image being replaced.
type resumeRecoverProbe struct {
	stdout bytes.Buffer

	// order names each seam in the sequence it was reached, so a recovery whose
	// steps ran out of order fails on the sequence rather than on call counts.
	order []string

	raised    bool
	readErr   error
	readCalls int

	clearErr   error
	clearCalls int

	execProg  string
	execArgs  []string
	execCalls int

	signalsOnCalls   int
	signalsOnErr     error
	execsAtSignalsOn int
}

// recoverWriter records that the session was written to before it passes the
// bytes on, so the leave sequence takes its place in the same sequence as the
// seams.
type recoverWriter struct {
	probe *resumeRecoverProbe
}

func (w recoverWriter) Write(p []byte) (int, error) {
	w.probe.order = append(w.probe.order, "stdout")
	return w.probe.stdout.Write(p)
}

func newResumeRecoverConfig(t *testing.T, p *resumeRecoverProbe) resumeRecoverConfig {
	t.Helper()
	cfg := resumeRecoverConfig{
		Session: recoverSession,
		Pending: func() (bool, error) {
			p.order = append(p.order, "read")
			p.readCalls++
			return p.raised, p.readErr
		},
		ClearPending: func() error {
			p.order = append(p.order, "clear")
			p.clearCalls++
			return p.clearErr
		},
	}
	return withRecoverProbe(cfg, p)
}

// withRecoverProbe hands cfg's writes, its exec and its terminal to probe,
// leaving its flag as cfg reads and lowers it.
func withRecoverProbe(cfg resumeRecoverConfig, p *resumeRecoverProbe) resumeRecoverConfig {
	cfg.Stdout = recoverWriter{probe: p}
	cfg.Logger = hydrateLogger
	cfg.EnableTTYSignals = func() error {
		p.signalsOnCalls++
		p.execsAtSignalsOn = p.execCalls
		return p.signalsOnErr
	}
	cfg.ExecShell = func(prog string, args []string) {
		p.order = append(p.order, "exec")
		p.execProg = prog
		p.execArgs = args
		p.execCalls++
	}
	return cfg
}

func TestRunResumeRecover(t *testing.T) {
	t.Run("it does nothing for a session that was already answered", func(t *testing.T) {
		var probe resumeRecoverProbe

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		if probe.readCalls != 1 {
			t.Errorf("Pending called %d times, want exactly 1", probe.readCalls)
		}
		assertNotRecovered(t, &probe)
	})

	t.Run("it recovers a session whose flag is still raised", func(t *testing.T) {
		probe := resumeRecoverProbe{raised: true}
		sink := logtest.Install(t)

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		assertRecovered(t, &probe, sink)
	})

	t.Run("it treats a flag it can't look for as still raised", func(t *testing.T) {
		probe := resumeRecoverProbe{readErr: errors.New("permission denied")}
		sink := logtest.Install(t)

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		assertRecovered(t, &probe, sink)
	})

	t.Run("it execs the shell even when the clear failed", func(t *testing.T) {
		probe := resumeRecoverProbe{raised: true, clearErr: errors.New("read-only file system")}
		sink := logtest.Install(t)

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		assertRecovered(t, &probe, sink)
	})

	t.Run("it leaves the panel's screen before it drops the protection", func(t *testing.T) {
		probe := resumeRecoverProbe{raised: true}

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		want := []string{"read", "stdout", "clear", "exec"}
		if !slices.Equal(probe.order, want) {
			t.Errorf("recovery order = %v, want %v", probe.order, want)
		}
		if got := probe.stdout.String(); got != hydrateResetPreamble {
			t.Errorf("stdout = %q, want the leave sequence %q", got, hydrateResetPreamble)
		}
	})

	t.Run("it reads no store and draws nothing", func(t *testing.T) {
		probe := resumeRecoverProbe{raised: true}

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}
		if got := probe.stdout.String(); got != hydrateResetPreamble {
			t.Errorf("stdout = %q, want the leave sequence alone", got)
		}

		source := sourceguardtest.PackageSource(t, ".", "state_resume_recover.go")
		forbidden := []string{
			"github.com/leeovery/portal/internal/hooks",
			"github.com/leeovery/portal/internal/prefs",
			"github.com/leeovery/portal/internal/theme",
			"github.com/leeovery/portal/internal/tui",
		}
		for _, imp := range source.File.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if slices.Contains(forbidden, path) {
				t.Errorf("%s imports %s; the tail makes a session usable and consults no registration", source.Path, path)
			}
		}
		for _, callee := range []string{"loadHookStore", "loadPrefsStore", "loadPrefsStoreNoMigrate", "newThemeLoader"} {
			assertCalleeAbsent(t, source, callee)
		}
	})
}

func assertCalleeAbsent(t *testing.T, source sourceguardtest.ParsedSource, callee string) {
	t.Helper()
	sourceguardtest.ForEachFuncCall(source.File, func(_ string, call *ast.CallExpr) bool {
		if sourceguardtest.CalleeName(call) == callee {
			t.Errorf("%s calls %s; the tail reads no store and draws nothing", source.Path, callee)
		}
		return true
	})
}

// assertNotRecovered pins a tail that left an answered session alone: nothing
// written, nothing cleared, no second shell.
func assertNotRecovered(t *testing.T, p *resumeRecoverProbe) {
	t.Helper()
	if p.stdout.Len() != 0 {
		t.Errorf("the tail wrote %q to stdout, want nothing", p.stdout.String())
	}
	if p.clearCalls != 0 {
		t.Errorf("ClearPending called %d times, want 0", p.clearCalls)
	}
	if p.execCalls != 0 {
		t.Errorf("ExecShell called %d times, want 0: an answered session already ran its own shell", p.execCalls)
	}
}

// assertRecovered pins the whole recovery: the session leaves the panel's
// screen, its flag is lowered, and it is handed the user's shell whether or not
// the lowering landed.
func assertRecovered(t *testing.T, p *resumeRecoverProbe, sink *logtest.Sink) {
	t.Helper()
	if got := p.stdout.String(); got != hydrateResetPreamble {
		t.Errorf("stdout = %q, want the leave sequence %q", got, hydrateResetPreamble)
	}
	if p.execCalls != 1 {
		t.Fatalf("ExecShell called %d times, want exactly 1", p.execCalls)
	}
	shell := resolveShell()
	if p.execProg != shell {
		t.Errorf("exec target = %q, want %q", p.execProg, shell)
	}
	if !slices.Equal(p.execArgs, []string{shell}) {
		t.Errorf("exec argv = %q, want %q", p.execArgs, []string{shell})
	}

	rec := sink.Records().Matching("hydrate", "exec").Only(t, "the tail's exec marker")
	if rec.Level != slog.LevelInfo {
		t.Errorf("exec marker level = %v, want INFO", rec.Level)
	}
	if got := rec.AttrOrEmpty("target"); got != shell {
		t.Errorf("exec marker target = %q, want %q", got, shell)
	}
	if got := rec.AttrOrEmpty("args"); got != shell {
		t.Errorf("exec marker args = %q, want %q", got, shell)
	}
	if got := rec.AttrOrEmpty("hook_present"); got != "false" {
		t.Errorf("exec marker hook_present = %q, want %q", got, "false")
	}
}

func TestRunResumeRecover_FailedClearRecord(t *testing.T) {
	const message = "clear resume pending flag failed"

	t.Run("it records a failed clear as a WARN naming the session and the error", func(t *testing.T) {
		probe := resumeRecoverProbe{raised: true, clearErr: errors.New("read-only file system")}
		sink := logtest.Install(t)

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		warned := sink.Records().AtOrAboveLevel(slog.LevelWarn)
		rec := warned.Only(t, "the failed clear's record")
		if !rec.Matches("hydrate", message) {
			t.Fatalf("WARN = (%q, %q), want (%q, %q)", rec.AttrOrEmpty("component"), rec.Msg, "hydrate", message)
		}
		if got := rec.AttrOrEmpty("session"); got != recoverSession {
			t.Errorf("WARN session = %q, want %q", got, recoverSession)
		}
		for _, gone := range []string{"pane_key", "hook_key"} {
			if rec.HasAttr(gone) {
				t.Errorf("WARN carries %s, a key the chain no longer has", gone)
			}
		}
		if err := rec.ErrorAttr(t, "error"); !errors.Is(err, probe.clearErr) {
			t.Errorf("WARN error = %v, want %v", err, probe.clearErr)
		}
	})

	t.Run("it words the record apart from every other WARN a session emits", func(t *testing.T) {
		if occurrences := warnMessages(t)[message]; occurrences != 1 {
			t.Errorf("%q is emitted by %d WARN sites, want 1: a shared wording stops a grep separating a session that came back eager from one left flagged", message, occurrences)
		}
	})

	t.Run("it records nothing when the clear succeeded", func(t *testing.T) {
		probe := resumeRecoverProbe{raised: true}
		sink := logtest.Install(t)

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		if warned := sink.Records().AtOrAboveLevel(slog.LevelWarn); len(warned) != 0 {
			t.Errorf("records at or above WARN = %v, want none", warned)
		}
		if recs := sink.Records(); len(recs) != 1 {
			t.Errorf("records = %v, want the exec marker alone", recs)
		}
	})
}

func TestStateResumeRecoverCommand(t *testing.T) {
	t.Run("it parses the chain argv the waiter parks it with", func(t *testing.T) {
		payload := resumeChainPayload{Command: "make deploy", Session: recoverSession}

		var got resumeRecoverConfig
		withFuncSeam(t, &resumeRecoverRunFunc, func(cfg resumeRecoverConfig) error {
			got = cfg
			return nil
		})

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		errBuf := new(bytes.Buffer)
		rootCmd.SetErr(errBuf)
		rootCmd.SetArgs(resumeChainArgv("portal", resumeRecoverSubcommand, payload)[1:])
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("executing the composed argv: %v\nstderr: %s", err, errBuf)
		}

		if got.Session != payload.Session {
			t.Errorf("session = %q, want %q", got.Session, payload.Session)
		}
	})

	t.Run("it recovers a session parked by a build whose tail named no session", func(t *testing.T) {
		resumeStateDir(t)
		var probe resumeRecoverProbe
		sink := logtest.Install(t)

		parsed, err := executeResumeRecover(t, []string{"--pane", "%7", "--pane-key", "proj-a1b2:0.1"}, &probe)
		if err != nil {
			t.Fatalf("the tail refused an argv naming no session: %v", err)
		}

		if parsed.Session != "" {
			t.Errorf("session = %q, want empty", parsed.Session)
		}
		if probe.execCalls != 1 {
			t.Errorf("ExecShell called %d times, want the session recovered", probe.execCalls)
		}
		rec := sink.Records().WithMessage("clear resume pending flag failed").Only(t, "the failed clear's record")
		if !errors.Is(rec.ErrorAttr(t, "error"), state.ErrUnsafeName) {
			t.Errorf("WARN error = %v, want the unnamed session's refusal", rec.ErrorAttr(t, "error"))
		}
	})
}

// The production flag seams run here against a state directory of the test's
// own: only the session's writes, its exec and its terminal are probed.
func TestStateResumeRecoverCommand_Flag(t *testing.T) {
	args := []string{"--session", recoverSession}

	t.Run("it lowers a raised flag and hands the session its shell", func(t *testing.T) {
		dir := resumeStateDir(t)
		raiseFlag(t, dir, recoverSession)
		var probe resumeRecoverProbe
		sink := logtest.Install(t)

		if _, err := executeResumeRecover(t, args, &probe); err != nil {
			t.Fatal(err)
		}

		if flagRaised(t, dir, recoverSession) {
			t.Error("the flag is still raised after the tail recovered the session")
		}
		assertRecovered(t, &probe, sink)
	})

	t.Run("it leaves a session whose flag is lowered alone", func(t *testing.T) {
		resumeStateDir(t)
		var probe resumeRecoverProbe

		if _, err := executeResumeRecover(t, args, &probe); err != nil {
			t.Fatal(err)
		}

		assertNotRecovered(t, &probe)
	})

	t.Run("it recovers a session whose flag it can't look for", func(t *testing.T) {
		dir := resumeStateDir(t)
		raiseFlag(t, dir, recoverSession)
		unsearchable(t, filepath.Join(dir, string(state.ResumePending)))
		var probe resumeRecoverProbe
		sink := logtest.Install(t)

		if _, err := executeResumeRecover(t, args, &probe); err != nil {
			t.Fatal(err)
		}

		assertRecovered(t, &probe, sink)
		sink.Records().WithMessage("clear resume pending flag failed").Only(t, "the refused clear's record")
	})
}

// warnMessages counts the WARN sites in cmd's production sources by the message
// they emit, so a wording shared with another site is a finding rather than a
// literal a test restates.
func warnMessages(t *testing.T) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		sourceguardtest.ForEachFuncCall(source.File, func(_ string, call *ast.CallExpr) bool {
			if sourceguardtest.CalleeName(call) != "Warn" || len(call.Args) == 0 {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if msg, err := strconv.Unquote(lit.Value); err == nil {
					counts[msg]++
				}
			}
			return true
		})
	}
	if len(counts) == 0 {
		t.Fatal("no WARN sites found in cmd's production sources; the scan has stopped looking")
	}
	return counts
}

// executeResumeRecover runs the tail's command line through the real parse,
// its flag read and lowered by the production seams, and hands back the config
// the parse produced. Its writes, its exec and its terminal go to probe, so no
// process image is replaced.
func executeResumeRecover(t *testing.T, args []string, probe *resumeRecoverProbe) (resumeRecoverConfig, error) {
	t.Helper()
	var parsed resumeRecoverConfig
	withFuncSeam(t, &resumeRecoverRunFunc, func(cfg resumeRecoverConfig) error {
		parsed = cfg
		return runResumeRecover(withRecoverProbe(cfg, probe))
	})

	resetRootCmd()
	rootCmd.SetOut(new(bytes.Buffer))
	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"state", resumeRecoverSubcommand}, args...))
	err := rootCmd.Execute()
	if err != nil {
		t.Logf("stderr: %s", errBuf)
	}
	return parsed, err
}

func TestStateResumeRecoverCommand_ForeignArgv(t *testing.T) {
	full := resumeChainPayload{
		Command:   "claude --resume abc",
		Report:    "could not clear the resume-pending flag",
		Session:   recoverSession,
		Width:     120,
		Height:    40,
		Screen:    resumeScreenDiscard,
		DropInput: true,
	}
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "the whole payload an earlier build could have handed it",
			args: resumeChainArgv("portal", resumeDrawSubcommand, full)[3:],
		},
		{
			name: "a flag a later build added, with its value attached",
			args: []string{"--session", recoverSession, "--resume-attempt=3"},
		},
		{
			name: "a flag a later build added, with its value separate",
			args: []string{"--resume-attempt", "3", "--session", recoverSession},
		},
		{
			name: "a switch a later build added, last on the line",
			args: []string{"--session", recoverSession, "--quiet"},
		},
		{
			name: "a positional word this build takes none of",
			args: []string{"attempt-3", "--session", recoverSession},
		},
		{
			name: "a shorthand this build does not register",
			args: []string{"-q", "--session", recoverSession},
		},
		{
			name: "the pane flags a tmux-era build parked it with",
			args: []string{"--hook-key", "tok123", "--pane", "%7", "--pane-key", "proj-a1b2:0.1", "--session", recoverSession},
		},
	}
	for _, tc := range cases {
		t.Run("it recovers the session when handed "+tc.name, func(t *testing.T) {
			dir := resumeStateDir(t)
			raiseFlag(t, dir, recoverSession)
			var probe resumeRecoverProbe
			sink := logtest.Install(t)

			parsed, err := executeResumeRecover(t, tc.args, &probe)
			if err != nil {
				t.Fatalf("the tail refused its argv %q: %v", tc.args, err)
			}

			if parsed.Session != recoverSession {
				t.Errorf("session = %q, want %q", parsed.Session, recoverSession)
			}
			assertRecovered(t, &probe, sink)
			if flagRaised(t, dir, recoverSession) {
				t.Error("the flag is still raised")
			}
		})
	}
}

func TestStateResumeDrawCommand_RefusesAFlagItDoesNotRegister(t *testing.T) {
	withFuncSeam(t, &resumeDrawRunFunc, func(resumeDrawConfig) error { return nil })

	resetRootCmd()
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs(append(resumeChainArgv("portal", resumeDrawSubcommand, samplePayload())[1:], "--resume-attempt=3"))
	if err := rootCmd.Execute(); err == nil {
		t.Error("resume-draw accepted a flag it does not register; only the chain's tail tolerates one")
	}
}
