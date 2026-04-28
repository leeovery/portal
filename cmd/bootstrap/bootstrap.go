// Package bootstrap composes the eight-step PersistentPreRunE sequence
// pinned by the resurrection spec. Step ordering is load-bearing:
//
//  1. EnsureServer
//  2. RegisterPortalHooks
//  3. Set @portal-restoring (MUST precede step 4)
//  4. EnsureSaver (best-effort; SaverDownError on failure)
//  5. Restore
//  6. Clear @portal-restoring
//  7. CleanStale (best-effort)
//  8. Return
package bootstrap

import (
	"context"
	"fmt"

	"github.com/leeovery/portal/internal/state"
)

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
type Restorer interface {
	Restore() error
}

// StaleCleaner prunes stale entries from the on-disk hooks store.
type StaleCleaner interface {
	CleanStale() error
}

// Logger is an optional sink for failure diagnostics. It is nil-safe:
// Orchestrator.Run checks for nil before invoking it.
//
// Soft failures (best-effort steps that degrade-and-continue) emit via
// Warn. Fatal failures emit via Error before the orchestrator returns
// the wrapped *FatalError so the same line lands in portal.log via
// ComponentBootstrap as well as on stderr at the top-level Execute path.
type Logger interface {
	Warn(component, format string, args ...any)
	Error(component, format string, args ...any)
}

// SaverDownError indicates that step 4 (EnsureSaver) failed. Bootstrap
// continues past it — saves are paused, but the user is not blocked. The
// concrete value is recorded on Orchestrator.LastSaverErr after Run.
type SaverDownError struct {
	Cause error
}

// Error returns a human-readable message describing the saver failure.
func (e *SaverDownError) Error() string {
	return fmt.Sprintf("portal save daemon failed to start: %v", e.Cause)
}

// Unwrap exposes the underlying cause for errors.Is / errors.As.
func (e *SaverDownError) Unwrap() error { return e.Cause }

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
	Logger    Logger // optional; nil-safe

	// LastSaverErr is populated when step 4 (EnsureSaver) fails. Run
	// continues past the failure (per spec: saves paused, user not
	// blocked); the caller can read this field afterwards to decide
	// whether to surface a SaverDownError to observability.
	LastSaverErr error
}

// Run executes the eight bootstrap steps in spec order. It returns the
// serverStarted flag from step 1 (EnsureServer) verbatim. The ctx
// parameter is reserved for Phase 6 timeout/cancel wiring.
func (o *Orchestrator) Run(ctx context.Context) (bool, error) {
	_ = ctx // reserved for Phase 6 timeout/cancel

	// Step 1 — EnsureServer.
	serverStarted, err := o.Server.EnsureServer()
	if err != nil {
		return false, o.fatal("Portal failed to start tmux server: "+err.Error(), err)
	}

	// Step 2 — RegisterPortalHooks.
	if err := o.Hooks.RegisterPortalHooks(); err != nil {
		return serverStarted, o.fatal("Portal failed to register tmux hooks: "+err.Error(), err)
	}

	// Step 3 — Set @portal-restoring (MUST precede step 4).
	if err := o.Restoring.Set(); err != nil {
		return serverStarted, o.fatal("Portal failed to set @portal-restoring marker: "+err.Error(), err)
	}

	// Step 4 — EnsureSaver (best-effort).
	if err := o.Saver.EnsureSaver(); err != nil {
		o.LastSaverErr = &SaverDownError{Cause: err}
		if o.Logger != nil {
			o.Logger.Warn(state.ComponentBootstrap, "step 4 (EnsureSaver) failed: %v", err)
		}
		// Continue per spec — saves paused, user not blocked.
	}

	// Step 5 — Restore. Capture err but defer return until after step 6.
	restoreErr := o.Restore.Restore()

	// Step 6 — Clear @portal-restoring (fatal on failure).
	if err := o.Restoring.Clear(); err != nil {
		return serverStarted, o.fatal("Portal failed to clear @portal-restoring marker: "+err.Error(), err)
	}

	// Step 7 — CleanStale (best-effort).
	if err := o.Clean.CleanStale(); err != nil {
		if o.Logger != nil {
			o.Logger.Warn(state.ComponentBootstrap, "step 7 (CleanStale) failed: %v", err)
		}
		// Continue per spec.
	}

	// Step 8 — Return. Surface step-5 error if any; otherwise nil.
	if restoreErr != nil {
		return serverStarted, fmt.Errorf("step 5 (Restore): %w", restoreErr)
	}
	return serverStarted, nil
}

// fatal logs the user-facing message at ERROR level (when a Logger is
// configured) and returns a *FatalError pairing that message with the
// underlying cause. Centralising the construction keeps the
// log-then-return discipline impossible to drift across step sites.
func (o *Orchestrator) fatal(userMsg string, cause error) error {
	if o.Logger != nil {
		o.Logger.Error(state.ComponentBootstrap, "%s", userMsg)
	}
	return NewFatal(userMsg, cause)
}
