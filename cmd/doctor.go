package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// ErrDoctorUnhealthy is returned by `portal doctor` when any check reports a
// failure. Like ErrStatusUnhealthy (the retired `state status` sentinel it
// mirrors), it drives a non-zero process exit while emitting nothing to
// stderr: the rendered report is already on stdout, and IsSilentExitError
// (cmd/state_commit_now.go) compile-time-links the stderr-suppression
// contract. The sentinel exists solely to signal the unhealthy exit code.
var ErrDoctorUnhealthy = errors.New("doctor unhealthy")

// doctorRuntimeNotRunning is the byte-exact detail the daemon, saver and hooks
// checks all report when the tmux server is down. It is distinct from the
// corruption / dead-daemon details on purpose: a down server is an honest
// "Portal isn't running" state (doctor is bootstrap-exempt and starts nothing),
// not evidence of corruption. Reported unhealthy → non-zero, per the spec's
// Exit-code contract.
const doctorRuntimeNotRunning = "Portal runtime not running — run portal open to start"

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
	// ServerRunning reports whether a tmux server is up. It is the front gate
	// for the three runtime checks (daemon / saver / hooks): a down server
	// short-circuits all three to the distinct not-running detail without
	// touching tmux. Production wires tmux.Client.ServerRunning.
	ServerRunning func() bool
	// SaverPresent reports whether the _portal-saver session's pane is live.
	// Production wraps tmux.SaverPanePIDOrAbsent (discarding the pid); a
	// non-nil error is the transient path (not-evaluable, never a hard fail).
	SaverPresent func() (present bool, err error)
	// HookCounts returns the per-managed-event count of Portal-authored global
	// hook entries. Production wraps tmux.PortalHookCountsByEvent; a non-nil
	// error is the transient path (not-evaluable).
	HookCounts func() (map[string]int, error)
}

// doctorDeps is the package-level DI seam; nil in production.
var doctorDeps *DoctorDeps

// resolveDoctorDeps returns a fully-populated *DoctorDeps for one doctor
// invocation. Unset fields in the package-level doctorDeps fall through to the
// production defaults independently — same per-field nil-check idiom as
// commitNowDeps / bootstrapDeps. The returned value is never nil; Now and the
// three tmux probe seams are guaranteed non-nil.
//
// doctor is bootstrap-exempt, so there is no shared tmux.Client in
// cmd.Context(); the production defaults build ONE tmux.DefaultClient() here
// and wire all three runtime seams off it (constructing the client is pure —
// no I/O — so it is cheap even when tests override every seam).
func resolveDoctorDeps() *DoctorDeps {
	client := tmux.DefaultClient()
	deps := &DoctorDeps{
		Now:           time.Now,
		ServerRunning: client.ServerRunning,
		SaverPresent: func() (bool, error) {
			_, present, err := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)
			return present, err
		},
		HookCounts: func() (map[string]int, error) {
			return tmux.PortalHookCountsByEvent(client)
		},
	}
	if doctorDeps == nil {
		return deps
	}
	if doctorDeps.StateDir != "" {
		deps.StateDir = doctorDeps.StateDir
	}
	if doctorDeps.Now != nil {
		deps.Now = doctorDeps.Now
	}
	if doctorDeps.ServerRunning != nil {
		deps.ServerRunning = doctorDeps.ServerRunning
	}
	if doctorDeps.SaverPresent != nil {
		deps.SaverPresent = doctorDeps.SaverPresent
	}
	if doctorDeps.HookCounts != nil {
		deps.HookCounts = doctorDeps.HookCounts
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

	// Read the server gate once: a down server routes daemon / saver / hooks to
	// the distinct not-running detail without probing tmux at all. The state-dir
	// and sessions.json checks are server-independent and always run.
	serverUp := deps.ServerRunning()

	return []checkResult{
		checkDaemonAlive(serverUp, dir, dirErr, now),
		checkSaverUp(serverUp, deps.SaverPresent),
		checkHooksRegistered(serverUp, deps.HookCounts),
		checkStateDirSane(dir, dirErr),
		checkSessionsJSON(dir, dirErr),
	}, nil
}

// checkDaemonAlive reports whether the save daemon is running. With the server
// down it reports the distinct not-running detail (doctor starts nothing, so a
// down server is honestly unhealthy, not corrupt). With the server up it is a
// STATE-based probe: a live daemon.pid passes with a "running (pid N, version
// V)" detail; a missing, unparseable, or dead PID fails with "not running".
func checkDaemonAlive(serverUp bool, dir string, dirErr error, now time.Time) checkResult {
	const name = "daemon"
	if !serverUp {
		return checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning}
	}
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

// checkSaverUp reports whether the _portal-saver session's pane is live. With
// the server down it reports the distinct not-running detail. With the server
// up it probes via the injected saverPresent seam: present passes, absent
// (present=false, err=nil) fails, and a transient tmux error is not-evaluable
// (never a hard fail — an unreadable probe must not drive the exit code).
func checkSaverUp(serverUp bool, saverPresent func() (bool, error)) checkResult {
	const name = "saver"
	if !serverUp {
		return checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning}
	}
	present, err := saverPresent()
	switch {
	case err != nil:
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not read saver (transient tmux error)"}
	case present:
		return checkResult{name: name, status: checkPass, detail: "_portal-saver up"}
	default:
		return checkResult{name: name, status: checkFail, detail: "_portal-saver not running"}
	}
}

// checkHooksRegistered reports whether Portal's global hooks are registered
// exactly once per managed event. With the server down it reports the distinct
// not-running detail. With the server up it inspects the per-event count map
// from the injected hookCounts seam:
//
//   - a read error is not-evaluable (transient — never a hard fail);
//   - any event with >=2 entries fails as a duplicate (the first offending
//     event in sorted order, so the message is deterministic);
//   - else any event with 0 entries fails as not-registered (first in sorted
//     order);
//   - else (every event == 1) passes.
//
// Duplicates are reported ahead of missing entries: a stacked duplicate is the
// runaway-append failure mode this check exists to catch (tmux 3.6b's blind
// no-arg read let pane-focus-out / window-layout-changed dups accumulate).
func checkHooksRegistered(serverUp bool, hookCounts func() (map[string]int, error)) checkResult {
	const name = "hooks"
	if !serverUp {
		return checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning}
	}
	counts, err := hookCounts()
	if err != nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not read hooks (transient tmux error)"}
	}

	events := make([]string, 0, len(counts))
	for ev := range counts {
		events = append(events, ev)
	}
	sort.Strings(events)

	for _, ev := range events {
		if counts[ev] >= 2 {
			return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("duplicate hook entries on %s (%d)", ev, counts[ev])}
		}
	}
	for _, ev := range events {
		if counts[ev] == 0 {
			return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("hooks not registered on %s", ev)}
		}
	}
	return checkResult{name: name, status: checkPass, detail: "hooks registered (one per event)"}
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
