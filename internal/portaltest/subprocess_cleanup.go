package portaltest

import (
	"os/exec"
	"testing"
)

// RegisterSubprocessCleanup arranges a guaranteed SIGKILL and reap for cmd on
// test exit, returning a channel that closes once it has been reaped so a caller
// can time its death. The reaper goroutine and the cleanup hook coordinate
// through that channel rather than both calling Wait, which would be two
// concurrent Waits on one Process. An unreaped child stays kernel-resident after
// SIGKILL — kill(pid, 0) keeps returning 0 — which would hang any ESRCH poll.
func RegisterSubprocessCleanup(t *testing.T, cmd *exec.Cmd) <-chan struct{} {
	t.Helper()
	reaped := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(reaped)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-reaped
	})
	return reaped
}
