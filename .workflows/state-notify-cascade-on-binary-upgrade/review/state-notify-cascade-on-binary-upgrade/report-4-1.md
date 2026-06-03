TASK: Make teardown reap the converged session-closed commit-now hook (close the fingerprint seam, AC #5) (state-notify-cascade-on-binary-upgrade-4-1)

STATUS: Complete

ACCEPTANCE CRITERIA: portalCommandSubstrings includes commit-now (via derived union) so teardown reaps the converged session-closed entry; teardown fingerprint set DERIVED from union of managedEvents fingerprints + explicit legacy migrate-rename (not hand-authored); registration unchanged (no migrate-rename; session-closed keeps {notify,commit-now}); teardown retains migrate-rename; real-tmux teardown-at-depth seeds stacked commitNowCommand on session-closed + non-vacuous pre-condition + reaps to zero + user hook survives; build/test/vet pass.

SPEC CONTEXT: §§ Teardown Rewrite, AC#5; What is intentionally not consolidated. The bug: teardown's portalCommandSubstrings omitted commit-now, so the converged session-closed commit-now entry was classified non-Portal and survived teardown (AC#5 violation); the prior test was vacuously green (never seeded commit-now).

IMPLEMENTATION:
- Status: Implemented (fix is real)
- hooks_register.go:88-101 managedEventFingerprintUnion() (dedup, order-stable, excludes migrate-rename). :114-125 teardownFingerprints() (union + explicit migrateRenameSubstring last, double-add guard). :79 migrateRenameSubstring. :47 session-closed {notifySubstring, commitNowSubstring} → commitNowCommand. hooks_unregister.go:43 portalCommandSubstrings = teardownFingerprints() (DERIVED).
- portalCommandSubstrings now {notify, commit-now, signal-hydrate, migrate-rename}; commit-now flows in via session-closed's union → portalEntriesFor now classifies the converged commit-now entry as Portal-owned → evictPortalEntries removes it. Registration genuinely unchanged (managedEvents carries no migrate-rename; union excludes it; convergeEvent uses each event's own fingerprints). Both asymmetries preserved. Derivation chain makes managedEvents single source of truth — seam cannot silently reopen.

TESTS:
- Status: Adequate (non-vacuous)
- hooks_register_realtmux_test.go:396-476 extended to seed stackDepth(=5) commitNowCommand + user hook on session-closed; pre-condition assert :438 (count==stackDepth) is NON-VACUOUS (fatals if not 5-deep); after one UnregisterPortalHooks :469 asserts commit-now→0, :473 user hook survives. Oracle reads per-event (only un-blind oracle).
- hooks_event_parity_test.go:63-102 TestPortalTeardownFingerprintParity asserts every managedEvents fingerprint (commitNowFingerprint :80) in teardown set, migrate-rename retained (:87), registration union does NOT carry migrate-rename (:95). TestPortalManagedEventSetParity present. migrate-rename teardown test (hooks_unregister_test.go:139) present.
- WOULD FAIL RED if commit-now dropped from union (parity test + real-tmux reap). No over-testing (parity = predicate layer; real-tmux = behaviour layer; spec mandates both).

CODE QUALITY:
- Project conventions: Followed (log.For, no t.Parallel, SkipIfNoTmux, export_test seam, closed taxonomy). SOLID: Good (single source of truth, open/closed for new categories). Complexity: Low. Doc comments document seam + asymmetry citing spec.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Real-tmux test (:450-455) hand-authors a literal teardown-fingerprint slice for the blind-event zero-assertion rather than deriving from tmux.PortalTeardownFingerprints() — a deliberate independent oracle (good; catches a derivation bug). A one-line comment noting the intentional duplication would stop a future contributor DRY-ing it back and weakening the guard.
