//go:build linux

package cmd

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/saver"
)

// olderSaverDriver is the zmx driver bound to a directory whose _portal-saver
// answers as a zmx 0.8.1 daemon does: nothing in Portal's protocol, and 0.8.1's
// Info in its framing.
type olderSaverDriver struct{ *zmx.Backend }

func (olderSaverDriver) ProtocolVersion(context.Context, string) (int, error) { return 0, errNoAnswer }

func (olderSaverDriver) OlderZmx(context.Context, string) (bool, error) { return true, nil }

// foundSaver is a saver found running, read through the wiring's saverDaemon,
// that nothing may make another of.
type foundSaver struct {
	saverDaemon
	t *testing.T
}

func (foundSaver) Present(context.Context) (bool, error) { return true, nil }

func (s foundSaver) EnsureSaver(context.Context, func(context.Context) error) error {
	s.t.Error("a saver was ensured past one found that couldn't be ended")
	return nil
}

// quietSteps is every other seam of the bootstrap, each doing nothing.
type quietSteps struct{}

func (quietSteps) CheckZmx(context.Context) error           { return nil }
func (quietSteps) RemoveLatch() error                       { return nil }
func (quietSteps) WriteLatch() error                        { return nil }
func (quietSteps) Raise() error                             { return nil }
func (quietSteps) Lower() (bool, error)                     { return false, nil }
func (quietSteps) SweepOrphanDaemons(context.Context) error { return nil }
func (quietSteps) SweepStaleFlags(context.Context) error    { return nil }
func (quietSteps) MarkOwed(context.Context) error           { return nil }
func (quietSteps) Restore(context.Context) restore.Result   { return restore.Result{} }

// Off darwin the process table can't be read, so no saver found can be ended
// by the proved signal: a zmx 0.8.1 saver found running is the fatal naming
// the way out, never the judgement's own, which sends the user to a zmx that
// would leave it running.
func TestBootstrap_AnOlderSaverFoundOnLinuxIsTheFatalNamingTheWayOut(t *testing.T) {
	dir := t.TempDir()
	l, err := net.Listen("unix", filepath.Join(dir, saver.SessionName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	driver, err := bindZmxAt(zmxProgram, dir)
	if err != nil {
		t.Fatal(err)
	}
	withFuncSeam(t, &signalProcess, func(pid int, sig syscall.Signal) error {
		t.Errorf("signalled %d %v with no process table to prove it by", pid, sig)
		return nil
	})
	steps := quietSteps{}
	o := &bootstrap.Orchestrator{
		Zmx: steps, Latch: steps, Restoring: steps, OrphanSweeper: steps, FlagSweeper: steps, Restore: steps,
		Saver:    foundSaver{saverDaemon: saverDaemon{driver: func() (saverZmx, error) { return olderSaverDriver{driver}, nil }}, t: t},
		Protocol: zmx.ProtocolVersion,
	}

	_, err = o.Run(t.Context())

	fatal, ok := errors.AsType[*bootstrap.FatalError](err)
	if !ok || fatal.UserMessage != bootstrap.SaverLeftMessage {
		t.Fatalf("Run = %v, want the fatal %q", err, bootstrap.SaverLeftMessage)
	}
	if !errors.Is(err, bootstrap.ErrOlderZmx) || !errors.Is(err, proctable.ErrUnsupported) {
		t.Errorf("cause = %v, want the older zmx found, and the table it couldn't read to end it", fatal.Cause)
	}
}
