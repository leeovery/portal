//go:build integration

package state

import (
	"testing"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

func TestBootTimeSandbox(t *testing.T) {
	kernel := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	read := func() (time.Time, error) { return kernel, nil }

	t.Run("it reads the boot time while a test names none", func(t *testing.T) {
		t.Setenv(BootTimeEnv, "")
		if got, err := sandboxBootTime(read)(); err != nil || !got.Equal(kernel) {
			t.Errorf("boot time = %v, %v; want the one read, %v", got, err, kernel)
		}
	})

	t.Run("it judges by the boot time a test names, read at each call", func(t *testing.T) {
		bootTime := sandboxBootTime(read)
		named := kernel.Add(time.Hour)
		t.Setenv(BootTimeEnv, named.Format(time.RFC3339Nano))
		if got, err := bootTime(); err != nil || !got.Equal(named) {
			t.Errorf("boot time = %v, %v; want the one named, %v", got, err, named)
		}
	})

	t.Run("a boot time it can't parse is an error, never the one read", func(t *testing.T) {
		t.Setenv(BootTimeEnv, "after lunch")
		if got, err := sandboxBootTime(read)(); err == nil {
			t.Errorf("boot time = %v, nil; want an error", got)
		}
	})

	t.Run("the claim rule judges by it", func(t *testing.T) {
		saved := sessionindex.Entry{Name: "work", CreatedAt: time.Now()}
		t.Setenv(BootTimeEnv, time.Now().Add(time.Hour).Format(time.RFC3339Nano))
		if !ClaimRule(t.TempDir()).MayPredateBoot(saved) {
			t.Error("an entry created before the boot named is judged from this boot")
		}
	})
}
