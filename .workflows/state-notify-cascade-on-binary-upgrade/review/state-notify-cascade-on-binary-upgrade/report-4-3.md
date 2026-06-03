TASK: Fix the stale doc comment in the six-event routing test (deleted test name + prior-spec heading) (state-notify-cascade-on-binary-upgrade-4-3)

STATUS: Complete

ACCEPTANCE CRITERIA: file-level comment references TestRegisterPortalHooks_FreshTable (exists), not TestRegisterPortalHooks_SessionClosedMigration (doesn't); spec-section pointer in BOTH file-level comment and in-body t.Errorf names this work unit's heading "Registration Redesign — Ensure Exactly One" not prior spec's; no test logic/sub-test names/assertions change; build/test/vet pass.

SPEC CONTEXT: Comment-only chore. Prior comment cited a deleted test name + a prior-spec heading.

IMPLEMENTATION:
- Status: Implemented
- hooks_register_six_event_routing_test.go:22 now references TestRegisterPortalHooks_FreshTable (confirmed exists at hooks_register_test.go:283). Deleted name TestRegisterPortalHooks_SessionClosedMigration appears in NO .go source (remaining grep hits are .workflows/.tick artifacts). Prior-spec pointer "Hook Registration Migration → Registration Strategy" fully removed (no matches). t.Errorf (:102) cites "Registration Redesign — Ensure Exactly One". The comment's § references to the PRIOR killed-session-resurrects spec are correct (gate authored under that prior spec) and rightly left untouched.

TESTS:
- Status: Adequate (comment-only). Per-event loop, exactly-one set-hook assertion, notifyCommand match, commitNowCommand anti-regression check intact. Full suite green.

CODE QUALITY:
- Readability: Good. Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] hooks_register_six_event_routing_test.go:102 — cited heading omits the spec's internal double-quotes; spec heading is `Registration Redesign — "Ensure Exactly One"` (specification.md:54). Add the quotes for exact fidelity. Unambiguous as-is (grep lands on the right section); the task description itself spelled it without quotes. Cosmetic.
