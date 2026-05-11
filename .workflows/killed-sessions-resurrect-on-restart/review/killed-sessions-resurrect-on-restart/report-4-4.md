TASK: killed-sessions-resurrect-on-restart-4-4 — Promote sessions.json seeding helpers into shared package consumed by both cmd and cmd/bootstrap tests

ACCEPTANCE CRITERIA:
- Promote seedSessionsJSON / seedSessionsJSONWithSavedAt into a non-test-file location reachable by both cmd and cmd/bootstrap test packages.
- Collapse the six bootstrap-integration inline blocks and the cmd-local copy to one-liners.
- Preserve SeedSessionsJSONWithSavedAt as a distinct entry point.
- Helper imports testing deliberately.

STATUS: Complete

SPEC CONTEXT: The shared SeedSessionsJSONWithSavedAt underpins phase5_marker_suppression_integration_test.go, which guards the "Save-Side -> Triggers -> Restoration guard" invariant.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/restoretest/sessions_json.go (new; SeedSessionsJSON at line 28; SeedSessionsJSONWithSavedAt at line 39).
  - /Users/leeovery/Code/portal/internal/restoretest/doc.go:9-14 — package doc updated.
  - /Users/leeovery/Code/portal/cmd/reattach_integration_test.go:296, 384, 451, 517, 631, 724 — six sites dispatch through restoretest.SeedSessionsJSON{,WithSavedAt}; local inline definition removed.
  - cmd/bootstrap/phase5_marker_suppression_integration_test.go:90, phase5_integration_test.go:154,232, phase2_hook_fire_integration_test.go:129, eager_signal_hydrate_integration_test.go:172,309 — six bootstrap blocks collapsed.
- Notes: Helper file untagged so default-tagged callers can consume it. SeedSessionsJSON delegates to SeedSessionsJSONWithSavedAt with time.Time{}. Encoded shape uses state.EncodeIndex (same encoder Restore reads).

TESTS:
- Status: Adequate
- Coverage (internal/restoretest/sessions_json_test.go):
  1. TestSeedSessionsJSON_BytesEquivalentToInlineBlock — byte-equivalence.
  2. TestSeedSessionsJSONWithSavedAt_PreservesSavedAt — round-trip SavedAt through DecodeIndex.
  3. TestSeedSessionsJSON_WritesAtCanonicalPath — asserts state.SessionsJSON(stateDir) path and negates stray scrollback creation.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Single responsibility.
- Complexity: Low.
- Modern idioms: Yes. t.Helper() on both functions, variadic `names ...string`, preallocated slice.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] SeedSessionsJSON's doc comment references "seven inline blocks this helper replaced". That historical note will go stale if sites move. Consider trimming to "replaces inline shape used across cmd and cmd/bootstrap integration tests".
