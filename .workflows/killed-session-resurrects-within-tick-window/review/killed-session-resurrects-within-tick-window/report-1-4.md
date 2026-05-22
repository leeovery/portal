TASK: Migrate `session-closed` hook registration to `commitNowCommand` (killed-session-resurrects-within-tick-window-1-4)

ACCEPTANCE CRITERIA:
- Empty hook state: registers commitNowCommand on session-closed.
- Pre-fix install with notifyCommand on session-closed: stale entry removed, commitNowCommand appended exactly once (eviction precedes append).
- Post-fix install: no-op idempotent.
- ShowGlobalHooks failure: migration aborts with wrapped error, WARN logged.
- Per-index UnsetGlobalHookAt failure: best-effort WARN, loop continues, follow-up append still attempted.
- Exact-string match must not remove user-customised notify-referencing hooks.
- Descending-index eviction order.

STATUS: Complete

SPEC CONTEXT:
§ Hook Registration Migration mandates (1) show-hooks → filter, (2) exact-string match on the historical notifyCommand literal with descending-index unset, (3) post-eviction exact-string check before AppendGlobalHook. Plan acceptance requires zero duplicates and zero notifyCommand entries on session-closed after migration, plus idempotency.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tmux/hooks_register.go`
  - `commitNowCommand` constant: lines 51-61
  - `sessionClosedEvent` constant: line 164
  - `migrateSessionClosedHook`: lines 304-343
  - Routing inside `RegisterPortalHooks`: lines 383-395
- Notes:
  - Exact-string match via `switch entry.Command` against `notifyCommand`/`commitNowCommand` literals (not substring/regex) — preserves user-customised hooks.
  - Descending-index unset via `sort.Reverse(sort.IntSlice(staleIndices))`.
  - Post-eviction commitNow-present check computed during pre-removal scan (lines 314-321) — deliberate optimisation (no second show-hooks round-trip).
  - On ShowGlobalHooks failure: WARN emitted AND error returned; folded into `errors.Join` aggregate.

TESTS:
- Status: Adequate
- Coverage: `internal/tmux/hooks_register_test.go` lines 715-1146 — `TestRegisterPortalHooks_SessionClosedMigration` 10 sub-tests:
  - empty state appends commitNowCommand (line 716)
  - six non-session-closed events get notifyCommand (line 743)
  - stale notifyCommand removed (line 785)
  - eviction precedes append (line 813)
  - post-fix no-op (line 846)
  - user-customised hook preserved (line 870)
  - ShowGlobalHooks failure aborts session-closed leg, six others still processed, WARN (line 908)
  - per-index UnsetGlobalHookAt failure WARN+continue (line 988)
  - stateful idempotency across two bootstraps (line 1058)
  - descending-index order for multi-entry eviction (line 1115)
- Cross-routing safeguard: `internal/tmux/hooks_register_six_event_routing_test.go` pins per-event routing.
- Notes: Each sub-test maps to a distinct plan edge case. Mock-only (correct — per-index UnsetGlobalHookAt failure can't be injected against real tmux).

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`, `%w` wrapping, structured logger via `state.ComponentBootstrap`.
- SOLID: Single responsibility. `MigrationLogger` interface (2 methods) minimal.
- Complexity: Low. ~40-line function, two linear scans.
- Modern idioms: `switch entry.Command`, `sort.Reverse(sort.IntSlice(...))`, `errors.Join`.
- Readability: Good. Docstring (lines 272-303) cites spec section and explains why exact-match is load-bearing.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] `migrateSessionClosedHook` lacks the `if log == nil { log = (*state.Logger)(nil) }` guard that `migrateHydrationHooks` has. Safe in practice (only caller substitutes nil at entry) but asymmetric. Add guard for sibling-symmetry or document precondition.
- [idea] ShowGlobalHooks failure emits WARN AND returns wrapped error here, whereas `migrateHydrationHooks` only wraps+returns. Both defensible; converge or document.
