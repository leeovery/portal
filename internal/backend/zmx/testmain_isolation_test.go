package zmx_test

// The poisoned ZMX_DIR makes any zmx run that misses the Backend's own
// directory fail loudly instead of reaching the developer's live sessions.
// ZMX_SESSION would turn an attach into a switch of a live session, and
// ZMX_SESSION_PREFIX would rename every session a test names.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("ZMX_DIR", "/nonexistent/zmx-poison")
	_ = os.Unsetenv("ZMX_SESSION")
	_ = os.Unsetenv("ZMX_SESSION_PREFIX")
	os.Exit(m.Run())
}
