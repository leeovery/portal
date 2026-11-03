package spawn

// Detection inside a session reads zmx. The poisoned ZMX_DIR makes any zmx run
// that misses a test's own directory fail loudly instead of reaching the
// developer's live sessions, and ZMX_SESSION would put every detection inside
// one of them.

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/zmxtest"
)

func TestMain(m *testing.M) {
	for _, err := range []error{
		os.Setenv("ZMX_DIR", zmxtest.PoisonedDir),
		os.Unsetenv("ZMX_SESSION"),
		os.Unsetenv("ZMX_SESSION_PREFIX"),
	} {
		if err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}
