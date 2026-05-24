package tmux

import (
	"time"

	"github.com/leeovery/portal/internal/state"
)

// Test-only re-exports of unexported identifiers so the external tmux_test
// package can exercise the kill-barrier helper and swap its seams. Production
// code never references these aliases; their existence is gated by the _test.go
// suffix.
//
// The package collapses every saver-side mutable seam into a single SaverSeams
// struct (saver). Two accessor shapes are exported for tests:
//
//   - Struct-pointer accessors (e.g. SaverBarrier()) return a pointer to the
//     relevant sub-struct (or the parent for shared fields) so tests that
//     need to swap a whole cluster can do so atomically.
//   - Per-field *Seam() accessors return pointers into the same backing
//     fields so the existing swapSeam helper continues to work field-by-
//     field without test changes.
//
// Both shapes alias the same backing fields — pick whichever fits the
// test's call site.

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

// WaitForSaverDaemonReady re-exports waitForSaverDaemonReady for tests that
// exercise the readiness barrier directly.
var WaitForSaverDaemonReady = waitForSaverDaemonReady

// ---------------------------------------------------------------------------
// Struct-pointer accessors — return pointers into the unified saver instance
// so tests can swap whole clusters atomically.
// ---------------------------------------------------------------------------

// Saver returns a pointer to the unified SaverSeams instance backing every
// saver-side seam (shared PID-read + identity primitives at the top level,
// plus the Barrier / Readiness / Version / Ops sub-clusters).
func Saver() *SaverSeams { return &saver }

// SaverBarrier returns a pointer to the Barrier sub-cluster backing
// killSaverAndWaitForDaemon's poll loop and escalation path, plus the shared
// WARN-emission Logger sink consumed by waitForSaverDaemonReady.
func SaverBarrier() *SaverBarrierSeams { return &saver.Barrier }

// SaverReadiness returns a pointer to the Readiness sub-cluster backing
// waitForSaverDaemonReady's poll loop.
func SaverReadiness() *SaverReadinessSeams { return &saver.Readiness }

// SaverVersion returns a pointer to the Version sub-cluster backing
// EnsurePortalSaverVersion's read/write primitives and the bootstrap-side
// defensive-write logger.
func SaverVersion() *SaverVersionSeams { return &saver.Version }

// SaverOps returns a pointer to the Ops sub-cluster backing the two
// operation-level function seams that callers route through to substitute
// the entire kill-and-wait or readiness-wait flows.
func SaverOps() *SaverOperationSeams { return &saver.Ops }

// ---------------------------------------------------------------------------
// Per-field *Seam() accessors — return pointers into the same backing
// fields so the existing swapSeam helper continues to work field-by-field.
// ---------------------------------------------------------------------------

// SaverReadPIDSeam returns a pointer to the shared ReadPID seam. The seam
// is shared between the kill barrier (priorPID read) and the readiness
// barrier (poll-for-PID-file).
func SaverReadPIDSeam() *func(string) (int, error) { return &saver.ReadPID }

// SaverIdentifyDaemonSeam returns a pointer to the shared IdentifyDaemon
// seam so tests can deterministically drive identity-check outcomes without
// shelling out to ps. The seam is shared between the kill barrier's
// escalation path (pre-SIGKILL identity check) and the readiness barrier
// (post-respawn classification of daemon.pid).
func SaverIdentifyDaemonSeam() *func(int) (state.IdentifyResult, error) {
	return &saver.IdentifyDaemon
}

// BarrierIsAliveSeam returns a pointer to the kill-barrier IsAlive seam.
func BarrierIsAliveSeam() *func(int) bool { return &saver.Barrier.IsAlive }

// BarrierPollIntervalSeam returns a pointer to the kill-barrier
// PollInterval seam.
func BarrierPollIntervalSeam() *time.Duration { return &saver.Barrier.PollInterval }

// BarrierTimeoutSeam returns a pointer to the kill-barrier Timeout seam.
func BarrierTimeoutSeam() *time.Duration { return &saver.Barrier.Timeout }

// BarrierEscalationTimeoutSeam returns a pointer to the kill-barrier
// EscalationTimeout seam so escalation-path tests can shrink the
// post-SIGKILL poll budget.
func BarrierEscalationTimeoutSeam() *time.Duration { return &saver.Barrier.EscalationTimeout }

// BarrierSendSIGKILLSeam returns a pointer to the kill-barrier SendSIGKILL
// seam so escalation-path tests can record invocations and inject errors
// without signalling real processes.
func BarrierSendSIGKILLSeam() *func(int) error { return &saver.Barrier.SendSIGKILL }

// BarrierLoggerSeam returns a pointer to the shared saver-barrier Logger
// seam so tests can install a recording fake satisfying the BarrierLogger
// interface. The Logger sink is consumed by BOTH the kill-barrier
// WARN-on-timeout / escalation paths AND the readiness-barrier
// WARN-on-timeout path (waitForSaverDaemonReady).
func BarrierLoggerSeam() *BarrierLogger { return &saver.Barrier.Logger }

// SaverReadinessPollIntervalSeam returns a pointer to the readiness-barrier
// PollInterval seam so tests can shrink the poll cadence to keep
// wall-clock bounded.
func SaverReadinessPollIntervalSeam() *time.Duration {
	return &saver.Readiness.PollInterval
}

// SaverReadinessTimeoutSeam returns a pointer to the readiness-barrier
// Timeout seam so tests can shrink the total readiness budget to keep
// wall-clock bounded.
func SaverReadinessTimeoutSeam() *time.Duration {
	return &saver.Readiness.Timeout
}

// PortalSaverReadVersionFileSeam returns a pointer to the version
// ReadVersionFile seam so tests can simulate version-file read behaviour
// (including non-absent I/O errors) without touching the filesystem.
func PortalSaverReadVersionFileSeam() *func(string) (string, error) {
	return &saver.Version.ReadVersionFile
}

// PortalSaverWriteVersionFileSeam returns a pointer to the version
// WriteVersionFile seam so tests can record invocations and inject errors
// for the defensive alive+absent write performed by EnsurePortalSaverVersion
// before BootstrapPortalSaver.
func PortalSaverWriteVersionFileSeam() *func(string, string) error {
	return &saver.Version.WriteVersionFile
}

// VersionWriterLoggerSeam returns a pointer to the version WriterLogger
// sink so tests can install a capturing *state.Logger via
// SetVersionWriterLogger and restore the prior value via t.Cleanup.
func VersionWriterLoggerSeam() **state.Logger { return &saver.Version.WriterLogger }

// WaitForSaverDaemonReadyFnSeam returns a pointer to the operation-level
// WaitForReady seam so create-branch tests can stub the readiness barrier
// with a no-op without exercising the full poll flow.
func WaitForSaverDaemonReadyFnSeam() *func(string) error {
	return &saver.Ops.WaitForReady
}

// KillSaverAndWaitForDaemonFnSeam returns a pointer to the operation-level
// KillAndWait seam so tests can stub the helper invoked from the production
// call sites without exercising the full barrier flow.
func KillSaverAndWaitForDaemonFnSeam() *func(*Client, string) error {
	return &saver.Ops.KillAndWait
}
