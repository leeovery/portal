//go:build integration

package state

import (
	"fmt"
	"os"
	"time"
)

// Boot time sandbox, integration builds only — boot_time_sandbox_prod.go
// supplies the inert stub for every other build.

// sandboxBootTime is read, unless BootTimeEnv names a boot time. The variable
// is read at each call, so a test sets it in its own process as well as in a
// portal binary's environment.
func sandboxBootTime(read func() (time.Time, error)) func() (time.Time, error) {
	return func() (time.Time, error) {
		at := os.Getenv(BootTimeEnv)
		if at == "" {
			return read()
		}
		booted, err := time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s: %w", BootTimeEnv, err)
		}
		return booted, nil
	}
}
