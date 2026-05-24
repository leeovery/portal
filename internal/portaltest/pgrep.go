package portaltest

// pgrep.go — test-only enumeration of live `portal state daemon` processes
// via `pgrep -fx <state.PortalDaemonArgvPattern>`. Mirrors the production
// adapter's enumeration in internal/bootstrapadapter/orphan_sweep.go so
// integration tests observe the same candidate set the production sweep
// would observe (mod inherent race windows).
//
// Test-only. The package-level *testing.T enforcement is structural: this
// file ships no testing dependency itself, but the package's other helpers
// do, so importing portaltest from production code would fail to build.

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/state"
)

// PgrepPortalDaemons enumerates live `portal state daemon` PIDs via
// `pgrep -fx state.PortalDaemonArgvPattern`. Three-shape return contract,
// matching the production adapter's pgrepPortalDaemons:
//
//   - ([]int{...}, nil) on a successful match (one or more candidates).
//   - (nil, nil) when pgrep exits status 1 AND stdout is empty — pgrep's
//     documented "no matches" signal.
//   - (nil, err) on any other non-zero exit or OS-layer failure.
//
// PIDs that cannot be parsed as integers are skipped silently — best-
// effort posture, consistent with the production adapter.
//
// We use `pgrep -fx` (not `-fxc`) because BSD pgrep (darwin / FreeBSD)
// does not implement `-c`; counting via len() is the portable equivalent.
func PgrepPortalDaemons() ([]int, error) {
	out, err := exec.Command("pgrep", "-fx", state.PortalDaemonArgvPattern).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(strings.TrimSpace(string(out))) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("pgrep -fx %q: %w", state.PortalDaemonArgvPattern, err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		// Exit 0 with empty output: defensive guard. pgrep's documented
		// contract is exit 1 on no matches, but handle exit-0 + empty
		// stdout symmetrically for robustness.
		return nil, nil
	}

	var pids []int
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, perr := strconv.Atoi(line)
		if perr != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}
