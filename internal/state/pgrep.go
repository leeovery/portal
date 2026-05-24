package state

// pgrep.go — canonical enumeration of live `portal state daemon` PIDs via
// `pgrep -fx <PortalDaemonArgvPattern>`. Single source of truth shared by:
//
//   - The production bootstrap-step-4 orphan-sweep adapter
//     (internal/bootstrapadapter/orphan_sweep.go), which wires this into
//     bootstrap.OrphanSweepCore.Pgrep.
//   - The portaltest integration-test helper (internal/portaltest/pgrep.go),
//     which forwards directly so test observations match production's
//     candidate set (mod inherent race windows).
//
// Both sites previously held byte-equivalent copies; promoting the helper
// here eliminates the duplicate body and pins one canonical enumeration
// shape against the canonical PortalDaemonArgvPattern regex.

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PgrepPortalDaemons enumerates candidate PIDs via
// `pgrep -fx PortalDaemonArgvPattern`. The `-f` flag matches against the
// full argv string; `-x` requires an exact match (the regex must consume
// the whole argv); the anchored regex prevents false positives from e.g.
// `portal state daemon-foo` or `portal state daemon --some-flag` (the
// trailing ` |$` clause allows trailing argv tokens separated by a space,
// while still anchoring the prefix).
//
// Three-shape return contract:
//
//   - ([]int{...}, nil) on a successful match (one or more candidates).
//   - (nil, nil) when pgrep exits with status 1 AND empty stdout — pgrep's
//     documented "no matches" signal. This is the steady-state form on a
//     clean install where no orphan daemons exist; treating it as an error
//     would force callers to log a WARN on every bootstrap.
//   - (nil, err) on any other non-zero exit or OS-layer failure. Callers
//     surface the error via their component's logger and continue —
//     orphan-sweep is best-effort.
//
// PIDs that cannot be parsed as integers are skipped silently — best-
// effort posture.
//
// We use `pgrep -fx` (not `-fxc`) because BSD pgrep (darwin / FreeBSD)
// does not implement `-c`; counting via len() is the portable equivalent.
func PgrepPortalDaemons() ([]int, error) {
	out, err := exec.Command("pgrep", "-fx", PortalDaemonArgvPattern).Output()
	if err != nil {
		// pgrep status 1 + empty stdout = no matches (canonical pgrep
		// "nothing found" signal). Treat as a normal empty result.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(strings.TrimSpace(string(out))) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("pgrep -fx %q: %w", PortalDaemonArgvPattern, err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		// Exit 0 with empty output: defensive guard. pgrep's documented
		// contract is exit 1 on no matches, but the empty-output / exit-0
		// shape is handled here for robustness.
		return nil, nil
	}

	var pids []int
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, parseErr := strconv.Atoi(line)
		if parseErr != nil {
			// Skip malformed lines silently — best-effort posture.
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}
