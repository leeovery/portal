package cmd

import (
	"context"
	"errors"
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

// errSignalsNothing is every signal a read-only saver host is asked to send.
var errSignalsNothing = errors.New("this saver host signals nothing")

// newSaverHost runs b's saver's lifecycle over the state directory stateDir,
// proving its daemon by the marks of b's saver session and signalling
// through signalProcess.
func newSaverHost(b backend.Backend, stateDir string) (*saverhost.Host, error) {
	return saverHostSignalling(b, stateDir, func(pid int, sig syscall.Signal) error { return signalProcess(pid, sig) })
}

// readOnlySaverHost is a saver host's reads and nothing else. It holds the
// reads alone, never the host, so nothing holding it can reach the host's
// stop, ensure or signal: doctor reads the saver through it.
type readOnlySaverHost struct {
	present  func(ctx context.Context) (bool, error)
	saverPID func(ctx context.Context) (int, bool, error)
	prove    func(pid int) error
}

func (h readOnlySaverHost) Present(ctx context.Context) (bool, error) { return h.present(ctx) }

func (h readOnlySaverHost) SaverPID(ctx context.Context) (pid int, present bool, err error) {
	return h.saverPID(ctx)
}

func (h readOnlySaverHost) Prove(pid int) error { return h.prove(pid) }

// newReadOnlySaverHost reads b's saver as newSaverHost's host does. Its host
// refuses every signal besides, though none of its reads sends one.
func newReadOnlySaverHost(b backend.Backend, stateDir string) (readOnlySaverHost, error) {
	host, err := saverHostSignalling(b, stateDir, func(int, syscall.Signal) error { return errSignalsNothing })
	if err != nil {
		return readOnlySaverHost{}, err
	}
	return readOnlySaverHost{present: host.Present, saverPID: host.SaverPID, prove: host.Prove}, nil
}

// saverHostSignalling is b's saver's host over stateDir, sending its signals
// through signal. It refuses a b whose reach names no directory, by whose
// saver session's name alone a saver in any directory would be proved.
func saverHostSignalling(b backend.Backend, stateDir string, signal func(pid int, sig syscall.Signal) error) (*saverhost.Host, error) {
	marks, err := zmx.SessionMarks(saver.SessionName, b.ReachEnv())
	if err != nil {
		return nil, err
	}
	return saverhost.NewHost(saverhost.Config{
		Backend:  b,
		StateDir: stateDir,
		Marks:    marks,
		Signal:   signal,
	})
}
