//go:build integration

package migrate

import (
	"fmt"
	"os"
)

// Stop sandbox, integration builds only — stop_sandbox_prod.go supplies the
// inert stub for every other build.

// stopAt stops a run at point when StopEnv names it.
func stopAt(point string) error {
	if os.Getenv(StopEnv) != point {
		return nil
	}
	return fmt.Errorf("stopped %s, as %s asks", point, StopEnv)
}
