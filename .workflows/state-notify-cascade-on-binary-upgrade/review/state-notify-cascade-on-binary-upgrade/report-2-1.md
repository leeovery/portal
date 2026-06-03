TASK: Make managedEvents the single source of truth for the Portal-managed event set (state-notify-cascade-on-binary-upgrade-2-1)

STATUS: Complete

ACCEPTANCE CRITERIA: managedEvents single production source OR parity test fails on divergence; registration + teardown over identical event set; migrate-rename teardown-only divergence unchanged; no production doc comment asserts dead ordering contract or notify-only framing; build/vet/full internal/tmux suite pass.

SPEC CONTEXT: spec 126-141, 79-81, 180 — registration converges per-event over managedEvents; teardown scans portalEvents; migrate-rename retained teardown-only, never registered.

IMPLEMENTATION:
- Status: Implemented — FULL DERIVATION variant (preferred, not the parity-test fallback)
- hooks_register.go:45-55 managedEvents (single source; doc :29-32 states order NOT load-bearing). managedEventNames() (64-70) projects event field preserving order. hooks_unregister.go:64 `var portalEvents = managedEventNames()` (derived). managedEventFingerprintUnion() + teardownFingerprints() (88-125); hooks_unregister.go:43 `var portalCommandSubstrings = teardownFingerprints()`.
- saveTriggerEvents FULLY RETIRED (grep zero refs in internal/tmux). HydrationTriggerEvents retained (own external consumers; accurate comment). migrate-rename divergence preserved verbatim (migrateRenameSubstring in NO managedEvents entry; teardownFingerprints explicitly appends it).

TESTS:
- Status: Adequate
- hooks_event_parity_test.go:24-39 TestPortalManagedEventSetParity (ManagedEventNames()==PortalTeardownEvents() WITH order). :63-102 TestPortalTeardownFingerprintParity (every managedEvents fingerprint incl. commit-now reapable; migrate-rename present in teardown but ABSENT from registration union — asymmetry guard). export_test.go seams rebuild from live tables. Existing real-tmux guards operate over derived set; full suite green. Two distinct domain facts, one focused guard each — not over-tested.

CODE QUALITY:
- Project conventions: Followed (derivation-over-duplication, table-driven, no t.Parallel, export_test seam). SOLID: Good. Complexity: Low. Readability: Good — stale framing corrected (no surviving "Order is significant" / notify-only framing; notifyCommand comment excludes session-closed).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] teardownFingerprints() returns union unmodified if migrateRenameSubstring already present — currently-unreachable defensive branch (asymmetry guard makes that state asserted-impossible). Cheap, self-documenting; no action.
