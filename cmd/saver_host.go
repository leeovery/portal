package cmd

import (
	"syscall"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
)

// signalProcess signals a process the saver's lifecycle has proved the saver's
// daemon. cmd's TestMain makes it panic: the pids a test's backend reports
// are no processes of the test's.
var signalProcess = syscall.Kill

// saverMarks are the entries the environment of every process b's saver
// session runs carries, and no other process's does: the session's name,
// which zmx sets, and b's directory, which every zmx run the driver makes
// sets.
func saverMarks(b backend.Backend) []string {
	return append([]string{zmx.SessionEntry(saver.SessionName)}, b.ReachEnv()...)
}

// newSaverStopper stops b's saver, signalling through signalProcess.
func newSaverStopper(b backend.Backend) (*saverhost.Stopper, error) {
	return saverhost.NewStopper(saverhost.Config{
		Backend: b,
		Marks:   saverMarks(b),
		Signal:  func(pid int, sig syscall.Signal) error { return signalProcess(pid, sig) },
	})
}
