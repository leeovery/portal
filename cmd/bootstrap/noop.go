package bootstrap

import "context"

// The fatal step (RestoringMarker) deliberately has no NoOp form: defaulting
// it would silently violate the bootstrap contract.

type NoOpOrphanSweeper struct{}

func (NoOpOrphanSweeper) SweepOrphanDaemons(context.Context) error { return nil }

type NoOpSaver struct{}

func (NoOpSaver) EnsureSaver(context.Context) error { return nil }

type NoOpRestorer struct{}

func (NoOpRestorer) Restore() (bool, error) { return false, nil }
