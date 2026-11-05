package cmd

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/proctest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// stageVersion1State points the state dir at one holding the tmux version's
// sessions.json, which every command but the exempt refuses.
func stageVersion1State(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), []byte(`{"version": 1, "sessions": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORTAL_STATE_DIR", dir)
}

// executeQuiet runs the root command on args, returning its error and all it
// wrote. Flags left set on the commands it reaches are reset at cleanup.
func executeQuiet(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetRootCmd()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		if c, _, err := rootCmd.Find(args); err == nil {
			resetFlags(c)
		}
	})
	err := rootCmd.Execute()
	return out.String(), err
}

func resetFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

func refuseBootstrap(t *testing.T) {
	t.Helper()
	withBootstrapDeps(t, BootstrapDeps{Owed: func(context.Context) bool {
		t.Error("the gate was asked: the command ran the bootstrap's path")
		return false
	}})
}

func TestUpgradeHookSubcommands(t *testing.T) {
	forms := [][]string{
		nil,
		{"--", "#{session_name}"},
		{"--help", "--pane", "%1", "-x", "loose"},
	}
	for _, hook := range stateUpgradeHookCmds {
		name := hook.Name()
		for _, form := range forms {
			t.Run(strings.Join(append([]string{name}, form...), " "), func(t *testing.T) {
				stageVersion1State(t)
				refuseBootstrap(t)
				sink := logtest.Install(t)

				out, err := executeQuiet(t, append([]string{"state", name}, form...)...)

				if err != nil {
					t.Fatalf("exit with %v, want 0", err)
				}
				if out != "" {
					t.Errorf("wrote %q, want nothing", out)
				}
				if loud := sink.Records().AtOrAboveLevel(slog.LevelInfo); len(loud) > 0 {
					t.Errorf("logged above DEBUG: %v", loud)
				}
			})
		}
	}
}

// heldForms are the forms the tmux version execs into this binary, each as
// it composes them: its chain's draw, waiter and tail, and its hydrate helper.
var heldForms = []struct {
	name string
	args []string
}{
	{"draw", []string{"state", "resume-draw", "--command", "claude --resume", "--report", "r", "--hook-key", "k", "--pane", "%3", "--pane-key", "work:1.0", "--width", "80", "--height", "24", "--screen", "discard", "--drop-input", "--input-arriving"}},
	{"draw with no token", []string{"state", "resume-draw", "--command", "c", "--hook-key", "", "--pane", "%3", "--pane-key", "work:1.0"}},
	{"waiter", []string{"state", "resume-wait", "--command", "c", "--hook-key", "k", "--pane", "%3", "--pane-key", "work:1.0", "--width", "80", "--height", "24", "--input-arriving"}},
	{"tail", []string{"state", "resume-recover", "--hook-key", "k", "--pane", "%3", "--pane-key", "work:1.0"}},
	{"tail beside a session", []string{"state", "resume-recover", "--hook-key", "k", "--pane", "%3", "--pane-key", "work:1.0", "--session", "work"}},
	{"hydrate", []string{"state", "hydrate", "--fifo", "/s/fifos/work-1-0", "--file", "/s/scrollback/work.bin", "--hook-key", "k"}},
	{"hydrate with no token", []string{"state", "hydrate", "--fifo", "/s/fifos/work-1-0", "--file", "/s/scrollback/work.bin"}},
	{"hydrate beside a session", []string{"state", "hydrate", "--session", "work", "--hook-key", "k"}},
}

// refuseBodies fails the test should any held command's body run.
func refuseBodies(t *testing.T) {
	t.Helper()
	withFuncSeam(t, &resumeDrawRunFunc, func(resumeDrawConfig) error { t.Error("the draw ran"); return nil })
	withFuncSeam(t, &resumeWaitRunFunc, func(resumeWaitConfig) error { t.Error("the waiter ran"); return nil })
	withFuncSeam(t, &resumeRecoverRunFunc, func(resumeRecoverConfig) error { t.Error("the tail ran"); return nil })
	withFuncSeam(t, &hydrateRunFunc, func(context.Context, hydrateConfig) { t.Error("the helper ran") })
}

func TestUpgradeHold_TmuxFormIsHeld(t *testing.T) {
	for _, form := range heldForms {
		t.Run(form.name, func(t *testing.T) {
			stageVersion1State(t)
			refuseBootstrap(t)
			refuseBodies(t)
			var held []upgradeHold
			withFuncSeam(t, &upgradeHoldFunc, func(h upgradeHold) { held = append(held, h) })

			out, err := executeQuiet(t, form.args...)

			if len(held) != 1 {
				t.Fatalf("held %d times, want once", len(held))
			}
			if !slices.Equal(held[0].Argv, os.Args) {
				t.Errorf("held argv %q, want the process's own %q", held[0].Argv, os.Args)
			}
			if !errors.Is(err, errUpgradeHoldEnded) {
				t.Errorf("a hold that returned ended with %v, want errUpgradeHoldEnded", err)
			}
			if out != "" {
				t.Errorf("wrote %q, want nothing", out)
			}
		})
	}
}

func TestUpgradeHold_ZmxFormRuns(t *testing.T) {
	withFuncSeam(t, &upgradeHoldFunc, func(upgradeHold) { t.Error("the zmx form was held") })
	ran := ""
	withFuncSeam(t, &resumeDrawRunFunc, func(resumeDrawConfig) error { ran = "draw"; return nil })
	withFuncSeam(t, &resumeWaitRunFunc, func(resumeWaitConfig) error { ran = "wait"; return nil })
	withFuncSeam(t, &resumeRecoverRunFunc, func(resumeRecoverConfig) error { ran = "recover"; return nil })
	withFuncSeam(t, &hydrateRunFunc, func(context.Context, hydrateConfig) { ran = "hydrate" })

	for _, tc := range []struct {
		want string
		args []string
	}{
		{"draw", resumeChainArgv("portal", resumeDrawSubcommand, samplePayload())[1:]},
		{"wait", resumeChainArgv("portal", resumeWaitSubcommand, samplePayload())[1:]},
		{"recover", resumeChainArgv("portal", resumeRecoverSubcommand, samplePayload())[1:]},
		{"hydrate", []string{"state", "hydrate", "--session", "work", "--file", "/s/scrollback/work.bin", "--lazy"}},
	} {
		t.Run(tc.want, func(t *testing.T) {
			ran = ""
			if _, err := executeQuiet(t, tc.args...); err != nil {
				t.Fatalf("exit with %v", err)
			}
			if ran != tc.want {
				t.Errorf("ran %q, want %q", ran, tc.want)
			}
		})
	}
}

// fakeHoldSignals is a hold's signals, delivered by the test.
func fakeHoldSignals(sigs ...os.Signal) func() <-chan os.Signal {
	return func() <-chan os.Signal {
		ch := make(chan os.Signal, len(sigs))
		for _, s := range sigs {
			ch <- s
		}
		close(ch)
		return ch
	}
}

type heldExec struct {
	prog string
	argv []string
}

func TestHoldUntilResized(t *testing.T) {
	withFuncSeam(t, &osExecutable, func() (string, error) { return "/opt/homebrew/bin/portal", nil })
	argv := []string{"/opt/homebrew/bin/portal", "state", "resume-draw", "--command", "c", "--hook-key", "k", "--pane", "%3", "--pane-key", "work:1.0"}

	t.Run("a SIGTERM execs nothing and holds on", func(t *testing.T) {
		var execs []heldExec
		holdUntilResized(upgradeHold{
			Argv:    argv,
			Signals: fakeHoldSignals(syscall.SIGTERM, syscall.SIGTERM),
			Exec:    func(prog string, a []string) error { execs = append(execs, heldExec{prog, a}); return nil },
		})
		if len(execs) != 0 {
			t.Errorf("exec'd %v on SIGTERM, want nothing", execs)
		}
	})

	t.Run("each resize execs its own executable with the identical argv", func(t *testing.T) {
		var execs []heldExec
		holdUntilResized(upgradeHold{
			Argv:    argv,
			Signals: fakeHoldSignals(syscall.SIGTERM, syscall.SIGWINCH, syscall.SIGWINCH),
			Exec: func(prog string, a []string) error {
				execs = append(execs, heldExec{prog, a})
				return errors.New("exec refused")
			},
		})
		if len(execs) != 2 {
			t.Fatalf("exec'd %d times, want once per resize, a failed exec holding on", len(execs))
		}
		for _, e := range execs {
			if e.prog != "/opt/homebrew/bin/portal" || !slices.Equal(e.argv, argv) {
				t.Errorf("exec'd %s %q, want /opt/homebrew/bin/portal %q", e.prog, e.argv, argv)
			}
		}
	})
}

// The held process is run for real, as this test binary re-executed into
// upgradeHoldProcessName: a signal's disposition belongs to a whole process.
const (
	upgradeHoldProcessName = "TestUpgradeHoldProcess"
	upgradeHoldDirEnv      = "PORTAL_TEST_UPGRADE_HOLD_DIR"
)

// stubUpgradeHoldExe stands in for the executable a resize execs, recording
// the arguments it was handed.
const stubUpgradeHoldExe = `#!/bin/sh
printf '%s\n' "$@" > "$` + upgradeHoldDirEnv + `/reexec"
`

// TestUpgradeHoldProcess is not a test: it is the body of the held process,
// and does nothing unless started as one.
func TestUpgradeHoldProcess(t *testing.T) {
	dir := os.Getenv(upgradeHoldDirEnv)
	if dir == "" {
		return
	}
	withFuncSeam(t, &osExecutable, func() (string, error) { return filepath.Join(dir, "portal"), nil })
	withFuncSeam(t, &upgradeHoldFunc, func(h upgradeHold) {
		install := h.Signals
		h.Signals = func() <-chan os.Signal {
			ch := install()
			_ = os.WriteFile(filepath.Join(dir, "held"), nil, 0o600)
			return ch
		}
		holdUntilResized(h)
	})
	resetRootCmd()
	rootCmd.SetArgs(upgradeHoldArgs()[1:])
	err := rootCmd.Execute()
	t.Fatalf("the held process returned: %v", err)
}

func upgradeHoldArgs() []string {
	i := slices.Index(os.Args, "--")
	if i < 0 {
		return nil
	}
	return os.Args[i+1:]
}

type upgradeHoldProcess struct {
	dir  string
	args []string
	held *proctest.Held
}

func startUpgradeHoldProcess(t *testing.T) upgradeHoldProcess {
	t.Helper()
	dir := t.TempDir()
	stageStub(t, dir, "portal", stubUpgradeHoldExe)
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve the test binary: %v", err)
	}
	// The re-executed binary's TestMain makes a temp home it never removes
	// once its image is replaced, so its temp root is one this test removes.
	tmp := filepath.Join(dir, "tmp")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = out.Close() })

	args := append([]string{"-test.run=^" + upgradeHoldProcessName + "$", "--", "portal"}, heldForms[0].args...)
	cmd := exec.Command(bin, args...)
	marker := upgradeHoldDirEnv + "=" + dir
	cmd.Env = append(os.Environ(), marker, "TMPDIR="+tmp)
	cmd.Stdout, cmd.Stderr = out, out
	p := upgradeHoldProcess{dir: dir, args: args, held: proctest.Start(t, cmd, marker)}

	held := harnesstest.PollUntil(t, 10*time.Second, 10*time.Millisecond, func() bool {
		_, err := os.Stat(filepath.Join(dir, "held"))
		return err == nil
	})
	if !held {
		t.Fatalf("the process never held; it wrote %q", p.output(t))
	}
	return p
}

func (p upgradeHoldProcess) signal(t *testing.T, sig syscall.Signal) {
	t.Helper()
	if err := p.held.Signal(p.held.PID(), sig); err != nil {
		t.Fatal(err)
	}
}

func (p upgradeHoldProcess) output(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestUpgradeHold_Process(t *testing.T) {
	t.Run("it catches SIGTERM, draws nothing, and on a resize execs itself with the identical argv", func(t *testing.T) {
		p := startUpgradeHoldProcess(t)

		p.signal(t, syscall.SIGTERM)
		select {
		case <-p.held.Exited():
			t.Fatalf("a SIGTERM ended the held process: %v", p.held.Status(t))
		case <-time.After(300 * time.Millisecond):
		}

		p.signal(t, syscall.SIGWINCH)
		select {
		case <-p.held.Exited():
		case <-time.After(10 * time.Second):
			t.Fatal("a resize didn't replace the held process")
		}
		raw, err := os.ReadFile(filepath.Join(p.dir, "reexec"))
		if err != nil {
			t.Fatalf("the exec'd executable recorded nothing: %v", err)
		}
		if got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n"); !slices.Equal(got, p.args) {
			t.Errorf("re-exec'd with %q, want the identical %q", got, p.args)
		}
		if out := p.output(t); out != "" {
			t.Errorf("wrote %q, want nothing", out)
		}
	})

	t.Run("a SIGHUP still ends it", func(t *testing.T) {
		p := startUpgradeHoldProcess(t)

		p.signal(t, syscall.SIGHUP)
		select {
		case <-p.held.Exited():
		case <-time.After(10 * time.Second):
			t.Fatal("a SIGHUP didn't end the held process")
		}
		if status := p.held.Status(t); !status.Signaled() || status.Signal() != syscall.SIGHUP {
			t.Errorf("ended %v, want by SIGHUP", status)
		}
	})
}
