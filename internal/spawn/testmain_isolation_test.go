package spawn

// Detection inside a session reads zmx. The poisoned ZMX_DIR makes any zmx run
// that misses a test's own directory fail loudly instead of reaching the
// developer's live sessions, and ZMX_SESSION would put every detection inside
// one of them. Detection outside one reads the terminal's identity from the
// environment, scrubbed here, and from this process's ancestry, which no
// test can scrub: it leads to the developer's own terminal. So the adapter
// boundary is poisoned too, and a test that reaches a production adapter
// panics rather than open a window there.

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/zmxtest"
)

func TestMain(m *testing.M) {
	errs := []error{
		os.Setenv("ZMX_DIR", zmxtest.PoisonedDir),
		os.Unsetenv("ZMX_SESSION"),
		os.Unsetenv("ZMX_SESSION_PREFIX"),
	}
	for _, name := range IdentityVars() {
		errs = append(errs, os.Unsetenv(name))
	}
	for _, err := range errs {
		if err != nil {
			panic(err)
		}
	}
	PoisonAdaptersForTest()
	os.Exit(m.Run())
}

// realAdapterExec lets the adapter boundary run for the rest of t, for a test
// whose adapter runs only programs of the test's own.
func realAdapterExec(t *testing.T) {
	t.Helper()
	poisoned := runAdapterArgv
	runAdapterArgv = runArgvCombined
	t.Cleanup(func() { runAdapterArgv = poisoned })
}

func TestTheProductionAdaptersArePoisoned(t *testing.T) {
	id := NewIdentity(ghosttyBundleID, "Ghostty")
	adapter, _ := NewResolver(TerminalsConfig{}).Resolve(id)
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
