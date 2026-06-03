TASK: Consolidate the test-side read-per-event → ParseShowHooks → count-by-fingerprint helper (state-notify-cascade-on-binary-upgrade-3-2)

STATUS: Complete

ACCEPTANCE CRITERIA: exactly one tmux_test-package helper implements read-per-event→parse→fingerprint-match; countSignalHydrateEntries + both inline loops route through it; canonical helper reads exclusively via ShowGlobalHooksForEvent (no caller reverts to no-arg/blind); verifyHydrationHookEntries (cmd/bootstrap) UNCHANGED; go test ./internal/tmux/... passes with identical coverage/assertions.

SPEC CONTEXT: Test-side count helpers MUST read per-event or count assertions on the two blind events are vacuous. Pure test-side DRY consolidation.

IMPLEMENTATION:
- Status: Implemented
- Canonical primitive portalEntryCommandsForEvent (hooks_register_realtmux_test.go:76-90, reads exclusively via ShowGlobalHooksForEvent — doc 71-75 forbids reverting); countPortalEntriesForEvent = len(...) (96-99). countSignalHydrateEntries (hooks_migration_test.go:41-48) now a thin map-builder over HydrationTriggerEvents routing through countPortalEntriesForEvent. Two former inline loops (hooks_migration_test.go:101, :262) now call portalEntryCommandsForEvent. verifyHydrationHookEntries (cmd/bootstrap, different package) UNCHANGED.
- `ShowGlobalHooks\b` grep repo-wide = no matches. One remaining no-arg read (ts.Run :217) is the intentional blind-spot regression test. hasPortalEntry (:254-261) is a pure predicate over an already-read slice (no own read) — correctly not routed.

TESTS:
- Status: Adequate. All five real-tmux guards + migration convergence + read-failure tests retained; non-vacuous pre-conditions preserved; blind-event count assertions still route through per-event helper (non-vacuous). Identical match semantics.

CODE QUALITY:
- Project conventions: Followed (external pkg, no t.Parallel, t.Helper, SkipIfNoTmux). SOLID: Good. Complexity: Low. Readability: Good.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] SelfHealsKDeepStack test (:334-351) still hand-rolls a read+parse+filter loop; expressible via portalEntryCommandsForEvent. Residual, not a regression — outside the AC's named targets.
- [idea] Body-equality cross-check (:155-167) and snapshotEventIndices (:552-567) read per-event but extract body-equality / HookEntry.Index (not count) — legitimately distinct, correctly left out.
