//go:build integration

package spawn

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Adapter sandbox, integration builds only — adapter_sandbox_prod.go supplies
// the inert stub for every other build. A portal binary a test builds detects
// the terminal by walking to the developer's own, so it may run only a program
// in a directory the test names in AdapterSandboxEnv: default-deny, so a test
// that forgets to give its burst a terminal of its own opens no real window.

// errSandboxed is a program the adapter sandbox refuses.
var errSandboxed = errors.New("adapter sandbox: a test allowed no such program")

func adapterSandbox(argv []string) error {
	program := filepath.Clean(argv[0])
	if filepath.IsAbs(program) {
		for _, dir := range filepath.SplitList(os.Getenv(AdapterSandboxEnv)) {
			if dir != "" && strings.HasPrefix(program, filepath.Clean(dir)+string(filepath.Separator)) {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: %q", errSandboxed, argv)
}
