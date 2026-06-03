---
topic: state-notify-cascade-on-binary-upgrade
cycle: 4
total_proposed: 1
---
# Analysis Tasks: state-notify-cascade-on-binary-upgrade (Cycle 4)

## Task 1: Route the three teardown tests' inline dispatch RunFuncs through perEventDispatchWithFaults
status: pending
severity: low
sources: duplication

**Problem**: Three teardown tests in `internal/tmux` hand-roll the per-event `show-hooks -g <event>` read / `set-hook -gu` dispatch skeleton inline as bespoke `RunFunc` literals, duplicating ~30 lines of dispatch wiring the package already centralises in `perEventDispatchWithFaults` (`internal/tmux/hooks_register_test.go:116`) and its teardown shim `dispatchUnregisterHooks` (`internal/tmux/hooks_unregister_test.go:28`). Beyond the duplication, each inline `RunFunc` carries only a generic catch-all `t.Fatalf` and therefore bypasses the shared helper's load-bearing no-arg-global-read tripwire — the exact guard `dispatchUnregisterHooks`' docstring advertises as protecting the teardown path from silently reverting to a blind no-arg `show-hooks -g` read. This is copy-paste drift across the register/teardown test boundary: the register-side warn suite already routes these identical scenarios through the shared dispatcher (`hooks_register_warn_test.go:90`, `:214`), the teardown-side suite does not. It is the residual pocket of the test-harness-consolidation theme — Task 4-2 consolidated the dispatch/line-scoping but explicitly left these three bespoke inline RunFuncs out of scope.

The three sites, all confirmed on disk:
- `internal/tmux/hooks_unregister_test.go:203-211` — "every per-event read fails" (fail-every-read).
- `internal/tmux/hooks_unregister_test.go:237-251` — "single-event read failure folds" (per-event read fault on `pane-focus-out`, calls `linesForEvent` for the readable events).
- `internal/tmux/hooks_unregister_warn_test.go:29-37` — "every per-event read fails" (fail-every-read).

**Solution**: Replace the three inline `RunFunc` literals with calls to the existing shared helpers — no new abstraction. Use the read-fault channel (`readErrFor` parameter) of `perEventDispatchWithFaults` (signature: `perEventDispatchWithFaults(t *testing.T, seededTable string, setHookErrFor, readErrFor, unsetErrFor map[string]error)`), mirroring the register-side warn tests. This collapses the divergent dispatch wiring onto one definition and gives all three teardown tests the helper's no-arg-global-read `t.Fatalf` guard for free.

**Outcome**: The three teardown tests no longer hand-roll dispatch wiring; all per-event read/unset dispatch in the teardown suite flows through the single `perEventDispatchWithFaults` owner. The no-arg-global-read tripwire now protects the teardown path, so a regression that reverts to the blind no-arg `show-hooks -g` read fails loudly instead of passing silently. Register-side and teardown-side warn suites express the identical fail-every-read / single-event-read-fault scenarios through the same shared dispatcher, eliminating the register/teardown copy-paste drift. The package compiles and `go test ./internal/tmux/...` passes with behaviour-preserving results (same assertions, same observed `set-hook -gu` calls, same aggregate error and WARN expectations).

**Do**:
1. `internal/tmux/hooks_unregister_test.go:203-211` (the "every per-event read fails" case inside `TestUnregisterPortalHooks`): replace the inline `RunFunc: func(args ...string) ...` literal with `RunFunc: perEventDispatchWithFaults(t, "", nil, readErrForAllManagedEvents(sentinel), nil)`. Keep the surrounding `sentinel`, `client`, and all assertions (`errors.Is(err, sentinel)`, the `show-hooks failed` wrap substring, the zero-removals check) unchanged.
2. `internal/tmux/hooks_unregister_test.go:237-251` (the "folds a single-event read failure" case): replace the inline `RunFunc` literal with `RunFunc: perEventDispatchWithFaults(t, raw, nil, map[string]error{"pane-focus-out": sentinel}, nil)`. Keep the existing `raw` seeded table and all assertions unchanged. The shared helper performs the same per-event line scoping the removed inline literal did via `linesForEvent`, so the `pane-focus-out` read returns the sentinel while every other event reads its scoped lines.
3. `internal/tmux/hooks_unregister_warn_test.go:29-37` (inside `TestUnregisterPortalHooks_ShowHooksFailureEmitsCanonicalWarn`): replace the inline `RunFunc` literal with `RunFunc: perEventDispatchWithFaults(t, "", nil, readErrForAllManagedEvents(sentinel), nil)`. Keep the `recordingSlogHandler`, the `UnregisterPortalHooksWithLogger` call, and the one-WARN-per-teardown-event assertions unchanged.
4. If a leftover helper (e.g. `linesForEvent`) becomes unused after the edits, leave it only if still referenced elsewhere; otherwise remove it to avoid a dead-code/unused-import compile failure. Verify imports (`errors`, `strings`, `slog`) are still all used in each edited file and prune any that the removed literals were the sole users of.
5. Do NOT introduce a new `readErrFor` parameter on `dispatchUnregisterHooks` — calling `perEventDispatchWithFaults` directly (the read-fault channel) is sufficient and avoids widening the teardown shim's surface. Prefer the direct call.

**Acceptance Criteria**:
- All three cited inline `RunFunc` literals (`hooks_unregister_test.go:203-211`, `hooks_unregister_test.go:237-251`, `hooks_unregister_warn_test.go:29-37`) are gone, replaced by `perEventDispatchWithFaults` calls.
- The two fail-every-read sites use the read-fault channel via `readErrForAllManagedEvents(sentinel)`; the single-event fold site uses `map[string]error{"pane-focus-out": sentinel}` with the existing `raw` seeded table.
- All three tests inherit the shared helper's no-arg-global-read `t.Fatalf` guard (no remaining generic catch-all `t.Fatalf` standing in for the per-event tripwire in these three sites).
- No production code under `internal/tmux/` is modified — the change is test-only.
- `go build ./...` succeeds; no unused-import or unused-variable compile errors in the edited test files.
- No `t.Parallel()` is introduced.

**Tests**:
- `go test ./internal/tmux/...` passes — specifically `TestUnregisterPortalHooks` (both the "every per-event read fails" and "folds a single-event read failure" sub-tests) and `TestUnregisterPortalHooks_ShowHooksFailureEmitsCanonicalWarn` continue to pass with identical assertions (aggregate error wraps the sentinel, error contains the `show-hooks failed` wrap, zero/correct `set-hook -gu` removals, one canonical `show-hooks failed` WARN per teardown event with `error_class=unexpected` / `component=bootstrap`).
- No new test is required; this is a behaviour-preserving harness consolidation. Confirm by running the package suite before and after and observing identical pass results.
