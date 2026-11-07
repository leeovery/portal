package bootstrap

// The emitter rides the context rather than an Orchestrator field, so Run's
// signature is untouched and the synchronous route simply resolves it to nil.

import "context"

// StepEvent is the per-step progress signal on the concurrent cold-boot route.
// Index is 1-based. RestoreN and RestoreM are zero except on the restore step's
// per-session events, which set RestoreM > 0 and RestoreN in 1..M. Waiting
// marks the one event, before any step, sent once a wait for bootstrap.lock
// has lasted past Locked's notice.
type StepEvent struct {
	Index    int
	Name     string
	RestoreN int
	RestoreM int
	Waiting  bool
}

// ProgressEmitter is invoked once per completed bootstrap step, and once on a
// long wait for another bootstrap. It must not block the orchestrator.
type ProgressEmitter func(StepEvent)

type progressEmitterKey struct{}

func WithProgressEmitter(ctx context.Context, emit ProgressEmitter) context.Context {
	return context.WithValue(ctx, progressEmitterKey{}, emit)
}

func progressEmitterFromContext(ctx context.Context) ProgressEmitter {
	if ctx == nil {
		return nil
	}
	emit, _ := ctx.Value(progressEmitterKey{}).(ProgressEmitter)
	return emit
}

func ProgressEmitterFromContextForTest(ctx context.Context) ProgressEmitter {
	return progressEmitterFromContext(ctx)
}
