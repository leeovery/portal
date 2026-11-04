// Package bootstrap runs the bootstrap sequence invoked from root's
// PersistentPreRunE. Step ordering is load-bearing.
package bootstrap

import (
	"context"
	"log/slog"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
)

// Deliberately a second component in this package: the taxonomy files the
// orphan sweep's kills under clean, beside the other sweeps.
var cleanLogger = log.For("clean")

const totalSteps = 5

const (
	stepSetRestoring       = "SetRestoring"
	stepSweepOrphanDaemons = "SweepOrphanDaemons"
	stepEnsureSaver        = "EnsureSaver"
	stepRestore            = "Restore"
	stepClearRestoring     = "ClearRestoring"
)

// The emitted step indices keep the loading page's numbering (internal/tui's
// stepLabelTable), whose per-session restore events are keyed to step 6.
const (
	indexSetRestoring       = 3
	indexSweepOrphanDaemons = 4
	indexEnsureSaver        = 5
	indexRestore            = 6
	indexClearRestoring     = 8
)

// Runner abstracts the bootstrap run so PersistentPreRunE need not import the
// concrete *Orchestrator.
type Runner interface {
	Run(ctx context.Context) (bool, []Warning, error)
}

// RestoringMarker manages the @portal-restoring server option that
// suppresses the save daemon while skeleton restoration is in flight.
type RestoringMarker interface {
	Set() error
	Clear() error
}

// SaverBootstrapper ensures the _portal-saver detached session exists
// and matches the current binary version.
type SaverBootstrapper interface {
	EnsureSaver() error
}

// Restorer performs session restoration. err is non-nil only when corrupt is
// true; per-session failures are logged and swallowed inside the
// implementation.
type Restorer interface {
	Restore() (corrupt bool, err error)
}

// RestoreProgressSink is the optional seam a Restorer may also satisfy. The
// restore step calls SetProgress only when a progress emitter is wired.
type RestoreProgressSink interface {
	SetProgress(fn func(n, m int))
}

// LatchWriter records the version-stamped @portal-bootstrapped latch as the
// final action of a successful bootstrap. Nil-tolerant, and best-effort.
type LatchWriter interface {
	SetServerOption(name, value string) error
}

// Orchestrator runs the bootstrap sequence. A nil Logger is tolerated — Run
// substitutes a discard sink.
type Orchestrator struct {
	Restoring     RestoringMarker
	OrphanSweeper OrphanSweeper
	Saver         SaverBootstrapper
	Restore       Restorer
	Latch         LatchWriter
	Version       string
	Logger        *slog.Logger
}

// Run executes the bootstrap steps in order and returns the soft warnings in
// step order. It starts no server, so its started report is always false. Only
// the @portal-restoring steps are fatal; every other step logs its failure and
// continues.
func (o *Orchestrator) Run(ctx context.Context) (bool, []Warning, error) {
	emit := progressEmitterFromContext(ctx)
	emitStep := func(index int, name string) {
		if emit != nil {
			emit(StepEvent{Index: index, Name: name})
		}
	}

	o.Logger = log.OrDiscard(o.Logger)

	orchestrationStart := time.Now()

	var warnings []Warning

	// Must precede the orphan sweep and the saver: the daemon and the hydrate
	// helpers key their suppression window off this marker.
	o.Logger.Debug("step entering", "step", stepSetRestoring)
	stepStart := time.Now()
	if err := o.Restoring.Set(); err != nil {
		return false, nil, o.fatalf("set @portal-restoring marker", err)
	}
	o.Logger.Info("step complete", "step", stepSetRestoring, log.Took(stepStart))
	emitStep(indexSetRestoring, stepSetRestoring)

	// Must precede EnsureSaver so the new saver-pane daemon's first tick is
	// uncontested by leftover daemons from prior server lifetimes.
	o.Logger.Debug("step entering", "step", stepSweepOrphanDaemons)
	stepStart = time.Now()
	if err := o.OrphanSweeper.SweepOrphanDaemons(); err != nil {
		o.Logger.Warn("step failed", "step", stepSweepOrphanDaemons, "error", err)
	}
	o.Logger.Info("step complete", "step", stepSweepOrphanDaemons, log.Took(stepStart))
	emitStep(indexSweepOrphanDaemons, stepSweepOrphanDaemons)

	o.Logger.Debug("step entering", "step", stepEnsureSaver)
	stepStart = time.Now()
	if err := o.Saver.EnsureSaver(); err != nil {
		warnings = append(warnings, SaverDownWarning())
		o.Logger.Warn("step failed", "step", stepEnsureSaver, "error", err)
	}
	o.Logger.Info("step complete", "step", stepEnsureSaver, log.Took(stepStart))
	emitStep(indexEnsureSaver, stepEnsureSaver)

	o.Logger.Debug("step entering", "step", stepRestore)
	stepStart = time.Now()
	if emit != nil {
		if sink, ok := o.Restore.(RestoreProgressSink); ok {
			sink.SetProgress(func(n, m int) {
				emit(StepEvent{Index: indexRestore, Name: stepRestore, RestoreN: n, RestoreM: m})
			})
		}
	}
	corrupt, restoreErr := o.Restore.Restore()
	switch {
	case corrupt:
		warnings = append(warnings, CorruptSessionsJSONWarning())
		if restoreErr != nil {
			o.Logger.Warn("step failed: corrupt sessions.json", "step", stepRestore, "error", restoreErr)
		}
	case restoreErr != nil:
		// Contract violation (corrupt=false with a non-nil err): swallowed so
		// restore can never escalate to a bootstrap abort.
		o.Logger.Warn("step returned non-corrupt error (treated as soft per Restorer contract)", "step", stepRestore, "error", restoreErr)
	}
	o.Logger.Info("step complete", "step", stepRestore, log.Took(stepStart))
	emitStep(indexRestore, stepRestore)

	o.Logger.Debug("step entering", "step", stepClearRestoring)
	stepStart = time.Now()
	if err := o.Restoring.Clear(); err != nil {
		return false, warnings, o.fatalf("clear @portal-restoring marker", err)
	}
	o.Logger.Info("step complete", "step", stepClearRestoring, log.Took(stepStart))
	emitStep(indexClearRestoring, stepClearRestoring)

	// Must stay Run's last action: the latch has to land before the concurrent
	// route's terminal Done event, so a present latch always means a bootstrap
	// that ran to completion.
	if o.Latch != nil {
		if err := o.Latch.SetServerOption(state.BootstrappedMarkerName, o.Version); err != nil {
			o.Logger.Warn("latch write failed for "+state.BootstrappedMarkerName, "error", err)
		}
	}

	o.Logger.Info("orchestration complete", "steps", totalSteps, "warnings", len(warnings), log.Took(orchestrationStart))
	return false, warnings, nil
}

func (o *Orchestrator) fatal(userMsg string, cause error) error {
	o.Logger.Error(userMsg, "error", cause)
	return NewFatal(userMsg, cause)
}

func (o *Orchestrator) fatalf(verb string, err error) error {
	return o.fatal("Portal failed to "+verb+": "+err.Error(), err)
}
