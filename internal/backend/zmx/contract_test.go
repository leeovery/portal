package zmx_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
)

// daemonSubject runs the driver over fake daemons, each answering as zmx
// does, staying silent, or leaving a socket that refuses.
type daemonSubject struct {
	t       *testing.T
	dir     string
	info    []byte
	current string
}

func (s *daemonSubject) Backend() backend.Backend {
	return newBackend(s.t, s.dir, "PATH=/usr/bin:/bin", "ZMX_SESSION="+s.current)
}

func (s *daemonSubject) Answering(t *testing.T, name string) {
	serve(t, s.dir, name, anyRequest(s.info))
}

func (s *daemonSubject) Silent(t *testing.T, name string) { serve(t, s.dir, name, silent) }

func (s *daemonSubject) Refusing(t *testing.T, name string) { refusing(t, s.dir, name) }

func (s *daemonSubject) SetCurrent(name string) { s.current = name }

func (s *daemonSubject) Lacks(b backendtest.Behaviour) string { return lacks(s.t, s.dir, b) }

// lacks says why the filesystem holding dir cannot show a folding behaviour,
// found by making one spelling of a name and looking for the other.
func lacks(t *testing.T, dir string, b backendtest.Behaviour) string {
	t.Helper()
	made, other, why := "fold-probe", "FOLD-PROBE", "the filesystem does not fold case"
	if b == backendtest.FoldsNormalisation {
		made, other, why = "fold-probe-é", "fold-probe-é", "the filesystem does not fold Unicode normalisation"
	}
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
		return &daemonSubject{t: t, dir: socketDir(t), info: infoFrame(t)}
	})
}
