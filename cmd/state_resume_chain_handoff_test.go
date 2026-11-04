package cmd

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
)

func TestResumeHandOff(t *testing.T) {
	t.Run("it emits the exec marker before it hands the process image over", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		var atExec logtest.Records
		var gotProg string
		var gotArgs []string

		err := resumeHandOff(logger, func(prog string, args []string) {
			atExec = sink.Records().WithMessage("exec")
			gotProg, gotArgs = prog, args
		}, resumeWaitSubcommand, samplePayload())
		if err != nil {
			t.Fatalf("resumeHandOff() error = %v", err)
		}

		if len(atExec) != 1 {
			t.Fatalf("exec records captured when the hand-off ran = %d, want exactly 1", len(atExec))
		}
		rec := atExec[0]
		if rec.Level != slog.LevelInfo {
			t.Errorf("exec marker level = %v, want %v", rec.Level, slog.LevelInfo)
		}
		if got := rec.AttrString(t, "target"); got != gotProg {
			t.Errorf("exec marker target = %q, want the handed-over program %q", got, gotProg)
		}
		if got := rec.AttrString(t, "args"); got != strings.Join(gotArgs, " ") {
			t.Errorf("exec marker args = %q, want %q", got, strings.Join(gotArgs, " "))
		}
		if rec.HasAttr("hook_present") {
			t.Error("the chain's hand-off marker carries hook_present")
		}
		if want := resumeChainArgv(gotProg, resumeWaitSubcommand, samplePayload()); !slices.Equal(gotArgs, want) {
			t.Errorf("handed-over argv = %q, want %q", gotArgs, want)
		}
	})

	t.Run("it renders one resolve portal executable clause for an empty path", func(t *testing.T) {
		withFuncSeam(t, &osExecutable, func() (string, error) { return "", nil })
		logger, _ := logtest.NewCaptureLogger(t)

		execs := 0
		err := resumeHandOff(logger, func(string, []string) { execs++ }, resumeWaitSubcommand, samplePayload())
		if err == nil {
			t.Fatal("resumeHandOff() error = nil, want a resolution failure")
		}
		if got := strings.Count(err.Error(), "resolve portal executable:"); got != 1 {
			t.Errorf("hand-off error = %q, carries the prefix %d times, want exactly 1", err, got)
		}
		if execs != 0 {
			t.Errorf("exec called %d times for an unresolvable binary, want 0", execs)
		}
	})
}

func TestResumeChainExeResolutionFailure(t *testing.T) {
	t.Run("it renders one resolve portal executable clause for an empty path", func(t *testing.T) {
		withFuncSeam(t, &osExecutable, func() (string, error) { return "", nil })

		_, err := resumeChainExe()
		if err == nil {
			t.Fatal("resumeChainExe() error = nil, want a refusal of the empty path")
		}
		if got := strings.Count(err.Error(), "resolve portal executable:"); got != 1 {
			t.Errorf("error = %q, carries the prefix %d times, want exactly 1", err, got)
		}
	})

	t.Run("it renders one resolve portal executable clause for a failed resolution", func(t *testing.T) {
		cause := errors.New("boom")
		withFuncSeam(t, &osExecutable, func() (string, error) { return "", cause })

		_, err := resumeChainExe()
		if err == nil {
			t.Fatal("resumeChainExe() error = nil, want the resolution failure")
		}
		if got := strings.Count(err.Error(), "resolve portal executable:"); got != 1 {
			t.Errorf("error = %q, carries the prefix %d times, want exactly 1", err, got)
		}
		if !errors.Is(err, cause) {
			t.Errorf("error = %q, does not wrap the resolution failure", err)
		}
	})
}

func TestExecHandOff(t *testing.T) {
	t.Run("it emits the exec marker before it hands the process image over", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		var atExec logtest.Records

		execHandOff(logger, func(string, []string) {
			atExec = sink.Records().WithMessage("exec")
		}, "/bin/zsh", []string{"zsh"}, true)

		if len(atExec) != 1 {
			t.Fatalf("exec records captured when the hand-off ran = %d, want exactly 1", len(atExec))
		}
		rec := atExec[0]
		if rec.Level != slog.LevelInfo {
			t.Errorf("exec marker level = %v, want %v", rec.Level, slog.LevelInfo)
		}
		if got := rec.AttrString(t, "target"); got != "/bin/zsh" {
			t.Errorf("exec marker target = %q, want %q", got, "/bin/zsh")
		}
		if got := rec.AttrString(t, "args"); got != "zsh" {
			t.Errorf("exec marker args = %q, want %q", got, "zsh")
		}
		if got := rec.AttrString(t, "hook_present"); got != "true" {
			t.Errorf("exec marker hook_present = %q, want %q", got, "true")
		}
	})

	t.Run("it reports a hand-off carrying no registered command", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)

		execHandOff(logger, func(string, []string) {}, "/bin/sh", []string{"sh"}, false)

		rec := sink.Records().WithMessage("exec").Only(t, "the exec marker")
		if got := rec.AttrString(t, "hook_present"); got != "false" {
			t.Errorf("exec marker hook_present = %q, want %q", got, "false")
		}
	})
}

func TestHandOffToHookOrShell(t *testing.T) {
	tests := []struct {
		name        string
		shellEnv    string
		command     string
		wantProg    string
		wantArgs    []string
		wantPresent string
	}{
		{
			name:        "it hands a session with no command its login shell, as zmx starts a fresh one's",
			shellEnv:    "/bin/zsh",
			wantProg:    "/bin/zsh",
			wantArgs:    []string{"-zsh"},
			wantPresent: "false",
		},
		{
			name:        "it hands a session with no command /bin/sh when SHELL is unset",
			wantProg:    "/bin/sh",
			wantArgs:    []string{"-sh"},
			wantPresent: "false",
		},
		{
			name:        "it hands a session its command under sh followed by its login shell",
			shellEnv:    "/bin/zsh",
			command:     "claude --resume 'x'",
			wantProg:    "/bin/sh",
			wantArgs:    []string{"sh", "-c", "trap : TERM; claude --resume 'x'; exec '/bin/zsh' -l"},
			wantPresent: "true",
		},
		{
			name:        "it follows the command with /bin/sh when SHELL is unset",
			command:     "make dev",
			wantProg:    "/bin/sh",
			wantArgs:    []string{"sh", "-c", "trap : TERM; make dev; exec '/bin/sh' -l"},
			wantPresent: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.shellEnv)
			logger, sink := logtest.NewCaptureLogger(t)
			var atExec logtest.Records
			var gotProg string
			var gotArgs []string
			execs := 0

			handOffToHookOrShell(logger, func(prog string, args []string) {
				execs++
				atExec = sink.Records().WithMessage("exec")
				gotProg, gotArgs = prog, args
			}, tt.command)

			if execs != 1 {
				t.Fatalf("exec called %d times, want exactly 1", execs)
			}
			if gotProg != tt.wantProg {
				t.Errorf("handed-over program = %q, want %q", gotProg, tt.wantProg)
			}
			if !slices.Equal(gotArgs, tt.wantArgs) {
				t.Errorf("handed-over argv = %q, want %q", gotArgs, tt.wantArgs)
			}
			if len(atExec) != 1 {
				t.Fatalf("exec records captured when the hand-off ran = %d, want exactly 1", len(atExec))
			}
			rec := atExec[0]
			if got := rec.AttrString(t, "target"); got != tt.wantProg {
				t.Errorf("exec marker target = %q, want %q", got, tt.wantProg)
			}
			if got := rec.AttrString(t, "args"); got != strings.Join(tt.wantArgs, " ") {
				t.Errorf("exec marker args = %q, want %q", got, strings.Join(tt.wantArgs, " "))
			}
			if got := rec.AttrString(t, "hook_present"); got != tt.wantPresent {
				t.Errorf("exec marker hook_present = %q, want %q", got, tt.wantPresent)
			}
		})
	}
}

// The exec a session's shell is handed to searches no PATH, while zmx's
// execvpe does, so a $SHELL naming no path is found on PATH first. Whatever
// it finds is quoted into a command's argv.
func TestHandOffToHookOrShell_HandsTheShellItFinds(t *testing.T) {
	bin := t.TempDir()
	shell := filepath.Join(bin, "myshell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	handOff := func(t *testing.T, command string) (string, []string) {
		t.Helper()
		var prog string
		var args []string
		handOffToHookOrShell(log.Discard(), func(p string, a []string) { prog, args = p, a }, command)
		return prog, args
	}

	t.Run("it hands a session with no command the shell PATH finds, still a login shell", func(t *testing.T) {
		t.Setenv("SHELL", "myshell")
		if prog, args := handOff(t, ""); prog != shell || !slices.Equal(args, []string{"-myshell"}) {
			t.Errorf("handed %q %q, want %q %q", prog, args, shell, []string{"-myshell"})
		}
	})

	t.Run("it follows a command with the shell PATH finds", func(t *testing.T) {
		t.Setenv("SHELL", "myshell")
		want := []string{"sh", "-c", "trap : TERM; make dev; exec '" + shell + "' -l"}
		if _, args := handOff(t, "make dev"); !slices.Equal(args, want) {
			t.Errorf("handed argv %q, want %q", args, want)
		}
	})

	t.Run("it quotes a shell whose path a word split would break", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "My Shells")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		spaced := stageExecutable(t, dir, "fish", "#!/bin/sh\n")
		t.Setenv("SHELL", spaced)
		want := []string{"sh", "-c", "trap : TERM; make dev; exec '" + spaced + "' -l"}
		if _, args := handOff(t, "make dev"); !slices.Equal(args, want) {
			t.Errorf("handed argv %q, want %q", args, want)
		}
	})
}

// An exec of a shell that isn't there fails, which would end the restored
// session; the saver would then drop it, and its transcript with it.
func TestHandOffToHookOrShell_FallsBackToShForAShellItCannotFind(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for name, shell := range map[string]string{
		"a path with no shell at it": filepath.Join(t.TempDir(), "zsh"),
		"a name PATH doesn't hold":   "nosuchshell",
	} {
		t.Run("it hands a session /bin/sh for "+name+", run as -sh, with one WARN", func(t *testing.T) {
			t.Setenv("SHELL", shell)
			logger, sink := logtest.NewCaptureLogger(t)
			var prog string
			var args []string

			handOffToHookOrShell(logger, func(p string, a []string) { prog, args = p, a }, "")

			if prog != "/bin/sh" || !slices.Equal(args, []string{"-sh"}) {
				t.Errorf("handed %q %q, want /bin/sh run as -sh", prog, args)
			}
			warn := sink.Records().AtExactLevel(slog.LevelWarn).Only(t, "the fallback's WARN")
			if got := warn.AttrString(t, "path"); got != shell {
				t.Errorf("WARN path = %q, want the shell it couldn't find, %q", got, shell)
			}
		})

		t.Run("it follows a command with /bin/sh for "+name, func(t *testing.T) {
			t.Setenv("SHELL", shell)
			var args []string

			handOffToHookOrShell(log.Discard(), func(_ string, a []string) { args = a }, "make dev")

			if want := []string{"sh", "-c", "trap : TERM; make dev; exec '/bin/sh' -l"}; !slices.Equal(args, want) {
				t.Errorf("handed argv %q, want %q", args, want)
			}
		})
	}
}

// wantLoginShell is the argv a session's shell is handed with: run as a login
// shell, as zmx runs a fresh session's, its base name after a dash.
func wantLoginShell(shell string) []string {
	return []string{"-" + filepath.Base(shell)}
}
