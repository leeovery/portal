package tmux_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

func failingListSessions(t *testing.T, stderr string) *commandertest.Scripted {
	t.Helper()

	return commandertest.New(t, commandertest.Fails(&tmux.CommandError{
		Stderr: stderr,
		Err:    errors.New("exit status 1"),
		Args:   []string{"list-sessions"},
	}, "list-sessions"))
}

func TestListSessionsProbe_ReportsAFailedRead(t *testing.T) {
	t.Run("it returns tmux's error when the session list cannot be read", func(t *testing.T) {
		const stderr = "no server running on /tmp/tmux-501/default"
		client := tmux.NewClient(failingListSessions(t, stderr))

		got, err := client.ListSessionsProbe()

		if err == nil {
			t.Fatal("a failed list-sessions must be an error, not an empty session set")
		}
		if got != nil {
			t.Errorf("sessions = %v, want nil for a failed read", got)
		}
		var cmdErr *tmux.CommandError
		if !errors.As(err, &cmdErr) {
			t.Fatalf("errors.As did not recover *tmux.CommandError from %v (%T)", err, err)
		}
		if cmdErr.Stderr != stderr {
			t.Errorf("CommandError.Stderr = %q, want %q", cmdErr.Stderr, stderr)
		}
		if !strings.Contains(err.Error(), stderr) {
			t.Errorf("error %q does not carry tmux's stderr %q", err, stderr)
		}
	})

	t.Run("it still returns an empty slice and no error when ListSessions cannot read", func(t *testing.T) {
		client := tmux.NewClient(failingListSessions(t, "no server running on /tmp/tmux-501/default"))

		got, err := client.ListSessions()

		if err != nil {
			t.Fatalf("ListSessions must swallow a failed read: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("sessions = %v, want an empty slice", got)
		}
	})
}

func TestListSessionsProbe_ParsesTheSameOutputAsListSessions(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []tmux.Session
	}{
		{
			name:   "it returns the same sessions as ListSessions for identical output",
			output: "dev|3|1|/Users/me/code/portal\nwork|5|0|",
			want: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true, Dir: "/Users/me/code/portal"},
				{Name: "work", Windows: 5, Attached: false},
			},
		},
		{
			name:   "it filters Portal's internal sessions",
			output: "_portal-saver|1|0|\ndev|2|0|\n_portal-bootstrap|1|0|",
			want:   []tmux.Session{{Name: "dev", Windows: 2}},
		},
		{
			name:   "it parses the recorded directory including an embedded pipe",
			output: "dev|3|1|/Users/me/weird|path/portal",
			want:   []tmux.Session{{Name: "dev", Windows: 3, Attached: true, Dir: "/Users/me/weird|path/portal"}},
		},
		{
			name:   "it returns an empty slice and no error for a live server with no sessions",
			output: "",
			want:   []tmux.Session{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readers := map[string]func(*tmux.Client) ([]tmux.Session, error){
				"ListSessions":      (*tmux.Client).ListSessions,
				"ListSessionsProbe": (*tmux.Client).ListSessionsProbe,
			}
			for reader, read := range readers {
				mock := commandertest.New(t, commandertest.Returns(tt.output, "list-sessions"))

				got, err := read(tmux.NewClient(mock))

				if err != nil {
					t.Fatalf("%s: unexpected error: %v", reader, err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("%s: sessions = %#v, want %#v", reader, got, tt.want)
				}
			}
		})
	}
}

func TestListSessionsProbe_ReportsAMalformedLine(t *testing.T) {
	t.Run("it reports a malformed line as an error", func(t *testing.T) {
		readers := map[string]func(*tmux.Client) ([]tmux.Session, error){
			"ListSessions":      (*tmux.Client).ListSessions,
			"ListSessionsProbe": (*tmux.Client).ListSessionsProbe,
		}
		for reader, read := range readers {
			mock := commandertest.New(t, commandertest.Returns("dev|3", "list-sessions"))

			if _, err := read(tmux.NewClient(mock)); err == nil {
				t.Errorf("%s: a malformed line must be an error", reader)
			}
		}
	})
}

func TestListSessionsProbe_IssuesTheSameFormatAsListSessions(t *testing.T) {
	// A directory may contain a literal '|', so @portal-dir survives only in the
	// trailing SplitN slot: the two readers must request the same field order.
	listMock := commandertest.New(t, commandertest.Returns("dev|1|0|", "list-sessions"))
	if _, err := tmux.NewClient(listMock).ListSessions(); err != nil {
		t.Fatalf("ListSessions: unexpected error: %v", err)
	}

	probeMock := commandertest.New(t, commandertest.Returns("dev|1|0|", "list-sessions"))
	if _, err := tmux.NewClient(probeMock).ListSessionsProbe(); err != nil {
		t.Fatalf("ListSessionsProbe: unexpected error: %v", err)
	}

	if !reflect.DeepEqual(listMock.Calls(), probeMock.Calls()) {
		t.Errorf("ListSessionsProbe argv = %v, want ListSessions' %v", probeMock.Calls(), listMock.Calls())
	}
}
