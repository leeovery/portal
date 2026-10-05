package tmux_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

func TestServerPIDFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantPID int
		wantOK  bool
	}{
		{"a value tmux set for a pane", "/private/tmp/tmux-501/default,61569,3", 61569, true},
		{"a value tmux set for a hook's run-shell job", "/tmp/ptl-x/s,4242,-1", 4242, true},
		{"a socket path carrying a comma", "/tmp/a,b/s,4242,0", 4242, true},
		{"an unset TMUX", "", 0, false},
		{"a socket with no pid field", "/tmp/s", 0, false},
		{"a pid that is not a number", "/tmp/s,abc,0", 0, false},
		{"the pid 0 a test pin carries", "/tmp/s,0,0", 0, false},
		{"a negative pid", "/tmp/s,-4,0", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid, ok := tmux.ServerPIDFromEnv(tt.value)
			if pid != tt.wantPID || ok != tt.wantOK {
				t.Errorf("ServerPIDFromEnv(%q) = (%d, %v), want (%d, %v)", tt.value, pid, ok, tt.wantPID, tt.wantOK)
			}
		})
	}
}

func TestConfirmAnsweringNamesTheAnsweringServer(t *testing.T) {
	t.Run("an answer naming a server returns its pid", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Returns("4242", "display-message", "-p", "#{pid}")))

		pid, err := client.ConfirmAnswering()

		if err != nil || pid != 4242 {
			t.Errorf("ConfirmAnswering() = (%d, %v), want (4242, nil)", pid, err)
		}
	})

	t.Run("an answer with exit status 0 and no output names no server", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Returns("", "display-message", "-p", "#{pid}")))

		pid, err := client.ConfirmAnswering()

		if err != nil || pid != 0 {
			t.Errorf("ConfirmAnswering() = (%d, %v), want (0, nil)", pid, err)
		}
	})

	t.Run("an answer that is not a pid is an error", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Returns("not-a-pid", "display-message", "-p", "#{pid}")))

		if _, err := client.ConfirmAnswering(); err == nil {
			t.Error("ConfirmAnswering() error = nil, want the unparseable answer reported")
		}
	})

	t.Run("a refused read returns the failure", func(t *testing.T) {
		refused := &tmux.CommandError{Args: []string{"display-message"}, Stderr: "no server running", Err: errors.New("exit status 1")}
		client := tmux.NewClient(commandertest.New(t, commandertest.Fails(refused, "display-message", "-p", "#{pid}")))

		_, err := client.ConfirmAnswering()

		if !errors.Is(err, refused) {
			t.Errorf("ConfirmAnswering() error = %v, want the refusal", err)
		}
	})
}
