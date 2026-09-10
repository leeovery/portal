package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// ErrDoctorUnhealthy drives a non-zero exit and is deliberately silent on
// stderr — the rendered report is already on stdout.
var ErrDoctorUnhealthy = errors.New("doctor unhealthy")

const doctorRuntimeNotRunning = "Portal runtime not running — run portal open to start"

func runtimeDownResult(name string) checkResult {
	return checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning}
}

type checkStatus int

const (
	// Zero value: an unset status counts as unhealthy, never as a passing check.
	checkUnknown checkStatus = iota
	checkPass
	checkFail
	// checkInfo and checkNotEvaluable never drive the exit code.
	checkInfo
	checkNotEvaluable
)

type checkResult struct {
	name   string
	status checkStatus
	detail string
}

// advisory sits outside the pass/fail catalog, and so outside the exit code.
// line is the whole rendered string, glyph included — the renderer only indents.
type advisory struct {
	line string
}

// DoctorDeps fields are all optional: an unset one falls through to the
// production default in resolveDoctorDeps, and nothing left nil aborts
// diagnosis.
type DoctorDeps struct {
	StateDir      string
	ThemesDir     string
	ServerRunning func() bool
	SaverPresent  func() (present bool, err error)
	HookCounts    func() (map[string]int, error)
	HookLister    hooksweep.Reader
	HookStore     *hooks.Store
	ProjectStore  *project.Store
	PrefsStore    *prefs.Store
	Detector      TerminalDetector
	Resolve       spawn.AdapterResolver
}

var doctorDeps *DoctorDeps

// resolveDoctorDeps returns the seams doctor runs against: whatever a test
// injected, with the production default filled in for each seam it left unset.
// A new seam costs one fill line here; leaving it out is a nil dereference at
// first use rather than an injection the diagnosis silently ignores.
func resolveDoctorDeps() *DoctorDeps {
	deps := &DoctorDeps{}
	if doctorDeps != nil {
		*deps = *doctorDeps
	}

	client := tmux.DefaultClient()
	if deps.ServerRunning == nil {
		deps.ServerRunning = client.ServerRunning
	}
	if deps.SaverPresent == nil {
		deps.SaverPresent = func() (bool, error) {
			_, present, err := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName)
			return present, err
		}
	}
	if deps.HookCounts == nil {
		deps.HookCounts = func() (map[string]int, error) {
			return tmux.PortalHookCountsByEvent(client)
		}
	}
	if deps.HookLister == nil {
		deps.HookLister = client
	}
	if deps.Detector == nil || deps.Resolve == nil {
		seams := buildProductionSpawnSeams(client)
		if deps.Detector == nil {
			deps.Detector = seams.Detector
		}
		if deps.Resolve == nil {
			deps.Resolve = seams.Resolve
		}
	}

	// Each best-effort load is constructed only for a seam left unset, so an
	// injected store costs no config read.
	if deps.HookStore == nil {
		if hookStore, err := loadHookStore(); err == nil {
			deps.HookStore = hookStore
		}
	}
	if deps.ProjectStore == nil {
		if projectStore, err := loadProjectStore(); err == nil {
			deps.ProjectStore = projectStore
		}
	}
	if deps.PrefsStore == nil {
		// Never the migrating loadPrefsStore: doctor heals nothing on the
		// read-only path, and that one dispatches the one-shot appearance
		// translation.
		if prefsStore, err := loadPrefsStoreNoMigrate(); err == nil {
			deps.PrefsStore = prefsStore
		}
	}
	if deps.ThemesDir == "" {
		if themesDir, err := themesDirPath(); err == nil {
			deps.ThemesDir = themesDir
		}
	}

	return deps
}

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
		renderDoctorReport(cmd.OutOrStdout(), results, collectThemeAdvisories(deps))

		fix, _ := cmd.Flags().GetBool("fix")
		if !fix {
			if doctorUnhealthy(results) {
				return ErrDoctorUnhealthy
			}
			return nil
		}

		// The exit is driven solely by the post-repair re-diagnosis below; the
		// repairs never touch it directly.
		runDoctorFix(cmd, deps)
		postResults, err := runDoctorDiagnosis(deps)
		if err != nil {
			return err
		}
		// Re-collected, not carried down from the first pass: the whole second
		// report must describe one moment.
		renderDoctorReport(cmd.OutOrStdout(), postResults, collectThemeAdvisories(deps))
		if doctorUnhealthy(postResults) {
			return ErrDoctorUnhealthy
		}
		return nil
	},
}

// runDoctorFix applies only repairs that are reversible by reconstruction, each
// best-effort so a failure is left for the re-diagnosis to report. Themes get no
// repair step at all: rewriting a broken theme file would destroy user-authored
// content, and creating or seeding the themes directory is something Portal
// never does — diagnosed or not.
func runDoctorFix(cmd *cobra.Command, deps *DoctorDeps) {
	w := cmd.OutOrStdout()
	pruneDoctorStaleHooks(w, deps)
	pruneDoctorStaleProjects(w, deps)
	sweepDoctorLogs(deps)
}

// The sweep's own mass-deletion hazard guard is the down-server protection
// here — do not add a second guard. A cycle that removed nothing for a reason
// renders that reason — why it declined, or that it failed — so a prune that
// could not run never looks like one that ran and found nothing. A store that
// could not be opened is one such reason: to a user it is the file that could
// not be read, which is the words the diagnosis already uses for it.
func pruneDoctorStaleHooks(w io.Writer, deps *DoctorDeps) {
	if deps.HookStore == nil {
		reportSkippedPrune(w, hooksweep.ReasonStoreReadFailed)
		return
	}
	outcome, err := hooksweep.Run(deps.HookLister, deps.HookStore)
	if err != nil {
		reportFailedPrune(w)
		return
	}
	for _, key := range outcome.Removed {
		_, _ = fmt.Fprintf(w, "Pruned stale hook: %s\n", key)
	}
	if outcome.DeclineReason != "" {
		reportSkippedPrune(w, outcome.DeclineReason)
	}
}

// The phrases both surface vocabularies share are written once here, so
// re-wording one moves every line that prints it. A restore that is running and
// a marker that could not be read stand the cycle down alike, but they are
// different conditions: the server is routinely down when a user reaches for a
// diagnosis, and reporting that as a restore would assert something that is not
// happening.
const (
	restoreStandDownPhrase    = "restore in progress"
	markerReadStandDownPhrase = "could not read the restore marker"
	storeReadStandDownPhrase  = "could not read hooks.json"
	paneReadStandDownPhrase   = "could not enumerate live panes"
	lockStandDownPhrase       = "hooks.json is locked"
)

// sweepFailedStandDownPhrase words a sweep that ran and failed. It reads
// alongside the stand-down phrases above and is deliberately not one of them:
// no reason names it, because the cycle declined nothing.
const sweepFailedStandDownPhrase = "the sweep could not complete"

// skippedPrunePhrases completes "Skipped stale hook prune: …" for a user who
// asked for a repair. A failed enumeration and a successful one that answered
// nothing are separate conditions, so neither borrows the other's words.
var skippedPrunePhrases = map[hooksweep.Reason]string{
	hooksweep.ReasonRestoring:        restoreStandDownPhrase,
	hooksweep.ReasonMarkerReadFailed: markerReadStandDownPhrase,
	hooksweep.ReasonStoreReadFailed:  storeReadStandDownPhrase,
	hooksweep.ReasonPaneReadFailed:   paneReadStandDownPhrase,
	hooksweep.ReasonEmptyPaneRead:    "live pane list came back empty",
	hooksweep.ReasonLockTimeout:      lockStandDownPhrase,
}

// notEvaluableDetails renders a stand-down reason as the reason the count cannot
// be taken, so the diagnostic reports exactly what the reaper declined to judge.
var notEvaluableDetails = map[hooksweep.Reason]string{
	hooksweep.ReasonRestoring:        restoreStandDownPhrase + " (not evaluable)",
	hooksweep.ReasonMarkerReadFailed: markerReadStandDownPhrase,
	hooksweep.ReasonStoreReadFailed:  storeReadStandDownPhrase,
	hooksweep.ReasonPaneReadFailed:   paneReadStandDownPhrase,
	hooksweep.ReasonEmptyPaneRead:    "zero live panes with hooks present (not evaluable)",
	hooksweep.ReasonLockTimeout:      lockStandDownPhrase + " (not evaluable)",
}

// phraseFor renders a stand-down reason through one of the surface vocabularies
// above. A reason no vocabulary words renders nothing rather than its own slug:
// exhaustiveness is a test's to enforce, and a runtime fall-through would only
// make internal words look like copy on the command that explains what happened.
func phraseFor(m map[hooksweep.Reason]string, reason hooksweep.Reason) string {
	return m[reason]
}

// reportSkippedPrune renders a prune that removed nothing from the shared
// reason vocabulary, so no branch can reach for words of its own.
func reportSkippedPrune(w io.Writer, reason hooksweep.Reason) {
	_, _ = fmt.Fprintf(w, "Skipped stale hook prune: %s\n", phraseFor(skippedPrunePhrases, reason))
}

// reportFailedPrune renders a prune that ran and failed. It reads as a skipped
// prune because that is what the user got, but it is no stand-down — the sweep
// declined nothing — so it names its own phrase rather than drawing one from
// the stand-down vocabulary.
func reportFailedPrune(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Skipped stale hook prune: %s\n", sweepFailedStandDownPhrase)
}

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

// sweepDoctorLogs is unconditional maintenance, not the repair of a diagnosed
// condition — there is no stale-logs check.
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

// runDoctorDiagnosis resolves the state directory read-only — never EnsureDir;
// doctor creates nothing.
func runDoctorDiagnosis(deps *DoctorDeps) ([]checkResult, error) {
	dir := deps.StateDir
	var dirErr error
	if dir == "" {
		dir, dirErr = state.Dir()
	}

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
	if deps.Detector != nil && deps.Resolve != nil {
		results = append(results, checkHostTerminal(deps.Detector, deps.Resolve))
	}
	return results, nil
}

// An unsupported or remote host is an environmental state, not a Portal-health
// defect, so this line never drives the exit code. A NULL identity short-circuits
// before Resolve, so a config `*` catch-all cannot reclassify a remote client.
func checkHostTerminal(detector TerminalDetector, resolve spawn.AdapterResolver) checkResult {
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

// staleHooksNotEvaluable reports a stand-down in the diagnosis's register, so a
// branch reporting one names its reason and the vocabulary words the copy.
func staleHooksNotEvaluable(name string, reason hooksweep.Reason) checkResult {
	return checkResult{name: name, status: checkNotEvaluable, detail: phraseFor(notEvaluableDetails, reason)}
}

// The check reads its store first and hands the result to the shared ladder,
// so an unreadable hooks.json is reported as itself rather than as whatever the
// tmux reads make of it. Past the ladder every judgeable entry is counted: an
// unreadable pane list, or a server with no panes at all, would otherwise
// report the lot stale and mislead a --fix into mass-deleting user-authored
// on-resume commands. A live pane carrying no token is not that case — under
// lazy stamping it is the ordinary one, and the count proceeds.
func checkStaleHooks(reader hooksweep.Reader, store *hooks.Store) checkResult {
	const name = "stale hooks"
	// No store to read and a read that failed are one condition to a user: the
	// file could not be read, so both render the vocabulary's words for it.
	if store == nil {
		return staleHooksNotEvaluable(name, hooksweep.ReasonStoreReadFailed)
	}
	persisted, err := store.Load(hooks.ViaDoctor)
	if err != nil {
		return staleHooksNotEvaluable(name, hooksweep.ReasonStoreReadFailed)
	}

	if decline := hooksweep.StalenessStandDown(reader); decline.Declined() {
		return staleHooksNotEvaluable(name, decline.Reason())
	}

	view := hooksweep.JudgeAgainstLivePanes(reader, len(persisted))
	if view.Decline.Declined() {
		// Nothing persisted is an answer the sweep gives without reading a
		// pane, so a pane read that failed cannot make it unknowable here
		// either: with no key to judge, the count is zero whatever tmux said.
		if len(persisted) == 0 {
			return checkResult{name: name, status: checkPass, detail: "no hooks"}
		}
		return staleHooksNotEvaluable(name, view.Decline.Reason())
	}

	if view.PaneRows == 0 && len(persisted) == 0 {
		return checkResult{name: name, status: checkPass, detail: "no hooks"}
	}
	stale := len(hooks.StaleKeys(persisted, view.LiveTokens))
	if stale > 0 {
		return checkResult{name: name, status: checkFail, detail: pluralCount(stale, "stale hook entry", "stale hook entries")}
	}
	return checkResult{name: name, status: checkPass, detail: "no stale hooks"}
}

// projectStoreReadStandDownPhrase words a projects.json the check could not
// read. It is declared rather than written at each branch that renders it, so
// a re-wording moves them together. It names no stand-down reason from the
// hooks vocabulary: that vocabulary words a hook cycle's declines, and this
// check runs no cycle and has no repair line to word.
const projectStoreReadStandDownPhrase = "could not read projects.json"

func checkStaleProjects(store *project.Store) checkResult {
	const name = "stale projects"
	if store == nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: projectStoreReadStandDownPhrase}
	}
	stale, err := store.StaleEntries()
	if err != nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: projectStoreReadStandDownPhrase}
	}
	if len(stale) > 0 {
		return checkResult{name: name, status: checkFail, detail: pluralCount(len(stale), "stale project", "stale projects")}
	}
	return checkResult{name: name, status: checkPass, detail: "no stale projects"}
}

// checkDaemonAlive is a narrow state-file probe: it deliberately does not walk
// the state-dir tree or scan portal.log, so a routine doctor run stays cheap.
func checkDaemonAlive(serverUp bool, dir string, dirErr error) checkResult {
	const name = "daemon"
	if !serverUp {
		return runtimeDownResult(name)
	}
	if dirErr != nil {
		return checkResult{name: name, status: checkFail, detail: "not running"}
	}
	pid, err := state.ReadPIDFile(dir)
	if err != nil || !state.IsProcessAlive(pid) {
		return checkResult{name: name, status: checkFail, detail: "not running"}
	}
	version, _ := state.ReadVersionFile(dir)
	return checkResult{
		name:   name,
		status: checkPass,
		detail: fmt.Sprintf("running (pid %d, version %s)", pid, doctorDaemonVersion(version)),
	}
}

func checkSaverUp(serverUp bool, saverPresent func() (bool, error)) checkResult {
	const name = "saver"
	if !serverUp {
		return runtimeDownResult(name)
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

// Events are reported in sorted order so the message is deterministic, and
// duplicates ahead of missing entries: a stacked duplicate is the runaway-append
// failure this check exists to catch.
func checkHooksRegistered(serverUp bool, hookCounts func() (map[string]int, error)) checkResult {
	const name = "hooks"
	if !serverUp {
		return runtimeDownResult(name)
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

func doctorDaemonVersion(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

func pluralCount(n int, singular, plural string) string {
	unit := plural
	if n == 1 {
		unit = singular
	}
	return fmt.Sprintf("%d %s", n, unit)
}

// A not-yet-created state directory passes: a fresh install is healthy.
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

// state.ReadIndex, not the lossy HasLastSave: absent and corrupt must report
// differently.
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
			detail: fmt.Sprintf("%s, %s", pluralCount(len(idx.Sessions), "session", "sessions"), pluralCount(state.CountPanes(idx), "pane", "panes")),
		}
	}
}

func renderDoctorReport(w io.Writer, results []checkResult, advisories []advisory) {
	_, _ = fmt.Fprintln(w, "Portal doctor:")
	for _, r := range results {
		_, _ = fmt.Fprintf(w, "  %s %s: %s\n", checkMarker(r.status), r.name, r.detail)
	}
	// The variable-length advisory block trails the catalog and never interleaves,
	// so a reader can expect a given check at a given place.
	for _, a := range advisories {
		_, _ = fmt.Fprintf(w, "  %s\n", a.line)
	}
	_, _ = fmt.Fprintf(w, "  %s\n", doctorSummaryLine(results, advisories))
}

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

func doctorUnhealthy(results []checkResult) bool {
	for _, r := range results {
		if r.status == checkFail || r.status == checkUnknown {
			return true
		}
	}
	return false
}

// doctorCheckCounts explains the exit code and never computes it, so its arms
// must agree with doctorUnhealthy.
func doctorCheckCounts(results []checkResult) (passed, total int) {
	for _, r := range results {
		switch r.status {
		case checkPass:
			passed++
			total++
		case checkFail, checkUnknown:
			total++
		case checkInfo, checkNotEvaluable:
			// Outside the exit-code class: counted by neither, deliberately.
		}
	}
	return passed, total
}

// The checks count deliberately skips pluralCount — doctor's catalog never has
// a single member — while the advisory count, which routinely does, takes it.
func doctorSummaryLine(results []checkResult, advisories []advisory) string {
	passed, total := doctorCheckCounts(results)
	summary := fmt.Sprintf("%d checks passed", passed)
	if passed != total {
		summary = fmt.Sprintf("%d of %d checks passed", passed, total)
	}
	if m := len(advisories); m > 0 {
		summary += " · " + pluralCount(m, "advisory", "advisories")
	}
	return summary
}

func init() {
	doctorCmd.Flags().Bool("fix", false, "apply low-stakes reversible repairs, then re-diagnose")
	rootCmd.AddCommand(doctorCmd)
}
