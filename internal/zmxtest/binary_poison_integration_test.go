//go:build integration

package zmxtest_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/zmxtest"
)

// Binary fails its test, so it runs in this test binary re-executed into
// binaryPoisonProcessName, whose failure the test reads.
const (
	binaryPoisonProcessName = "TestBinaryPoisonProcess"
	binaryPoisonEnv         = "PORTAL_TEST_ZMXTEST_BINARY_POISON"
)

// TestBinaryPoisonProcess is not a test: it is the body of a process asking
// Binary for a zmx with the poison binaryPoisonEnv names, NAME=value, undone
// after the TestMain set it.
func TestBinaryPoisonProcess(t *testing.T) {
	name, value, ok := strings.Cut(os.Getenv(binaryPoisonEnv), "=")
	if !ok {
		return
	}
	t.Setenv(name, value)
	zmxtest.Binary(t)
}

func TestBinary_RefusesAnUnpoisonedEnvironment(t *testing.T) {
	for name, value := range map[string]string{"ZMX_DIR": "/Users/lee/.local/state/zmx", "TMUX_TMPDIR": ""} {
		t.Run(name, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=^"+binaryPoisonProcessName+"$")
			child.Env = append(os.Environ(), binaryPoisonEnv+"="+name+"="+value)

			out, err := child.CombinedOutput()

			if err == nil || !strings.Contains(string(out), name+" is ") {
				t.Errorf("Binary with %s=%q: %v\n%s\nwant the test failed naming %s", name, value, err, out, name)
			}
		})
	}
}
