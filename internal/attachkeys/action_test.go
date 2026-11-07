package attachkeys_test

import (
	"testing"

	"github.com/leeovery/portal/internal/attachkeys"
)

func TestActionString(t *testing.T) {
	for action, want := range map[attachkeys.Action]string{
		attachkeys.Detach:        "detach",
		attachkeys.DetachToShell: "detach_to_shell",
		attachkeys.Kill:          "kill",
		0:                        "Action(0)",
	} {
		if got := action.String(); got != want {
			t.Errorf("Action(%d).String() = %q, want %q", int(action), got, want)
		}
	}
}
