package tmux

import (
	"time"

	"github.com/leeovery/portal/internal/state"
)

// Test-only re-exports of unexported identifiers so the external tmux_test
// package can exercise the kill-barrier helper and swap its seams. Production
// code never references these aliases; their existence is gated by the _test.go
// suffix.

// KillSaverAndWaitForDaemon re-exports killSaverAndWaitForDaemon for tests.
var KillSaverAndWaitForDaemon = killSaverAndWaitForDaemon

// PortalSaverPlaceholderCommand re-exports portalSaverPlaceholderCommand so
// the external tmux_test package can pin its literal value. The placeholder
// is the create-time pane process used in Component F before
// destroy-unattached=off has been applied; see portal_saver.go for the full
// rationale (macOS BSD sleep rejects "infinity", placeholder cannot contend
// for the daemon lock, etc.).
const PortalSaverPlaceholderCommand = portalSaverPlaceholderCommand

// PortalSaverDaemonCommand re-exports portalSaverDaemonCommand so the
// external tmux_test package can pin its literal value. This is the real
// saver pane process installed by `respawn-pane -k` once
// destroy-unattached=off is in effect on the session.
const PortalSaverDaemonCommand = portalSaverDaemonCommand

// ShouldKillSaverOnVersionDecision re-exports the
// shouldKillSaverOnVersionDecision predicate so the external tmux_test
// package can drive its truth matrix directly. The predicate encodes the
// alive-daemon kill-decision rules consulted by EnsurePortalSaverVersion;
// the alive-check is consulted first in the caller. See
// TestShouldKillSaverOnVersionDecision_PredicateMatrix.
var ShouldKillSaverOnVersionDecision = shouldKillSaverOnVersionDecision

// SaverReadPIDSeam returns a pointer to the saverReadPID seam so tests can
// swap and restore it via t.Cleanup. The seam is shared between the kill
// barrier (priorPID read) and the readiness barrier (poll-for-PID-file).
func SaverReadPIDSeam() *func(string) (int, error) { return &saverReadPID }

// BarrierIsAliveSeam returns a pointer to the killBarrierIsAlive seam.
func BarrierIsAliveSeam() *func(int) bool { return &killBarrierIsAlive }

// BarrierPollIntervalSeam returns a pointer to the killBarrierPollInterval seam.
func BarrierPollIntervalSeam() *time.Duration { return &killBarrierPollInterval }

// BarrierTimeoutSeam returns a pointer to the killBarrierTimeout seam.
func BarrierTimeoutSeam() *time.Duration { return &killBarrierTimeout }

// BarrierEscalationTimeoutSeam returns a pointer to the
// killBarrierEscalationTimeout seam so escalation-path tests can shrink the
// post-SIGKILL poll budget.
func BarrierEscalationTimeoutSeam() *time.Duration { return &killBarrierEscalationTimeout }

// SaverIdentifyDaemonSeam returns a pointer to the saverIdentifyDaemon seam
// so tests can deterministically drive identity-check outcomes without
// shelling out to ps. The seam is shared between the kill barrier's
// escalation path (pre-SIGKILL identity check) and the readiness barrier
// (post-respawn classification of daemon.pid).
func SaverIdentifyDaemonSeam() *func(int) (state.IdentifyResult, error) {
	return &saverIdentifyDaemon
}

// BarrierSendSIGKILLSeam returns a pointer to the killBarrierSendSIGKILL seam
// so escalation-path tests can record invocations and inject errors without
// signalling real processes.
func BarrierSendSIGKILLSeam() *func(int) error { return &killBarrierSendSIGKILL }

// BarrierLoggerSeam returns a pointer to the killBarrierLogger seam so tests
// can install a recording fake satisfying the BarrierLogger interface.
func BarrierLoggerSeam() *BarrierLogger { return &killBarrierLogger }

// KillSaverAndWaitForDaemonFnSeam returns a pointer to the
// killSaverAndWaitForDaemonFn seam so tests can stub the helper invoked from
// the production call sites without exercising the full barrier flow.
func KillSaverAndWaitForDaemonFnSeam() *func(*Client, string) error {
	return &killSaverAndWaitForDaemonFn
}

// PortalSaverReadVersionFileSeam returns a pointer to the
// portalSaverReadVersionFile seam so tests can simulate version-file read
// behaviour (including non-absent I/O errors) without touching the filesystem.
func PortalSaverReadVersionFileSeam() *func(string) (string, error) {
	return &portalSaverReadVersionFile
}

// PortalSaverWriteVersionFileSeam returns a pointer to the
// portalSaverWriteVersionFile seam so tests can record invocations and inject
// errors for the defensive alive+absent write performed by
// EnsurePortalSaverVersion before BootstrapPortalSaver.
func PortalSaverWriteVersionFileSeam() *func(string, string) error {
	return &portalSaverWriteVersionFile
}

// VersionWriterLoggerSeam returns a pointer to the versionWriterLogger
// package-level sink so tests can install a capturing *state.Logger via
// SetVersionWriterLogger and restore the prior value via t.Cleanup.
func VersionWriterLoggerSeam() **state.Logger { return &versionWriterLogger }

// WaitForSaverDaemonReady re-exports waitForSaverDaemonReady for tests that
// exercise the readiness barrier directly.
var WaitForSaverDaemonReady = waitForSaverDaemonReady

// SaverReadinessPollIntervalSeam returns a pointer to the
// saverReadinessPollInterval seam so tests can shrink the poll cadence to
// keep wall-clock bounded.
func SaverReadinessPollIntervalSeam() *time.Duration {
	return &saverReadinessPollInterval
}

// SaverReadinessTimeoutSeam returns a pointer to the saverReadinessTimeout
// seam so tests can shrink the total readiness budget to keep wall-clock
// bounded.
func SaverReadinessTimeoutSeam() *time.Duration {
	return &saverReadinessTimeout
}

// WaitForSaverDaemonReadyFnSeam returns a pointer to the
// waitForSaverDaemonReadyFn seam so create-branch tests can stub the
// readiness barrier with a no-op without exercising the full poll flow.
func WaitForSaverDaemonReadyFnSeam() *func(string) error {
	return &waitForSaverDaemonReadyFn
}
