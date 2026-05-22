TASK: Remove redundant MigrationLogger noop fallback (killed-session-resurrects-within-tick-window-2-7)

ACCEPTANCE CRITERIA:
- `noopMigrationLogger` no longer exists.
- Either `MigrationLogger` retained with `(*state.Logger)(nil)` no-op callers OR `MigrationLogger` deleted with `*state.Logger` consumed directly.
- `go build ./...` succeeds with no new import cycles.
- `go test ./internal/tmux/...` passes.

STATUS: Complete

SPEC CONTEXT: Phase-2 architectural cleanup from cycle 1. `MigrationLogger` was introduced earlier inside `internal/tmux` to give migration code a logging seam without forcing `internal/state` import. A separate `noopMigrationLogger` fallback was reinvented locally, redundant with `*state.Logger`'s already-documented nil-receiver no-op. Cleanup picks lighter option: keep interface (preserves test seam value via `recordingMigrationLogger`) and drop redundant noop type, substituting `(*state.Logger)(nil)` at default-construction sites.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tmux/hooks_register.go:182` (interface retained), `:234` (`migrateHydrationHooks` nil-guard substitutes `(*state.Logger)(nil)`), `:374` (`RegisterPortalHooks` nil-guard substitutes `(*state.Logger)(nil)`). Nil-receiver no-op verified in `internal/state/logger.go:247-250` (`write` early-return on `l == nil`).
- Notes: `noopMigrationLogger` confirmed removed (grep shows only unrelated `noopBarrierLogger`). Production wiring via `internal/bootstrapadapter/adapters.go:83-87` passes `*state.Logger` directly — nil-tolerant end-to-end.

TESTS:
- Status: Adequate
- Coverage: `hooks_register_test.go` exercises both arms — nil `MigrationLogger` across ~20 call sites and `recordingMigrationLogger` seam at lines 939, 1019, 1022. `hooks_migration_test.go` further exercises the recording seam.
- Notes: No new tests required per task spec.

CODE QUALITY:
- Project conventions: Followed. Small-interface seam pattern (2 methods). Comment block on `MigrationLogger` explains structural-satisfaction rationale and nil-handling contract.
- SOLID: Good. ISP — minimal 2-method surface. DIP — production code depends on abstraction.
- Complexity: Low. Two single-line guards `if log == nil { log = (*state.Logger)(nil) }` replace previous parallel no-op type.
- Modern idioms: Typed-nil cast is idiomatic Go when concrete type documents nil-receiver semantics.
- Readability: Good. Comments at seam declaration and at adapter both call out nil-toleration contract.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] `migrateSessionClosedHook` has no internal `if log == nil` guard, unlike `migrateHydrationHooks`. Safe (caller `RegisterPortalHooks` normalises nil) but asymmetric. Add symmetric guard or precondition doc-line. (Also flagged in report-1-4.)
- [idea] `MigrationLogger` interface shape duplicates `BarrierLogger` and bootstrap `Logger`. Future consolidation possible but wider than this task's scope.
