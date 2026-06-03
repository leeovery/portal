TASK: Fix the stale migrateHydrationHooks comment in reboot_roundtrip_test.go (state-notify-cascade-on-binary-upgrade-2-5)

STATUS: Complete

ACCEPTANCE CRITERIA: comment describes the unified RegisterPortalHooks per-event ensure-exactly-one convergence; no longer references deleted migrateHydrationHooks / Task-1-2; no test logic changed; no remaining migrateHydrationHooks references outside intentional historical context; cmd/bootstrap test package passes.

SPEC CONTEXT: spec 105-122 — three legacy registration shapes folded into one per-event convergence; migrateHydrationHooks deleted.

IMPLEMENTATION:
- Status: Implemented
- cmd/bootstrap/reboot_roundtrip_test.go:1200-1204 now reads "HookRegistrar runs RegisterPortalHooks' per-event ensure-exactly-one convergence, which evicts any stale un-separated signal-hydrate body and registers the --separated signalHydrateCommand end-to-end." No reference to migrateHydrationHooks or Task 1-2. Matches spec line 115. Downstream assertion comment (1227-1231) also correct.

TESTS:
- Status: Adequate (unchanged — comment-only). verifyHydrationHookEntries (1284-1308) unchanged: per HydrationTriggerEvents entry asserts exactly one entry contains "portal state signal-hydrate" AND the "portal state signal-hydrate -- " separator. Comment + assertion aligned.

REMAINING migrateHydrationHooks REFERENCES (grep): only intentional historical doc comments — hooks_migration_test.go:6, hooks_register_warn_test.go:12,83 (all explaining the helper was deleted). Other matches are .workflows/.tick artifacts. Zero func definitions remain.

CODE QUALITY:
- Readability: Good (comment now accurate + self-consistent). Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
