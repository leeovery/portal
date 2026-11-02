//go:build integration

package zmx_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
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
		Binary:  zmxtest.Binary(s.t),
		Dir:     s.b.Dir(),
		Environ: func() []string { return []string{"PATH=/usr/bin:/bin", "ZMX_SESSION=" + s.current} },
	})
	zmx.ShortenCeilings(b, contractCeiling)
	return b
}

func (s *zmxSubject) Answering(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command(zmxtest.Binary(t), "run", name, "-d", "sleep", "1000")
	cmd.Env = []string{"ZMX_DIR=" + s.b.Dir(), "HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "SHELL=/bin/sh"}
	cmd.Dir = t.TempDir()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("zmx run %q = %q, %v", name, out, err)
	}
}

func (s *zmxSubject) Silent(t *testing.T, name string) {
	s.Answering(t, name)
	zmxtest.StopDaemon(t, s.b, name)
}

func (s *zmxSubject) Refusing(t *testing.T, name string) {
	s.Answering(t, name)
	crash(t, s.b, name)
}

func (s *zmxSubject) SetCurrent(name string) { s.current = name }

func (s *zmxSubject) Lacks(b backendtest.Behaviour) string { return lacks(s.t, s.b.Dir(), b) }

func TestContractAgainstZmx(t *testing.T) {
	backendtest.Contract(t, func(t *testing.T) backendtest.Subject {
		b := zmxtest.New(t)
		zmx.ShortenCeilings(b, contractCeiling)
		return &zmxSubject{t: t, b: b}
	})
}
