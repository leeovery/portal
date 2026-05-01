# Plan: Scrollback Not Restored With Non-Zero Base Index

## Phases

### Phase 1: Fix signal-hydrate Argv Parse and Migrate Existing Hook Installations
status: approved
approved_at: 2026-05-01

**Goal**: Restore scrollback hydration for sessions whose names begin with `-` by adding the `--` end-of-flags separator to the `signal-hydrate` hook command, tightening the dedupe substring, and migrating any pre-existing un-separated hook entries on bootstrap.

**Why this order**: This phase delivers the actual user-facing fix — without it, leading-dash sessions remain broken regardless of any other change. It establishes the corrected hook contract that all subsequent verification depends on, and leaves the system fully working before any dead-code excision begins.

**Acceptance**:
- [ ] `signalHydrateCommand` in `internal/tmux/hooks_register.go` contains `portal state signal-hydrate -- #{session_name}` (with the `--` separator before the format token).
- [ ] `signalHydrateSubstring` is tightened to `"portal state signal-hydrate --"` so the migration distinguishes fixed from broken entries.
- [ ] `RegisterPortalHooks` evicts pre-existing hook entries matching the eviction predicate (command contains both `command -v portal >/dev/null 2>&1 &&` AND `portal state signal-hydrate`, and does NOT contain `portal state signal-hydrate --`) before installing the fixed entry, scanning every event listed in `hydrationTriggerEvents`.
- [ ] Eviction processes indices highest-first and is best-effort: per-index `UnsetGlobalHookAt` failures emit `WARN | bootstrap | failed to evict stale signal-hydrate hook on <event> at index <i>: <err>` and do not abort bootstrap.
- [ ] When at least one entry is evicted on a bootstrap, a single `INFO | bootstrap | evicted N stale signal-hydrate hook(s) lacking '--' separator` line is written to `portal.log`; bootstraps with no evictions emit no INFO line.
- [ ] After bootstrap, for each event in `hydrationTriggerEvents` the count of hook entries containing `portal state signal-hydrate` is exactly 1, and is unchanged across two consecutive back-to-back bootstraps.
- [ ] Cobra-level argv parse test exercises `runSignalHydrate` via the cobra `Execute()` path with a leading-dash session name and asserts exit 0 plus a FIFO byte written.
- [ ] Reboot round-trip integration test using a leading-dash session name runs against a real tmux server (via `internal/tmuxtest` socket fixture) and confirms scrollback replays end-to-end with no `hydrate timeout` WARN in `portal.log`.
- [ ] Hook content unit test asserts `signalHydrateCommand` includes the `--` separator before `#{session_name}`.
- [ ] Migration test (preferring real-tmux socket fixture) verifies eviction-then-install behaviour and idempotent no-op on a second invocation.
- [ ] Sessions whose names do not start with `-` continue to restore and hydrate as before; pane-count mismatch logging at `armPanes:202` is preserved.
- [ ] No test uses `t.Parallel()`; existing mock injection patterns (`bootstrapDeps` etc.) are respected.

#### Tasks
status: approved
approved_at: 2026-05-01

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| scrollback-not-restored-with-non-zero-base-index-1-1 | Add `--` Separator to signal-hydrate Hook Command | leading-dash session name, internal-dash-only session name, future regression of the constant losing the `--` |
| scrollback-not-restored-with-non-zero-base-index-1-2 | Migrate Pre-Existing Un-Separated Hook Entries on Bootstrap | zero pre-existing entries (silent no-op), one stale entry per event, multiple stale entries on same event (index-shift safety), `UnsetGlobalHookAt` partial failure, hand-authored user hooks lacking `command -v portal` prefix must not be evicted, back-to-back bootstraps idempotent, `hydrationTriggerEvents` slice extended later |
| scrollback-not-restored-with-non-zero-base-index-1-3 | Reboot Round-Trip Integration Test with Leading-Dash Session Name | non-zero `base-index` / `pane-base-index` on test server, isolated test socket only (developer's primary server untouched), regression check that non-dash session names still hydrate |

### Phase 2: Delete PredictLiveIndices and the Misleading Drift Diagnostic
status: approved
approved_at: 2026-05-01

**Goal**: Remove the dead diagnostic prediction path (`PredictLiveIndices`, `readIndexOption` if unused, `flattenSavedPanePositions`, `warnOnPaneKeyDrift`, and its call site) so the misleading `predicted=...__0.0 live=...__X.Y` WARN can never fire under any tmux config.

**Why this order**: Sequenced after Phase 1 because Phase 1 delivers the real fix; Phase 2 is pure dead-code excision. With hydration already correct, removing the diagnostic carries no risk of masking a live failure mode. Independently valuable but lower priority than the user-visible bug, and cleanly separable so a regression in one phase cannot contaminate the other.

**Acceptance**:
- [ ] Pre-deletion repo-wide grep confirms zero remaining references to `PredictLiveIndices`, `warnOnPaneKeyDrift`, `flattenSavedPanePositions`, and `readIndexOption` outside the deletion list; any unexpected reference is surfaced for review rather than silently deleted.
- [ ] `internal/restore/session.go::PredictLiveIndices` is removed.
- [ ] `internal/restore/session.go::flattenSavedPanePositions` is removed.
- [ ] `internal/restore/session.go::readIndexOption` is removed if it has no remaining callers after the above deletions.
- [ ] `internal/restore/restore.go::Orchestrator.warnOnPaneKeyDrift` is removed.
- [ ] The call site in the orchestrator's restore loop that invoked `warnOnPaneKeyDrift` is removed.
- [ ] `_test.go` audit across the whole repo (including `internal/restore/session_test.go`, `internal/restore/restore_test.go`, and `cmd/bootstrap/`) removes or refactors any tests, mocks, or fixtures referencing the deleted symbols; no dead test scaffolding remains.
- [ ] Reboot round-trip integration test, run with non-zero `base-index` and `pane-base-index` on the test tmux server, confirms `portal.log` contains zero lines matching the regex `predicted=.*__\d+\.\d+ live=.*__\d+\.\d+` after bootstrap completes.
- [ ] `go build ./...` and `go test ./...` pass with no compilation errors and no failing tests.
- [ ] Live-index path is unchanged; pane-count mismatch logging at `armPanes:202` is preserved.

#### Tasks
status: approved
approved_at: 2026-05-01

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| scrollback-not-restored-with-non-zero-base-index-2-1 | Excise Diagnostic Prediction Path and Audit Test Scaffolding | unexpected reference outside deletion list (surface for review, do not silently delete), `readIndexOption` retains a caller after other deletions (keep it), test helpers/fixtures in `cmd/bootstrap/` still referencing deleted symbols, pane-count mismatch logging at `armPanes:202` must remain intact, live-index path untouched |
| scrollback-not-restored-with-non-zero-base-index-2-2 | Regression Assertion: No predicted-vs-live WARN Under Non-Zero base-index | isolated tmux socket only (developer's primary server untouched), session name without leading dash (orthogonal to Phase 1's leading-dash test), regex must not false-positive on unrelated diagnostic lines, assertion runs after bootstrap completes |
