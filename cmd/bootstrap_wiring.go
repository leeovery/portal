package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/bootid"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

var restoreLogger = log.For("restore")

// bootstrapExe is the portal binary the saver and restore run, as an absolute
// path. cmd's TestMain makes it panic: under go test it is the test binary,
// which a saver or a restored session would run as the whole suite.
var bootstrapExe = os.Executable

// productionBootstrap is the gate every command asks over the state
// directory and the session backend, and the bootstrap it owes.
type productionBootstrap struct {
	dir     string
	backend backend.Backend
	host    *saverhost.Host
	running state.Latch
	// bootErr is why this boot's id couldn't be read, leaving running
	// incomplete.
	bootErr error
	gate    bootstrap.Gate
}

func newProductionBootstrap() (*productionBootstrap, error) {
	dir, err := state.Dir()
	if err != nil {
		return nil, err
	}
	b, err := sessionBackend()
	if err != nil {
		return nil, err
	}
	host, err := newSaverHost(b, dir)
	if err != nil {
		return nil, err
	}
	p := &productionBootstrap{dir: dir, backend: b, host: host}
	p.running, p.bootErr = runningLatch()
	p.gate = bootstrap.Gate{StateDir: dir, Running: p.running, Present: host.Present, Logger: bootstrapLogger}
	return p, nil
}

// runningLatch is the latch a bootstrap of this boot and this binary writes.
// A boot id the kernel won't give leaves it incomplete, so never current: every
// command then bootstraps, none can record it, and each warns of it.
func runningLatch() (state.Latch, error) {
	id, err := bootid.Read()
	return state.Latch{BootID: id, Version: Version()}, err
}

// runner is the bootstrap, run one at a time under bootstrap.lock. The saver
// and every restored session run bootstrapExe, which restore refuses unless
// it is named portal.
func (p *productionBootstrap) runner() (bootstrap.Runner, error) {
	exe, err := bootstrapExe()
	if err != nil {
		return nil, fmt.Errorf("find Portal's own executable: %w", err)
	}
	restorer, err := restore.New(restore.Config{Backend: p.backend, StateDir: p.dir, Exe: exe, Logger: restoreLogger})
	if err != nil {
		return nil, err
	}
	ensurer, err := p.host.Ensurer(saverhost.Daemon{Exe: exe, Version: Version()})
	if err != nil {
		return nil, err
	}
	steps := &bootstrap.Orchestrator{
		Zmx:       zmxCheck{dir: zmx.DirFor(os.Environ())},
		Restoring: restoringFlag(p.dir),
		OrphanSweeper: &bootstrap.OrphanSweepCore{
			Pgrep:     state.PgrepPortalDaemons,
			SaverPID:  p.host.SaverPID,
			Prove:     p.host.Prove,
			Signal:    func(pid int, sig syscall.Signal) error { return signalProcess(pid, sig) },
			AwaitExit: p.host.AwaitExit,
			Logger:    bootstrapLogger,
		},
		FlagSweeper: &bootstrap.FlagSweepCore{Dir: p.dir, List: p.backend.List, RunsHelper: restorer.RunsHelper, Logger: bootstrapLogger},
		Saver:       bootSaver{Ensurer: ensurer, host: p.host},
		Protocol:    zmx.ProtocolVersion,
		Restore:     restorer,
		Latch:       latchWriter{dir: p.dir, latch: p.running, bootErr: p.bootErr},
		Logger:      bootstrapLogger,
	}
	return bootstrap.Locked{Lock: p.lock, Owed: p.gate.Owed, Bootstrap: steps, Logger: bootstrapLogger}, nil
}

// lock takes bootstrap.lock, bounded by the run of a bootstrap restoring
// every entry sessions.json holds now.
func (p *productionBootstrap) lock(ctx context.Context) (func(), error) {
	return state.AcquireBootstrapLock(ctx, p.dir, bootstrap.LockWait(savedEntries(p.dir)))
}

// savedEntries counts the entries in dir's sessions.json, none when it can't
// be read, as restore then restores none.
func savedEntries(dir string) int {
	idx, err := sessionindex.Read(dir)
	if err != nil {
		return 0
	}
	return idx.Len()
}

// zmxCheck is the bootstrap's first step over the zmx directory dir: zmx is
// on PATH and runs.
type zmxCheck struct{ dir string }

func (c zmxCheck) CheckZmx(ctx context.Context) error {
	binary, err := exec.LookPath(zmxProgram)
	if err != nil {
		return errZmxMissing
	}
	driver, err := bindZmxAt(binary, c.dir)
	if err != nil {
		return err
	}
	if err := driver.Runs(ctx); err != nil {
		return fmt.Errorf("zmx at %s doesn't run: %w", binary, err)
	}
	return nil
}

// restoringFlag is the restoring flag in the state directory it names.
type restoringFlag string

func (dir restoringFlag) Raise() error { return state.Restoring.Set(string(dir)) }

func (dir restoringFlag) Lower() error {
	_, err := state.Restoring.Clear(string(dir))
	return err
}

var (
	_ bootstrap.Restorer            = (*restore.Restorer)(nil)
	_ bootstrap.RestoreProgressSink = (*restore.Restorer)(nil)
)

// bootSaver is the saver's lifecycle as the bootstrap's saver step asks for
// it, the saver session's daemon read, and ended, by the zmx driver on the
// directory the environment names.
type bootSaver struct {
	*saverhost.Ensurer
	host *saverhost.Host
}

func (s bootSaver) Present(ctx context.Context) (bool, error) { return s.host.Present(ctx) }

// Protocol is the protocol version the saver session's daemon answers in, or
// bootstrap.ErrOlderZmx for one that takes Portal's request and answers it
// nothing, but answers zmx 0.8.1's framing.
func (s bootSaver) Protocol(ctx context.Context) (int, error) {
	driver, err := saverDriver()
	if err != nil {
		return 0, err
	}
	version, err := driver.ProtocolVersion(ctx, saver.SessionName)
	if !errors.Is(err, backend.ErrTimeout) || errors.Is(err, zmx.ErrRefusing) {
		return version, err
	}
	if older, _ := driver.OlderZmx(ctx, saver.SessionName); older {
		return 0, fmt.Errorf("%w: %w", bootstrap.ErrOlderZmx, err)
	}
	return 0, err
}

// End ends the saver session with the kill of the zmx on PATH, the binary
// that made it.
func (s bootSaver) End(ctx context.Context) error {
	driver, err := saverDriver()
	if err != nil {
		return err
	}
	return driver.KillByCLI(ctx, saver.SessionName)
}

// saverDriver is the zmx driver on PATH's zmx in the directory the
// environment names.
func saverDriver() (*zmx.Backend, error) {
	return bindZmxAt(zmxProgram, zmx.DirFor(os.Environ()))
}

// latchWriter writes latch in the state directory dir, or fails with
// bootstrap.ErrNoBootID when bootErr says this boot's id couldn't be read.
type latchWriter struct {
	dir     string
	latch   state.Latch
	bootErr error
}

func (w latchWriter) WriteLatch() error {
	if w.bootErr != nil {
		return fmt.Errorf("%w: %w", bootstrap.ErrNoBootID, w.bootErr)
	}
	return state.WriteLatch(w.dir, w.latch)
}
