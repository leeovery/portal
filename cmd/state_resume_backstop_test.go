package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// backstopShell stands in for the user's shell: it reports whether the terminal
// had been restored by the time it started.
const backstopShell = `#!/bin/sh
if [ -s "$STTY_LOG" ]; then echo SHELL-AFTER-STTY; else echo SHELL-BEFORE-STTY; fi
`

const backstopStty = `#!/bin/sh
echo "$@" >> "$STTY_LOG"
`

// statusChainExe stands in for the portal binary under a parked chain: each hop
// exits with the status the test names, as the shell a hop exec'd would.
const statusChainExe = `#!/bin/sh
case "$2" in
resume-draw) exit "$DRAW_STATUS" ;;
resume-recover) exit "$RECOVER_STATUS" ;;
esac
`

type parkedRun struct {
	out     string
	status  int
	sttyLog string
}

// runParkedChain runs the chain a waiting pane parks under /bin/sh, with the
// user's shell and stty replaced by stubs that record what reached them.
func runParkedChain(t *testing.T, exe string, env ...string) parkedRun {
	t.Helper()
	dir := t.TempDir()
	shell := stageExecutable(t, dir, "user-shell", backstopShell)
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatalf("stage the stub bin dir: %v", err)
	}
	stageExecutable(t, bin, "stty", backstopStty)
	sttyLog := filepath.Join(dir, "stty.log")

	parked := exec.Command("/bin/sh", "-c", parkedResumeChain(exe, samplePayload()))
	parked.Env = append(os.Environ(),
		"SHELL="+shell,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STTY_LOG="+sttyLog,
	)
	parked.Env = append(parked.Env, env...)
	out, err := parked.Output()

	run := parkedRun{out: string(out)}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		run.status = exitErr.ExitCode()
	case err != nil:
		t.Fatalf("run the parked chain: %v", err)
	}
	if logged, err := os.ReadFile(sttyLog); err == nil {
		run.sttyLog = string(logged)
	}
	return run
}

func stageExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatalf("stage %s: %v", name, err)
	}
	return path
}

func TestParkedResumeChain_Backstop(t *testing.T) {
	unstartable := []struct {
		name  string
		stage func(t *testing.T) string
	}{
		{name: "gone from the baked path", stage: func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "portal")
		}},
		{name: "no longer executable at the baked path", stage: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "portal")
			if err := os.WriteFile(path, []byte(statusChainExe), 0o600); err != nil {
				t.Fatalf("stage the unexecutable binary: %v", err)
			}
			return path
		}},
	}
	for _, tc := range unstartable {
		t.Run("a tail "+tc.name+" leaves the pane at the user's shell", func(t *testing.T) {
			run := runParkedChain(t, tc.stage(t))

			if !strings.Contains(run.out, "SHELL-") {
				t.Fatalf("the parked shell ended without handing the pane a shell (status %d, output %q)", run.status, run.out)
			}
			if run.status != 0 {
				t.Errorf("parked chain status = %d, want the user's shell's 0", run.status)
			}
		})

		t.Run("a tail "+tc.name+" restores the terminal before the shell starts", func(t *testing.T) {
			run := runParkedChain(t, tc.stage(t))

			if run.sttyLog != "sane\n" {
				t.Errorf("stty invoked with %q, want %q", run.sttyLog, "sane\n")
			}
			if strings.TrimSpace(run.out) != "SHELL-AFTER-STTY" {
				t.Errorf("output = %q, want the shell started after the terminal was restored", run.out)
			}
		})
	}

	started := []struct {
		name       string
		draw       string
		recover    string
		wantStatus int
	}{
		{name: "a pane answered on the panel", draw: "1", recover: "0", wantStatus: 0},
		{name: "a pane the tail recovered", draw: "0", recover: "1", wantStatus: 1},
		{name: "a pane the tail recovered, its shell exiting after a command not found", draw: "0", recover: "127", wantStatus: 127},
		{name: "a pane the tail recovered, its shell exiting after a command it could not run", draw: "0", recover: "126", wantStatus: 126},
	}
	for _, tc := range started {
		t.Run(tc.name+" closes on the first exit of its shell", func(t *testing.T) {
			exe := stageExecutable(t, t.TempDir(), "portal", statusChainExe)

			run := runParkedChain(t, exe, "DRAW_STATUS="+tc.draw, "RECOVER_STATUS="+tc.recover)

			if run.out != "" {
				t.Errorf("the backstop handed the pane a second shell; output %q", run.out)
			}
			if run.sttyLog != "" {
				t.Errorf("stty invoked with %q, want untouched", run.sttyLog)
			}
			if run.status != tc.wantStatus {
				t.Errorf("parked chain status = %d, want %d", run.status, tc.wantStatus)
			}
		})
	}
}
