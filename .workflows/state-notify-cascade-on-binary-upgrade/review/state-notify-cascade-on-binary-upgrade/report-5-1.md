TASK: Route the three teardown tests' inline dispatch RunFuncs through perEventDispatchWithFaults (state-notify-cascade-on-binary-upgrade-5-1)

STATUS: Complete

ACCEPTANCE CRITERIA: all three cited inline RunFunc literals gone, replaced by perEventDispatchWithFaults calls; two fail-every-read sites use readErrForAllManagedEvents(sentinel); single-event fold site uses map{"pane-focus-out": sentinel} with existing raw table; all three inherit the no-arg-global-read t.Fatalf guard; NO production code under internal/tmux/ modified; go build ./... succeeds, no unused-import/var; no t.Parallel; behaviour-preserving.

SPEC CONTEXT: Phase 5 test-only consolidation chore — collapse three bespoke teardown RunFunc literals onto the shared dispatcher so teardown inherits the no-arg-global-read tripwire.

IMPLEMENTATION:
- Status: Implemented
- hooks_unregister_test.go:203-205 → perEventDispatchWithFaults(t, "", nil, readErrForAllManagedEvents(sentinel), nil). :231-233 → perEventDispatchWithFaults(t, raw, nil, map[string]error{"pane-focus-out": sentinel}, nil) with existing raw table. hooks_unregister_warn_test.go:29-31 → perEventDispatchWithFaults(t, "", nil, readErrForAllManagedEvents(sentinel), nil).
- All three inherit the t.Fatalf tripwire (hooks_register_test.go:128-131). dispatchUnregisterHooks retains 3-param signature (NOT widened with readErrFor) — sites bypass it, call helper directly. NO production file modified (hooks_unregister.go untouched). linesForEvent retained (still referenced by stateful idempotent test :334 — pruning would break build). The one remaining inline show-hooks RunFunc (:329-345) is the stateful idempotency test, NOT one of the three cited, legitimately needs bespoke stateful behaviour. No t.Parallel; imports all used.

TESTS:
- Status: Adequate. Behaviour-preserving: aggregate non-nil, errors.Is wraps sentinel, contains "show-hooks failed", zero removals on fail-every-read, session-created[0] still reaped on single-event fold (all-or-nothing gone), exactly one canonical WARN per teardown event via assertShowHooksWarnShape. Migration strengthens coverage (sites now fail loudly on regression to blind no-arg read). readErrForAllManagedEvents derives key set from tmux.ManagedEventNames(). No over-testing.

CODE QUALITY:
- Test-only, external tmux_test package; conventions followed (no t.Parallel, single-owner dispatch helper). DRY improved (~30 lines removed). Complexity: Low. Readability: Good.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
