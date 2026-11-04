package cmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// The user's shell and stty are replaced by stubs appending one line each to
// the session's transcript, so the transcript orders the bytes the chain wrote
// among the programs it ran.
const (
	backstopShell = `#!/bin/sh
echo "shell $*" >> "$PANE_TRANSCRIPT"
`
	backstopStty = `#!/bin/sh
echo "stty $*" >> "$PANE_TRANSCRIPT"
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

// termedOnceExe stands in for the portal binary under a parked chain whose
// first draw a SIGTERM ends: each draw records itself in the transcript, the
// first ending 143 and any later one 0.
const termedOnceExe = `#!/bin/sh
case "$2" in
resume-draw)
	echo draw >> "$PANE_TRANSCRIPT"
	[ "$(grep -c draw "$PANE_TRANSCRIPT")" -eq 1 ] && exit 143
	exit 0
	;;
resume-recover) exit 0 ;;
esac
`

// answeredThenGoneExe stands in for the portal binary under a session answered
// on its panel: its draw hands the session its shell, which exits, and by the
// time the tail runs the binary has left the baked path — or, with
// $KEEP_UNEXECUTABLE set, stays there with its execute bit gone.
const answeredThenGoneExe = `#!/bin/sh
case "$2" in
resume-draw)
	if [ -n "$KEEP_UNEXECUTABLE" ]; then /bin/chmod a-x "$0"; else /bin/rm -f "$0"; fi
	exit 0
	;;
esac
exit 99
`

type parkedRun struct {
	transcript string
	stderr     string
	status     int

	// backstopStderr is stderr less the shell's own report of the unstartable
	// executable: what the backstop's steps wrote.
	backstopStderr string
}

// runParkedChain runs the chain a waiting session parks under /bin/sh, its
// flag at flag, with the stubs ahead of the system's own tools on PATH.
func runParkedChain(t *testing.T, exe, flag string, env ...string) parkedRun {
	t.Helper()
	dir := t.TempDir()
	shell := stageExecutable(t, dir, "user-shell", backstopShell)
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatalf("stage the stub bin dir: %v", err)
	}
	stageExecutable(t, bin, "stty", backstopStty)
	transcriptPath := filepath.Join(dir, "transcript")
	transcript, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open the session transcript: %v", err)
	}
	t.Cleanup(func() { _ = transcript.Close() })

	parked := exec.Command("/bin/sh", "-c", parkedResumeChain(parkedChain{Exe: exe, Flag: flag}, samplePayload()))
	parked.Env = append(os.Environ(),
		"SHELL="+shell,
		"PATH="+bin+":/bin:/usr/bin",
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
		t.Fatalf("read the session transcript: %v", err)
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

// backstopFlag is where the sample session's flag lives in a state directory
// of the test's own; raise raises it there first.
func backstopFlag(t *testing.T, raise bool) (stateDir, flag string) {
	t.Helper()
	stateDir = t.TempDir()
	session := samplePayload().Session
	if raise {
		return stateDir, raiseFlag(t, stateDir, session)
	}
	flag, err := state.ResumePending.Path(stateDir, session)
	if err != nil {
		t.Fatal(err)
	}
	return stateDir, flag
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
	recovered := hydrateResetPreamble + "stty sane\n" + "shell -l\n"
	for _, tc := range unstartable {
		t.Run("a tail "+tc.name+" resets the session, lowers its flag and restores the terminal before the user's login shell", func(t *testing.T) {
			stateDir, flag := backstopFlag(t, true)

			run := runParkedChain(t, tc.stage(t), flag)

			if run.transcript != recovered {
				t.Errorf("session transcript = %q, want %q", run.transcript, recovered)
			}
			if flagRaised(t, stateDir, samplePayload().Session) {
				t.Error("the backstop left the session's flag raised")
			}
			if run.backstopStderr != "" {
				t.Errorf("the backstop wrote %q to the session's stderr, want nothing", run.backstopStderr)
			}
			if run.status != 0 {
				t.Errorf("parked chain status = %d, want the user's shell's 0", run.status)
			}
		})

		t.Run("a tail "+tc.name+" counts a flag it can't look for as raised, and reaches the shell though it can't lower it", func(t *testing.T) {
			_, flag := backstopFlag(t, true)
			unsearchable(t, filepath.Dir(flag))

			run := runParkedChain(t, tc.stage(t), flag)

			if run.transcript != recovered {
				t.Errorf("session transcript = %q, want %q", run.transcript, recovered)
			}
			if run.backstopStderr != "" {
				t.Errorf("the failed removal reached the session: stderr %q", run.backstopStderr)
			}
			if run.status != 0 {
				t.Errorf("parked chain status = %d, want the user's shell's 0", run.status)
			}
		})

		t.Run("a tail "+tc.name+" counts a flag whose state directory is gone as raised", func(t *testing.T) {
			stateDir, flag := backstopFlag(t, false)
			if err := os.Remove(stateDir); err != nil {
				t.Fatal(err)
			}

			run := runParkedChain(t, tc.stage(t), flag)

			if run.transcript != recovered {
				t.Errorf("session transcript = %q, want %q", run.transcript, recovered)
			}
			if run.status != 0 {
				t.Errorf("parked chain status = %d, want the user's shell's 0", run.status)
			}
		})
	}

	lowered := []struct {
		name  string
		stage func(t *testing.T) string
	}{
		{name: "a lowered flag", stage: func(t *testing.T) string {
			stateDir, flag := backstopFlag(t, true)
			if _, err := state.ResumePending.Clear(stateDir, samplePayload().Session); err != nil {
				t.Fatal(err)
			}
			return flag
		}},
		{name: "no flag ever raised", stage: func(t *testing.T) string {
			_, flag := backstopFlag(t, false)
			return flag
		}},
	}
	answered := []struct {
		name       string
		env        []string
		wantStatus int
	}{
		{name: "gone from the baked path", wantStatus: 127},
		{name: "no longer executable at the baked path", env: []string{"KEEP_UNEXECUTABLE=1"}, wantStatus: 126},
	}
	for _, flag := range lowered {
		for _, tc := range answered {
			t.Run("a session answered on the panel, "+flag.name+", its binary "+tc.name+" by its shell's exit, closes on that first exit", func(t *testing.T) {
				exe := stageExecutable(t, t.TempDir(), "portal", answeredThenGoneExe)

				run := runParkedChain(t, exe, flag.stage(t), tc.env...)

				if run.transcript != "" {
					t.Errorf("session transcript = %q, want nothing from the backstop", run.transcript)
				}
				if run.backstopStderr != "" {
					t.Errorf("the backstop wrote %q to the session's stderr, want nothing", run.backstopStderr)
				}
				if run.status != tc.wantStatus {
					t.Errorf("parked chain status = %d, want the could-not-run %d", run.status, tc.wantStatus)
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
		{name: "a session answered on the panel", draw: "1", recover: "0", wantStatus: 0},
		{name: "a session the tail recovered", draw: "0", recover: "1", wantStatus: 1},
		{name: "a session the tail recovered, its shell exiting after a command not found", draw: "0", recover: "127", wantStatus: 127},
		{name: "a session the tail recovered, its shell exiting after a command it could not run", draw: "0", recover: "126", wantStatus: 126},
	}
	for _, tc := range started {
		t.Run(tc.name+" closes on the first exit of its shell", func(t *testing.T) {
			exe := stageExecutable(t, t.TempDir(), "portal", statusChainExe)
			stateDir, flag := backstopFlag(t, true)

			run := runParkedChain(t, exe, flag, "DRAW_STATUS="+tc.draw, "RECOVER_STATUS="+tc.recover)

			if run.transcript != "" {
				t.Errorf("the backstop acted on a session whose tail started; transcript %q", run.transcript)
			}
			if !flagRaised(t, stateDir, samplePayload().Session) {
				t.Error("the backstop lowered the flag of a session whose tail started")
			}
			if run.status != tc.wantStatus {
				t.Errorf("parked chain status = %d, want %d", run.status, tc.wantStatus)
			}
		})
	}
}

func TestParkedResumeChain_DrawLoop(t *testing.T) {
	for _, w := range flagWorlds() {
		want, verdict := "draw\n", "ends the loop"
		if w.raised {
			want, verdict = "draw\ndraw\n", "draws again"
		}
		t.Run("a draw a SIGTERM ended over a flag "+w.name+" "+verdict, func(t *testing.T) {
			exe := stageExecutable(t, t.TempDir(), "portal", termedOnceExe)
			flag := w.stage(t, samplePayload().Session)

			run := runParkedChain(t, exe, flag)

			if run.transcript != want {
				t.Errorf("session transcript = %q, want %q", run.transcript, want)
			}
			if run.status != 0 {
				t.Errorf("parked chain status = %d, want the tail's 0; stderr %q", run.status, run.stderr)
			}
		})
	}
}
