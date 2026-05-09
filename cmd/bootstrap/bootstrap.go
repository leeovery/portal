// Package bootstrap composes the nine-step PersistentPreRunE sequence
// pinned by the resurrection spec. Step ordering is load-bearing;
// "Return" is the post-step boundary, not a numbered step:
//
//  1. EnsureServer
//  2. RegisterPortalHooks
//  3. Set @portal-restoring (MUST precede step 4)
//  4. EnsureSaver (best-effort; SaverDownWarning on failure)
//  5. Restore
//  6. Clear @portal-restoring
//  7. CleanStaleMarkers (best-effort; diffs `@portal-skeleton-*` markers
//     against the live-pane set and unsets markers whose paneKey is no
//     longer represented by a live pane — runs after Clear so it observes
//     post-restore tmux state, and before Sweep so any stale markers
//     protecting orphan FIFOs are unset first, allowing those FIFOs to be
//     reclaimed in the same bootstrap)
//  8. SweepOrphanFIFOs (best-effort; observes still-set per-pane
//     @portal-skeleton-* markers from step 5 — those outlive
//     @portal-restoring and are cleared per-pane on hydration)
//  9. CleanStale (best-effort)
//
// Return is the post-step boundary that collects accumulated warnings.
package bootstrap

import (
	"context"

	"github.com/leeovery/portal/internal/state"
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
// The bool exists so Orchestrator step 5 can branch on a typed signal
// rather than a string-equality check on the error chain. A future
// implementation that violates the contract by returning (false, err)
// is treated defensively by Run: the err is logged and the orchestrator
// continues without escalating to a PersistentPreRunE abort. This guards
// the "degrade locally, log, continue" principle against silent drift.
type Restorer interface {
	Restore() (corrupt bool, err error)
}

// MarkerCleaner diffs the live `@portal-skeleton-*` server-option marker
// set against the live-pane set and unsets every marker whose paneKey is
// no longer represented by a live pane. Best-effort: a non-nil return is
// logged via Logger.Warn and swallowed by the orchestrator — a
// stale-marker cleanup failure must never block PersistentPreRunE.
//
// The concrete *StaleMarkerCleaner in stale_marker_cleanup.go satisfies
// this interface; the interface name is distinct from the concrete type
// only to avoid an identifier collision within the same package.
//
// Step 7 of the bootstrap sequence: runs strictly after Clear (step 6)
// so it observes the post-restore tmux state, and strictly before Sweep
// (step 8) so any stale markers protecting orphan FIFOs are unset first,
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
// Step 8 of the bootstrap sequence: runs after CleanStaleMarkers (step 7)
// so any stale markers protecting orphan FIFOs are unset first, but
// before CleanStale (step 9) so the per-pane skeleton markers it
// observes via state.ListSkeletonMarkers are still set on the live tmux
// server.
type FIFOSweeper interface {
	Sweep() error
}

// StaleCleaner prunes stale entries from the on-disk hooks store.
type StaleCleaner interface {
	CleanStale() error
}

// Logger is the sink for failure diagnostics. It is internally nil-safe:
// Orchestrator.Run substitutes a no-op default when Logger is unset, so
// callers never need to nil-check before invoking it.
//
// Step-entry diagnostics emit via Debug — per the spec's Observability
// section, bootstrap events are logged at DEBUG so PORTAL_LOG_LEVEL=debug
// surfaces the step boundary an operator scans when a session fails to come
// back; production runs (default LevelWarn) drop these lines. Soft failures
// (best-effort steps that degrade-and-continue) emit via Warn. Fatal
// failures emit via Error before the orchestrator returns the wrapped
// *FatalError so the same line lands in portal.log via ComponentBootstrap
// as well as on stderr at the top-level Execute path.
type Logger interface {
	Debug(component, format string, args ...any)
	Warn(component, format string, args ...any)
	Error(component, format string, args ...any)
}

// noopLogger is the default Logger Run substitutes when Orchestrator.Logger
// is nil. It exists so step sites can call o.Logger.Debug / o.Logger.Warn /
// o.Logger.Error unconditionally — trusting the contract uniformly with the
// rest of the codebase, where *state.Logger's nil-receiver no-op is relied
// on directly.
type noopLogger struct{}

// Debug is a no-op.
func (noopLogger) Debug(component, format string, args ...any) {}

// Warn is a no-op.
func (noopLogger) Warn(component, format string, args ...any) {}

// Error is a no-op.
func (noopLogger) Error(component, format string, args ...any) {}

// Orchestrator runs the nine-step bootstrap sequence. Wiring of
// production implementations lives in cmd/root.go (task 5-3); this
// package stays pure (interfaces + Run) so the ordering contract is
// independently testable.
type Orchestrator struct {
	Server       ServerBootstrapper
	Hooks        HookRegistrar
	Restoring    RestoringMarker
	Saver        SaverBootstrapper
	Restore      Restorer
	StaleMarkers MarkerCleaner
	Sweeper      FIFOSweeper
	Clean        StaleCleaner
	Logger       Logger // nil tolerated; Run substitutes a no-op default
}

// Run executes the nine bootstrap steps in spec order. It returns the
// serverStarted flag from step 1 (EnsureServer) verbatim, the slice of
// soft Warnings accumulated across steps 4-5 (in step order), and any
// fatal error. The ctx parameter is reserved for Phase 6 timeout/cancel
// wiring.
//
// Soft warning paths (do NOT short-circuit Run, do NOT produce fatal err):
//   - Step 4 (EnsureSaver) returns non-nil → SaverDownWarning.
//   - Step 5 (Restore) returns corrupt=true → CorruptSessionsJSONWarning;
//     restoreErr is treated as soft and the final return swallows it (per
//     spec, corrupt sessions.json is a non-fatal no-op warning).
//   - Step 5 (Restore) returns (false, err) — a contract violation under
//     the Restorer contract — is treated defensively as soft: logged and
//     swallowed. Step 5 NEVER escalates to a fatal abort, so a future
//     Restorer implementation cannot silently break PersistentPreRunE.
//   - Step 7 (CleanStaleMarkers) returns non-nil → logged via Warn and
//     swallowed.
//   - Step 8 (Sweep) returns non-nil → logged via Warn and swallowed.
//   - Step 9 (CleanStale) returns non-nil → logged via Warn and swallowed.
func (o *Orchestrator) Run(ctx context.Context) (bool, []Warning, error) {
	_ = ctx // reserved for Phase 6 timeout/cancel

	// Substitute a no-op Logger when none was injected so step sites can
	// call o.Logger.Warn / o.Logger.Error unconditionally. Tests that pass
	// nil for the Logger field rely on this default.
	if o.Logger == nil {
		o.Logger = noopLogger{}
	}

	var warnings []Warning

	// Step 1 — EnsureServer.
	o.Logger.Debug(state.ComponentBootstrap, "step 1 (EnsureServer): entering")
	serverStarted, err := o.Server.EnsureServer()
	if err != nil {
		return false, nil, o.fatalf("start tmux server", err)
	}

	// Step 2 — RegisterPortalHooks.
	o.Logger.Debug(state.ComponentBootstrap, "step 2 (RegisterPortalHooks): entering")
	if err := o.Hooks.RegisterPortalHooks(); err != nil {
		return serverStarted, nil, o.fatalf("register tmux hooks", err)
	}

	// Step 3 — Set @portal-restoring (MUST precede step 4).
	o.Logger.Debug(state.ComponentBootstrap, "step 3 (Set @portal-restoring): entering")
	if err := o.Restoring.Set(); err != nil {
		return serverStarted, nil, o.fatalf("set @portal-restoring marker", err)
	}

	// Step 4 — EnsureSaver (best-effort).
	o.Logger.Debug(state.ComponentBootstrap, "step 4 (EnsureSaver): entering")
	if err := o.Saver.EnsureSaver(); err != nil {
		warnings = append(warnings, SaverDownWarning())
		o.Logger.Warn(state.ComponentBootstrap, "step 4 (EnsureSaver) failed: %v", err)
		// Continue per spec — saves paused, user not blocked.
	}

	// Step 5 — Restore. The Restorer contract returns (corrupt, err) so
	// the orchestrator can branch on a typed signal rather than walking
	// the error chain. Per the contract, corrupt=true is the only case
	// that produces a non-nil err (wrapped state.ErrCorruptIndex); a
	// (false, err) result is a contract violation and is handled
	// defensively as a soft per-session failure to keep step 5 from
	// escalating to a PersistentPreRunE abort.
	o.Logger.Debug(state.ComponentBootstrap, "step 5 (Restore): entering")
	corrupt, restoreErr := o.Restore.Restore()
	switch {
	case corrupt:
		warnings = append(warnings, CorruptSessionsJSONWarning())
		if restoreErr != nil {
			o.Logger.Warn(state.ComponentBootstrap, "step 5 (Restore) corrupt sessions.json: %v", restoreErr)
		}
	case restoreErr != nil:
		// Defensive: contract says corrupt=false implies err==nil. Log
		// and continue — soft per-session failures must not abort.
		o.Logger.Warn(state.ComponentBootstrap, "step 5 (Restore) returned non-corrupt error (treated as soft per Restorer contract): %v", restoreErr)
	}

	// Step 6 — Clear @portal-restoring (fatal on failure).
	o.Logger.Debug(state.ComponentBootstrap, "step 6 (Clear @portal-restoring): entering")
	if err := o.Restoring.Clear(); err != nil {
		return serverStarted, warnings, o.fatalf("clear @portal-restoring marker", err)
	}

	// Step 7 — CleanStaleMarkers (best-effort). Runs strictly after Clear
	// (step 6) so it observes the post-restore tmux state, and strictly
	// before Sweep (step 8) so any stale markers protecting orphan FIFOs
	// are unset first, allowing those FIFOs to be reclaimed in the same
	// bootstrap. A non-nil err is logged and swallowed — a stale-marker
	// cleanup failure must never block PersistentPreRunE.
	o.Logger.Debug(state.ComponentBootstrap, "step 7 (CleanStaleMarkers): entering")
	if err := o.StaleMarkers.CleanStaleMarkers(); err != nil {
		o.Logger.Warn(state.ComponentBootstrap, "step 7 (CleanStaleMarkers) failed: %v", err)
		// Continue per spec.
	}

	// Step 8 — SweepOrphanFIFOs (best-effort). Runs after Clear so the
	// daemon's suppression window has closed and after CleanStaleMarkers
	// so any stale markers protecting orphan FIFOs are unset first, but
	// before CleanStale so the per-pane @portal-skeleton-* markers from
	// step 5 are still observable (those outlive @portal-restoring and
	// are cleared per-pane on hydration). A non-nil err is logged and
	// swallowed — a stuck FIFO must never block PersistentPreRunE.
	o.Logger.Debug(state.ComponentBootstrap, "step 8 (SweepOrphanFIFOs): entering")
	if err := o.Sweeper.Sweep(); err != nil {
		o.Logger.Warn(state.ComponentBootstrap, "step 8 (SweepOrphanFIFOs) failed: %v", err)
		// Continue per spec.
	}

	// Step 9 — CleanStale (best-effort).
	o.Logger.Debug(state.ComponentBootstrap, "step 9 (CleanStale): entering")
	if err := o.Clean.CleanStale(); err != nil {
		o.Logger.Warn(state.ComponentBootstrap, "step 9 (CleanStale) failed: %v", err)
		// Continue per spec.
	}

	// Return — post-step boundary (not numbered). Step 5 never produces a
	// fatal error; warnings already carry the user-facing surface.
	o.Logger.Debug(state.ComponentBootstrap, "Return: exiting with %d warning(s)", len(warnings))
	return serverStarted, warnings, nil
}

// fatal logs the user-facing message at ERROR level and returns a
// *FatalError pairing that message with the underlying cause. Centralising
// the construction keeps the log-then-return discipline impossible to drift
// across step sites. Run substitutes a no-op Logger when none was injected,
// so this method need not nil-check.
func (o *Orchestrator) fatal(userMsg string, cause error) error {
	o.Logger.Error(state.ComponentBootstrap, "%s", userMsg)
	return NewFatal(userMsg, cause)
}

// fatalf composes the spec-mandated Portal-failed-to-<verb>:-<cause>
// user-facing message in one place, then delegates to fatal. Defining
// the format here makes drift across step sites structurally impossible.
func (o *Orchestrator) fatalf(verb string, err error) error {
	return o.fatal("Portal failed to "+verb+": "+err.Error(), err)
}
