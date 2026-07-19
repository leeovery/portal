package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

// ErrDoctorUnhealthy is returned by `portal doctor` when any check reports a
// failure. Like ErrStatusUnhealthy (the retired `state status` sentinel it
// mirrors), it drives a non-zero process exit while emitting nothing to
// stderr: the rendered report is already on stdout, and IsSilentExitError
// (cmd/state_commit_now.go) compile-time-links the stderr-suppression
// contract. The sentinel exists solely to signal the unhealthy exit code.
var ErrDoctorUnhealthy = errors.New("doctor unhealthy")

// checkStatus is the outcome of a single doctor health check.
type checkStatus int

const (
	// checkPass — the check succeeded; the subject is healthy.
	checkPass checkStatus = iota
	// checkFail — the check found a problem; drives a non-zero exit.
	checkFail
	// checkInfo — informational only (e.g. host-terminal identity). Rendered
	// without a pass/fail marker and never drives the exit code.
	checkInfo
	// checkNotEvaluable — the check could not be evaluated in the current
	// environment (e.g. dead-pane hook staleness with the server down). Never
	// drives the exit code.
	checkNotEvaluable
)

// checkResult is one line of the doctor report: the check's name, its outcome,
// and a short human-readable detail.
type checkResult struct {
	name   string
	status checkStatus
	detail string
}

// DoctorDeps is the DI seam for the doctor command. In production doctorDeps
// is nil and resolveDoctorDeps supplies the real collaborators (StateDir from
// state.Dir(), Now from time.Now). Tests assign a *DoctorDeps with a hermetic
// StateDir temp dir so diagnosis runs against seeded fixtures without touching
// the developer's real ~/.config/portal/state.
type DoctorDeps struct {
	// StateDir overrides the resolved state directory. Empty means "resolve
	// via state.Dir()" (the production path).
	StateDir string
	// Now supplies the clock used by the daemon check's CollectStatus call.
	Now func() time.Time
}

// doctorDeps is the package-level DI seam; nil in production.
var doctorDeps *DoctorDeps

// resolveDoctorDeps returns a fully-populated *DoctorDeps for one doctor
// invocation. Unset fields in the package-level doctorDeps fall through to the
// production defaults independently — same per-field nil-check idiom as
// commitNowDeps / bootstrapDeps. The returned value is never nil and Now is
// guaranteed non-nil.
func resolveDoctorDeps() *DoctorDeps {
	deps := &DoctorDeps{Now: time.Now}
	if doctorDeps == nil {
		return deps
	}
	if doctorDeps.StateDir != "" {
		deps.StateDir = doctorDeps.StateDir
	}
	if doctorDeps.Now != nil {
		deps.Now = doctorDeps.Now
	}
	return deps
}

// doctorCmd runs an ordered catalog of read-only health checks, renders one
// line per check to stdout, and drives a scriptable exit code (0 iff every
// check passes, non-zero if any check fails).
//
// doctor is bootstrap-exempt (see skipTmuxCheck in cmd/root.go): it starts no
// server, registers no hooks, and respawns no daemon, so it observes raw state
// and heals nothing.
var doctorCmd = &cobra.Command{
	Use:           "doctor",
	Short:         "Diagnose Portal's health across the resurrection machinery",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		results, err := runDoctorDiagnosis(resolveDoctorDeps())
		if err != nil {
			return err
		}
		renderDoctorReport(cmd.OutOrStdout(), results)
		if doctorUnhealthy(results) {
			return ErrDoctorUnhealthy
		}
		return nil
	},
}

// runDoctorDiagnosis resolves the state directory READ-ONLY (state.Dir() when
// deps.StateDir is empty — never EnsureDir) and runs the ordered catalog of
// state-directory checks that need no tmux. The error return is reserved for a
// resolution failure that prevents diagnosis entirely; today it is always nil
// (a state.Dir() failure is folded into per-check failures so the report still
// carries every check).
func runDoctorDiagnosis(deps *DoctorDeps) ([]checkResult, error) {
	dir := deps.StateDir
	var dirErr error
	if dir == "" {
		dir, dirErr = state.Dir()
	}
	now := deps.Now()

	return []checkResult{
		checkDaemonAlive(dir, dirErr, now),
		checkStateDirSane(dir, dirErr),
		checkSessionsJSON(dir, dirErr),
	}, nil
}

// checkDaemonAlive reports whether the save daemon is running. A live
// daemon.pid passes with a "running (pid N, version V)" detail; a missing,
// unparseable, or dead PID fails with "not running". The distinct down-server
// message is Task 4-2, not here.
func checkDaemonAlive(dir string, dirErr error, now time.Time) checkResult {
	const name = "daemon"
	if dirErr != nil {
		return checkResult{name: name, status: checkFail, detail: "not running"}
	}
	report, err := state.CollectStatus(dir, now)
	if err != nil || report == nil || !report.DaemonRunning {
		return checkResult{name: name, status: checkFail, detail: "not running"}
	}
	return checkResult{
		name:   name,
		status: checkPass,
		detail: fmt.Sprintf("running (pid %d, version %s)", report.DaemonPID, doctorDaemonVersion(report.DaemonVersion)),
	}
}

// doctorDaemonVersion substitutes "unknown" when the daemon never recorded a
// version marker, so the detail never renders a bare "version )".
func doctorDaemonVersion(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

// checkStateDirSane reports whether the state directory is a usable directory.
// A not-yet-created directory passes ("not created yet") — a fresh install is
// healthy — while an existing-but-non-directory path or an unreadable stat
// fails.
func checkStateDirSane(dir string, dirErr error) checkResult {
	const name = "state dir"
	if dirErr != nil {
		return checkResult{name: name, status: checkFail, detail: "unresolvable"}
	}
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return checkResult{name: name, status: checkPass, detail: "not created yet"}
	case err != nil:
		return checkResult{name: name, status: checkFail, detail: "unreadable"}
	case info.IsDir():
		return checkResult{name: name, status: checkPass, detail: dir}
	default:
		return checkResult{name: name, status: checkFail, detail: "not a directory"}
	}
}

// checkSessionsJSON reports whether sessions.json is absent (healthy — nothing
// saved yet), corrupt, or a valid index. It reads via state.ReadIndex directly
// rather than the lossy HasLastSave so absent and corrupt are distinguished:
// ReadIndex returns (idx, false, nil) for a valid document, (Index{}, true,
// nil) for an absent file, and (Index{}, true, err-wrapping-ErrCorruptIndex)
// for a present-but-unusable one.
func checkSessionsJSON(dir string, dirErr error) checkResult {
	const name = "sessions.json"
	if dirErr != nil {
		return checkResult{name: name, status: checkFail, detail: "unresolvable"}
	}
	idx, skip, err := state.ReadIndex(dir)
	switch {
	case err != nil:
		return checkResult{name: name, status: checkFail, detail: "sessions.json corrupt"}
	case skip:
		return checkResult{name: name, status: checkPass, detail: "no sessions saved yet"}
	default:
		return checkResult{
			name:   name,
			status: checkPass,
			detail: fmt.Sprintf("%d sessions, %d panes", len(idx.Sessions), doctorPaneCount(idx)),
		}
	}
}

// doctorPaneCount returns the total number of panes across every window in
// every session of idx.
func doctorPaneCount(idx state.Index) int {
	total := 0
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			total += len(w.Panes)
		}
	}
	return total
}

// renderDoctorReport writes the "Portal doctor:" header followed by one line
// per result: a status marker, the check name, and the detail. checkInfo lines
// render without a pass/fail marker (a space keeps the name column aligned).
func renderDoctorReport(w io.Writer, results []checkResult) {
	_, _ = fmt.Fprintln(w, "Portal doctor:")
	for _, r := range results {
		_, _ = fmt.Fprintf(w, "  %s %s: %s\n", checkMarker(r.status), r.name, r.detail)
	}
}

// checkMarker maps a check status to its single-column report glyph. checkInfo
// renders as a blank so an informational line carries no pass/fail marker while
// still aligning with the marked lines.
func checkMarker(s checkStatus) string {
	switch s {
	case checkPass:
		return "✓"
	case checkFail:
		return "✗"
	case checkNotEvaluable:
		return "·"
	case checkInfo:
		return " "
	default:
		return " "
	}
}

// doctorUnhealthy reports whether any check failed. checkInfo and
// checkNotEvaluable never count toward the exit code.
func doctorUnhealthy(results []checkResult) bool {
	for _, r := range results {
		if r.status == checkFail {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
