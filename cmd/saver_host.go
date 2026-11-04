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

// newSaverStopper stops b's saver, proving its daemon by the marks of b's
// saver session and signalling through signalProcess. It refuses a b whose
// reach names no directory, by whose saver session's name alone a saver in
// any directory would be proved.
func newSaverStopper(b backend.Backend) (*saverhost.Stopper, error) {
	marks, err := zmx.SessionMarks(saver.SessionName, b.ReachEnv())
	if err != nil {
		return nil, err
	}
	return saverhost.NewStopper(saverhost.Config{
		Backend: b,
		Marks:   marks,
		Signal:  func(pid int, sig syscall.Signal) error { return signalProcess(pid, sig) },
	})
}
