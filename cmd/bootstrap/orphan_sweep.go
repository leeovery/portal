package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/saverhost"
)

// OrphanSweeper kills leftover `portal state daemon` processes. Best-effort:
// SweepOrphanDaemons logs and swallows every failure and always returns nil.
type OrphanSweeper interface {
	SweepOrphanDaemons(ctx context.Context) error
}

// OrphanSweepCore SIGKILLs every orphaned saver daemon, and waits for each to
// finish exiting. An orphan is a pid pgrep found that Prove proves, read just
// before its kill, the saver's daemon of this backend's sessions, and that is
// neither the saver session's own process nor this one. Argv alone never
// kills: a sweep would otherwise kill another install's daemon, the tmux
// version's among them, which lacks one of the saver session's marks. Every
// field must be supplied; a sweep missing one kills nothing.
type OrphanSweepCore struct {
	// Pgrep finds the candidates. No default: shelling out would pull
	// os/exec into this pure-orchestration package.
	Pgrep func() ([]int, error)

	// SaverPID is the saver session's process. Absence rides the present
	// bool, never pid 0; a read that fails leaves the saver's process
	// unknown, and the sweep then kills nothing, since it can't spare it.
	SaverPID func(ctx context.Context) (pid int, present bool, err error)

	// Prove proves a pid the saver's daemon of this backend's sessions, as
	// saverhost.Host.Prove does, its error wrapping saverhost's.
	Prove func(pid int) error

	// Signal sends sig to pid; the sweep sends only SIGKILL, and only to a
	// pid it has just proved.
	Signal func(pid int, sig syscall.Signal) error

	// AwaitExit waits, bounded, for a pid sent SIGKILL to finish exiting, as
	// saverhost.Host.AwaitExit does, so the saver started next meets no
	// orphan still holding daemon.lock.
	AwaitExit func(ctx context.Context, pid int) error

	Logger *slog.Logger
}

var _ OrphanSweeper = (*OrphanSweepCore)(nil)

// SweepOrphanDaemons sends SIGKILL, not SIGTERM: an orphan daemon's view of
// the world is untrusted, so it must not get the chance to flush a final
// state on the way out.
func (c *OrphanSweepCore) SweepOrphanDaemons(ctx context.Context) error {
	logger := log.OrDiscard(c.Logger)
	if c.Pgrep == nil || c.SaverPID == nil || c.Prove == nil || c.Signal == nil || c.AwaitExit == nil {
		logger.Warn("sweep: not wired, nothing killed")
		return nil
	}

	start := time.Now()
	candidates, err := c.Pgrep()
	if err != nil {
		logger.Warn("sweep: pgrep failed", "error", err)
		return nil
	}
	killed, err := c.sweep(ctx, logger, candidates)
	if err != nil {
		logger.Warn("sweep: saver pid unreadable, nothing killed", "error", err)
		return nil
	}
	cleanLogger.Info("orphan-daemon sweep complete", "killed", killed, log.Took(start))
	return nil
}

// sweep kills the orphans among candidates and counts them. With no
// candidates it reads nothing more.
func (c *OrphanSweepCore) sweep(ctx context.Context, logger *slog.Logger, candidates []int) (int, error) {
	if len(candidates) == 0 {
		return 0, nil
	}
	spared, err := c.spared(ctx)
	if err != nil {
		return 0, err
	}
	var killed int
	for _, pid := range candidates {
		if !spared[pid] && c.killOrphan(ctx, logger, pid) {
			killed++
		}
	}
	return killed, nil
}

// spared is the processes no sweep kills: this one, and the saver session's.
func (c *OrphanSweepCore) spared(ctx context.Context) (map[int]bool, error) {
	spared := map[int]bool{os.Getpid(): true}
	pid, present, err := c.SaverPID(ctx)
	switch {
	case err != nil:
		return nil, err
	// Host.SaverPID never answers this, but the sweep SIGKILLs every proved
	// daemon it doesn't spare, the live saver's included, so a seam that did
	// must kill nothing.
	case present && pid <= 0:
		return nil, errors.New("the saver session names no process")
	case present:
		spared[pid] = true
	}
	return spared, nil
}

// killOrphan sends pid SIGKILL once proved an orphaned saver daemon, with
// nothing between the proof and the signal, and reports whether it has
// finished exiting.
func (c *OrphanSweepCore) killOrphan(ctx context.Context, logger *slog.Logger, pid int) bool {
	if err := c.Prove(pid); err != nil {
		if errors.Is(err, saverhost.ErrUnreadable) {
			logger.Warn("sweep: pid unprovable, skipping", "target_pid", pid, "error", err)
		} else {
			logger.Debug("sweep: pid not proved an orphaned saver daemon, skipping", "target_pid", pid, "error", err)
		}
		return false
	}
	err := c.Signal(pid, syscall.SIGKILL)
	if err == nil {
		err = c.AwaitExit(ctx, pid)
	}
	if err != nil {
		logger.Warn("sweep: kill failed", "target_pid", pid, "error", err)
		return false
	}
	cleanLogger.Debug("orphan killed", "target_pid", pid)
	return true
}
