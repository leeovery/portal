//go:build integration

package cmd

import (
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/saverhost"
)

// ErrSignalsNothing is every signal the saver host doctor reads through is
// asked to send.
var ErrSignalsNothing = errSignalsNothing

// ReadOnlySaverHost is the saver host doctor reads b's saver through.
func ReadOnlySaverHost(b backend.Backend, stateDir string) (*saverhost.Host, error) {
	return newReadOnlySaverHost(b, stateDir)
}
