TASK: Delete migrateHydrationHooks and migrateSessionClosedHook and their dedicated paths (state-notify-cascade-on-binary-upgrade-1-3)

STATUS: Complete

ACCEPTANCE CRITERIA: both helpers no longer exist (grep no definition); hydration -- convergence still holds; session-closed → commit-now convergence still holds; substring predicate exercised AND documented; no production c.ShowGlobalHooks() except (temporarily) hooks_unregister.go; build + tests pass.

SPEC CONTEXT: §§ Migration-Helper Consolidation / One behavioral change to record / What is intentionally not consolidated.

IMPLEMENTATION:
- Status: Implemented (net code removal as planned)
- migrateHydrationHooks/migrateSessionClosedHook: ZERO definitions remain (grep finds only comments + retargeted test names). isStaleSignalHydrateEntry, staleSignalHydratePrefix, staleSignalHydrateMarker, RegisterHookIfAbsent, hookCategory, portalHookCategories, showGlobalHooksOrWarn: ZERO matches. sessionClosedEvent const RETAINED (referenced by managedEvents:47) — correct ("delete if unreferenced"). Fingerprint consts retained as live predicates.
- AC#5 stronger than required: the no-arg ShowGlobalHooks() is deleted everywhere (grep zero); hooks_unregister.go already on ShowGlobalHooksForEvent.

TESTS:
- Status: Adequate (balanced, no over-testing)
- hooks_migration_test.go fully retargeted to drive RegisterPortalHooks directly: -- convergence (real tmux), silent second bootstrap, fresh-install silence, multi-stale collapse (reaped=N), per-index evict-failure WARN+continue, per-event read-failure fold.
- hooks_register_test.go: StaleNotifyOnSessionClosedMigratesToCommitNow (:461 AC#3, evict-before-append asserted); SessionClosedUnionFastPath (:515); SessionClosedSubstringEvictsPortalStateNotifyBody (:547 AC#4 NEW — `portal state notify --debug` IS now evicted, doc comment ties to deleting migrateSessionClosedHook); SessionClosedNonMatchingUserHookSurvives (:599 — replaces old "--debug preserved" with fingerprint-free user hook). Both halves of substring-change contract covered.
- hooks_event_parity_test.go TestPortalTeardownFingerprintParity enforces the asymmetry (registration union must NOT contain migrate-rename, teardown MUST). Old TestRegisterPortalHooks_SessionClosedMigration name fully gone.

CODE QUALITY:
- Single-responsibility shared primitives reused by register/teardown while keeping divergent fingerprint sets + error-wrap shapes at call site. Three-shape branching collapsed to one table-driven loop. Doc comments traceable to spec. No issues.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The no-arg ShowGlobalHooks is already fully deleted everywhere incl. hooks_unregister.go (Task 1-4's migration already landed); nothing retains it. Informational for the 1-4 reviewer.
