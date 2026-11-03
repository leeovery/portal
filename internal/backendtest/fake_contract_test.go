package backendtest_test

import (
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
)

// fakeSubject puts the Fake's sessions in each state the contract covers. The
// Fake has no daemon to stay silent, refuse or hang up: each is
// StopAnswering.
type fakeSubject struct{ f *backendtest.Fake }

func (s fakeSubject) Backend() backend.Backend { return s.f }

func (s fakeSubject) Hold(_ *testing.T, name string, state backendtest.State) backend.Session {
	held := backend.Session{
		Name:    name,
		Dir:     "/srv/" + name,
		PID:     4321,
		Created: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC),
		Command: "sleep 1000",
	}
	s.f.Add(held)
	held.Answered = true
	switch state {
	case backendtest.Answering:
		s.f.SetHistory(name, []byte("screen"))
	case backendtest.Historyless:
	default:
		s.f.StopAnswering(name)
		return backend.Session{Name: name}
	}
	return held
}

func (s fakeSubject) SetCurrent(name string) { s.f.SetCurrent(name) }

// Lead has the Fake report the variable for the session's leader: it runs no
// client to mark.
func (s fakeSubject) Lead(_ *testing.T, session, variable, value string) {
	s.f.SetLeaderEnv(session, backend.ClientEnv{{Name: variable, Value: value}})
}

func (s fakeSubject) Lacks(b backendtest.Behaviour) string {
	switch b {
	case backendtest.FoldsNormalisation:
		return "the Fake folds case only: folding Unicode normalisation would need golang.org/x/text"
	case backendtest.NameLengthLimit:
		return "the Fake has no directory to measure a name against"
	case backendtest.AttachEnvironment:
		return "the Fake's attach command runs no zmx, so carries no environment"
	}
	return ""
}

func TestFakeContract(t *testing.T) {
	backendtest.Contract(t, func(*testing.T) backendtest.Subject { return fakeSubject{backendtest.New()} })
}
