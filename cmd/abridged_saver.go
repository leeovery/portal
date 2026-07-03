package cmd

import (
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
)

// ensureSaverLiveness is the abridged-path, liveness-only EnsureSaver (Class 2).
//
// On an already-bootstrapped ("warm") server the full 10-step orchestrator is
// skipped, but the _portal-saver daemon can still die mid-lifetime — its own
// self-supervision may os.Exit(0), tearing down its pane and killing the
// _portal-saver session — so every warm command must re-probe and revive it.
// This helper does exactly that and nothing more: it probes _portal-saver
// presence and, when absent, re-ensures it via the idempotent
// BootstrapPortalSaver primitive.
//
// It deliberately does NOT call tmux.EnsurePortalSaverVersion. Contrast with
// saverAdapter.EnsureSaver (cmd/bootstrap_production.go), the FULL-bootstrap
// step, which retains the version-gate: it kills and recreates a stale-binary
// daemon via a guarded kill-barrier. On the abridged path a satisfied
// @portal-bootstrapped latch already proves the running daemon is the current
// binary, so the version re-check is redundant and its kill-barrier is a
// needless concurrency hazard under a reopen burst. The version-gate therefore
// lives solely in the full-bootstrap orchestrator step. This helper must never
// run a kill-barrier of its own.
//
// Failure posture (spec § Abridged EnsureSaver hard-failure): a revive failure
// is soft. It funnels a bootstrap.SaverDownWarning into the same package-level
// bootstrapWarnings sink the sync path uses (CLI flushes it to stderr; the TUI
// drains it to the notice band) and the command PROCEEDS — attach/switch still
// works and capture resumes on the next successful revival. There is no error
// return: all failure surfaces through the warning sink. Additionally, before
// funneling that user-facing warning it emits one bootstrap-component WARN
// carrying the underlying cause (via the package-level bootstrapLogger),
// mirroring the full-bootstrap step-5 "step failed" breadcrumb so portal.log
// records why the revive failed — the SaverDownWarning itself is causeless.
//
// stateDir is a parameter (rather than resolved internally) so the caller owns
// resolution and the helper stays unit-testable; the production caller passes
// state.Dir().
func ensureSaverLiveness(client *tmux.Client, stateDir string) {
	// Cheap presence probe. present && err == nil is the only "alive" shape;
	// an absent saver OR a transient probe error both fold into "needs revive"
	// (spec: treat any probe error as absent and attempt a revive).
	if _, present, err := tmux.SaverPanePIDOrAbsent(client, tmux.PortalSaverName); present && err == nil {
		return
	}

	// Absent (or unprovable) — re-ensure idempotently. A revive failure is
	// non-fatal: log the underlying cause, record the soft warning, and let the
	// command proceed. The WARN mirrors the full-bootstrap step-5 "step failed"
	// breadcrumb (cmd/bootstrap/bootstrap.go) so the abridged path retains
	// diagnosability parity — the SaverDownWarning alone carries no cause.
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		bootstrapLogger.Warn("abridged EnsureSaver: saver revive failed", "error", err)
		bootstrapWarnings.Add(bootstrap.SaverDownWarning())
	}
}
