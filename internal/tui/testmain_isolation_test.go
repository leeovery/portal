package tui

// Detection walks a test's ancestry to the developer's own terminal, and
// these tests resolve its real adapters, so every adapter panics rather than
// open a window there.

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/spawn"
)

func TestMain(m *testing.M) {
	spawn.PoisonAdaptersForTest()
	os.Exit(m.Run())
}

func TestTheProductionAdaptersArePoisoned(t *testing.T) {
	adapter, _ := nativeResolve()(ghosttyIdentity())
	if adapter == nil {
		t.Fatal("Ghostty resolved to no adapter")
	}
	defer func() {
		if recover() == nil {
			t.Error("Ghostty's adapter asked for a window without panicking")
		}
	}()
	adapter.OpenWindow([]string{"/usr/bin/true"})
}
