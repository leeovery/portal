// Package bootstrap composes the eight-step PersistentPreRunE sequence
// pinned by the resurrection spec. Step ordering is load-bearing:
//
//  1. EnsureServer
//  2. RegisterPortalHooks
//  3. Set @portal-restoring (MUST precede step 4)
//  4. EnsureSaver (best-effort; SaverDownWarning on failure)
//  5. Restore
//  6. Clear @portal-restoring
//  7. CleanStale (best-effort)
//  8. Return
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

// StaleCleaner prunes stale entries from the on-disk hooks store.
type StaleCleaner interface {
	CleanStale() error
}

// Logger is the sink for failure diagnostics. It is internally nil-safe:
// Orchestrator.Run substitutes a no-op default when Logger is unset, so
// callers never need to nil-check before invoking it.
//
// Soft failures (best-effort steps that degrade-and-continue) emit via
// Warn. Fatal failures emit via Error before the orchestrator returns
// the wrapped *FatalError so the same line lands in portal.log via
// ComponentBootstrap as well as on stderr at the top-level Execute path.
type Logger interface {
	Warn(component, format string, args ...any)
	Error(component, format string, args ...any)
}

// noopLogger is the default Logger Run substitutes when Orchestrator.Logger
// is nil. It exists so step sites can call o.Logger.Warn / o.Logger.Error
// unconditionally — trusting the contract uniformly with the rest of the
// codebase, where *state.Logger's nil-receiver no-op is relied on directly.
type noopLogger struct{}

// Warn is a no-op.
func (noopLogger) Warn(component, format string, args ...any) {}

// Error is a no-op.
func (noopLogger) Error(component, format string, args ...any) {}

// Orchestrator runs the eight-step bootstrap sequence. Wiring of
// production implementations lives in cmd/root.go (task 5-3); this
// package stays pure (interfaces + Run) so the ordering contract is
// independently testable.
type Orchestrator struct {
	Server    ServerBootstrapper
	Hooks     HookRegistrar
	Restoring RestoringMarker
	Saver     SaverBootstrapper
	Restore   Restorer
	Clean     StaleCleaner
	Logger    Logger // nil tolerated; Run substitutes a no-op default
}

// Run executes the eight bootstrap steps in spec order. It returns the
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
	serverStarted, err := o.Server.EnsureServer()
	if err != nil {
		return false, nil, o.fatal("Portal failed to start tmux server: "+err.Error(), err)
	}

	// Step 2 — RegisterPortalHooks.
	if err := o.Hooks.RegisterPortalHooks(); err != nil {
		return serverStarted, nil, o.fatal("Portal failed to register tmux hooks: "+err.Error(), err)
	}

	// Step 3 — Set @portal-restoring (MUST precede step 4).
	if err := o.Restoring.Set(); err != nil {
		return serverStarted, nil, o.fatal("Portal failed to set @portal-restoring marker: "+err.Error(), err)
	}

	// Step 4 — EnsureSaver (best-effort).
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
	if err := o.Restoring.Clear(); err != nil {
		return serverStarted, warnings, o.fatal("Portal failed to clear @portal-restoring marker: "+err.Error(), err)
	}

	// Step 7 — CleanStale (best-effort).
	if err := o.Clean.CleanStale(); err != nil {
		o.Logger.Warn(state.ComponentBootstrap, "step 7 (CleanStale) failed: %v", err)
		// Continue per spec.
	}

	// Step 8 — Return. Step 5 never produces a fatal error; warnings
	// already carry the user-facing surface.
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
