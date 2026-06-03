TASK: Collapse the eight hand-rolled per-event dispatch RunFuncs onto perEventDispatch with optional fault injection (state-notify-cascade-on-binary-upgrade-2-2)

STATUS: Complete

ACCEPTANCE CRITERIA: skeleton + no-arg-global-read fatal guard in exactly one helper; 8 bespoke RunFuncs collapsed to one-line calls; each migrated test exercises its original fault (read/unset/CommandError) with same outcome; no verbatim guard-string copies outside the shared helper; full internal/tmux suite passes.

SPEC CONTEXT: Analysis-cycle test refactor consolidating duplicated mock-dispatch skeletons; preserves the load-bearing per-event-read invariant guard.

IMPLEMENTATION:
- Status: Implemented
- hooks_register_test.go:95-151 — perEventDispatch is now a zero-fault wrapper delegating to perEventDispatchWithFaults(t, table, setHookErrFor, nil, nil); perEventDispatchWithFaults(t, seededTable, setHookErrFor, readErrFor, unsetErrFor) is the single skeleton owner. No-arg-global-read fatal guard at exactly line 129 (grep confirms single occurrence; other "no-arg" matches are doc prose). readErrForAllManagedEvents (159-165) derives fail-every-read map from tmux.ManagedEventNames() (drift-proof). hooks_unregister_test.go:28-31 dispatchUnregisterHooks now a thin shim into perEventDispatchWithFaults — teardown inherits the tripwire it previously lacked.
- All three fault channels exercised: readErrFor (incl. CommandError via errors.As), unsetErrFor, setHookErrFor.

TESTS:
- Status: Adequate. Migrated tests are the coverage (no new test required). Original fault paths + observable outcomes preserved; inline set-hook tripwires migrated to assertNoSetHookCalls post-condition. Not over/under-tested.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; nil-disables-channel map seams; accurate doc comments). SOLID/DRY: Good (single skeleton + thin wrappers, no premature abstraction). Complexity: Low. Readability: Good.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] hooks_unregister_test.go:329-345 ("idempotent second run") retains a hand-rolled inline RunFunc — legitimate exception: STATEFUL mock (removed bool flips output between runs) the stateless helper can't express; does NOT re-implement the fatal-guard branch, so AC#4 holds. Not one of the 8 targets.
- [idea] hooks_unregister_test.go:41-43 linesForEvent now consumed only by that stateful exception — near-orphan; task 5-1 contemplates pruning. Informational.
