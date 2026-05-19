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

// ShouldKillSaverOnVersionDecision re-exports the
// shouldKillSaverOnVersionDecision predicate so the external tmux_test
// package can drive its truth matrix directly. The predicate encodes the
// alive-daemon kill-decision rules consulted by EnsurePortalSaverVersion;
// the alive-check is consulted first in the caller. See
// TestShouldKillSaverOnVersionDecision_PredicateMatrix.
var ShouldKillSaverOnVersionDecision = shouldKillSaverOnVersionDecision

// BarrierReadPIDSeam returns a pointer to the killBarrierReadPID seam so tests
// can swap and restore it via t.Cleanup.
func BarrierReadPIDSeam() *func(string) (int, error) { return &killBarrierReadPID }

// BarrierIsAliveSeam returns a pointer to the killBarrierIsAlive seam.
func BarrierIsAliveSeam() *func(int) bool { return &killBarrierIsAlive }

// BarrierPollIntervalSeam returns a pointer to the killBarrierPollInterval seam.
func BarrierPollIntervalSeam() *time.Duration { return &killBarrierPollInterval }

// BarrierTimeoutSeam returns a pointer to the killBarrierTimeout seam.
func BarrierTimeoutSeam() *time.Duration { return &killBarrierTimeout }

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
