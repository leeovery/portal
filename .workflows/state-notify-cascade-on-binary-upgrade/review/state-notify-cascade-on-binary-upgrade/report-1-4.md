TASK: Move UnregisterPortalHooks to the per-event read seam (state-notify-cascade-on-binary-upgrade-1-4)

STATUS: Complete

ACCEPTANCE CRITERIA: reads per-event for every portalEvents event (no c.ShowGlobalHooks() remains); Portal entries removed descending at any depth (count→0); user entry survives; migrate-rename retained + stale one on session-renamed reaped; per-event read failure folds into errors.Join + WARN (no abort); per-removal failures fold with leaf shape, every removal attempted; build + tests pass.

SPEC CONTEXT: §§ Teardown Rewrite — UnregisterPortalHooks; Logging/ordering/failure semantics. Second half of deleting ShowGlobalHooks.

IMPLEMENTATION:
- Status: Implemented, no drift.
- hooks_unregister.go:98-144. UnregisterPortalHooks (98) thin wrapper preserving exported func(*Client) error (consumed by cmd/state_cleanup.go:44,52 as a function value). Logger via package-level bootstrapLogger (17). unregisterPortalHooks (112) loops portalEvents, reads ShowGlobalHooksForEvent (117). No no-arg ShowGlobalHooks() remains (grep zero in internal/tmux). Read failure → canonical WARN via shared warnShowHooksFailure (error_class="unexpected"), folds `show-hooks failed on %s: %w`, continues. Success → portalEntriesFor(parseEventEntries(...)) then shared evictPortalEntries (descending). Per-removal failures folded with `unset hook on %s[%d]: %w`. Returns errors.Join.
- portalCommandSubstrings (43) = teardownFingerprints() = managed-event fingerprint union + migrateRenameSubstring — migrate-rename retained (derivation is task 4-1's work, out of 1-4 scope; 1-4 contract holds). portalEvents (64) = managedEventNames(); session-renamed among projected events. portalEntriesFor/containsAny unchanged.

TESTS:
- Status: Adequate (not over/under-tested)
- Per-event read enforced structurally via perEventDispatchWithFaults t.Fatalf guard. Tests: reverse index order; interleaved Portal entries w/ user entries untouched; migrate-rename reaped on session-renamed; event-scoping (substrings outside portalEvents ignored); fail-every-read → aggregate + zero removals; NEW single-event read failure folds + others reaped; per-removal fold (joined error naming every failed index, errors.Is both sentinels); predicate strictness; idempotency; no-op-empty. readErrForAllManagedEvents derives key-set from ManagedEventNames().

CODE QUALITY:
- Project conventions: Followed (component logger via log.For, log.OrDiscard, errors.Join, no t.Parallel). SOLID/DRY: Strong (WARN + eviction shared with register; deliberate divergence kept at call site). Complexity: Low. Readability: Good.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] portalCommandSubstrings/portalEvents are init-time derivations of managedEvents (intended design, guarded by parity tests). Informational only.
