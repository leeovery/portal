package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/spawn"
)

func TestConfirmSessionOfADeadRunner(t *testing.T) {
	for name, tc := range map[string]struct {
		runners  spawn.RunnerLocator
		wantLeft bool
	}{
		"the state registry removes its entry":    {runners: stateRunners{}, wantLeft: false},
		"the read-only registry leaves its entry": {runners: readOnlyRunners{}, wantLeft: true},
	} {
		t.Run(name, func(t *testing.T) {
			stateDir := t.TempDir()
			t.Setenv("PORTAL_STATE_DIR", stateDir)
			entry := filepath.Join(stateDir, "runners", "tok123.json")
			if err := os.MkdirAll(filepath.Dir(entry), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(entry, fmt.Appendf(nil, `{"pid":%d,"session":"dev"}`, exitedPID(t)), 0o600); err != nil {
				t.Fatal(err)
			}

			if _, err := tc.runners.ConfirmSession(t.Context(), "tok123", "dev"); !errors.Is(err, runnerreg.ErrNoRunner) {
				t.Errorf("ConfirmSession = %v, want ErrNoRunner", err)
			}
			if _, err := os.Stat(entry); (err == nil) != tc.wantLeft {
				t.Errorf("the dead runner's entry: %v; want left %v", err, tc.wantLeft)
			}
		})
	}
}

// exitedPID is the pid of a process that has exited and been reaped.
func exitedPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("/usr/bin/true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}
