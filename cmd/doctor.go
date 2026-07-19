package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// ErrDoctorUnhealthy is returned by `portal doctor` when any check reports a
// failure. It drives a non-zero process exit while emitting nothing to
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
// state.Dir()). Tests assign a *DoctorDeps with a hermetic StateDir temp dir so
// diagnosis runs against seeded fixtures without touching the developer's real
// ~/.config/portal/state.
type DoctorDeps struct {
	// StateDir overrides the resolved state directory. Empty means "resolve
	// via state.Dir()" (the production path).
	StateDir string
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
	// HookLister enumerates every live pane's hook key — the live set the
	// stale-hooks check tests persisted keys against. Production wires the same
	// tmux.DefaultClient() used by the other runtime seams (it implements
	// AllPaneLister via ListAllPaneHookKeys). An enumeration error, or a
	// zero-length result while hooks are persisted, is the not-evaluable path
	// (never "all stale").
	HookLister AllPaneLister
	// HookStore reads hooks.json for the stale-hooks check. Production wraps
	// loadHookStore(); a nil pointer (unresolvable config path) makes the check
	// not-evaluable rather than crashing diagnosis.
	HookStore *hooks.Store
	// ProjectStore reads projects.json for the stale-projects check. Production
	// wraps loadProjectStore(); a nil pointer makes the check not-evaluable. The
	// stale-projects check is filesystem-only (directory existence) and runs
	// independently of the tmux server state.
	ProjectStore *project.Store
	// Detector resolves the host-terminal identity for the informational
	// host-terminal line — the SAME Detect() seam the picker and the multi-target
	// open burst use (cmd/spawn_seams.go). Production wires spawn.NewDetector over the doctor tmux
	// client. When nil (a direct-call unit test that does not exercise the line),
	// the host-terminal line is omitted rather than invoking a real detector.
	Detector TerminalDetector
	// Resolve maps a detected identity to its adapter + resolution class — the
	// SAME config-aware resolver the burst uses (buildResolver().Resolve, loaded
	// from terminals.json). Only its Resolution is read here (a NULL identity
	// short-circuits before Resolve). When nil the host-terminal line is omitted.
	Resolve func(spawn.Identity) (spawn.Adapter, spawn.Resolution)
}

// doctorDeps is the package-level DI seam; nil in production.
var doctorDeps *DoctorDeps

// resolveDoctorDeps returns a fully-populated *DoctorDeps for one doctor
// invocation. Unset fields in the package-level doctorDeps fall through to the
// production defaults independently — same per-field nil-check idiom as
// commitNowDeps / bootstrapDeps. The returned value is never nil; the three
// tmux probe seams are guaranteed non-nil.
//
// doctor is bootstrap-exempt, so there is no shared tmux.Client in
// cmd.Context(); the production defaults build ONE tmux.DefaultClient() here
// and wire all three runtime seams off it (constructing the client is pure —
// no I/O — so it is cheap even when tests override every seam).
func resolveDoctorDeps() *DoctorDeps {
	client := tmux.DefaultClient()
	deps := &DoctorDeps{
		ServerRunning: client.ServerRunning,
		SaverPresent: func() (bool, error) {
			_, present, err := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)
			return present, err
		},
		HookCounts: func() (map[string]int, error) {
			return tmux.PortalHookCountsByEvent(client)
		},
		HookLister: client,
		// The host-terminal line reuses the picker/burst seams verbatim: the
		// process-tree/tmux detector over the doctor client, and the config-aware
		// terminals.json resolver. spawn.NewDetector is pure construction (no I/O
		// until Detect); Resolve is deferred through a closure so buildResolver
		// only reads terminals.json when the line is actually computed (a
		// non-null identity), and never when a test overrides the seam.
		Detector: spawn.NewDetector(client),
		Resolve: func(id spawn.Identity) (spawn.Adapter, spawn.Resolution) {
			return buildResolver().Resolve(id)
		},
	}
	// The stale-entry stores are built best-effort: a load-path error (an
	// unresolvable config dir) leaves the pointer nil, and the corresponding
	// check reports checkNotEvaluable rather than crashing diagnosis. NewStore
	// itself does no I/O — the file is read lazily by the check's Load call.
	if hookStore, err := loadHookStore(); err == nil {
		deps.HookStore = hookStore
	}
	if projectStore, err := loadProjectStore(); err == nil {
		deps.ProjectStore = projectStore
	}
	if doctorDeps == nil {
		return deps
	}
	if doctorDeps.StateDir != "" {
		deps.StateDir = doctorDeps.StateDir
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
	if doctorDeps.HookLister != nil {
		deps.HookLister = doctorDeps.HookLister
	}
	if doctorDeps.HookStore != nil {
		deps.HookStore = doctorDeps.HookStore
	}
	if doctorDeps.ProjectStore != nil {
		deps.ProjectStore = doctorDeps.ProjectStore
	}
	if doctorDeps.Detector != nil {
		deps.Detector = doctorDeps.Detector
	}
	if doctorDeps.Resolve != nil {
		deps.Resolve = doctorDeps.Resolve
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
		deps := resolveDoctorDeps()
		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			return err
		}
		renderDoctorReport(cmd.OutOrStdout(), results)

		fix, _ := cmd.Flags().GetBool("fix")
		if !fix {
			if doctorUnhealthy(results) {
				return ErrDoctorUnhealthy
			}
			return nil
		}

		// --fix: apply the reversible repairs (rendering the initial report
		// above first), then re-diagnose against the same deps so the checks
		// observe post-repair on-disk state. The exit is driven SOLELY by the
		// post-repair results — the repairs never touch it directly.
		if err := runDoctorFix(cmd, deps); err != nil {
			return err
		}
		postResults, err := runDoctorDiagnosis(deps)
		if err != nil {
			return err
		}
		renderDoctorReport(cmd.OutOrStdout(), postResults)
		if doctorUnhealthy(postResults) {
			return ErrDoctorUnhealthy
		}
		return nil
	},
}

// runDoctorFix applies doctor's low-stakes, reversible-by-reconstruction repairs
// in a fixed order — prune stale hooks, prune stale projects — then runs the
// unconditional log-sweep maintenance side-action. It is invoked AFTER the
// initial diagnosis render and BEFORE the re-diagnosis.
//
// The exit code is driven exclusively by the post-repair re-diagnosis, never by
// these repairs directly (per the spec's Exit-code contract), so every repair is
// best-effort: a failure is logged under the bootstrap component and swallowed,
// leaving the condition for the re-diagnosis to observe and report. The error
// return is reserved for a future catastrophic pre-diagnosis failure; today
// runDoctorFix always returns nil.
//
// The down-server data-loss safety is NOT a bespoke branch here: the stale-hook
// prune delegates to runHookStaleCleanup, whose mass-deletion hazard guard
// already defers when live-pane enumeration is empty or errored (the
// down/rebooted-server state), so a user-authored — non-reconstructable —
// on-resume command is never wiped. The stale-project prune is filesystem-only
// and runs regardless of server state.
func runDoctorFix(cmd *cobra.Command, deps *DoctorDeps) error {
	w := cmd.OutOrStdout()
	pruneDoctorStaleHooks(w, deps)
	pruneDoctorStaleProjects(w, deps)
	sweepDoctorLogs(deps)
	return nil
}

// pruneDoctorStaleHooks prunes hooks.json of entries whose pane key no longer
// matches a live tmux pane, printing "Pruned stale hook: <key>" per removal to w.
// It reuses runHookStaleCleanup VERBATIM: that helper's mass-deletion hazard
// guard is the sole down-server protection (an empty or errored live set defers
// with no prune), so there is deliberately no extra down-server branch here — a
// separate branch would double-guard and risk drift. A nil store (unresolvable
// config path) skips the prune. The helper's error return is discarded: a
// hookStore.Load / CleanStale failure leaves the entries in place for the
// re-diagnosis to report, honouring the repairs-never-drive-the-exit contract
// (the daemon's idle-tick hook cleanup, maybeRunHookCleanup, swallows it the same way).
func pruneDoctorStaleHooks(w io.Writer, deps *DoctorDeps) {
	if deps.HookStore == nil {
		return
	}
	_ = runHookStaleCleanup(deps.HookLister, deps.HookStore, bootstrapLogger, func(key string) {
		_, _ = fmt.Fprintf(w, "Pruned stale hook: %s\n", key)
	})
}

// pruneDoctorStaleProjects prunes projects.json of records whose directory no
// longer exists via project.Store.CleanStale (filesystem-only, os.Stat-based),
// printing "Pruned stale project: <name> (<path>)" per removal to w. It runs
// regardless of tmux server state. A nil store skips the prune; a CleanStale
// error is logged under the bootstrap component and swallowed.
func pruneDoctorStaleProjects(w io.Writer, deps *DoctorDeps) {
	if deps.ProjectStore == nil {
		return
	}
	removed, err := deps.ProjectStore.CleanStale()
	if err != nil {
		bootstrapLogger.Warn("doctor --fix: stale-project prune failed", "error", err)
		return
	}
	for _, p := range removed {
		_, _ = fmt.Fprintf(w, "Pruned stale project: %s (%s)\n", p.Name, p.Path)
	}
}

// sweepDoctorLogs runs the log-retention sweep — a deliberate unconditional
// maintenance side-action OUTSIDE the diagnose→repair loop. It is NOT the repair
// of a diagnosed condition (there is no stale-logs health check — logs
// auto-rotate and retention-sweep in the log handler) and its outcome NEVER
// touches the exit code: an unresolvable state dir or a sweep error is logged
// under the bootstrap component and swallowed. It reuses deps.StateDir (the
// hermetic test override) when set, else resolves READ-ONLY via state.Dir().
func sweepDoctorLogs(deps *DoctorDeps) {
	stateDir := deps.StateDir
	if stateDir == "" {
		dir, err := state.Dir()
		if err != nil {
			bootstrapLogger.Warn("doctor --fix: state dir unresolvable, skipping log sweep", "error", err)
			return
		}
		stateDir = dir
	}
	if err := log.SweepLogsForClean(stateDir); err != nil {
		bootstrapLogger.Warn("doctor --fix: log sweep failed", "error", err)
	}
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

	// Read the server gate once: a down server routes daemon / saver / hooks to
	// the distinct not-running detail without probing tmux at all. The state-dir
	// and sessions.json checks are server-independent and always run.
	serverUp := deps.ServerRunning()

	results := []checkResult{
		checkDaemonAlive(serverUp, dir, dirErr),
		checkSaverUp(serverUp, deps.SaverPresent),
		checkHooksRegistered(serverUp, deps.HookCounts),
		checkStateDirSane(dir, dirErr),
		checkSessionsJSON(dir, dirErr),
		checkStaleHooks(deps.HookLister, deps.HookStore),
		checkStaleProjects(deps.ProjectStore),
	}
	// The host-terminal identity is INFORMATIONAL — it lives at the END of the
	// report, after the pass/fail catalog, and never drives the exit code.
	// Production always wires both seams (resolveDoctorDeps), so the line is
	// always present there; direct-call unit tests that do not exercise it leave
	// the seams nil, in which case it is omitted.
	if deps.Detector != nil && deps.Resolve != nil {
		results = append(results, checkHostTerminal(deps.Detector, deps.Resolve))
	}
	return results, nil
}

// checkHostTerminal reports which host terminal Portal would drive for a
// multi-window spawn burst, computed from the SAME Detect()+Resolve seams the
// picker and the multi-target open burst use — no bespoke detection path. It is
// INFORMATIONAL ONLY (checkInfo, rendered without a pass/fail marker): an
// unsupported or remote host is an environmental state, not a Portal-health
// defect (single-target `open` still works — only the multi-window burst is
// unavailable), so the line sits OUTSIDE the pass/fail set and can NEVER drive
// the exit code (doctorUnhealthy counts only checkFail). Classification:
//
//   - a NULL identity (remote/mosh, no host-local client, or a transient detect
//     failure Detect folds to NULL) → "unsupported (remote session)";
//   - a recognised-but-undriven terminal (non-null, Resolution == Unsupported) →
//     "<Name> (unsupported)";
//   - a driven terminal (Resolution != Unsupported) → "<Name> (supported)".
//
// A NULL identity short-circuits before Resolve is consulted, so even a config
// `*` catch-all can never reclassify a remote client as supported.
func checkHostTerminal(detector TerminalDetector, resolve func(spawn.Identity) (spawn.Adapter, spawn.Resolution)) checkResult {
	const name = "host terminal"
	id := detector.Detect()
	if id.IsNull() {
		return checkResult{name: name, status: checkInfo, detail: "unsupported (remote session)"}
	}
	if _, resolution := resolve(id); resolution == spawn.ResolutionUnsupported {
		return checkResult{name: name, status: checkInfo, detail: fmt.Sprintf("%s (unsupported)", id.Name)}
	}
	return checkResult{name: name, status: checkInfo, detail: fmt.Sprintf("%s (supported)", id.Name)}
}

// checkStaleHooks reports whether hooks.json holds entries whose pane key no
// longer matches a live tmux pane. It is a strictly READ-ONLY mirror of
// runHookStaleCleanup's mass-deletion hazard guard (cmd/run_hook_stale_cleanup.go):
// it computes the stale set but NEVER prunes — pruning is `doctor --fix`, a
// separate surface. The guard order is load-bearing: when live-pane enumeration
// is empty-or-errored while hooks are present it reports checkNotEvaluable,
// NEVER "all stale", so a false failure can never mislead a --fix into a
// mass-delete of user-authored, non-reconstructable on-resume commands.
func checkStaleHooks(lister AllPaneLister, store *hooks.Store) checkResult {
	const name = "stale hooks"
	if store == nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not read hooks.json"}
	}
	persisted, err := store.Load()
	if err != nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not read hooks.json"}
	}
	live, err := lister.ListAllPaneHookKeys()
	if err != nil {
		// Server-down / transient read — NEVER report "all stale".
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not enumerate live panes"}
	}
	if len(live) == 0 {
		// Hazard-guard deferral: an empty live set with hooks present would make
		// every entry look orphaned. Mirror runHookStaleCleanup — defer, never
		// classify them all stale.
		if len(persisted) == 0 {
			return checkResult{name: name, status: checkPass, detail: "no hooks"}
		}
		return checkResult{name: name, status: checkNotEvaluable, detail: "zero live panes with hooks present (not evaluable)"}
	}
	stale := countStaleHookKeys(persisted, live)
	if stale > 0 {
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("%d stale hook entries", stale)}
	}
	return checkResult{name: name, status: checkPass, detail: "no stale hooks"}
}

// countStaleHookKeys counts persisted hook keys absent from the live key set —
// the same ∉ classification hooks.Store.CleanStale uses to select entries for
// deletion, computed here READ-ONLY (no prune, no Save).
func countStaleHookKeys(persisted map[string]map[string]string, live []string) int {
	liveSet := make(map[string]struct{}, len(live))
	for _, k := range live {
		liveSet[k] = struct{}{}
	}
	stale := 0
	for key := range persisted {
		if _, ok := liveSet[key]; !ok {
			stale++
		}
	}
	return stale
}

// checkStaleProjects reports whether projects.json holds records whose directory
// no longer exists. It mirrors project.Store.CleanStale's os.Stat classification
// READ-ONLY (nil → live, ErrNotExist → stale, any other error such as
// permission-denied → retained, NOT stale) without saving. It is filesystem-only
// and therefore independent of the tmux server state.
func checkStaleProjects(store *project.Store) checkResult {
	const name = "stale projects"
	if store == nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not read projects.json"}
	}
	projects, err := store.Load()
	if err != nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not read projects.json"}
	}
	stale := 0
	for _, p := range projects {
		_, statErr := os.Stat(p.Path)
		switch {
		case statErr == nil:
			// Directory present → live.
		case errors.Is(statErr, os.ErrNotExist):
			stale++
		default:
			// Permission-denied or other error → retained (NOT stale), matching
			// project.Store.CleanStale's conservative default branch.
		}
	}
	if stale > 0 {
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("%d stale projects", stale)}
	}
	return checkResult{name: name, status: checkPass, detail: "no stale projects"}
}

// checkDaemonAlive reports whether the save daemon is running. With the server
// down it reports the distinct not-running detail (doctor starts nothing, so a
// down server is honestly unhealthy, not corrupt). With the server up it is a
// narrow STATE-based probe reading only the three facts the detail needs: the
// recorded pid (state.ReadPIDFile), its liveness (state.IsProcessAlive), and the
// recorded version (state.ReadVersionFile). A live daemon.pid passes with a
// "running (pid N, version V)" detail; a missing, unparseable, or dead PID fails
// with "not running". It deliberately does NOT walk the state-dir tree or scan
// portal.log — a routine doctor run stays cheap.
func checkDaemonAlive(serverUp bool, dir string, dirErr error) checkResult {
	const name = "daemon"
	if !serverUp {
		return checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning}
	}
	if dirErr != nil {
		return checkResult{name: name, status: checkFail, detail: "not running"}
	}
	pid, err := state.ReadPIDFile(dir)
	if err != nil || !state.IsProcessAlive(pid) {
		return checkResult{name: name, status: checkFail, detail: "not running"}
	}
	// A missing or unreadable daemon.version is the normal "never recorded"
	// condition — swallow the error and let doctorDaemonVersion substitute
	// "unknown" so the detail never renders a bare "version )".
	version, _ := state.ReadVersionFile(dir)
	return checkResult{
		name:   name,
		status: checkPass,
		detail: fmt.Sprintf("running (pid %d, version %s)", pid, doctorDaemonVersion(version)),
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
			detail: fmt.Sprintf("%d sessions, %d panes", len(idx.Sessions), state.CountPanes(idx)),
		}
	}
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
	doctorCmd.Flags().Bool("fix", false, "apply low-stakes reversible repairs, then re-diagnose")
	rootCmd.AddCommand(doctorCmd)
}
