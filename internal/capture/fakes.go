// Package capture provides the in-memory fakes and deterministic fixtures the
// offline visual-capture harness (cmd/capturetool) renders. Every backend seam
// the TUI model depends on is canned, and nothing here discovers config, so a
// capture never reaches a zmx session, spawns a daemon or reads the real
// ~/.config/portal.
//
// The shipped portal binary must not import this package.
package capture

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/tui"
)

// Blocks forever once the seeded events drain — BootstrapCompleteMsg never
// arrives, so the capture stays parked on the loading page.
func loadingProgressReceiver(events []tui.BootstrapProgressMsg) tea.Cmd {
	ch := make(chan tui.BootstrapProgressMsg, len(events))
	for _, e := range events {
		ch <- e
	}
	return func() tea.Msg {
		return <-ch
	}
}

// Blocks forever after the fatal, so the error frame never transitions to the
// picker.
func loadingFatalReceiver(events []tui.BootstrapProgressMsg, fatal tui.BootstrapFatalMsg) tea.Cmd {
	ch := make(chan tea.Msg, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	ch <- fatal
	return func() tea.Msg {
		return <-ch
	}
}

type fakeLister struct {
	sessions []tui.Session
}

func (f *fakeLister) ListSessions() ([]tui.Session, error) {
	// Copy: a caller mutating the slice must not perturb the fixture across rebuilds.
	out := make([]tui.Session, len(f.sessions))
	copy(out, f.sessions)
	return out, nil
}

// timedLister lists a scenario's sessions as a cold start's listings read them
// either side of its restore: none before restoredAt into the scenario, every
// one from then on.
type timedLister struct {
	elapsed    func() time.Duration
	restoredAt time.Duration
	sessions   []tui.Session
}

func (l timedLister) ListSessions() ([]tui.Session, error) {
	if l.elapsed() < l.restoredAt {
		return nil, nil
	}
	return slices.Clone(l.sessions), nil
}

// timedPendingReader is the pending-resume set as a scenario's restore leaves
// it: empty before restoredAt, the sessions waiting to resume from then on.
type timedPendingReader struct {
	elapsed    func() time.Duration
	restoredAt time.Duration
	sessions   []string
}

func (r timedPendingReader) ListPendingResumeSessions() (map[string]struct{}, error) {
	if r.elapsed() < r.restoredAt {
		return map[string]struct{}{}, nil
	}
	return fakePendingReader{sessions: r.sessions}.ListPendingResumeSessions()
}

// bootReceiver is a scenario's bootstrap as the loading page receives it, one
// event a call, as the model re-issues the receive after each: a call waits
// until its event's moment, then hands it over. The cues end at the complete,
// after which the model asks no more; a call past them answers nil at once, so
// no receive outlives the scenario, as the fixtures' parked one would.
type bootReceiver struct {
	elapsed func() time.Duration
	mu      sync.Mutex
	cues    []bootCue
}

func (r *bootReceiver) receive() tea.Msg {
	r.mu.Lock()
	if len(r.cues) == 0 {
		r.mu.Unlock()
		return nil
	}
	next := r.cues[0]
	r.cues = r.cues[1:]
	r.mu.Unlock()
	time.Sleep(next.at - r.elapsed())
	return next.msg
}

type fakePendingReader struct {
	sessions []string
}

func (f fakePendingReader) ListPendingResumeSessions() (map[string]struct{}, error) {
	pending := make(map[string]struct{}, len(f.sessions))
	for _, name := range f.sessions {
		pending[name] = struct{}{}
	}
	return pending, nil
}

type fakeKiller struct{}

func (fakeKiller) KillSession(string) error { return nil }

// fakeLabeller refuses what the index refuses, on an index of its own that
// nothing reads, and lands every other label nowhere.
type fakeLabeller struct{}

func (fakeLabeller) SetLabel(name, label string) error {
	var idx sessionindex.Index
	return idx.SetLabel(name, label)
}

type fakeCreator struct{}

func (fakeCreator) CreateFromDir(string, []string) (string, error) { return "", nil }

type fakeProjectStore struct {
	projects []project.Project
}

func (f *fakeProjectStore) List() ([]project.Project, error) {
	out := make([]project.Project, len(f.projects))
	copy(out, f.projects)
	return out, nil
}

func (f *fakeProjectStore) CleanStale() ([]project.Project, error) { return nil, nil }

func (f *fakeProjectStore) Remove(string, string) error { return nil }

type fakeProjectEditor struct{}

func (fakeProjectEditor) Rename(string, string, string) error { return nil }

func (fakeProjectEditor) AddTag(string, string) error { return nil }

func (fakeProjectEditor) RemoveTag(string, string) error { return nil }

type fakeAliasEditor struct {
	aliases map[string]string
}

func (f fakeAliasEditor) Load() (map[string]string, error) {
	out := make(map[string]string, len(f.aliases))
	maps.Copy(out, f.aliases)
	return out, nil
}

func (fakeAliasEditor) SetAndSave(string, string, string) error { return nil }

func (fakeAliasEditor) DeleteAndSave(string, string) (bool, error) { return true, nil }

// fakeHistory answers a session's History with its own content where histories
// holds some, and otherwise every session's with the same content, or with
// file's bytes when it names one. Seeded content stays generic terminal output
// — Portal's preview is tool-agnostic, so no fixture references any specific
// tool.
type fakeHistory struct {
	content   string
	file      string
	histories map[string]string
}

func (h fakeHistory) History(_ context.Context, name string, _ int) ([]byte, error) {
	if own, ok := h.histories[name]; ok {
		return []byte(own), nil
	}
	if h.file != "" {
		return os.ReadFile(h.file)
	}
	if h.content != "" {
		return []byte(h.content), nil
	}
	return []byte("$ portal open\n(canned preview history)\n"), nil
}

// zmxHistoryFile is a History answer zmx itself serialised, recorded by the zmx
// package's integration test. It is found beside this source file rather than
// from the working directory, which differs between capturetool's live view
// and the test lanes.
func zmxHistoryFile() string {
	_, self, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(self), "..", "backend", "zmx", "testdata", "history.vt")
}
