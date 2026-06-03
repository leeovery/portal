TASK: Rebuild RegisterPortalHooks as per-event ensure-exactly-one (state-notify-cascade-on-binary-upgrade-1-2)

STATUS: Complete

ACCEPTANCE CRITERIA: fresh table exactly-one per event (correct bodies, never migrate-rename); idempotent fast path zero set-hook; K-deep collapse = K descending unsets + one append; stale-body migration → count 1; session-closed union fast path; user hook untouched; per-event read failure folded + WARN, others converge; per-index unset WARN + continue + append fires; single reaped INFO only on eviction; build + tests pass.

SPEC CONTEXT: §§ Registration Redesign, Per-event parameters (71-75), Hook body shapes (85-89), User-hook coexistence (91-93), Logging/ordering/failure (95-101). Real-tmux oracle deferred to 1-6/1-7; this task owns the convergence engine + mock-commander unit coverage.

IMPLEMENTATION:
- Status: Implemented (faithful to spec)
- Location: internal/tmux/hooks_register.go — managedEvent struct + managedEvents table (23-55) mirrors spec parameter table exactly; migrate-rename intentionally absent (documented 39-44, 72-79). convergeEvent (322-366): reads via ShowGlobalHooksForEvent, on error emits canonical WARN + returns (0, fmt.Errorf("show-hooks failed: %w")); Portal-authored via containsAny union; fast path body==desiredBody byte-for-byte → (0,nil); else descending best-effort unset (WARN+continue, not counted) then one AppendGlobalHook. RegisterPortalHooks (386-408): iterates, never short-circuits, folds `register hook on <event>: %w` into errors.Join, sums evicted, single INFO with reaped attr only when totalEvicted>0, nil-logger tolerant via log.OrDiscard.
- Fast-path equality is full wrapped body; signalHydrateCommand carries literal unexpanded #{session_name}. Union counting for session-closed correct. Closed taxonomy respected (reaped attr + bootstrap component only).

TESTS:
- Status: Adequate (not over/under-tested)
- hooks_register_test.go / hooks_register_warn_test.go / hooks_register_six_event_routing_test.go cover: FreshTable, IdempotentFastPath, KDeepStackCollapse (k=5), StaleSignalHydrateMigratesInPlace, StaleNotifyOnSessionClosedMigratesToCommitNow, SessionClosedUnionFastPath, UserHookUntouched, SessionClosedSubstringEvicts..., PerEventReadFailureFolds, PerIndexUnsetFailureWarnsAndContinues, SingleReapedInfoOnEviction (reaped==3), NoReapedInfoOnZeroEviction, WARN shape. Mock dispatch helper t.Fatalf's if engine ever issues no-arg global read (perEventDispatchWithFaults:128-131), structurally pinning the per-event invariant.

CODE QUALITY:
- Project conventions: Followed. SOLID: Good (convergeEvent/evictPortalEntries/warnShowHooksFailure/parseEventEntries single-responsibility). DRY: Good (managedEventNames/fingerprint union derive from single source, lockstep parity test). Complexity: Low. Idioms: errors.Join, sort.Reverse, %w. Readability: Good.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] No per-event eviction DEBUG detail line on the successful-eviction path (spec line 97 says "may" — optional enhancement, not a gap).
- [idea] The two convergence-path WARNs distinguished only by free-form message text; a structured attr would be cleaner if a future log-grep consumer needs to discriminate. Acceptable today given closed attr taxonomy.
