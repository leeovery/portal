package backendtest_test

import (
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
)

// fakeSubject puts the Fake's sessions in each state the contract covers. The
// Fake has no daemon to refuse or stay silent: both are StopAnswering.
type fakeSubject struct{ f *backendtest.Fake }

func (s fakeSubject) Backend() backend.Backend { return s.f }

func (s fakeSubject) Answering(_ *testing.T, name string) {
	s.f.Add(backend.Session{Name: name})
	s.f.SetHistory(name, []byte("screen"))
}

func (s fakeSubject) Silent(t *testing.T, name string) {
	s.Answering(t, name)
	s.f.StopAnswering(name)
}

func (s fakeSubject) Refusing(t *testing.T, name string) { s.Silent(t, name) }

func (s fakeSubject) SetCurrent(name string) { s.f.SetCurrent(name) }

func (s fakeSubject) Lacks(b backendtest.Behaviour) string {
	if b == backendtest.FoldsNormalisation {
		return "the Fake folds case only: folding Unicode normalisation would need golang.org/x/text"
	}
	return ""
}

func TestFakeContract(t *testing.T) {
	backendtest.Contract(t, func(*testing.T) backendtest.Subject { return fakeSubject{backendtest.New()} })
}
