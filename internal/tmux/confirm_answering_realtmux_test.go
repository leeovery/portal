package tmux_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestConfirmAnswering_RealTmux(t *testing.T) {
	ts, client, _ := seedRealTmuxServer(t, realTmuxFixture{
		socketPrefix: "ptl-confirm-",
		sessions:     []string{"confirm"},
	})
	ownPID, err := strconv.Atoi(strings.TrimSpace(ts.Run(t, "display-message", "-p", "#{pid}")))
	if err != nil {
		t.Fatalf("read server pid: %v", err)
	}

	pid, err := client.ConfirmAnswering()
	if err != nil || pid != ownPID {
		t.Fatalf("ConfirmAnswering on a live server = (%d, %v), want (%d, nil)", pid, err, ownPID)
	}

	ts.KillServer()

	_, err = client.ConfirmAnswering()
	if _, ok := errors.AsType[*tmux.CommandError](err); !ok {
		t.Fatalf("ConfirmAnswering after the server exited = %v, want a *tmux.CommandError", err)
	}

	ts.Run(t, "new-session", "-d", "-s", "successor")

	pid, err = client.ConfirmAnswering()
	if err != nil {
		t.Fatalf("ConfirmAnswering on the new server = %v, want nil", err)
	}
	if pid == ownPID || pid <= 0 {
		t.Errorf("ConfirmAnswering on a new server on the same socket = %d, want a pid other than %d", pid, ownPID)
	}
}
