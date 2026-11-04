package cmd

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

const (
	waitingName = "api-x7Kd9a"
	// waitingPID is the pid the Fake names for the waiting session. It may be
	// a live process on the machine running the tests, so nothing reads it or
	// signals it.
	waitingPID = 4242
)

// parkedProcess is a waiting session's process as the hydrate helper leaves
// it: leading its own group, running its parked chain.
var parkedProcess = sessionProcess{
	Group: waitingPID,
	Argv:  parkedChainArgv(parkedChain{Exe: "/opt/portal", Flag: "/state/resume-pending/x"}, resumeChainPayload{Command: "make dev", Session: waitingName}),
}

type sentSignal struct {
	pid int
	sig syscall.Signal
}

// repaintStage is a state directory and a backend holding the session
// waitingName at pid waitingPID, its process read as process and every
// signal recorded rather than sent.
type repaintStage struct {
	dir       string
	backend   *backendtest.Fake
	process   sessionProcess
	readErr   error
	signalErr error
	reads     []int
	sent      []sentSignal
	sink      *logtest.Sink
}

func stageRepaint(t *testing.T) *repaintStage {
	t.Helper()
	s := &repaintStage{
		dir:     t.TempDir(),
		backend: backendtest.New(backend.Session{Name: waitingName, PID: waitingPID}),
		process: parkedProcess,
		sink:    logtest.Install(t),
	}
	t.Setenv("PORTAL_STATE_DIR", s.dir)
	withFuncSeam(t, &readSessionProcess, func(pid int) (sessionProcess, error) {
		s.reads = append(s.reads, pid)
		return s.process, s.readErr
	})
	withFuncSeam(t, &repaintSignal, func(pid int, sig syscall.Signal) error {
		s.sent = append(s.sent, sentSignal{pid, sig})
		return s.signalErr
	})
	return s
}

func (s *repaintStage) wait(t *testing.T) {
	t.Helper()
	if err := state.ResumePending.Set(s.dir, waitingName); err != nil {
		t.Fatalf("raise the resume-pending flag: %v", err)
	}
}

func (s *repaintStage) repaint() {
	repaintWaiting(context.Background(), s.backend, waitingName)
}

func (s *repaintStage) assertNothingSent(t *testing.T) {
	t.Helper()
	if len(s.sent) != 0 {
		t.Errorf("signals sent = %+v, want none", s.sent)
	}
}

// failure is the repaint's one WARN, which says why it sent nothing.
func (s *repaintStage) failure(t *testing.T) logtest.Record {
	t.Helper()
	rec := s.sink.Records().Matching("attach", "repaint failed").AtExactLevel(slog.LevelWarn).Only(t, "the repaint's failure")
	if got := rec.AttrString(t, "session"); got != waitingName {
		t.Errorf("session = %q, want %q", got, waitingName)
	}
	return rec
}

func TestRepaintWaiting(t *testing.T) {
	t.Run("it sends SIGWINCH to the process group of a waiting session's process", func(t *testing.T) {
		s := stageRepaint(t)
		s.wait(t)

		s.repaint()

		if want := []sentSignal{{-waitingPID, unix.SIGWINCH}}; !slices.Equal(s.sent, want) {
			t.Errorf("signals sent = %+v, want %+v", s.sent, want)
		}
		if want := []int{waitingPID}; !slices.Equal(s.reads, want) {
			t.Errorf("processes read = %v, want %v: the one the backend named", s.reads, want)
		}
		rec := s.sink.Records().Matching("attach", "repaint sent").AtExactLevel(slog.LevelDebug).Only(t, "the repaint")
		if got := rec.IntAttr(t, "target_pid"); got != waitingPID {
			t.Errorf("target_pid = %d, want %d", got, waitingPID)
		}
	})

	t.Run("it sends nothing to a session that isn't waiting, and reads nothing of it", func(t *testing.T) {
		s := stageRepaint(t)

		s.repaint()

		s.assertNothingSent(t)
		if len(s.reads) != 0 || len(s.backend.CallsTo(backendtest.OpSession)) != 0 {
			t.Errorf("read processes %v and sessions %+v of a session not waiting", s.reads, s.backend.CallsTo(backendtest.OpSession))
		}
		if warned := s.sink.Records().AtOrAboveLevel(slog.LevelWarn); len(warned) != 0 {
			t.Errorf("a session not waiting logged %+v", warned)
		}
	})

	t.Run("it sends nothing when the flag can't be read", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads through a directory's permissions")
		}
		s := stageRepaint(t)
		s.wait(t)
		flags := filepath.Join(s.dir, string(state.ResumePending))
		if err := os.Chmod(flags, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(flags, 0o700) })

		s.repaint()

		s.assertNothingSent(t)
		s.failure(t)
	})

	t.Run("it sends nothing when the backend names no process for the session", func(t *testing.T) {
		s := stageRepaint(t)
		s.wait(t)
		s.backend.Remove(waitingName)

		s.repaint()

		s.assertNothingSent(t)
		if len(s.reads) != 0 {
			t.Errorf("read processes %v with no session to name one", s.reads)
		}
		if err := s.failure(t).ErrorAttr(t, "error"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("error = %v, want ErrNoSuchSession", err)
		}
	})

	for _, tc := range []struct {
		name string
		pid  int
	}{
		{"pid 0", 0},
		{"pid 1", 1},
		{"a negative pid", -7},
		{"this process", os.Getpid()},
		{"the process group this process is in", unix.Getpgrp()},
	} {
		t.Run("it never signals "+tc.name, func(t *testing.T) {
			s := stageRepaint(t)
			s.wait(t)
			s.backend.Add(backend.Session{Name: waitingName, PID: tc.pid})
			s.process = sessionProcess{Group: tc.pid, Argv: parkedProcess.Argv}

			s.repaint()

			s.assertNothingSent(t)
			s.failure(t)
		})
	}

	for _, tc := range []struct {
		name    string
		process sessionProcess
		readErr error
	}{
		{"a process that doesn't lead its own group", sessionProcess{Group: waitingPID + 1, Argv: parkedProcess.Argv}, nil},
		{"a process that runs no resume panel", sessionProcess{Group: waitingPID, Argv: []string{"-zsh"}}, nil},
		{"a shell running a command of another's", sessionProcess{Group: waitingPID, Argv: []string{"sh", "-c", "make dev"}}, nil},
		{"a process that can't be read", sessionProcess{}, errors.New("read arguments of process 4242: operation not permitted")},
	} {
		t.Run("it never signals "+tc.name, func(t *testing.T) {
			s := stageRepaint(t)
			s.wait(t)
			s.process, s.readErr = tc.process, tc.readErr

			s.repaint()

			s.assertNothingSent(t)
			s.failure(t)
		})
	}

	t.Run("it logs a signal that fails", func(t *testing.T) {
		s := stageRepaint(t)
		s.wait(t)
		s.signalErr = syscall.ESRCH

		s.repaint()

		if err := s.failure(t).ErrorAttr(t, "error"); !errors.Is(err, syscall.ESRCH) {
			t.Errorf("error = %v, want ESRCH", err)
		}
	})
}

// The repaint is asked of every way an attach reaches a session, and none
// waits on it.
func TestAttach_RepaintsAWaitingSessionFirst(t *testing.T) {
	stage := func(t *testing.T) (*repaintStage, *attachRecorder) {
		t.Helper()
		fake, rec := stageAttach(t, waitingName, "other-b2c3")
		s := stageRepaint(t)
		fake.Add(backend.Session{Name: waitingName, PID: waitingPID})
		s.backend = fake
		s.wait(t)
		return s, rec
	}

	t.Run("it repaints before the runner attaches", func(t *testing.T) {
		s, rec := stage(t)
		withFuncSeam(t, &repaintSignal, func(pid int, sig syscall.Signal) error {
			if len(rec.runs) != 0 {
				t.Error("the repaint was sent once the attach had begun")
			}
			s.sent = append(s.sent, sentSignal{pid, sig})
			return nil
		})
		rec.answer(outcome(attachkeys.DetachToShell))

		if err := openSession(contextCommand(), waitingName); err != nil {
			t.Fatalf("openSession = %v", err)
		}

		rec.onlyRun(t)
		if want := []sentSignal{{-waitingPID, unix.SIGWINCH}}; !slices.Equal(s.sent, want) {
			t.Errorf("signals sent = %+v, want %+v", s.sent, want)
		}
	})

	t.Run("it repaints before a switch from inside a session", func(t *testing.T) {
		s, rec := stage(t)
		s.backend.SetCurrent("other-b2c3")

		if err := openSession(contextCommand(), waitingName); err != nil {
			t.Fatalf("openSession = %v", err)
		}

		rec.onlyExec(t)
		if want := []sentSignal{{-waitingPID, unix.SIGWINCH}}; !slices.Equal(s.sent, want) {
			t.Errorf("signals sent = %+v, want %+v", s.sent, want)
		}
	})

	for _, tc := range []struct {
		name  string
		spoil func(s *repaintStage)
	}{
		{"a refused target", func(s *repaintStage) { s.process.Argv = []string{"-zsh"} }},
		{"a signal that fails", func(s *repaintStage) { s.signalErr = syscall.EPERM }},
	} {
		t.Run("it attaches all the same after "+tc.name, func(t *testing.T) {
			s, rec := stage(t)
			tc.spoil(s)
			rec.answer(outcome(attachkeys.DetachToShell))

			if err := openSession(contextCommand(), waitingName); err != nil {
				t.Fatalf("openSession = %v, want the attach unaffected", err)
			}

			if cfg := rec.onlyRun(t); cfg.Session != waitingName {
				t.Errorf("attached %q, want %q", cfg.Session, waitingName)
			}
			s.failure(t)
		})
	}
}

func TestParkedChainArgvIsWhatTheRepaintRecognises(t *testing.T) {
	var execed []string
	cfg := hydrateConfig{
		Session:           waitingName,
		Logger:            slog.New(slog.DiscardHandler),
		DisableTTYSignals: func() error { return nil },
		ExecShell:         func(_ string, args []string) { execed = args },
	}

	execResumeChainAndExit(cfg, "make dev", parkedChain{Exe: "/opt/portal", Flag: "/state/resume-pending/x"})

	if !isParkedResumeChain(execed) {
		t.Errorf("the hydrate helper parks a session under %q, which the repaint doesn't recognise", execed)
	}
	for _, argv := range [][]string{
		{"/bin/zsh", "-l"},
		{"sh", "-c", "make dev"},
		{"sh", "-c"},
		{"sh", "-x", parkedProcess.Argv[2]},
	} {
		if isParkedResumeChain(argv) {
			t.Errorf("isParkedResumeChain(%q) = true, want false", argv)
		}
	}
}

// A test that leaves the repaint's signal unstaged panics rather than signal
// a pid its backend made up. Signal 0 sends nothing, were the poison gone.
func TestRepaintSignalPanicsUnstaged(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("repaintSignal returned; cmd's TestMain must make it panic")
		}
	}()
	_ = repaintSignal(-waitingPID, 0)
}
