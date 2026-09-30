package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"

	"github.com/leeovery/portal/internal/state"
)

// The user's shell, stty and tmux are replaced by stubs appending one line each
// to the pane's transcript, so the transcript orders the bytes the chain wrote
// among the programs it ran.
const (
	backstopShell = `#!/bin/sh
echo shell >> "$PANE_TRANSCRIPT"
`
	backstopStty = `#!/bin/sh
echo "stty $*" >> "$PANE_TRANSCRIPT"
`
	backstopTmux = `#!/bin/sh
echo "tmux $*" >> "$PANE_TRANSCRIPT"
case "$1" in display-message) echo "${ALTERNATE_ON:-0}" ;; esac
`
	// flippingTmux reports the pane on the alternate screen for its first two
	// reads, as a leave tmux has not yet processed would.
	flippingTmux = `#!/bin/sh
echo "tmux $*" >> "$PANE_TRANSCRIPT"
if [ "$1" = display-message ]; then
	n=0
	while IFS= read -r line; do
		case $line in *display-message*) n=$((n+1)) ;; esac
	done < "$PANE_TRANSCRIPT"
	if [ "$n" -ge 3 ]; then echo 0; else echo 1; fi
fi
`
	failingTmux = `#!/bin/sh
echo "tmux $*" >> "$PANE_TRANSCRIPT"
echo "no server running on /tmp/tmux-501/default" >&2
exit 1
`
)

// statusChainExe stands in for the portal binary under a parked chain: each hop
// exits with the status the test names, as the shell a hop exec'd would.
const statusChainExe = `#!/bin/sh
case "$2" in
resume-draw) exit "$DRAW_STATUS" ;;
resume-recover) exit "$RECOVER_STATUS" ;;
esac
`

type parkedRun struct {
	transcript string
	stderr     string
	status     int

	// backstopStderr is stderr less the shell's own report of the unstartable
	// executable: what the backstop's steps wrote.
	backstopStderr string
}

type parkedStubs struct {
	tmux string // empty stages no tmux at all
}

// runParkedChain runs the chain a waiting pane parks under /bin/sh with the
// stubs alone on PATH, so no tmux but a stub is ever reached.
func runParkedChain(t *testing.T, exe string, stubs parkedStubs, env ...string) parkedRun {
	t.Helper()
	dir := t.TempDir()
	shell := stageExecutable(t, dir, "user-shell", backstopShell)
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatalf("stage the stub bin dir: %v", err)
	}
	stageExecutable(t, bin, "stty", backstopStty)
	if stubs.tmux != "" {
		stageExecutable(t, bin, "tmux", stubs.tmux)
	}
	transcriptPath := filepath.Join(dir, "transcript")
	transcript, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open the pane transcript: %v", err)
	}
	t.Cleanup(func() { _ = transcript.Close() })

	parked := exec.Command("/bin/sh", "-c", parkedResumeChain(exe, samplePayload()))
	parked.Env = append(os.Environ(),
		"SHELL="+shell,
		"PATH="+bin,
		"PANE_TRANSCRIPT="+transcriptPath,
	)
	parked.Env = append(parked.Env, env...)
	parked.Stdout = transcript
	var stderr strings.Builder
	parked.Stderr = &stderr
	err = parked.Run()

	run := parkedRun{stderr: stderr.String()}
	for line := range strings.Lines(run.stderr) {
		if !strings.Contains(line, exe) {
			run.backstopStderr += line
		}
	}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		run.status = exitErr.ExitCode()
	case err != nil:
		t.Fatalf("run the parked chain: %v", err)
	}
	logged, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("read the pane transcript: %v", err)
	}
	run.transcript = string(logged)
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
	target := string(tmux.PaneIDTarget(samplePayload().Pane))
	clearLine := "tmux set-option -pu -t " + target + " " + state.ResumePendingOption + "\n"
	readLine := "tmux display-message -p -t " + target + " -F #{" + alternateOnFormat + "}\n"
	unpinLine := "tmux set-option -pu -t " + target + " " + altScreenOption + "\n"
	recovered := hydrateResetPreamble + clearLine + readLine + unpinLine + "stty sane\n" + "shell\n"
	for _, tc := range unstartable {
		t.Run("a tail "+tc.name+" resets the pane, clears its marker, lifts its pin and restores the terminal before the user's shell", func(t *testing.T) {
			run := runParkedChain(t, tc.stage(t), parkedStubs{tmux: backstopTmux})

			if run.transcript != recovered {
				t.Errorf("pane transcript = %q, want %q", run.transcript, recovered)
			}
			if run.status != 0 {
				t.Errorf("parked chain status = %d, want the user's shell's 0", run.status)
			}
		})

		t.Run("a tail "+tc.name+" polls until the pane is off the alternate screen before it lifts the pin", func(t *testing.T) {
			run := runParkedChain(t, tc.stage(t), parkedStubs{tmux: flippingTmux})

			want := hydrateResetPreamble + clearLine + strings.Repeat(readLine, 3) + unpinLine + "stty sane\nshell\n"
			if run.transcript != want {
				t.Errorf("pane transcript = %q, want %q", run.transcript, want)
			}
		})

		t.Run("a tail "+tc.name+" keeps the pin when the leave is never confirmed", func(t *testing.T) {
			run := runParkedChain(t, tc.stage(t), parkedStubs{tmux: backstopTmux}, "ALTERNATE_ON=1")

			want := hydrateResetPreamble + clearLine + strings.Repeat(readLine, altScreenLeaveAttempts) + "stty sane\nshell\n"
			if run.transcript != want {
				t.Errorf("pane transcript = %q, want %q", run.transcript, want)
			}
			if run.backstopStderr != "" {
				t.Errorf("the backstop wrote %q to the pane's stderr, want nothing", run.backstopStderr)
			}
			if run.status != 0 {
				t.Errorf("parked chain status = %d, want the user's shell's 0", run.status)
			}
		})

		clearFailures := []struct {
			name, tmux, tmuxCalls, leak string
		}{
			{name: "tmux refusing the clear", tmux: failingTmux, tmuxCalls: clearLine + strings.Repeat(readLine, altScreenLeaveAttempts), leak: "no server running"},
			{name: "no tmux on PATH", tmux: "", tmuxCalls: "", leak: "tmux:"},
		}
		for _, cf := range clearFailures {
			t.Run("a tail "+tc.name+" still reaches the user's shell with "+cf.name, func(t *testing.T) {
				run := runParkedChain(t, tc.stage(t), parkedStubs{tmux: cf.tmux})

				if want := hydrateResetPreamble + cf.tmuxCalls + "stty sane\nshell\n"; run.transcript != want {
					t.Errorf("pane transcript = %q, want %q", run.transcript, want)
				}
				if strings.Contains(run.stderr, cf.leak) {
					t.Errorf("the failed clear reached the pane: stderr %q", run.stderr)
				}
				if run.backstopStderr != "" {
					t.Errorf("the backstop wrote %q to the pane's stderr, want nothing", run.backstopStderr)
				}
				if run.status != 0 {
					t.Errorf("parked chain status = %d, want the user's shell's 0", run.status)
				}
			})
		}
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

			run := runParkedChain(t, exe, parkedStubs{tmux: backstopTmux}, "DRAW_STATUS="+tc.draw, "RECOVER_STATUS="+tc.recover)

			if run.transcript != "" {
				t.Errorf("the backstop acted on a pane whose tail started; transcript %q", run.transcript)
			}
			if run.status != tc.wantStatus {
				t.Errorf("parked chain status = %d, want %d", run.status, tc.wantStatus)
			}
		})
	}
}
