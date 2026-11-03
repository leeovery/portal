package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// ErrDoctorUnhealthy drives a non-zero exit and is deliberately silent on
// stderr — the rendered report is already on stdout.
var ErrDoctorUnhealthy = errors.New("doctor unhealthy")

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
// diagnosis. The sessions themselves are listed through sessionBackend, staged
// with withBackendDeps.
type DoctorDeps struct {
	StateDir  string
	ThemesDir string
	ZmxDir    string
	LookPath  func(file string) (string, error)
	// ProtocolVersion reads the protocol a session's daemon speaks. It is the
	// zmx driver's own, on no backend interface.
	ProtocolVersion func(ctx context.Context, name string) (int, error)
	ProjectStore    *project.Store
	PrefsStore      *prefs.Store
	Detector        TerminalDetector
	Resolve         spawn.AdapterResolver
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

	if deps.ZmxDir == "" {
		deps.ZmxDir = zmx.DirFor(os.Environ())
	}
	if deps.LookPath == nil {
		deps.LookPath = exec.LookPath
	}
	if deps.ProtocolVersion == nil {
		deps.ProtocolVersion = zmxProtocolVersion(deps.ZmxDir)
	}
	if deps.Detector == nil || deps.Resolve == nil {
		seams := buildProductionSpawnSeams(tmux.DefaultClient())
		if deps.Detector == nil {
			deps.Detector = seams.Detector
		}
		if deps.Resolve == nil {
			deps.Resolve = seams.Resolve
		}
	}

	// Each best-effort load is constructed only for a seam left unset, so an
	// injected store costs no config read.
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

// zmxProtocolVersion is the driver's protocol read on dir. A dir the driver
// refuses to bind answers every read with that refusal.
func zmxProtocolVersion(dir string) func(context.Context, string) (int, error) {
	driver, err := zmx.New(zmx.Config{Binary: zmxProgram, Dir: dir})
	if err != nil {
		return func(context.Context, string) (int, error) { return 0, err }
	}
	return driver.ProtocolVersion
}

var doctorCmd = &cobra.Command{
	Use:           "doctor",
	Short:         "Diagnose Portal's health",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := resolveDoctorDeps()
		results, err := runDoctorDiagnosis(cmd.Context(), deps)
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
		postResults, err := runDoctorDiagnosis(cmd.Context(), deps)
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
// never does — diagnosed or not. Stale hooks get none in this phase: hooks.json
// is keyed by pane token until it is keyed by session name, and judged against
// session names every real key would read as stale.
func runDoctorFix(cmd *cobra.Command, deps *DoctorDeps) {
	pruneDoctorStaleProjects(cmd.OutOrStdout(), deps)
	sweepDoctorLogs(deps)
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
func runDoctorDiagnosis(ctx context.Context, deps *DoctorDeps) ([]checkResult, error) {
	dir := deps.StateDir
	var dirErr error
	if dir == "" {
		dir, dirErr = state.Dir()
	}

	results := []checkResult{
		checkZmxBinary(deps.LookPath),
		checkZmxDir(deps.ZmxDir),
		checkSessionsAnswer(ctx, deps.ProtocolVersion),
		checkStateDirSane(dir, dirErr),
		checkSessionsJSON(dir, dirErr),
		checkStaleProjects(deps.ProjectStore),
	}
	if deps.Detector != nil && deps.Resolve != nil {
		results = append(results, checkHostTerminal(deps.Detector, deps.Resolve))
	}
	results = append(results, checkResumeMode(deps.PrefsStore))
	return results, nil
}

func checkZmxBinary(lookPath func(string) (string, error)) checkResult {
	const name = "zmx"
	path, err := lookPath(zmxProgram)
	if err != nil {
		return checkResult{name: name, status: checkFail, detail: errZmxMissing.Error()}
	}
	return checkResult{name: name, status: checkPass, detail: path}
}

// zmx makes its directory with its first session, so one not made yet holds
// none and is healthy.
func checkZmxDir(dir string) checkResult {
	const name = "zmx dir"
	if !filepath.IsAbs(dir) {
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("%q is not an absolute path", dir)}
	}
	return checkDirReadable(name, dir)
}

// checkSessionsAnswer names every session that did not answer the listing,
// with what its daemon says when asked for its protocol alone. A failed
// listing is the zmx dir check's to judge, so it is not evaluable here.
func checkSessionsAnswer(ctx context.Context, protocolVersion func(context.Context, string) (int, error)) checkResult {
	const name = "sessions"
	sessions, err := listAllSessions(ctx)
	if err != nil {
		return checkResult{name: name, status: checkNotEvaluable, detail: "could not list sessions: " + err.Error()}
	}
	if len(sessions) == 0 {
		return checkResult{name: name, status: checkPass, detail: "no sessions"}
	}
	var silent []string
	for _, s := range sessions {
		if !s.Answered {
			silent = append(silent, s.Name)
		}
	}
	if len(silent) == 0 {
		return checkResult{name: name, status: checkPass, detail: pluralCount(len(sessions), "session answering", "sessions answering")}
	}
	return checkResult{
		name:   name,
		status: checkFail,
		detail: fmt.Sprintf("%d of %d not answering: %s", len(silent), len(sessions), strings.Join(unansweredReasons(ctx, protocolVersion, silent), ", ")),
	}
}

// listAllSessions keeps Portal's own sessions: one of them not answering is as
// much a fault as a user's.
func listAllSessions(ctx context.Context) ([]backend.Session, error) {
	b, err := sessionBackend()
	if err != nil {
		return nil, err
	}
	return b.List(ctx)
}

// unansweredReasons probes every silent session at once, so daemons that give
// no answer cost one probe ceiling between them. An Info request changes
// nothing on a daemon.
func unansweredReasons(ctx context.Context, protocolVersion func(context.Context, string) (int, error), names []string) []string {
	reasons := make([]string, len(names))
	var wg sync.WaitGroup
	for i, n := range names {
		wg.Go(func() {
			reasons[i] = n + " (" + unansweredReason(ctx, protocolVersion, n) + ")"
		})
	}
	wg.Wait()
	return reasons
}

// A daemon whose header reads protocol 1 still failed the listing every
// command reads, so it is named as not answering too.
func unansweredReason(ctx context.Context, protocolVersion func(context.Context, string) (int, error), name string) string {
	version, err := protocolVersion(ctx, name)
	switch {
	case errors.Is(err, backend.ErrNoSuchSession):
		return "ended"
	case err != nil || version == zmx.ProtocolVersion:
		return "no answer"
	default:
		return fmt.Sprintf("speaks protocol %d; Portal speaks %d", version, zmx.ProtocolVersion)
	}
}

func checkResumeMode(store *prefs.Store) checkResult {
	const name = "resume mode"
	return checkResult{name: name, status: checkInfo, detail: installResumeModeOf(store).String()}
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

// projectStoreReadStandDownPhrase words a projects.json the check could not
// read. It is declared rather than written at each branch that renders it, so
// a re-wording moves them together.
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
	return checkDirReadable(name, dir)
}

// checkDirReadable passes a directory that can be read, or that is not made
// yet. Opening it is the read: a stat alone passes a directory that refuses
// to be listed.
func checkDirReadable(name, dir string) checkResult {
	info, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return checkResult{name: name, status: checkPass, detail: "not created yet"}
	case err != nil:
		return checkResult{name: name, status: checkFail, detail: "unreadable"}
	case !info.IsDir():
		return checkResult{name: name, status: checkFail, detail: "not a directory"}
	}
	f, err := os.Open(dir)
	if err != nil {
		return checkResult{name: name, status: checkFail, detail: "unreadable"}
	}
	_ = f.Close()
	return checkResult{name: name, status: checkPass, detail: dir}
}

// An absent sessions.json is a fresh install, told apart from an empty index.
// A version 1 file never reaches here: the root refuses it first.
func checkSessionsJSON(dir string, dirErr error) checkResult {
	const name = "sessions.json"
	if dirErr != nil {
		return checkResult{name: name, status: checkFail, detail: "unresolvable"}
	}
	if _, err := os.Stat(sessionindex.Path(dir)); errors.Is(err, fs.ErrNotExist) {
		return checkResult{name: name, status: checkPass, detail: "no sessions indexed yet"}
	}
	idx, err := sessionindex.Read(dir)
	if versionErr, ok := errors.AsType[*sessionindex.VersionError](err); ok {
		return checkResult{name: name, status: checkFail, detail: fmt.Sprintf("version %d; this Portal reads version %d", versionErr.Found, sessionindex.SchemaVersion)}
	}
	if _, ok := errors.AsType[*fs.PathError](err); ok {
		return checkResult{name: name, status: checkFail, detail: "unreadable"}
	}
	if err != nil {
		return checkResult{name: name, status: checkFail, detail: "corrupt"}
	}
	return checkResult{name: name, status: checkPass, detail: pluralCount(len(idx.Sessions), "session indexed", "sessions indexed")}
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
