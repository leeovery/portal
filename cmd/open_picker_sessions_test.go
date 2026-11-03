package cmd

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// asPickerLister serves a tmux-shaped lister to the picker through the same
// adapter production wires.
func asPickerLister(l tmuxSessionLister) tui.SessionLister {
	return pickerSessionSource{sessions: l}
}

type pendingViewStub struct {
	view tmux.PendingResumeView
	err  error
}

func (s pendingViewStub) ListPendingResumePanes() (tmux.PendingResumeView, error) {
	return s.view, s.err
}

func TestPickerSessions_MapsEachTmuxSessionOntoAnAnsweringUnlabelledRow(t *testing.T) {
	got := pickerSessions([]tmux.Session{
		{Name: "portal-a1b2", Windows: 3, Attached: true, Dir: "/home/user/code/portal"},
		{Name: "blog-c3d4", Windows: 1},
	})
	want := []tui.Session{
		{Name: "portal-a1b2", Attached: true, Dir: "/home/user/code/portal", Answered: true},
		{Name: "blog-c3d4", Answered: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("pickerSessions() = %+v, want %+v", got, want)
	}
}

func TestPickerSessionSource_ListsTmuxSessionsAsPickerRows(t *testing.T) {
	src := pickerSessionSource{sessions: &mockSessionLister{sessions: []tmux.Session{{Name: "portal-a1b2", Attached: true}}}}

	got, err := src.ListSessions()

	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if want := []tui.Session{{Name: "portal-a1b2", Attached: true, Answered: true}}; !slices.Equal(got, want) {
		t.Errorf("ListSessions() = %+v, want %+v", got, want)
	}
}

func TestPickerSessionSource_PassesTheListingErrorThrough(t *testing.T) {
	listErr := errors.New("list-sessions: boom")
	src := pickerSessionSource{sessions: &mockSessionLister{err: listErr}}

	got, err := src.ListSessions()

	if !errors.Is(err, listErr) {
		t.Errorf("ListSessions() error = %v, want %v", err, listErr)
	}
	if len(got) != 0 {
		t.Errorf("ListSessions() = %+v on a failed listing, want none", got)
	}
}

// The picker reads a failed tmux listing as no server and no sessions, never as
// an error that would quit it.
func TestPickerSessionSource_ReadsAFailedTmuxListingAsNoSessions(t *testing.T) {
	mock := commandertest.New(t, commandertest.Fails(&tmux.CommandError{
		Args:   []string{"list-sessions"},
		Stderr: "no server running on /tmp/tmux-501/default",
		Err:    errors.New("exit status 1"),
	}, "list-sessions"))
	src := pickerSessionSource{sessions: tmux.NewClient(mock)}

	got, err := src.ListSessions()

	if err != nil {
		t.Errorf("ListSessions() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("ListSessions() = %+v, want none", got)
	}
}

func TestPickerSessionSource_NamesThePendingResumeSessions(t *testing.T) {
	view := tmux.PendingResumeView{
		Rows: []tmux.PendingResumeRow{
			{Pending: true, Session: "portal-a1b2"},
			{Pending: true, Session: "portal-a1b2"},
			{Pending: true, Session: "blog-c3d4"},
		},
		Panes:    3,
		Sessions: map[string]struct{}{"portal-a1b2": {}, "blog-c3d4": {}},
	}
	src := pickerSessionSource{pending: pendingViewStub{view: view}}

	got, err := src.ListPendingResumeSessions()

	if err != nil {
		t.Fatalf("ListPendingResumeSessions() error = %v", err)
	}
	if want := map[string]struct{}{"portal-a1b2": {}, "blog-c3d4": {}}; !maps.Equal(got, want) {
		t.Errorf("ListPendingResumeSessions() = %v, want %v", got, want)
	}
}

func TestPickerSessionSource_PassesThePendingReadErrorThrough(t *testing.T) {
	readErr := errors.New("list-panes: boom")
	src := pickerSessionSource{pending: pendingViewStub{err: readErr}}

	got, err := src.ListPendingResumeSessions()

	if !errors.Is(err, readErr) {
		t.Errorf("ListPendingResumeSessions() error = %v, want %v", err, readErr)
	}
	if got != nil {
		t.Errorf("ListPendingResumeSessions() = %v on a failed read, want nil", got)
	}
}
