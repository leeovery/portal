package zmx_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
)

// daemonSubject runs the driver over fake daemons: each answers as zmx does,
// stays silent, hangs up, or leaves a socket that refuses.
type daemonSubject struct {
	t       *testing.T
	dir     string
	info    []byte
	current string

	mu       sync.Mutex
	leaders  map[string][]byte
	refusing []zmx.Process
}

// Backend reads a process table in which a daemon holds each refusing socket
// held so far, so a refusal is a full backlog, never a crash.
func (s *daemonSubject) Backend() backend.Backend {
	b := newBackend(s.t, s.dir, "PATH=/usr/bin:/bin", "ZMX_DIR="+s.dir, "ZMX_SESSION="+s.current)
	zmx.FakeProcessesFrom(b, func() ([]zmx.Process, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return slices.Clone(s.refusing), nil
	})
	return b
}

func (s *daemonSubject) Hold(t *testing.T, name string, state backendtest.State) backend.Session {
	switch state {
	case backendtest.Answering:
		serve(t, s.dir, name, ledRequest(s.info, []byte("screen"), func() []byte { return s.leader(name) }))
	case backendtest.Historyless:
		serve(t, s.dir, name, anyRequest(s.info, nil))
	case backendtest.Silent:
		serve(t, s.dir, name, silent)
		return backend.Session{Name: name}
	case backendtest.Refusing:
		refusing(t, s.dir, name)
		s.mu.Lock()
		s.refusing = append(s.refusing, daemonOf(100+len(s.refusing), s.dir, name))
		s.mu.Unlock()
		return backend.Session{Name: name}
	case backendtest.HangingUp:
		serve(t, s.dir, name, hangingUp())
		return backend.Session{Name: name}
	}
	return fixtureListing(s.info, name)
}

func (s *daemonSubject) SetCurrent(name string) { s.current = name }

// Lead has the session's daemon answer EnvGet as zmx does for a client that
// tracks the variable alongside one its environment lacks.
func (s *daemonSubject) Lead(_ *testing.T, session, variable, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leaders[session] = []byte("-DISPLAY\n" + variable + "=" + value + "\n")
}

func (s *daemonSubject) leader(session string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leaders[session]
}

// Program is never asked: the subject lacks Creating, and a fake daemon's
// session runs no process of its own.
func (s *daemonSubject) Program(t *testing.T, pid int) string {
	t.Fatalf("Program(%d) asked of a subject with no session processes", pid)
	return ""
}

func (s *daemonSubject) Lacks(b backendtest.Behaviour) string {
	if b == backendtest.Creating {
		return "the daemon run has no zmx to create a session with"
	}
	return lacks(s.t, s.dir, b)
}

// fixtureListing is how List reports a session answering Info with the
// fixture frame: its cwd is on another host, so it has no Dir.
func fixtureListing(frame []byte, name string) backend.Session {
	record := frame[8:]
	return backend.Session{
		Name:     name,
		Answered: true,
		Clients:  int(binary.NativeEndian.Uint64(record[0:])),
		PID:      int(int32(binary.NativeEndian.Uint32(record[8:]))),
		Created:  time.Unix(int64(binary.NativeEndian.Uint64(record[528:])), 0),
		Command:  fixtureCommand,
	}
}

// lacks says why the filesystem holding dir cannot show a behaviour of names,
// found by making one name and looking for another; "" for any other.
func lacks(t *testing.T, dir string, b backendtest.Behaviour) string {
	t.Helper()
	switch b {
	case backendtest.FoldsCase:
		return folds(t, dir, "fold-probe", "FOLD-PROBE", "the filesystem does not fold case")
	case backendtest.FoldsNormalisation:
		return folds(t, dir, "fold-probe-é", "fold-probe-é", "the filesystem does not fold Unicode normalisation")
	case backendtest.NonUTF8Names:
		path := filepath.Join(dir, "probe-\xff")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return "the filesystem refuses a name that is not UTF-8"
		}
		_ = os.Remove(path)
	}
	return ""
}

func folds(t *testing.T, dir, made, other, why string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, made), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(filepath.Join(dir, made)) }()
	if _, err := os.Stat(filepath.Join(dir, other)); err != nil {
		return why
	}
	return ""
}

func TestContract(t *testing.T) {
	backendtest.Contract(t, func(t *testing.T) backendtest.Subject {
		return &daemonSubject{t: t, dir: socketDir(t), info: infoFrame(t), leaders: map[string][]byte{}}
	})
}
