TASK: Extract a shared per-event eviction helper used by both convergeEvent and UnregisterPortalHooks (state-notify-cascade-on-binary-upgrade-3-1)

STATUS: Complete

ACCEPTANCE CRITERIA: ParseShowHooks collapse one location (parseEventEntries); descending-index best-effort loop one definition (evictPortalEntries); `show-hooks failed` WARN one definition OR lockstep cross-ref; UnregisterPortalHooks retains exact func(*Client) error signature (cmd/state_cleanup.go compiles unchanged); DISTINCT error contracts preserved (convergeEvent best-effort/not-counted vs UnregisterPortalHooks errors.Join naming event[index]); migrate-rename teardown-only + filter parameterised; ParseShowHooks unchanged; no new component/attr; optional (b) injected-logger teardown variant + teardown-WARN unit test.

SPEC CONTEXT: §§ Registration Redesign / Teardown Rewrite / Migration-Helper Consolidation / What is intentionally not consolidated (79-81,128,140,180). Analysis-cycle DRY consolidation, not behaviour change.

IMPLEMENTATION:
- Status: Implemented — all 9 ACs satisfied
- AC1: parseEventEntries (hooks_register.go:225-227) sole ParseShowHooks(raw)[event] site; both callers route through.
- AC2: evictPortalEntries (259-290) owns descending sort + best-effort UnsetGlobalHookAt loop; both callers.
- AC3: warnShowHooksFailure (245-247) single WARN definition; error-wrap deliberately differs per caller (registration bare `show-hooks failed: %w`; teardown event-named) — documented why NOT centralised (237-242).
- AC4: UnregisterPortalHooks(c *Client) error (hooks_unregister.go:98-100); cmd/state_cleanup.go:52 function value + :90 invoke unchanged.
- AC5: evictPortalEntries returns []evictFailure with raw errors; convergeEvent WARN+continue not-counted; unregisterPortalHooks folds `unset hook on %s[%d]: %w` into errors.Join. Confirmed NOT collapsed.
- AC6: teardownFingerprints() appends migrateRenameSubstring to managedEvents-derived union; registration union omits it; evictPortalEntries takes pre-filtered entries (divergence at call site).
- AC7: hooks_parse.go ParseShowHooks untouched.
- AC8: reuses bootstrap component + error/error_class keys only.
- AC9: option (b) taken — inner unregisterPortalHooks(c, logger) + no-logger wrapper binding bootstrapLogger; test-only export bridge UnregisterPortalHooksWithLogger (export_test.go:62-68); production stays unexported.

TESTS:
- Status: Adequate. Teardown WARN through recording-logger seam (hooks_unregister_warn_test.go TestUnregisterPortalHooks_ShowHooksFailureEmitsCanonicalWarn — one WARN per event, no-double-log, reuses register-side assertShowHooksWarnShape). Register read-failure fold-and-continue (all-fail, single-event, *CommandError via errors.As). Teardown read-failure fold (every-read-fails aggregate+zero-removals, single-event folds + others reaped). Per-index best-effort (registration) + per-index fold (teardown, joined error naming every index). Both paths route through perEventDispatchWithFaults no-arg-global-read t.Fatalf guard. Parity guards. No-double-log on BOTH paths. No over-testing.

CODE QUALITY:
- Project conventions: Followed (component logger, log.OrDiscard, closed taxonomy, export_test pattern, no t.Parallel). SOLID: Good (evictPortalEntries open for both aggregation policies via []evictFailure). Complexity: Low. Idioms: errors.Join, sort.Reverse. Readability: Good (doc comments call out deliberate asymmetries).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Per-index evict WARN emitted inline by convergeEvent (not inside evictPortalEntries) because only registration logs per-index failures. Correct per distinct-contract requirement, single-caller today. Documentation-completeness observation; no action.
