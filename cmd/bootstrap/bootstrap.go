// Package bootstrap runs the bootstrap invoked from root's PersistentPreRunE:
// six steps that bring the saved sessions back and the saver up as this
// binary. Step ordering is load-bearing.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/saverhost"
)

// Deliberately a second component in this package: the taxonomy files the
// sweeps' kills and lowered flags under clean.
var cleanLogger = log.For("clean")

const (
	stepCheckZmx    = "CheckZmx"
	stepRemoveLatch = "RemoveLatch"
	stepSweep       = "Sweep"
	stepEnsureSaver = "EnsureSaver"
	stepRestore     = "Restore"
	stepWriteLatch  = "WriteLatch"
)

// The steps' indices, from 1: the loading page (internal/tui's
// stepLabelTable) numbers its labels by them, and the restore step's
// per-session events carry its index.
const (
	indexCheckZmx = iota + 1
	indexRemoveLatch
	indexSweep
	indexEnsureSaver
	indexRestore
	indexWriteLatch

	totalSteps = indexWriteLatch
)

var (
	// ErrOlderZmx is a saver whose daemon gives Portal's zmx protocol no
	// answer but answers zmx 0.8.1's framing: a zmx Portal doesn't support.
	ErrOlderZmx = errors.New("the saver's daemon speaks zmx 0.8.1's framing")
	// ErrSaverGone is a saver session gone by the time its daemon is asked
	// its protocol.
	ErrSaverGone = errors.New("the saver session is gone")
	// ErrNoBootID is a latch that can't be written for want of this boot's id.
	ErrNoBootID = errors.New("this boot's id can't be read")
)

// errJudged ends EnsureSaver once the saver it made is judged unusable.
var errJudged = errors.New("the saver was judged unusable")

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

// Latch is the record that the last bootstrap finished: this boot's, and this
// version's, which the gate reads.
type Latch interface {
	RemoveLatch() error
	// WriteLatch records it. Its error wraps ErrNoBootID when this boot's id
	// can't be read.
	WriteLatch() error
}

// RestoringFlag raises and lowers the state directory's restoring flag, which
// stands the saver and the hook sweep down while restore's creates are in
// flight.
type RestoringFlag interface {
	Raise() error
	// Lower lowers it, reporting whether it was raised.
	Lower() (bool, error)
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
	// EnsureSaver ensures the saver session runs this binary's daemon. It
	// judges a saver it creates by made before waiting for its daemon, and
	// made's error is its own; its other failures wrap saverhost's causes.
	EnsureSaver(ctx context.Context, made func(context.Context) error) error
	// Protocol is the zmx protocol the saver session's daemon speaks:
	// ErrOlderZmx for an older zmx's daemon, and ErrSaverGone for a saver
	// session gone.
	Protocol(ctx context.Context) (int, error)
	// End ends the saver session's daemon, whatever protocol it speaks, by a
	// signal to the one process proved that daemon.
	End(ctx context.Context) error
	// EndMade ends the saver session EnsureSaver made with the zmx that made
	// it, which speaks its daemon's protocol, and as End does when that zmx
	// can't.
	EndMade(ctx context.Context) error
}

// Restorer restores the saved sessions the backend doesn't hold.
type Restorer interface {
	// MarkOwed marks owed every saved session the backend doesn't hold, which
	// no saver saw end: every restore brings an owed session back, and no
	// saver drops it.
	MarkOwed(ctx context.Context) error
	Restore(ctx context.Context) restore.Result
}

// RestoreProgressSink is the optional seam a Restorer may also satisfy. The
// restore step calls SetProgress only when a progress emitter is wired.
type RestoreProgressSink interface {
	SetProgress(fn func(n, m int))
}

// Orchestrator runs the bootstrap's steps. Every seam must be set. A nil
// Logger is tolerated — Run substitutes a discard sink.
type Orchestrator struct {
	Zmx           ZmxChecker
	ZmxDir        ZmxDirClaimer
	Latch         Latch
	Restoring     RestoringFlag
	OrphanSweeper OrphanSweeper
	FlagSweeper   FlagSweeper
	Saver         Saver
	// Protocol is the zmx protocol Portal speaks. A saver whose daemon speaks
	// another is ended before restore creates anything.
	Protocol int
	Restore  Restorer
	Logger   *slog.Logger
}

// Run executes the steps in order and returns the soft warnings in step order.
// Checking zmx, claiming the state directory for its zmx directory, removing
// the latch, and raising and lowering the restoring flag are fatal, as is a
// saver Portal can't use that it made, or found and can't end, and a session
// restore made that the backend refused as one Portal can't talk to; every
// other failure is logged, and those a user can act on are warned of. The
// latch goes before the first change and is written only by a pass that ran
// to the end, so a pass cut short leaves none, and so does one that leaves the
// restore unfinished: its context's end, a saver it can't judge, sessions it
// can't mark owed or a listing it can't read. One whose saver may still run a
// daemon of another version, one it couldn't replace or one it couldn't read,
// restores, but writes no latch either. The next command then owes the
// bootstrap again.
func (o *Orchestrator) Run(ctx context.Context) ([]Warning, error) {
	p := &pass{Orchestrator: o, logger: log.OrDiscard(o.Logger), emit: progressEmitterFromContext(ctx)}
	start := time.Now()
	for _, s := range p.steps() {
		if why := p.skipping(s); why != "" {
			p.logger.Info("step skipped", "step", s.name, "detail", why)
			p.progress(StepEvent{Index: s.index, Name: s.name})
			continue
		}
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

// pass is one Run: its warnings so far, whether the saver's protocol has
// been judged Portal's, whether the restore is left unfinished, and why the
// latch is withheld, if it is.
type pass struct {
	*Orchestrator
	logger     *slog.Logger
	emit       ProgressEmitter
	warnings   []Warning
	judged     bool
	unfinished bool
	withheld   string
}

// skipping says why the pass skips s, and is empty when it runs it: a
// finishing step once the restore is left unfinished, and the latch once
// withheld.
func (p *pass) skipping(s step) string {
	switch {
	case s.finishing && p.unfinished:
		return "the restore is unfinished: no latch is written, and the next command bootstraps again"
	case s.index == indexWriteLatch && p.withheld != "":
		return p.withheld + ": no latch is written, and the next command bootstraps again"
	}
	return ""
}

// withholding says why the saver's failure err withholds the latch, and is
// empty when it doesn't: the saver may still run a daemon of another version,
// one a recycle couldn't stop or one unread whose version isn't proved this
// one's, so the next command bootstraps again and tries the replacement
// again. Restore runs all the same: that daemon saves as it did before the
// upgrade.
func withholding(err error) string {
	switch {
	case errors.Is(err, saverhost.ErrNotReplaced):
		return "the saver's daemon couldn't be replaced"
	case errors.Is(err, saverhost.ErrVersionUnproved):
		return "the saver couldn't be read, nor its daemon proved this version"
	}
	return ""
}

// step is one of the bootstrap's steps. run returns the step's fatal, nil
// otherwise: a soft failure it logs, and warns of, itself. A finishing step
// runs only while no step before it has left the restore unfinished.
type step struct {
	index     int
	name      string
	finishing bool
	run       func(ctx context.Context) *FatalError
}

func (p *pass) steps() []step {
	return []step{
		{indexCheckZmx, stepCheckZmx, false, p.checkZmx},
		{indexRemoveLatch, stepRemoveLatch, false, p.removeLatch},
		{indexSweep, stepSweep, false, p.sweep},
		{indexEnsureSaver, stepEnsureSaver, false, p.ensureSaver},
		{indexRestore, stepRestore, true, p.restore},
		{indexWriteLatch, stepWriteLatch, true, p.writeLatch},
	}
}

func (p *pass) progress(ev StepEvent) {
	if p.emit != nil {
		p.emit(ev)
	}
}

// warn records a soft failure of step, warning the user with w.
func (p *pass) warn(step string, w Warning, err error) {
	p.warnings = append(p.warnings, w)
	p.logger.Warn("step failed", "step", step, "error", err)
}

// leaveUnfinished is warn of a failure that leaves the restore unfinished.
func (p *pass) leaveUnfinished(step string, w Warning, err error) {
	p.warn(step, w, err)
	p.unfinished = true
}

// checkZmx checks zmx runs, then claims the state directory for the zmx
// directory Portal runs against, before the latch goes: a state directory
// that belongs to another is refused with nothing changed.
func (p *pass) checkZmx(ctx context.Context) *FatalError {
	if err := p.Zmx.CheckZmx(ctx); err != nil {
		return NewFatal(err.Error(), err)
	}
	if err := p.ZmxDir.Claim(); err != nil {
		return zmxDirFatal(err)
	}
	return nil
}

// removeLatch must precede the bootstrap's first change, so a pass cut short
// anywhere after it leaves no latch, and the next command finishes its work.
// It lowers a restoring flag left raised by a pass cut short mid-restore:
// under bootstrap.lock no restore's creates are in flight, and left up, the
// flag would stand the saver down until some later pass finished a restore.
func (p *pass) removeLatch(context.Context) *FatalError {
	if err := p.Latch.RemoveLatch(); err != nil {
		return NewFatal("Portal failed to remove the bootstrapped latch: "+err.Error(), err)
	}
	switch raised, err := p.Restoring.Lower(); {
	case err != nil:
		return lowerFatal(err)
	case raised:
		p.logger.Info("restoring flag lowered", "detail", "a restore cut short left it raised")
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

// ensureSaver judges a saver it finds before anything is made, ending one
// Portal can't use. Finding none, or ending one, it marks owed the sessions no
// saver saw end before any saver can commit, then ensures the saver, judging
// one it makes before waiting for its daemon. Restore runs only once a saver
// is judged to speak Portal's protocol: Portal creates no session on a zmx it
// hasn't proved it speaks.
func (p *pass) ensureSaver(ctx context.Context) *FatalError {
	kept, fatal := p.keepFound(ctx)
	switch {
	case fatal != nil || p.unfinished:
		return fatal
	case !kept:
		if err := p.Restore.MarkOwed(ctx); err != nil {
			p.leaveUnfinished(stepEnsureSaver, OwedUnmarkedWarning(), fmt.Errorf("mark owed the sessions no saver saw end: %w", err))
			return nil
		}
	}
	err := p.Saver.EnsureSaver(ctx, func(ctx context.Context) error {
		if fatal = p.judgeMade(ctx); fatal != nil || p.unfinished {
			return errJudged
		}
		return nil
	})
	switch {
	case fatal != nil || p.unfinished:
		return fatal
	case errors.Is(err, backend.ErrUnsupportedProtocol):
		// The create has ended what it made, and says in err whether it went.
		return UnsupportedCreate(err)
	case err != nil:
		p.warn(stepEnsureSaver, saverWarning(err), err)
		p.withheld = withholding(err)
	}
	if !p.judged {
		p.leaveUnfinished(stepEnsureSaver, UnjudgedWarning(), errors.New("no saver came up to judge zmx's protocol by"))
	}
	return nil
}

// keepFound judges the saver session it finds, and reports whether one runs
// that the pass keeps. One that speaks Portal's protocol is kept, judged; one
// it can't judge is kept, the restore left unfinished; one Portal can't use is
// ended, so the pass goes on as if none ran. One it can't end is the fatal
// that names the way out, SaverLeftMessage: the judgement's own would send
// the user to a zmx that leaves it there.
func (p *pass) keepFound(ctx context.Context) (bool, *FatalError) {
	if !p.saverPresent(ctx) {
		return false, nil
	}
	unusable, err := p.judge(ctx)
	switch {
	case err != nil:
		p.leaveUnfinished(stepEnsureSaver, UnjudgedWarning(), err)
		return true, nil
	case unusable == nil:
		p.judged = true
		return true, nil
	}
	if err := p.Saver.End(ctx); err != nil {
		return true, NewFatal(SaverLeftMessage, fmt.Errorf("%w; Portal couldn't end the saver running on it, which is left: %w", unusable.Cause, err))
	}
	p.logger.Info("saver ended", "detail", unusable.Cause.Error()+"; Portal ended the saver running on it")
	return false, nil
}

// saverPresent reports whether the saver session is there. One whose
// presence can't be read counts as there: it may have watched sessions end,
// so none is marked owed, and a session the user ended never comes back on a
// doubt.
func (p *pass) saverPresent(ctx context.Context) bool {
	present, err := p.Saver.Present(ctx)
	if err != nil {
		p.logger.Warn("step failed", "step", stepEnsureSaver, "error", fmt.Errorf("tell whether a saver ran: %w", err))
		return true
	}
	return present
}

// judgeMade judges the saver EnsureSaver made: one Portal can't use is ended,
// first by the zmx that made it, and fatal; one gone at once is a saver down,
// the restore left unfinished, as it is by one that answers neither protocol.
func (p *pass) judgeMade(ctx context.Context) *FatalError {
	unusable, err := p.judge(ctx)
	switch {
	case errors.Is(err, ErrSaverGone):
		p.warn(stepEnsureSaver, SaverDownWarning(), err)
		p.leaveUnfinished(stepEnsureSaver, UnjudgedWarning(), err)
	case err != nil:
		p.leaveUnfinished(stepEnsureSaver, UnjudgedWarning(), err)
	case unusable != nil:
		p.endMade(ctx, unusable)
		return unusable
	default:
		p.judged = true
	}
	return nil
}

// judge is decision 63's judgement of the saver session's daemon: nil for
// one that speaks Portal's protocol; the fatal for one Portal can't use, that
// speaks another or answers an older zmx's framing; and an error for one that
// answers neither.
func (p *pass) judge(ctx context.Context) (*FatalError, error) {
	speaks, err := p.Saver.Protocol(ctx)
	switch {
	case errors.Is(err, ErrOlderZmx):
		return NewFatal(OlderZmxMessage, err), nil
	case err != nil:
		return nil, fmt.Errorf("judge the saver's zmx protocol: %w", err)
	case speaks != p.Protocol:
		return NewFatal(fmt.Sprintf("Portal can't use this zmx: its sessions speak protocol %d, and Portal speaks %d. Upgrade Portal, or install the zmx release it supports.", speaks, p.Protocol),
			fmt.Errorf("the saver's daemon speaks zmx protocol %d, not %d", speaks, p.Protocol)), nil
	}
	return nil, nil
}

// endMade ends the saver the pass made that Portal can't use.
func (p *pass) endMade(ctx context.Context, unusable *FatalError) {
	p.endWith(ctx, unusable, p.Saver.EndMade)
}

// endWith ends the saver the pass made by end, recording in unusable's cause
// whether it went.
func (p *pass) endWith(ctx context.Context, unusable *FatalError, end func(context.Context) error) {
	if err := end(ctx); err != nil {
		unusable.Cause = fmt.Errorf("%w; Portal couldn't end the saver it made on it, which is left: %w", unusable.Cause, err)
		return
	}
	unusable.Cause = fmt.Errorf("%w; Portal ended the saver it made on it", unusable.Cause)
}

// saverWarning words the cause of a saver that isn't running this binary's
// daemon.
func saverWarning(err error) Warning {
	switch {
	case errors.Is(err, saverhost.ErrUnread):
		return SaverUnreadWarning()
	case errors.Is(err, saverhost.ErrNotReplaced):
		return SaverNotReplacedWarning()
	}
	return SaverDownWarning()
}

// restore raises the restoring flag just before restore's first create, the
// saver judged and the owed marks written, and lowers it once restore
// returns, finished or cut short, when none of its creates is in flight. It
// leaves the restore unfinished when the backend can't be listed or ctx ends
// while it runs: the sessions it didn't make are still owed.
func (p *pass) restore(ctx context.Context) *FatalError {
	if err := p.Restoring.Raise(); err != nil {
		return NewFatal("Portal failed to raise the restoring flag: "+err.Error(), err)
	}
	if sink, ok := p.Restore.(RestoreProgressSink); ok && p.emit != nil {
		sink.SetProgress(func(n, m int) {
			p.progress(StepEvent{Index: indexRestore, Name: stepRestore, RestoreN: n, RestoreM: m})
		})
	}
	res := p.Restore.Restore(ctx)
	switch {
	case res.IndexErr != nil:
		p.warnings = append(p.warnings, CorruptSessionsJSONWarning())
	case res.ListErr != nil:
		p.warnings = append(p.warnings, SessionsUnlistedWarning())
		p.unfinished = true
	}
	if res.Failed > 0 {
		p.warnings = append(p.warnings, UnrestoredWarning(res.Failed))
	}
	if ctx.Err() != nil {
		p.unfinished = true
	}
	if _, err := p.Restoring.Lower(); err != nil {
		return lowerFatal(err)
	}
	return UnsupportedCreate(res.Unsupported)
}

func lowerFatal(err error) *FatalError {
	return NewFatal("Portal failed to lower the restoring flag: "+err.Error(), err)
}

// writeLatch must stay the last step: the latch has to land before the
// concurrent route's terminal Done event, so a present latch always means a
// bootstrap that ran to completion. A latch it can't write is warned of:
// every command will bootstrap until one lands.
func (p *pass) writeLatch(context.Context) *FatalError {
	err := p.Latch.WriteLatch()
	switch {
	case errors.Is(err, ErrNoBootID):
		p.warn(stepWriteLatch, BootIDUnreadWarning(), err)
	case err != nil:
		p.warn(stepWriteLatch, LatchUnwrittenWarning(), err)
	}
	return nil
}
