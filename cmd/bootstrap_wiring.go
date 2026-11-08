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
	"github.com/leeovery/portal/internal/xdg"
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
	owner   bootstrap.ZmxDirOwner
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
	p := &productionBootstrap{dir: dir, backend: b, host: host, owner: zmxDirOwner(dir, zmx.DirFor(os.Environ()))}
	p.running, p.bootErr = runningLatch()
	p.gate = bootstrap.Gate{StateDir: dir, Running: p.running, ZmxDir: p.owner.Claimed, Saver: host.Recorded, Logger: bootstrapLogger}
	return p, nil
}

// zmxDirOwner pairs the state directory stateDir with the zmx directory
// zmxDir, compared as the zmx backend compares directories.
func zmxDirOwner(stateDir, zmxDir string) bootstrap.ZmxDirOwner {
	return bootstrap.ZmxDirOwner{StateDir: stateDir, ZmxDir: zmxDir, Canonical: zmx.CanonicalDir}
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
		ZmxDir:    p.owner,
		Latch:     latchFile{dir: p.dir, latch: p.running, bootErr: p.bootErr},
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
		Saver:       bootSaver{Ensurer: ensurer, host: p.host, saverDaemon: newSaverDaemon(p.dir)},
		Protocol:    zmx.ProtocolVersion,
		Restore:     restorer,
		Logger:      bootstrapLogger,
	}
	return bootstrap.Locked{Lock: bootstrapLock(p.dir), Owed: p.gate.Owed, Bootstrap: steps, Logger: bootstrapLogger}, nil
}

// holdBootstrapLock takes bootstrap.lock in the state directory dir alone, as
// a bootstrap takes it: within its bound, a wait past its notice told to
// ctx's progress emitter, and a lock it can't take fatal. It returns the
// release, for work that must not race a bootstrap's restore or a kept
// session's bring-back.
var holdBootstrapLock = func(ctx context.Context, dir string) (release func(), err error) {
	return bootstrap.Locked{Lock: bootstrapLock(dir), Logger: bootstrapLogger}.Hold(ctx)
}

// bootstrapLock takes bootstrap.lock in the state directory dir, bounded by
// the run of a bootstrap restoring every entry sessions.json holds now.
func bootstrapLock(dir string) func(ctx context.Context) (func(), error) {
	return func(ctx context.Context) (func(), error) {
		return state.AcquireBootstrapLock(ctx, dir, bootstrap.LockWait(savedEntries(dir)))
	}
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

func (dir restoringFlag) Lower() (bool, error) { return state.Restoring.Clear(string(dir)) }

var (
	_ bootstrap.Restorer            = (*restore.Restorer)(nil)
	_ bootstrap.RestoreProgressSink = (*restore.Restorer)(nil)
)

// bootSaver is the saver's lifecycle as the bootstrap's saver step asks for
// it, the saver session's daemon read, and ended, as saverDaemon reads and
// ends it.
type bootSaver struct {
	*saverhost.Ensurer
	host *saverhost.Host
	saverDaemon
}

func (s bootSaver) Present(ctx context.Context) (bool, error) { return s.host.Present(ctx) }

func (s bootSaver) CheckStateDir(ctx context.Context) error { return s.host.CheckStateDir(ctx) }

// saverZmx is the zmx driver's reach to the saver session's daemon, whatever
// protocol it speaks.
type saverZmx interface {
	ProtocolVersion(ctx context.Context, name string) (int, error)
	OlderZmx(ctx context.Context, name string) (bool, error)
	KillByCLI(ctx context.Context, name string) error
	EndDaemon(ctx context.Context, name string, signal func(pid int, sig syscall.Signal) error, dirs ...string) error
}

// saverDaemon is the saver session's daemon of the state directory stateDir,
// read and ended through the zmx driver its driver binds.
type saverDaemon struct {
	driver   func() (saverZmx, error)
	stateDir string
}

// newSaverDaemon reads the saver session's daemon of the state directory
// stateDir through the zmx on PATH, in the directory the environment names.
func newSaverDaemon(stateDir string) saverDaemon {
	return saverDaemon{stateDir: stateDir, driver: func() (saverZmx, error) {
		driver, err := bindZmxAt(zmxProgram, zmx.DirFor(os.Environ()))
		if err != nil {
			return nil, err
		}
		return driver, nil
	}}
}

// Protocol is the protocol version the saver session's daemon answers in:
// bootstrap.ErrSaverGone for a session gone, and bootstrap.ErrOlderZmx for a
// daemon that takes Portal's request and answers it nothing, but answers zmx
// 0.8.1's framing.
func (d saverDaemon) Protocol(ctx context.Context) (int, error) {
	driver, err := d.driver()
	if err != nil {
		return 0, err
	}
	version, err := driver.ProtocolVersion(ctx, saver.SessionName)
	switch {
	case errors.Is(err, backend.ErrNoSuchSession):
		return 0, fmt.Errorf("%w: %w", bootstrap.ErrSaverGone, err)
	case !errors.Is(err, backend.ErrTimeout) || errors.Is(err, zmx.ErrRefusing):
		return version, err
	}
	if older, _ := driver.OlderZmx(ctx, saver.SessionName); older {
		return 0, fmt.Errorf("%w: %w", bootstrap.ErrOlderZmx, err)
	}
	return 0, err
}

// End ends the saver session's daemon, whatever protocol it speaks, by a
// SIGTERM, through signalProcess, to the one process proved that daemon of
// the state directory's saver.
func (d saverDaemon) End(ctx context.Context) error {
	driver, err := d.driver()
	if err != nil {
		return err
	}
	return d.endBySignal(ctx, driver)
}

// EndMade ends the saver session the bootstrap made with zmx kill, run by the
// zmx on PATH that made it, which speaks its daemon's protocol, so needs no
// process table: off darwin, the only way. End's signal ends it when that
// kill fails. Create's refusal has no such fallback (decision 80): this
// proof targets only _portal-saver's daemon, so a wrong match ends a Portal
// process, never a user's session.
func (d saverDaemon) EndMade(ctx context.Context) error {
	driver, err := d.driver()
	if err != nil {
		return err
	}
	killErr := driver.KillByCLI(ctx, saver.SessionName)
	if killErr == nil {
		return nil
	}
	if err := d.endBySignal(ctx, driver); err != nil {
		return fmt.Errorf("%w; then: %w", killErr, err)
	}
	return nil
}

// endBySignal ends the saver session's daemon through driver, proved by the
// state directory's mark beside the session's own, so another state
// directory's saver is never signalled: one whose daemon names another state
// directory is the *saverhost.OtherStateDirError the proof's refusal is.
func (d saverDaemon) endBySignal(ctx context.Context, driver saverZmx) error {
	err := driver.EndDaemon(ctx, saver.SessionName, func(pid int, sig syscall.Signal) error { return signalProcess(pid, sig) },
		saverhost.StateDirMark(d.stateDir))
	if other, ok := errors.AsType[*zmx.OtherDirError](err); ok && other.Key == xdg.StateDir.EnvVar {
		return &saverhost.OtherStateDirError{StateDir: d.stateDir, Other: other.Dir, PID: other.PID}
	}
	return err
}

// unusable reports whether the saver session's daemon speaks a protocol
// Portal can't use: an older zmx's, or one other than Portal's.
func (d saverDaemon) unusable(ctx context.Context) bool {
	speaks, err := d.Protocol(ctx)
	return errors.Is(err, bootstrap.ErrOlderZmx) || err == nil && speaks != zmx.ProtocolVersion
}

// latchFile is latch in the state directory dir. Its write fails with
// bootstrap.ErrNoBootID when bootErr says this boot's id couldn't be read.
type latchFile struct {
	dir     string
	latch   state.Latch
	bootErr error
}

func (f latchFile) RemoveLatch() error { return state.RemoveLatch(f.dir) }

func (f latchFile) WriteLatch() error {
	if f.bootErr != nil {
		return fmt.Errorf("%w: %w", bootstrap.ErrNoBootID, f.bootErr)
	}
	return state.WriteLatch(f.dir, f.latch)
}
