package zmx_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
)

// On Linux a full listen backlog fails a connect with EAGAIN, so a refusal
// proves no listener without the process table, which Linux can't read.
func TestRefusingSocket_OnLinuxIsStaleWithoutTheProcessTable(t *testing.T) {
	dir := socketDir(t)
	refusing(t, dir, "dev")
	b := newBackend(t, dir)
	zmx.FakeProcesses(b, nil, errors.New("the process table was read"))

	if exists, err := b.Exists(t.Context(), "dev"); exists || err != nil {
		t.Errorf("Exists(dev) = %v, %v; want false, nil", exists, err)
	}
}
