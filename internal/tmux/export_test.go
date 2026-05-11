package tmux

import "time"

// Test-only re-exports of unexported identifiers so the external tmux_test
// package can exercise the kill-barrier helper and swap its seams. Production
// code never references these aliases; their existence is gated by the _test.go
// suffix.

// KillSaverAndWaitForDaemon re-exports killSaverAndWaitForDaemon for tests.
var KillSaverAndWaitForDaemon = killSaverAndWaitForDaemon

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
