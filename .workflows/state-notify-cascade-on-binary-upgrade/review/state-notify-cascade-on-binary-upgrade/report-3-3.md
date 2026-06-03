TASK: Reuse the existing set-hook argv extractors instead of inline mock.Calls scanning (state-notify-cascade-on-binary-upgrade-3-3)

STATUS: Complete

ACCEPTANCE CRITERIA: the `len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga"` (and -gu) argv guards no longer appear inline in test bodies (live only in extractor/accessor functions); new accessor(s) co-located with setHookCalls/unsetHookCalls; ordering + event-prefix assertions (K-deep-collapse, stale-notify-on-session-closed) unchanged in meaning; go test ./internal/tmux/... passes.

SPEC CONTEXT: Phase 3 low-severity test-only refactor consolidating duplicated argv-shape decoding. No production behaviour touched.

IMPLEMENTATION:
- Status: Implemented
- hooks_register_test.go:204-212 setHookCalls (pre-existing -ga extractor). NEW setHookEvent struct + setHookEvents (219-242): ordered cross-verb [verb,target] projection for append-vs-unset ordering, co-located. NEW eventOfUnsetTarget (244-254): event-name prefix before "[". hooks_unregister_test.go:48-56 unsetHookCalls (pre-existing -gu extractor).
- Consumers migrated: K-deep-collapse (:393 via setHookEvents); stale-notify append-follows-unset (:486 via setHookEvents); HydrationScans prefix-split (hooks_migration_test.go:361-362 via eventOfUnsetTarget); six-event routing (:76 via setHookCalls); warn test (:148 via setHookCalls).

TESTS:
- Status: Adequate. Grep confirms call-log-scanning argv guards survive ONLY inside the four accessors (assertNoSetHookCalls, setHookCalls, setHookEvents, unsetHookCalls); none inline in test bodies. Ordering/prefix assertions unchanged in meaning (K-deep: appendIdx > lastUnsetIdx + descending; stale-notify: unsetIdx < appendIdx; HydrationScans via eventOfUnsetTarget).

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; eventOfUnsetTarget mirrors production IndexByte('[') guard). SOLID: Good (single-responsibility accessors). Complexity: Low. Readability: Good (setHookEvent{Verb,Target} self-documenting).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] hooks_test.go:94,166 build literal set-hook wantArgs — OUT OF SCOPE (producer-contract tests for AppendGlobalHook/UnsetGlobalHookAt, not call-log scanning). Correctly untouched.
- [idea] hooks_unregister_test.go:329-345 idempotency sub-test hand-rolls stateful RunFunc with dispatch-side guards — dispatch/producer-side, not call-log scanning; not expressible via perEventDispatchWithFaults. Out of scope, correctly as-is.
