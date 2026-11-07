package spawn

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/log"
)

// AdapterSandboxEnv names, in an integration build, the directories whose
// programs an adapter may run, list-separated; every other program is refused.
// Every other build ignores it.
const AdapterSandboxEnv = "PORTAL_TEST_ADAPTER_DIRS"

// runAdapterArgv runs the command an adapter hands a terminal app: the one
// place a window is asked for. PoisonAdaptersForTest replaces it.
var runAdapterArgv = runSandboxedArgv

// runSandboxedArgv runs argv once the adapter sandbox, present in integration
// builds alone, allows it.
func runSandboxedArgv(argv []string) (string, int, error) {
	if err := adapterSandbox(argv); err != nil {
		return "", 0, err
	}
	return runArgvCombined(argv)
}

// PoisonAdaptersForTest makes every adapter panic as it asks the terminal for
// a window, for the TestMain of a package whose tests reach detection: it
// walks a test's ancestry to the developer's own terminal, whatever the
// environment says.
func PoisonAdaptersForTest() {
	runAdapterArgv = func(argv []string) (string, int, error) {
		panic(fmt.Sprintf("a test's adapter asked the terminal for a window: %q", argv))
	}
}

// runArgvCombined runs argv in Portal's environment without the variables no
// window may inherit.
func runArgvCombined(argv []string) (out string, exitCode int, err error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = withoutWindowStripped(os.Environ())
	combined, runErr := log.CombinedOutputWithContext(cmd)
	if runErr == nil {
		return string(combined), 0, nil
	}

	if exitErr, ok := errors.AsType[*exec.ExitError](runErr); ok {
		return combineOutput(combined, runErr), exitErr.ExitCode(), nil
	}
	return string(combined), 0, runErr
}

// withoutWindowStripped is environ without any entry for a variable in
// windowStripped.
func withoutWindowStripped(environ []string) []string {
	return slices.DeleteFunc(slices.Clone(environ), func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return slices.Contains(windowStripped, key)
	})
}

// CombinedOutputWithContext keeps stderr inside the wrapped error, so the runner
// seam's out = stdout+stderr contract needs both folded together here.
func combineOutput(stdout []byte, wrapErr error) string {
	parts := make([]string, 0, 2)
	if s := strings.TrimSpace(string(stdout)); s != "" {
		parts = append(parts, s)
	}
	if wrapErr != nil {
		parts = append(parts, wrapErr.Error())
	}
	return strings.Join(parts, "\n")
}

// fallbackLabel is a "%d" format string for the exit code, used so Detail is
// never empty.
func execFailureDetail(out string, exitCode int, err error, fallbackLabel string) string {
	detail := strings.TrimSpace(out)
	if err != nil {
		if detail == "" {
			return err.Error()
		}
		return detail + ": " + err.Error()
	}
	if detail == "" {
		return fmt.Sprintf(fallbackLabel, exitCode)
	}
	return detail
}
