//go:build integration

package zmx_test

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/zmxtest"
)

// contractCeiling bounds every call in the contract over real zmx, which
// answers in milliseconds, so the cases on sessions that do not answer cost
// little.
const contractCeiling = 500 * time.Millisecond

// zmxSubject runs the driver over the pinned zmx build. It makes sessions
// with zmx run, which takes names zmx attach reads as flags; a stopped daemon
// is silent, and a SIGKILLed one leaves a socket that refuses.
type zmxSubject struct {
	t       *testing.T
	b       *zmx.Backend
	current string
}

func (s *zmxSubject) Backend() backend.Backend {
	if s.current == "" {
		return s.b
	}
	b := mustNew(s.t, zmx.Config{
		Binary: zmxtest.Binary(s.t),
		Dir:    s.b.Dir(),
		Environ: func() []string {
			return []string{"PATH=/usr/bin:/bin", "ZMX_DIR=" + s.b.Dir(), "ZMX_SESSION=" + s.current}
		},
	})
	zmx.ShortenCeilings(b, contractCeiling)
	return b
}

func (s *zmxSubject) Hold(t *testing.T, name string, state backendtest.State) backend.Session {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command(zmxtest.Binary(t), "run", name, "-d", "sleep", "1000")
	cmd.Env = []string{"ZMX_DIR=" + s.b.Dir(), "HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "SHELL=/bin/sh"}
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("zmx run %q = %q, %v", name, out, err)
	}
	switch state {
	case backendtest.Silent:
		zmxtest.StopDaemon(t, s.b, name)
		return backend.Session{Name: name}
	case backendtest.Refusing:
		zmxtest.StopDaemon(t, s.b, name)
		fillBacklog(t, filepath.Join(s.b.Dir(), name))
		return backend.Session{Name: name}
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return backend.Session{Name: name, Answered: true, Dir: real}
}

func (s *zmxSubject) SetCurrent(name string) { s.current = name }

// Lead attaches a client by the Backend's attach command, which Track marks,
// to the session no client leads, which makes it the leader.
func (s *zmxSubject) Lead(t *testing.T, session, variable, value string) {
	t.Helper()
	attach, err := s.b.AttachCommand(t.Context(), session)
	if err != nil {
		t.Fatalf("AttachCommand(%q) = %v", session, err)
	}
	zmxtest.Run(t, s.b.Track(attach, variable, value))
	awaitLeader(t, s.b, session)
}

// runTyped is in the command RunConnected's run types into the session.
const runTyped = "run-typed"

// RunConnected starts a zmx run on the session, whose program, the sleep
// Hold typed, never reads the command, so the run stays connected waiting on
// its end. zmx handles a client's messages in order, and a run sends the
// Resize that makes it the leader of a session no client leads before its
// command; so once the session's screen shows the command typed, the run
// leads.
func (s *zmxSubject) RunConnected(t *testing.T, session string) {
	t.Helper()
	zmxtest.Run(t, backend.Command{
		Argv: []string{zmxtest.Binary(t), "run", session, "true", runTyped},
		Env:  []string{"ZMX_DIR=" + s.b.Dir(), "HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "SHELL=/bin/sh"},
	})
	// The run reads no stdin, so it ends only as its connection does: a
	// detach of every client, before zmxtest.Run's cleanup waits for it.
	t.Cleanup(func() { _ = s.b.Detach(context.Background(), session) })
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		screen, err := s.b.History(t.Context(), session, 0)
		return err == nil && bytes.Contains(screen, []byte(runTyped))
	}) {
		t.Fatalf("session %s never showed the run's command", session)
	}
}

// Program names the file pid exec'd as the kernel keeps it: zmx for the
// daemon's own child until it execs the session's command.
func (s *zmxSubject) Program(t *testing.T, pid int) string {
	t.Helper()
	procs, err := proctable.List()
	if err != nil {
		t.Fatalf("list processes: %v", err)
	}
	for _, p := range procs {
		if p.PID == pid {
			return p.Command
		}
	}
	return ""
}

func (s *zmxSubject) Lacks(b backendtest.Behaviour) string {
	switch b {
	case backendtest.HangingUpSessions:
		return "zmx answers every well-formed request it reads, so no real daemon hangs up without answering"
	case backendtest.HistorylessSessions:
		return "a live zmx session always has a screen to give"
	}
	return lacks(s.t, s.b.Dir(), b)
}

func TestContractAgainstZmx(t *testing.T) {
	backendtest.Contract(t, func(t *testing.T) backendtest.Subject {
		b := zmxtest.New(t)
		zmx.ShortenCeilings(b, contractCeiling)
		return &zmxSubject{t: t, b: b}
	})
}
