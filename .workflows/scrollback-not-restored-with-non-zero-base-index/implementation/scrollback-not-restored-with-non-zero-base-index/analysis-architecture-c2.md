AGENT: architecture
STATUS: clean
FINDINGS_COUNT: 0

SUMMARY: Architecture has converged; no remaining material gaps.

Cycle 1's three actionable findings are resolved:
- `tmux.HydrationTriggerEvents` is exported and consumed directly from both test files, eliminating cross-package shadow drift risk (T3-1).
- `RegisterPortalHooks(c, log)` is the single nil-tolerant entry point. The parallel WithLogger form is gone (T3-5).
- `migrateHydrationHooks` is unexported and sealed inside `RegisterPortalHooks`. Tests exercise it through the canonical entry point with a recording logger (T3-6).

The discarded MigrationLogger finding remains a stylistic preference — `*state.Logger` satisfies the two-method interface structurally, the package boundary stays clean (no `internal/state` import from `internal/tmux`), and a no-op sink fallback removes the nil-pointer footgun.

Seam quality is good across the implementation:
- `internal/restore/session.go` threads one live `[]tmux.PaneCoord` from a single `list-panes` re-query through arm → geometry → markers, removing prediction entirely.
- `bootstrapadapter.HookRegistrar` forwards `*state.Logger` to `tmux.RegisterPortalHooks` via structural satisfaction without leaking the state package.
- The eviction migration composes existing primitives rather than introducing new client surface.
- The `--` separator is pinned by layered tests: constant-shape unit test, cobra `Execute()` positive + negative cases (FIFO byte assertion now included per T3-4), real-tmux migration round-trip, and binary-driven reboot round-trip.
- Deletion of PredictLiveIndices/warnOnPaneKeyDrift/flattenSavedPanePositions/readIndexOption is complete.

Convergence achieved with no remaining material gaps.
