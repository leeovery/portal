package cmd

import (
	"bytes"
	"errors"
	"go/ast"
	"go/token"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// resumeRecoverProbe records what the tail hands to its seams, so a test asserts
// on the recovery without a marker being read from tmux or a process image being
// replaced.
type resumeRecoverProbe struct {
	stdout bytes.Buffer

	// order names each seam in the sequence it was reached, so a recovery whose
	// steps ran out of order fails on the sequence rather than on call counts.
	order []string

	markerValue string
	markerErr   error
	readCalls   int

	clearErr   error
	clearCalls int

	execProg  string
	execArgs  []string
	execCalls int
}

// recoverWriter records that the pane was written to before it passes the bytes
// on, so the leave sequence takes its place in the same sequence as the seams.
type recoverWriter struct {
	probe *resumeRecoverProbe
}

func (w recoverWriter) Write(p []byte) (int, error) {
	w.probe.order = append(w.probe.order, "stdout")
	return w.probe.stdout.Write(p)
}

func newResumeRecoverConfig(t *testing.T, p *resumeRecoverProbe) resumeRecoverConfig {
	t.Helper()
	return resumeRecoverConfig{
		Pane:    "%7",
		PaneKey: "proj-a1b2:0.1",
		Stdout:  recoverWriter{probe: p},
		Logger:  hydrateLogger,
		ReadMarker: func() (string, error) {
			p.order = append(p.order, "read")
			p.readCalls++
			return p.markerValue, p.markerErr
		},
		ClearMarker: func() error {
			p.order = append(p.order, "clear")
			p.clearCalls++
			return p.clearErr
		},
		ExecShell: func(prog string, args []string) {
			p.order = append(p.order, "exec")
			p.execProg = prog
			p.execArgs = args
			p.execCalls++
		},
	}
}

func TestRunResumeRecover(t *testing.T) {
	t.Run("it does nothing for a pane that was already answered", func(t *testing.T) {
		var probe resumeRecoverProbe

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		if probe.readCalls != 1 {
			t.Errorf("ReadMarker called %d times, want exactly 1", probe.readCalls)
		}
		if probe.stdout.Len() != 0 {
			t.Errorf("the tail wrote %q to stdout, want nothing", probe.stdout.String())
		}
		if probe.clearCalls != 0 {
			t.Errorf("ClearMarker called %d times, want 0", probe.clearCalls)
		}
		if probe.execCalls != 0 {
			t.Errorf("ExecShell called %d times, want 0: an answered pane already ran its own shell", probe.execCalls)
		}
	})

	t.Run("it recovers a pane whose marker is still set", func(t *testing.T) {
		var probe resumeRecoverProbe
		probe.markerValue = "1"
		sink := logtest.Install(t)

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		assertRecovered(t, &probe, sink)
	})

	t.Run("it treats a failed marker read as still pending", func(t *testing.T) {
		var probe resumeRecoverProbe
		probe.markerErr = errors.New("no pane answers to \"%7\"")
		sink := logtest.Install(t)

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		assertRecovered(t, &probe, sink)
	})

	t.Run("it execs the shell even when the clear failed", func(t *testing.T) {
		var probe resumeRecoverProbe
		probe.markerValue = "1"
		probe.clearErr = errors.New("no such pane")
		sink := logtest.Install(t)

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		assertRecovered(t, &probe, sink)
	})

	t.Run("it leaves the panel's screen before it drops the protection", func(t *testing.T) {
		var probe resumeRecoverProbe
		probe.markerValue = "1"

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
		var probe resumeRecoverProbe
		probe.markerValue = "1"

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
				t.Errorf("%s imports %s; the tail makes a pane usable and consults no registration", source.Path, path)
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

// assertRecovered pins the whole recovery: the pane leaves the panel's screen,
// its protection is dropped, and it is handed the user's shell whether or not
// the drop landed.
func assertRecovered(t *testing.T, p *resumeRecoverProbe, sink *logtest.Sink) {
	t.Helper()
	if p.readCalls != 1 {
		t.Errorf("ReadMarker called %d times, want exactly 1", p.readCalls)
	}
	if got := p.stdout.String(); got != hydrateResetPreamble {
		t.Errorf("stdout = %q, want the leave sequence %q", got, hydrateResetPreamble)
	}
	if p.clearCalls != 1 {
		t.Errorf("ClearMarker called %d times, want exactly 1", p.clearCalls)
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
	const message = "unset resume pending marker failed"

	t.Run("it records a failed clear as a WARN naming the pane and the error", func(t *testing.T) {
		var probe resumeRecoverProbe
		probe.markerValue = "1"
		probe.clearErr = errors.New("no such pane")
		sink := logtest.Install(t)

		if err := runResumeRecover(newResumeRecoverConfig(t, &probe)); err != nil {
			t.Fatalf("runResumeRecover() error = %v", err)
		}

		warned := sink.Records().AtOrAboveLevel(slog.LevelWarn)
		rec := warned.Only(t, "the failed clear's record")
		if !rec.Matches("hydrate", message) {
			t.Fatalf("WARN = (%q, %q), want (%q, %q)", rec.AttrOrEmpty("component"), rec.Msg, "hydrate", message)
		}
		if got := rec.AttrOrEmpty("pane_key"); got != "proj-a1b2:0.1" {
			t.Errorf("WARN pane_key = %q, want %q", got, "proj-a1b2:0.1")
		}
		if err := rec.ErrorAttr(t, "error"); !errors.Is(err, probe.clearErr) {
			t.Errorf("WARN error = %v, want %v", err, probe.clearErr)
		}
	})

	t.Run("it words the record apart from every other WARN a pane emits", func(t *testing.T) {
		if occurrences := warnMessages(t)[message]; occurrences != 1 {
			t.Errorf("%q is emitted by %d WARN sites, want 1: a shared wording stops a grep separating a pane that came back eager from one wrongly frozen", message, occurrences)
		}
	})

	t.Run("it records nothing when the clear succeeded", func(t *testing.T) {
		var probe resumeRecoverProbe
		probe.markerValue = "1"
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
		payload := resumeChainPayload{
			Command: "make deploy",
			HookKey: "tok123",
			Pane:    "%7",
			PaneKey: "proj-a1b2:0.1",
		}

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

		if got.Pane != payload.Pane {
			t.Errorf("pane = %q, want %q", got.Pane, payload.Pane)
		}
		if got.PaneKey != payload.PaneKey {
			t.Errorf("pane key = %q, want %q", got.PaneKey, payload.PaneKey)
		}
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
