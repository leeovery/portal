package bootstrap_test

import (
	"context"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
)

type stubRestoringMarker struct{}

func (stubRestoringMarker) Set() error   { return nil }
func (stubRestoringMarker) Clear() error { return nil }

type stubRestorer struct{}

func (stubRestorer) Restore() (bool, error) { return false, nil }

func TestNewWithDefaults_DefaultsAllDegradableStepsToNoOp(t *testing.T) {
	o := bootstrap.NewWithDefaults(nil, stubRestoringMarker{})

	if _, ok := o.OrphanSweeper.(bootstrap.NoOpOrphanSweeper); !ok {
		t.Errorf("OrphanSweeper type = %T; want bootstrap.NoOpOrphanSweeper", o.OrphanSweeper)
	}
	if _, ok := o.Saver.(bootstrap.NoOpSaver); !ok {
		t.Errorf("Saver type = %T; want bootstrap.NoOpSaver", o.Saver)
	}
	if _, ok := o.Restore.(bootstrap.NoOpRestorer); !ok {
		t.Errorf("Restore type = %T; want bootstrap.NoOpRestorer", o.Restore)
	}
}

func TestNewWithDefaults_WiresPositionalSeams(t *testing.T) {
	restoring := stubRestoringMarker{}

	o := bootstrap.NewWithDefaults(nil, restoring)

	if o.Restoring != restoring {
		t.Errorf("Restoring = %v; want stubRestoringMarker{}", o.Restoring)
	}
	if o.Logger != nil {
		t.Errorf("Logger = %v; want nil (preserved verbatim)", o.Logger)
	}
}

func TestNewWithDefaults_HonorsAllWithOptions(t *testing.T) {
	orphanSweeper := stubOrphanSweeper{}
	saver := stubSaver{}
	restore := stubRestorer{}

	o := bootstrap.NewWithDefaults(nil, stubRestoringMarker{},
		bootstrap.WithOrphanSweeper(orphanSweeper),
		bootstrap.WithSaver(saver),
		bootstrap.WithRestore(restore),
	)

	if o.OrphanSweeper != orphanSweeper {
		t.Errorf("OrphanSweeper = %v; want stubOrphanSweeper{}", o.OrphanSweeper)
	}
	if o.Saver != saver {
		t.Errorf("Saver = %v; want stubSaver{}", o.Saver)
	}
	if o.Restore != restore {
		t.Errorf("Restore = %v; want stubRestorer{}", o.Restore)
	}
}

func TestNewWithDefaults_RunCallableSmokeTest(t *testing.T) {
	o := bootstrap.NewWithDefaults(nil, stubRestoringMarker{})

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Errorf("Run returned error: %v; want nil (every default NoOp step succeeds)", err)
	}
}

type stubOrphanSweeper struct{}

func (stubOrphanSweeper) SweepOrphanDaemons() error { return nil }

type stubSaver struct{}

func (stubSaver) EnsureSaver() error { return nil }
