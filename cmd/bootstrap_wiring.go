package cmd

import (
	"context"
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
	p := &productionBootstrap{dir: dir, backend: b, host: host, running: runningLatch()}
	p.gate = bootstrap.Gate{StateDir: dir, Running: p.running, Present: host.Present, Logger: bootstrapLogger}
	return p, nil
}

// runningLatch is the latch a bootstrap of this boot and this binary writes.
// A boot id the kernel won't give leaves it incomplete, so never current: every
// command then bootstraps, and none can record it.
func runningLatch() state.Latch {
	id, _ := bootid.Read()
	return state.Latch{BootID: id, Version: Version()}
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
		Restore:     bootstrap.SessionRestorer{Restorer: restorer},
		Latch:       latchWriter{dir: p.dir, latch: p.running},
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

// bootSaver is the saver's lifecycle as the bootstrap's saver step asks for
// it, the protocol read from the saver session's daemon by the zmx driver on
// the directory the environment names.
type bootSaver struct {
	*saverhost.Ensurer
	host *saverhost.Host
}

func (s bootSaver) Present(ctx context.Context) (bool, error) { return s.host.Present(ctx) }

func (s bootSaver) Protocol(ctx context.Context) (int, error) {
	driver, err := bindZmxAt(zmxProgram, zmx.DirFor(os.Environ()))
	if err != nil {
		return 0, err
	}
	return driver.ProtocolVersion(ctx, saver.SessionName)
}

// latchWriter writes latch in the state directory dir.
type latchWriter struct {
	dir   string
	latch state.Latch
}

func (w latchWriter) WriteLatch() error { return state.WriteLatch(w.dir, w.latch) }
