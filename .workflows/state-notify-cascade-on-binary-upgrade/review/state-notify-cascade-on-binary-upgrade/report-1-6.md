TASK: Real-tmux no-growth + blind-spot regression guards (state-notify-cascade-on-binary-upgrade-1-6)

STATUS: Complete

ACCEPTANCE CRITERIA: no-growth test runs N>=2 on real tmux, asserts every managed event stays at one entry, names pane-focus-out + window-layout-changed, reads per-event (not blind); blind-spot test asserts no-arg omits the two blind events while including session-created AND per-event returns each; both SkipIfNoTmux + tmuxtest; no t.Parallel; pass on tmux. AC8 via no-growth structural guard.

SPEC CONTEXT: §§ Testing Requirements (1,2), Acceptance Criteria (1,8). Real-tmux is the only faithful oracle for the output-shape blind spot.

IMPLEMENTATION:
- Status: Implemented
- internal/tmux/hooks_register_realtmux_test.go: TestRegisterPortalHooks_NoGrowthAcrossBootstraps (117-168), TestShowHooksGlobalEnumeration_OmitsPaneAndGeometryEvents (189-250). Per-event read primitive portalEntryCommandsForEvent (76-90) routes through ShowGlobalHooksForEvent, never no-arg (rationale documented 70-75). ts.Run drives raw tmux socket (NOT a client method) for the no-arg blind form; deleted ShowGlobalHooks client method confirmed gone.

TESTS:
- Status: Adequate
- No-growth: N=3; per-event count==1 across all 9 managed events each run; explicit by-name count==1 on the two blind events; byte-for-byte body cross-check == expectedNotifyCommand. Blind-spot: appends on session-created + 2 blind events; no-arg via ts.Run asserts session-created PRESENT + 2 blind ABSENT (len==0); per-event ShowGlobalHooksForEvent returns each. SkipIfNoTmux + tmuxtest.New; no t.Parallel. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (external test pkg, real-tmux per spec mandate, fingerprint constants single-sourced). SOLID: Good. Complexity: Low. Readability: Good (comments explain WHY per-event + why real-tmux; failure messages distinguish "Portal bug" vs "tmux changed").

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] File also hosts sibling task 1-7 tests (by design — shared real-tmux file). Verified under 1-7.
- [idea] portalEntryCommandsForEvent []string return consumed via len() by 1-6 tests; body-returning richness used by 1-7. No action.
