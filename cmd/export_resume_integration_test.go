//go:build integration

package cmd

import (
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/backend"
)

// ParkedResumeChain is the shell text the hydrate helper parks a waiting
// session under, for the integration lane's sessions to run as restore leaves
// one.
func ParkedResumeChain(exe, flag, command, session string) string {
	return parkedResumeChain(parkedChain{Exe: exe, Flag: flag}, resumeChainPayload{Command: command, Session: session})
}

// RepaintWaiting runs the repaint an attach asks of session name over b, its
// one signal sent through signal in place of kill(2).
func RepaintWaiting(t *testing.T, b backend.Backend, name string, signal func(pid int, sig syscall.Signal) error) {
	t.Helper()
	withFuncSeam(t, &repaintSignal, signal)
	repaintWaiting(t.Context(), b, name)
}
