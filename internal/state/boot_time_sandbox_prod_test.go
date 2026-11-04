//go:build !integration

package state

import (
	"testing"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

func TestBootTimeSandbox_ProductionBuildIgnoresIt(t *testing.T) {
	saved := sessionindex.Entry{Name: "work", CreatedAt: time.Now()}
	t.Setenv(BootTimeEnv, time.Now().Add(time.Hour).Format(time.RFC3339Nano))
	if ClaimRule(t.TempDir()).MayPredateBoot(saved) {
		t.Error("an entry created now is judged from before the boot a test named: a production build read it")
	}
}
