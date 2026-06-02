// Package bootstrap composes the eleven-step PersistentPreRunE sequence
// pinned by the resurrection spec. Step ordering is load-bearing;
// "Return" is the post-step boundary, not a numbered step:
//
//  1. EnsureServer
//  2. RegisterPortalHooks
//  3. Set @portal-restoring (MUST precede steps 4 and 5)
//  4. SweepOrphanDaemons (best-effort; pgrep-based orphan `portal state
//     daemon` enumeration with identity-checked SIGKILL — runs before
//     EnsureSaver so the new saver-pane daemon's first tick is
//     uncontested by leftover daemons from prior server lifetimes)
//  5. EnsureSaver (best-effort; SaverDownWarning on failure)
//  6. Restore
//  7. EagerSignalHydrate (best-effort; iterates the freshly-armed
//     `@portal-skeleton-*` marker map and writes the hydrate signal byte
//     to each pane's FIFO so every helper — not just the user's attached
//     session's helper — proceeds to scrollback replay rather than
//     timing out and leaking markers; runs after Restore (markers must
//     exist) and before Clear so the daemon's restoring-marker
//     suppression window still covers the writes)
//  8. Clear @portal-restoring
//  9. CleanStaleMarkers (best-effort; diffs `@portal-skeleton-*` markers
//     against the live-pane set and unsets markers whose paneKey is no
//     longer represented by a live pane — runs after Clear so it observes
//     post-restore tmux state, and before Sweep so any stale markers
//     protecting orphan FIFOs are unset first, allowing those FIFOs to be
//     reclaimed in the same bootstrap)
//  10. SweepOrphanFIFOs (best-effort; observes still-set per-pane
//     @portal-skeleton-* markers from step 6 — those outlive
//     @portal-restoring and are cleared per-pane on hydration)
//  11. CleanStale (best-effort)
//
// Return is the post-step boundary that collects accumulated warnings.
package bootstrap

import (
	"context"
	"log/slog"
	"time"

	"github.com/leeovery/portal/internal/log"
)

// cleanLogger is the clean-component-bound logger used for the two
// cmd/bootstrap clean-sweep cycle summaries (orphan-daemon sweep, marker
// sweep) and the orphan-daemon sweep's own per-kill DEBUG detail. The
// component flips to "clean" on these lines per the Subsystem prefix taxonomy
// (clean owns sweep outcomes), while per-item identity-skip / kill-failure /
// per-unset-failure breadcrumbs stay on the bootstrap-bound logger seam
// injected into each step core. Bound once at package init via log.For so it
// routes through the shared handler indirection (observing later Init /
// SetTestHandler swaps).
var cleanLogger = log.For("clean")

// totalSteps is the fixed step count carried verbatim on the
// orchestration-complete summary's steps attr. The eleven-step sequence is a
// load-bearing contract; this constant is the single source of truth for the
// summary line.
const totalSteps = 11

// Closed StepName set — the canonical step= value for BOTH the per-step
// entering DEBUG breadcrumb and the per-step "step complete" INFO summary, so
// the two lines for a given step always agree. One literal per step; no
// ad-hoc names. The @portal-restoring marker steps are normalized to
// stepSetRestoring / stepClearRestoring (not "Set @portal-restoring") so the
// step= attr is a stable identifier rather than a prose phrase.
const (
	stepEnsureServer       = "EnsureServer"
	stepRegisterHooks      = "RegisterPortalHooks"
	stepSetRestoring       = "SetRestoring"
	stepSweepOrphanDaemons = "SweepOrphanDaemons"
	stepEnsureSaver        = "EnsureSaver"
	stepRestore            = "Restore"
	stepEagerSignalHydrate = "EagerSignalHydrate"
	stepClearRestoring     = "ClearRestoring"
	stepCleanStaleMarkers  = "CleanStaleMarkers"
	stepSweepOrphanFIFOs   = "SweepOrphanFIFOs"
	stepCleanStale         = "CleanStale"
)

// Runner is the abstraction cmd/root.go depends on so PersistentPreRunE
// does not import the concrete *Orchestrator type. Orchestrator implicitly
// satisfies Runner; tests inject lightweight fakes (no-op runners,
// recording fakes, panic guards) via BootstrapDeps.Orchestrator.
//
// The middle return value carries any soft Warnings accumulated during
// the run (Phase 6 task 6-9). Lightweight test fakes typically return a
// nil slice — only the full Orchestrator produces warnings.
type Runner interface {
	Run(ctx context.Context) (bool, []Warning, error)
}

// ServerBootstrapper starts the tmux server when not already running.
// EnsureServer reports whether Portal itself was the one that started it.
type ServerBootstrapper interface {
	EnsureServer() (bool, error)
}

// HookRegistrar registers Portal's global tmux hooks idempotently.
type HookRegistrar interface {
	RegisterPortalHooks() error
}

// RestoringMarker manages the @portal-restoring server option that
// suppresses the save daemon while skeleton restoration is in flight.
type RestoringMarker interface {
	Set() error
	Clear() error
}

// SaverBootstrapper ensures the _portal-saver detached session exists
// and matches the current binary version.
type SaverBootstrapper interface {
	EnsureSaver() error
}

// Restorer performs skeleton-only session restoration.
//
// Contract (self-enforcing via the typed return signature):
//   - Returns (false, nil) on the happy path and after isolating any
//     per-session failures. Per the spec's degrade-locally-and-continue
//     principle, every soft per-session error MUST be logged and swallowed
//     inside the implementation — they MUST NOT travel up through err.
//   - Returns (true, err) when sessions.json itself is unparseable; err
//     MUST wrap state.ErrCorruptIndex so callers downstream can match via
//     errors.Is. corrupt=true is the ONLY case in which err is non-nil.
//
// The bool exists so Orchestrator step 6 can branch on a typed signal
// rather than a string-equality check on the error chain. A future
// implementation that violates the contract by returning (false, err)
// is treated defensively by Run: the err is logged and the orchestrator
// continues without escalating to a PersistentPreRunE abort. This guards
// the "degrade locally, log, continue" principle against silent drift.
type Restorer interface {
	Restore() (corrupt bool, err error)
}

// EagerHydrateSignaler writes the hydrate signal byte to every freshly-armed
// `@portal-skeleton-*` pane's FIFO so every helper proceeds to scrollback
// replay rather than waiting on the per-pane client-attached hook (which only
// fires for the user's currently attached session, leaving N-1 helpers to time
// out and leak markers).
//
// Best-effort: a non-nil return is logged via Logger.Warn and swallowed by the
// orchestrator — eager-signal failures must never block PersistentPreRunE.
// Per-FIFO write failures are isolated inside the implementation (logged and
// continued); only marker-enumeration failures propagate via the return value.
//
// The concrete *EagerSignalCore in eager_signal_hydrate.go satisfies this
// interface and is the production implementation.
//
// Step 7 of the bootstrap sequence: runs strictly after Restore (step 6) so
// the marker map is populated, and strictly before Clear (step 8) so the
// daemon's @portal-restoring suppression window still covers the writes.
type EagerHydrateSignaler interface {
	EagerSignalHydrate() error
}

// MarkerCleaner diffs the live `@portal-skeleton-*` server-option marker
// set against the live-pane set and unsets every marker whose paneKey is
// no longer represented by a live pane. Best-effort: a non-nil return is
// logged via Logger.Warn and swallowed by the orchestrator — a
// stale-marker cleanup failure must never block PersistentPreRunE.
//
// The concrete *MarkerCleanupCore in stale_marker_cleanup.go satisfies
// this interface and is the production implementation; cmd/bootstrap_production.go
// constructs it inline at the wiring site.
//
// Step 9 of the bootstrap sequence: runs strictly after Clear (step 8) so
// it observes the post-restore tmux state, and strictly before Sweep
// (step 10) so any stale markers protecting orphan FIFOs are unset first,
// allowing those FIFOs to be reclaimed in the same bootstrap.
type MarkerCleaner interface {
	CleanStaleMarkers() error
}

// FIFOSweeper removes stale hydrate-*.fifo files in the state directory
// whose paneKey is no longer represented by a live `@portal-skeleton-*`
// marker. Best-effort: the implementation MUST swallow per-file failures
// internally and return nil unless the underlying directory enumeration
// itself fails. The orchestrator treats a non-nil err as a soft warning
// and continues — a stuck FIFO must never block PersistentPreRunE.
//
// Step 10 of the bootstrap sequence: runs after CleanStaleMarkers (step 9)
// so any stale markers protecting orphan FIFOs are unset first, but
// before CleanStale (step 11) so the per-pane skeleton markers it
// observes via state.ListSkeletonMarkers are still set on the live tmux
// server.
type FIFOSweeper interface {
	Sweep() error
}

// StaleCleaner prunes stale entries from the on-disk hooks store.
type StaleCleaner interface {
	CleanStale() error
}

// Orchestrator runs the eleven-step bootstrap sequence. Wiring of
// production implementations lives in cmd/root.go (task 5-3); this
// package stays pure (interfaces + Run) so the ordering contract is
// independently testable.
//
// Logger is the sink for failure diagnostics and cycle summaries. Run
// substitutes the shared internal/log discard sink via log.OrDiscard when it
// is nil, so step sites can dispatch unconditionally. Per the spec's cycle-summary cadence,
// each step emits a per-step entering DEBUG breadcrumb (surfaced only at
// PORTAL_LOG_LEVEL=debug) plus, on the non-fatal continuation path, one INFO
// "step complete step=<StepName> took=T" summary; the Return post-step
// boundary emits one INFO "orchestration complete steps=11 warnings=N took=T".
// The closed StepName set (the step* consts) is the single source of truth
// shared by the entering breadcrumb and the step-complete summary so their
// step= attrs always agree. A fatal abort at a fatal step (EnsureServer,
// RegisterPortalHooks, SetRestoring, ClearRestoring) emits neither a
// step-complete for the aborting step nor the orchestration summary — the
// fatal ERROR line is the terminal record. Soft failures emit via Warn; fatal
// failures emit via Error before the orchestrator returns the wrapped
// *FatalError so the same line lands in portal.log under the bootstrap
// component as well as on stderr.
type Orchestrator struct {
	Server        ServerBootstrapper
	Hooks         HookRegistrar
	Restoring     RestoringMarker
	OrphanSweeper OrphanSweeper
	Saver         SaverBootstrapper
	Restore       Restorer
	EagerSignaler EagerHydrateSignaler
	StaleMarkers  MarkerCleaner
	Sweeper       FIFOSweeper
	Clean         StaleCleaner
	Logger        *slog.Logger // nil tolerated; Run substitutes a discard default
}

// Run executes the eleven bootstrap steps in spec order. It returns the
// serverStarted flag from step 1 (EnsureServer) verbatim, the slice of
// soft Warnings accumulated across steps 5-6 (in step order), and any
// fatal error. The ctx parameter is reserved for Phase 6 timeout/cancel
// wiring.
//
// Soft warning paths (do NOT short-circuit Run, do NOT produce fatal err):
//   - Step 4 (SweepOrphanDaemons) returns non-nil → logged via Warn and
//     swallowed. Orphan-sweep failures must never block PersistentPreRunE;
//     the next bootstrap will sweep any survivors.
//   - Step 5 (EnsureSaver) returns non-nil → SaverDownWarning.
//   - Step 6 (Restore) returns corrupt=true → CorruptSessionsJSONWarning;
//     restoreErr is treated as soft and the final return swallows it (per
//     spec, corrupt sessions.json is a non-fatal no-op warning).
//   - Step 6 (Restore) returns (false, err) — a contract violation under
//     the Restorer contract — is treated defensively as soft: logged and
//     swallowed. Step 6 NEVER escalates to a fatal abort, so a future
//     Restorer implementation cannot silently break PersistentPreRunE.
//   - Step 7 (EagerSignalHydrate) returns non-nil → logged via Warn and
//     swallowed. Eager signaling failures must never block
//     PersistentPreRunE; the helper-driven recovery path remains available
//     via the per-pane client-attached hook for the user's attached session.
//   - Step 9 (CleanStaleMarkers) returns non-nil → logged via Warn and
//     swallowed.
//   - Step 10 (Sweep) returns non-nil → logged via Warn and swallowed.
//   - Step 11 (CleanStale) returns non-nil → logged via Warn and swallowed.
func (o *Orchestrator) Run(ctx context.Context) (bool, []Warning, error) {
	_ = ctx // reserved for Phase 6 timeout/cancel

	// Substitute a discard Logger when none was injected so step sites can
	// call o.Logger.Warn / o.Logger.Error unconditionally. Tests that pass
	// nil for the Logger field rely on this default.
	o.Logger = log.OrDiscard(o.Logger)

	// orchestrationStart anchors the took attr on the Return-boundary
	// orchestration-complete summary; per-step starts (stepStart) anchor the
	// took attr on each step-complete summary.
	orchestrationStart := time.Now()

	var warnings []Warning

	// Step 1 — EnsureServer (fatal on failure).
	o.Logger.Debug("step entering", "step", stepEnsureServer)
	stepStart := time.Now()
	serverStarted, err := o.Server.EnsureServer()
	if err != nil {
		// Fatal abort: o.fatalf logs the terminal ERROR line; emit no
		// step-complete for the aborting step and no orchestration summary.
		return false, nil, o.fatalf("start tmux server", err)
	}
	o.Logger.Info("step complete", "step", stepEnsureServer, log.Took(stepStart))

	// Step 2 — RegisterPortalHooks (fatal on failure).
	o.Logger.Debug("step entering", "step", stepRegisterHooks)
	stepStart = time.Now()
	if err := o.Hooks.RegisterPortalHooks(); err != nil {
		return serverStarted, nil, o.fatalf("register tmux hooks", err)
	}
	o.Logger.Info("step complete", "step", stepRegisterHooks, log.Took(stepStart))

	// Step 3 — Set @portal-restoring (MUST precede steps 4 and 5; fatal on failure).
	o.Logger.Debug("step entering", "step", stepSetRestoring)
	stepStart = time.Now()
	if err := o.Restoring.Set(); err != nil {
		return serverStarted, nil, o.fatalf("set @portal-restoring marker", err)
	}
	o.Logger.Info("step complete", "step", stepSetRestoring, log.Took(stepStart))

	// Step 4 — SweepOrphanDaemons (best-effort). Enumerates every live
	// `portal state daemon` process via pgrep, builds the legitimate set
	// from the `_portal-saver` pane's PID, and SIGKILLs every
	// identity-checked candidate that is not in the legitimate set. Runs
	// before EnsureSaver so the new saver-pane daemon's first tick is
	// uncontested by leftover daemons from prior server lifetimes. A
	// non-nil err is logged and swallowed — orphan-sweep failures must
	// never block PersistentPreRunE; the next bootstrap will sweep any
	// survivors.
	o.Logger.Debug("step entering", "step", stepSweepOrphanDaemons)
	stepStart = time.Now()
	if err := o.OrphanSweeper.SweepOrphanDaemons(); err != nil {
		o.Logger.Warn("step failed", "step", stepSweepOrphanDaemons, "error", err)
		// Continue per spec — best-effort sweep, next bootstrap retries.
	}
	o.Logger.Info("step complete", "step", stepSweepOrphanDaemons, log.Took(stepStart))

	// Step 5 — EnsureSaver (best-effort).
	o.Logger.Debug("step entering", "step", stepEnsureSaver)
	stepStart = time.Now()
	if err := o.Saver.EnsureSaver(); err != nil {
		warnings = append(warnings, SaverDownWarning())
		o.Logger.Warn("step failed", "step", stepEnsureSaver, "error", err)
		// Continue per spec — saves paused, user not blocked.
	}
	o.Logger.Info("step complete", "step", stepEnsureSaver, log.Took(stepStart))

	// Step 6 — Restore. The Restorer contract returns (corrupt, err) so
	// the orchestrator can branch on a typed signal rather than walking
	// the error chain. Per the contract, corrupt=true is the only case
	// that produces a non-nil err (wrapped state.ErrCorruptIndex); a
	// (false, err) result is a contract violation and is handled
	// defensively as a soft per-session failure to keep step 6 from
	// escalating to a PersistentPreRunE abort.
	o.Logger.Debug("step entering", "step", stepRestore)
	stepStart = time.Now()
	corrupt, restoreErr := o.Restore.Restore()
	switch {
	case corrupt:
		warnings = append(warnings, CorruptSessionsJSONWarning())
		if restoreErr != nil {
			o.Logger.Warn("step failed: corrupt sessions.json", "step", stepRestore, "error", restoreErr)
		}
	case restoreErr != nil:
		// Defensive: contract says corrupt=false implies err==nil. Log
		// and continue — soft per-session failures must not abort.
		o.Logger.Warn("step returned non-corrupt error (treated as soft per Restorer contract)", "step", stepRestore, "error", restoreErr)
	}
	o.Logger.Info("step complete", "step", stepRestore, log.Took(stepStart))

	// Step 7 — EagerSignalHydrate (best-effort). Runs while
	// @portal-restoring is still set so daemon captureAndCommit
	// suppression remains in force during helper-driven scrollback
	// replay (AC8). Iterates the freshly-set @portal-skeleton-* marker
	// map and writes the hydrate signal byte to each pane's FIFO. A
	// non-nil err is logged and swallowed — eager signaling failures
	// must never block PersistentPreRunE.
	o.Logger.Debug("step entering", "step", stepEagerSignalHydrate)
	stepStart = time.Now()
	if err := o.EagerSignaler.EagerSignalHydrate(); err != nil {
		o.Logger.Warn("step failed", "step", stepEagerSignalHydrate, "error", err)
		// Continue per spec.
	}
	o.Logger.Info("step complete", "step", stepEagerSignalHydrate, log.Took(stepStart))

	// Step 8 — Clear @portal-restoring (fatal on failure).
	o.Logger.Debug("step entering", "step", stepClearRestoring)
	stepStart = time.Now()
	if err := o.Restoring.Clear(); err != nil {
		return serverStarted, warnings, o.fatalf("clear @portal-restoring marker", err)
	}
	o.Logger.Info("step complete", "step", stepClearRestoring, log.Took(stepStart))

	// Step 9 — CleanStaleMarkers (best-effort). Runs strictly after Clear
	// (step 8) so it observes the post-restore tmux state, and strictly
	// before Sweep (step 10) so any stale markers protecting orphan FIFOs
	// are unset first, allowing those FIFOs to be reclaimed in the same
	// bootstrap. A non-nil err is logged and swallowed — a stale-marker
	// cleanup failure must never block PersistentPreRunE.
	o.Logger.Debug("step entering", "step", stepCleanStaleMarkers)
	stepStart = time.Now()
	if err := o.StaleMarkers.CleanStaleMarkers(); err != nil {
		o.Logger.Warn("step failed", "step", stepCleanStaleMarkers, "error", err)
		// Continue per spec.
	}
	o.Logger.Info("step complete", "step", stepCleanStaleMarkers, log.Took(stepStart))

	// Step 10 — SweepOrphanFIFOs (best-effort). Runs after Clear so the
	// daemon's suppression window has closed and after CleanStaleMarkers
	// so any stale markers protecting orphan FIFOs are unset first, but
	// before CleanStale so the per-pane @portal-skeleton-* markers from
	// step 6 are still observable (those outlive @portal-restoring and
	// are cleared per-pane on hydration). A non-nil err is logged and
	// swallowed — a stuck FIFO must never block PersistentPreRunE.
	o.Logger.Debug("step entering", "step", stepSweepOrphanFIFOs)
	stepStart = time.Now()
	if err := o.Sweeper.Sweep(); err != nil {
		o.Logger.Warn("step failed", "step", stepSweepOrphanFIFOs, "error", err)
		// Continue per spec.
	}
	o.Logger.Info("step complete", "step", stepSweepOrphanFIFOs, log.Took(stepStart))

	// Step 11 — CleanStale (best-effort).
	o.Logger.Debug("step entering", "step", stepCleanStale)
	stepStart = time.Now()
	if err := o.Clean.CleanStale(); err != nil {
		o.Logger.Warn("step failed", "step", stepCleanStale, "error", err)
		// Continue per spec.
	}
	o.Logger.Info("step complete", "step", stepCleanStale, log.Took(stepStart))

	// Return — post-step boundary (not numbered). Step 6 never produces a
	// fatal error; warnings already carry the user-facing surface. The
	// orchestration-complete INFO is the cycle summary an operator greps to
	// reconstruct a bootstrap run without scrolling per-step lines.
	o.Logger.Info("orchestration complete", "steps", totalSteps, "warnings", len(warnings), log.Took(orchestrationStart))
	return serverStarted, warnings, nil
}

// fatal logs the user-facing message at ERROR level and returns a
// *FatalError pairing that message with the underlying cause. Centralising
// the construction keeps the log-then-return discipline impossible to drift
// across step sites. Run substitutes a no-op Logger when none was injected,
// so this method need not nil-check.
func (o *Orchestrator) fatal(userMsg string, cause error) error {
	o.Logger.Error(userMsg, "error", cause)
	return NewFatal(userMsg, cause)
}

// fatalf composes the spec-mandated Portal-failed-to-<verb>:-<cause>
// user-facing message in one place, then delegates to fatal. Defining
// the format here makes drift across step sites structurally impossible.
func (o *Orchestrator) fatalf(verb string, err error) error {
	return o.fatal("Portal failed to "+verb+": "+err.Error(), err)
}
