package bootstrap

// Progress-emitter seam for the §10.2 concurrent cold-boot route.
//
// On the cold + TUI path the orchestrator runs in a goroutine and streams a
// per-step progress event to the loading-page TUI. The seam is threaded through
// the context (rather than an Orchestrator field) for the lowest-risk wiring:
//   - Run's signature is untouched (it already accepts a ctx).
//   - The synchronous warm/CLI path passes a context WITHOUT an emitter, so the
//     read resolves to nil and every emit is a no-op — the synchronous route is
//     byte-for-byte unchanged.
//   - The concurrent route wraps the context via WithProgressEmitter; only that
//     route observes any emit.
//
// Each of the eleven steps emits its StepEvent at the SAME site it logs "step
// complete", in step order — a fatal step that aborts emits no event for the
// aborting step (mirroring the no-step-complete-log contract) and nothing after.

import "context"

// StepEvent is the per-step progress signal streamed on the concurrent route.
// Index is the 1-based canonical step number (1..11); Name is the closed
// StepName for that step (the step* consts — the same identifier the
// step-complete log line carries). The cmd layer maps these onto the
// loading-page channel event (friendly-label grouping is task 5-4).
//
// RestoreN / RestoreM carry the §10.4 "Restoring sessions (N/M)" per-session
// counter on the restore-progress flavour of the event (Index 6 / Name
// "Restore"): the restore per-session loop is the one real per-item progress
// source in the whole bootstrap (task 5-3). They are zero on every step-complete
// event; a restore-progress event sets RestoreM > 0 (M = len(idx.Sessions)) and
// RestoreN in 1..M. The cmd layer forwards them onto the loading-page channel
// (task 5-4 maps them to the suppressible (N/M) display).
type StepEvent struct {
	Index    int
	Name     string
	RestoreN int
	RestoreM int
}

// ProgressEmitter is invoked once per completed bootstrap step on the
// concurrent cold-boot route. It must be non-blocking from the orchestrator's
// perspective — the cmd-layer receiver sends on a buffered channel so a fast
// orchestrator never stalls on a slow render.
type ProgressEmitter func(StepEvent)

// progressEmitterKey is the unexported context key for the ProgressEmitter.
type progressEmitterKey struct{}

// WithProgressEmitter returns a context carrying the emitter so Run streams a
// StepEvent per completed step. The concurrent cold-boot route (cmd/open.go)
// wires this; the synchronous route never calls it, leaving the emitter nil.
func WithProgressEmitter(ctx context.Context, emit ProgressEmitter) context.Context {
	return context.WithValue(ctx, progressEmitterKey{}, emit)
}

// progressEmitterFromContext extracts the emitter wired by WithProgressEmitter.
// Returns nil on the synchronous route (no emitter in the context), which the
// step sites treat as a no-op.
func progressEmitterFromContext(ctx context.Context) ProgressEmitter {
	if ctx == nil {
		return nil
	}
	emit, _ := ctx.Value(progressEmitterKey{}).(ProgressEmitter)
	return emit
}

// ProgressEmitterFromContextForTest exposes progressEmitterFromContext so the
// cmd-layer pipe tests can drive the same context-carried emitter the
// orchestrator reads — verifying the cmd-side goroutine wraps the seam exactly
// as Run consumes it. Production code must use WithProgressEmitter + Run.
func ProgressEmitterFromContextForTest(ctx context.Context) ProgressEmitter {
	return progressEmitterFromContext(ctx)
}
