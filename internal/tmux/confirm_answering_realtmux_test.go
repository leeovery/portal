package tmux_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestConfirmAnswering_RealTmux(t *testing.T) {
	ts, client, _ := seedRealTmuxServer(t, realTmuxFixture{
		socketPrefix: "ptl-confirm-",
		sessions:     []string{"confirm"},
	})

	if err := client.ConfirmAnswering(); err != nil {
		t.Fatalf("ConfirmAnswering on a live server = %v, want nil", err)
	}

	ts.KillServer()

	err := client.ConfirmAnswering()
	if _, ok := errors.AsType[*tmux.CommandError](err); !ok {
		t.Fatalf("ConfirmAnswering after the server exited = %v, want a *tmux.CommandError", err)
	}
}
