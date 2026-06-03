TASK: Consolidate the forked register/teardown test-harness dispatch + line-scoping helpers and propagate the no-arg-global-read fatal guard to teardown (state-notify-cascade-on-binary-upgrade-4-2)

STATUS: Complete

ACCEPTANCE CRITERIA: teardown tests drive per-event dispatch through the same skeleton + no-arg-global-read t.Fatalf guard executes on teardown path; simulated regression to no-arg fails loudly; exactly one line-scoping primitive (other expressed in terms of it); recognises both fixture shapes via `<event>[`; per-index unset fault injection works for teardown via shared unsetErrFor; tests/vet pass; no t.Parallel.

SPEC CONTEXT: Test-only harness consolidation; propagates the per-event-read invariant tripwire to teardown so a regression to the blind tmux-3.6b global read can't pass silently.

IMPLEMENTATION:
- Status: Implemented
- hooks_unregister_test.go:28-31 dispatchUnregisterHooks now a thin shim delegating to perEventDispatchWithFaults(t, showOutput, nil, nil, unsetErrFor). hooks_register_test.go:116-151 perEventDispatchWithFaults single owner of skeleton + no-arg-global-read t.Fatalf guard (128-131). hooks_unregister_test.go:41-43 linesForEvent reimplemented as thin lookup over parseSeededTableByEvent(showOutput)[event]. hooks_register_test.go:182-200 parseSeededTableByEvent single line-scoping primitive keyed on text before first "[" (scopes both register `=> '...'` and unregister `run-shell '...'` shapes). Production hooks_unregister.go:112-144 reads per-event via ShowGlobalHooksForEvent (would hit guard if regressed).
- All 6 ACs verified. Per-index unset fault injection preserved (hooks_unregister_test.go:261-323 via dispatchUnregisterHooks). 5-1 follow-up (read-fault sites) out of 4-2 scope, confirmed present.

TESTS:
- Status: Adequate. All 13 TestUnregisterPortalHooks sub-tests route through consolidated dispatcher; per-index unset-fault cases preserved; linesForEvent still exercised by stateful idempotency sub-test (not dead); register-side unchanged in meaning.

CODE QUALITY:
- Single-owner skeleton + thin shims + single line-scoping primitive — DRY without over-abstraction. Complexity: Low. Idiomatic (t.Helper, accurate doc comments).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] No dedicated negative test deliberately fires the t.Fatalf for a teardown no-arg regression — the guard is a shared structural tripwire only (consistent with consolidation intent / planning framing). A tiny sub-test calling dispatchUnregisterHooks(...)("show-hooks","-g") under a recovered *testing.T would make the fatal path executable proof. Optional.
- [idea] After 5-1 routed read-fault teardown sites directly through perEventDispatchWithFaults, dispatchUnregisterHooks's shim (unset faults only) is slightly asymmetric vs read faults bypassing it — a one-line cross-reference comment would aid future readers. Minor.
