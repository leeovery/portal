// Package bootstrap runs the bootstrap invoked from root's PersistentPreRunE:
// seven steps that bring the saved sessions back and the saver up as this
// binary. Step ordering is load-bearing.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/restore"
)

// Deliberately a second component in this package: the taxonomy files the
// sweeps' kills and lowered flags under clean.
var cleanLogger = log.For("clean")

const (
	stepCheckZmx       = "CheckZmx"
	stepRaiseRestoring = "RaiseRestoring"
	stepSweep          = "Sweep"
	stepEnsureSaver    = "EnsureSaver"
	stepRestore        = "Restore"
	stepLowerRestoring = "LowerRestoring"
	stepWriteLatch     = "WriteLatch"
)

// The steps' indices, from 1: the loading page (internal/tui's
// stepLabelTable) numbers its labels by them, and the restore step's
// per-session events carry its index.
const (
	indexCheckZmx = iota + 1
	indexRaiseRestoring
	indexSweep
	indexEnsureSaver
	indexRestore
	indexLowerRestoring
	indexWriteLatch

	totalSteps = indexWriteLatch
)

// Runner abstracts the bootstrap run so PersistentPreRunE need not import the
// concrete *Orchestrator.
type Runner interface {
	Run(ctx context.Context) ([]Warning, error)
}

// ZmxChecker checks that the zmx binary is on PATH and runs. Its error is
// worded for the user, who reads it as the bootstrap's fatal.
type ZmxChecker interface {
	CheckZmx(ctx context.Context) error
}

// RestoringFlag raises and lowers the state directory's restoring flag, which
// stands the saver and the hook sweep down while sessions are restored.
type RestoringFlag interface {
	Raise() error
	Lower() error
}

// FlagSweeper lowers the session flags nothing else ever would. Best-effort:
// its error is logged, never surfaced.
type FlagSweeper interface {
	SweepStaleFlags(ctx context.Context) error
}

// Saver is the saver's lifecycle as the bootstrap asks for it.
type Saver interface {
	// Present reports whether the saver session is there, whatever it runs.
	Present(ctx context.Context) (bool, error)
	// EnsureSaver ensures the saver session runs this binary's daemon.
	EnsureSaver(ctx context.Context) error
	// Protocol is the zmx protocol the saver session's daemon speaks.
	Protocol(ctx context.Context) (int, error)
}

// Restorer restores the saved sessions the backend doesn't hold. noSaver says
// the bootstrap found no saver running, so none saw this boot's sessions end
// and they come back too.
type Restorer interface {
	Restore(ctx context.Context, noSaver bool) restore.Result
}

// RestoreProgressSink is the optional seam a Restorer may also satisfy. The
// restore step calls SetProgress only when a progress emitter is wired.
type RestoreProgressSink interface {
	SetProgress(fn func(n, m int))
}

// LatchWriter records the bootstrapped latch: this boot's, and this
// version's.
type LatchWriter interface {
	WriteLatch() error
}

// Orchestrator runs the bootstrap's steps. Every seam must be set. A nil
// Logger is tolerated — Run substitutes a discard sink.
type Orchestrator struct {
	Zmx           ZmxChecker
	Restoring     RestoringFlag
	OrphanSweeper OrphanSweeper
	FlagSweeper   FlagSweeper
	Saver         Saver
	// Protocol is the zmx protocol Portal speaks. A saver whose daemon speaks
	// another is fatal before restore creates anything.
	Protocol int
	Restore  Restorer
	Latch    LatchWriter
	Logger   *slog.Logger
}

// Run executes the steps in order and returns the soft warnings in step order.
// Checking zmx and raising and lowering the restoring flag are fatal, as is a
// saver speaking another protocol; every other failure is logged, and those a
// user can act on are warned of. The latch is written last, so only a
// bootstrap that ran to the end records itself.
func (o *Orchestrator) Run(ctx context.Context) ([]Warning, error) {
	p := &pass{Orchestrator: o, logger: log.OrDiscard(o.Logger), emit: progressEmitterFromContext(ctx)}
	start := time.Now()
	for _, s := range p.steps() {
		p.logger.Debug("step entering", "step", s.name)
		stepStart := time.Now()
		if fatal := s.run(ctx); fatal != nil {
			fatal.Step = s.index
			p.logger.Error("step failed", "step", s.name, "error", fatal.Cause)
			return p.warnings, fatal
		}
		p.logger.Info("step complete", "step", s.name, log.Took(stepStart))
		p.progress(StepEvent{Index: s.index, Name: s.name})
	}
	p.logger.Info("orchestration complete", "steps", totalSteps, "warnings", len(p.warnings), log.Took(start))
	return p.warnings, nil
}

// pass is one Run: its warnings so far, and whether its saver step found a
// saver running.
type pass struct {
	*Orchestrator
	logger   *slog.Logger
	emit     ProgressEmitter
	warnings []Warning
	noSaver  bool
}

// step is one of the bootstrap's steps. run returns the step's fatal, nil
// otherwise: a soft failure it logs, and warns of, itself.
type step struct {
	index int
	name  string
	run   func(ctx context.Context) *FatalError
}

func (p *pass) steps() []step {
	return []step{
		{indexCheckZmx, stepCheckZmx, p.checkZmx},
		{indexRaiseRestoring, stepRaiseRestoring, p.raiseRestoring},
		{indexSweep, stepSweep, p.sweep},
		{indexEnsureSaver, stepEnsureSaver, p.ensureSaver},
		{indexRestore, stepRestore, p.restore},
		{indexLowerRestoring, stepLowerRestoring, p.lowerRestoring},
		{indexWriteLatch, stepWriteLatch, p.writeLatch},
	}
}

func (p *pass) progress(ev StepEvent) {
	if p.emit != nil {
		p.emit(ev)
	}
}

func (p *pass) checkZmx(ctx context.Context) *FatalError {
	if err := p.Zmx.CheckZmx(ctx); err != nil {
		return NewFatal(err.Error(), err)
	}
	return nil
}

// raiseRestoring must precede the sweep and the saver: the saver stands down
// while the flag is up, so a saver started next claims nothing restore is
// still making.
func (p *pass) raiseRestoring(context.Context) *FatalError {
	if err := p.Restoring.Raise(); err != nil {
		return NewFatal("Portal failed to raise the restoring flag: "+err.Error(), err)
	}
	return nil
}

// sweep must precede the saver, so the daemon started next meets no orphan
// still holding its lock, and restore, which raises the flags it lowers.
func (p *pass) sweep(ctx context.Context) *FatalError {
	if err := p.OrphanSweeper.SweepOrphanDaemons(ctx); err != nil {
		p.logger.Warn("step failed", "step", stepSweep, "error", err)
	}
	if err := p.FlagSweeper.SweepStaleFlags(ctx); err != nil {
		p.logger.Warn("step failed", "step", stepSweep, "error", err)
	}
	return nil
}

// ensureSaver notes whether a saver was running before it ensures one, as
// restore asks, then judges the protocol the saver's daemon speaks. A saver
// whose presence can't be read may have watched sessions end, so restore
// brings back only what predates the boot: a session the user ended never
// comes back on a doubt.
func (p *pass) ensureSaver(ctx context.Context) *FatalError {
	present, err := p.Saver.Present(ctx)
	if err != nil {
		p.logger.Warn("step failed", "step", stepEnsureSaver, "error", fmt.Errorf("tell whether a saver ran: %w", err))
		present = true
	}
	p.noSaver = !present
	if err := p.Saver.EnsureSaver(ctx); err != nil {
		p.warnings = append(p.warnings, SaverDownWarning())
		p.logger.Warn("step failed", "step", stepEnsureSaver, "error", err)
	}
	speaks, err := p.Saver.Protocol(ctx)
	switch {
	case err != nil:
		p.logger.Warn("step failed", "step", stepEnsureSaver, "error", fmt.Errorf("read the saver's protocol: %w", err))
	case speaks != p.Protocol:
		return NewFatal(fmt.Sprintf("Portal can't use this zmx: its sessions speak protocol %d, and Portal speaks %d. Upgrade Portal, or install the zmx release it supports.", speaks, p.Protocol),
			fmt.Errorf("the saver's daemon speaks zmx protocol %d, not %d", speaks, p.Protocol))
	}
	return nil
}

func (p *pass) restore(ctx context.Context) *FatalError {
	if sink, ok := p.Restore.(RestoreProgressSink); ok && p.emit != nil {
		sink.SetProgress(func(n, m int) {
			p.progress(StepEvent{Index: indexRestore, Name: stepRestore, RestoreN: n, RestoreM: m})
		})
	}
	res := p.Restore.Restore(ctx, p.noSaver)
	if res.IndexErr != nil {
		p.warnings = append(p.warnings, CorruptSessionsJSONWarning())
	}
	if res.ListErr != nil {
		p.warnings = append(p.warnings, SessionsUnlistedWarning())
	}
	if res.Failed > 0 {
		p.warnings = append(p.warnings, UnrestoredWarning(res.Failed))
	}
	return nil
}

func (p *pass) lowerRestoring(context.Context) *FatalError {
	if err := p.Restoring.Lower(); err != nil {
		return NewFatal("Portal failed to lower the restoring flag: "+err.Error(), err)
	}
	return nil
}

// writeLatch must stay the last step: the latch has to land before the
// concurrent route's terminal Done event, so a present latch always means a
// bootstrap that ran to completion.
func (p *pass) writeLatch(context.Context) *FatalError {
	if err := p.Latch.WriteLatch(); err != nil {
		p.logger.Warn("step failed", "step", stepWriteLatch, "error", err)
	}
	return nil
}
