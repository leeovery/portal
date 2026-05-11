TASK: killed-sessions-resurrect-on-restart-2-6 — Integration test (real tmux): cold-start with non-attached saved session + registered on-resume hook fires end-to-end (AC2)

ACCEPTANCE CRITERIA:
- Integration test under real tmux: register on-resume hook for non-attached saved session, cold-start, assert hook ran in restored pane.
- Edge cases: N>=2 saved sessions where hook is on non-attached session; hook stdout/effect observable; test still passes when eager-signaling has already cleared markers pre-timeout.
- Spec AC2: on-resume hooks fire end-to-end on cold-start for every restored pane with a hook registered, regardless of which session the user attached to.

STATUS: Complete

SPEC CONTEXT: AC2 satisfied by combined Fix 1 (eager-signaling drives hydration for non-attached sessions) + Fix 2 (timeout fall-through routes through execShellOrHookAndExit). Symptom B pre-fix: on-resume hooks never fire for panes that hit the timeout path.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/bootstrap/phase2_hook_fire_integration_test.go (lines 1-207; `TestPhase2_HookFiresOnNonAttachedSession_AC2` at lines 95-207)
- Notes: Builds portal binary, PATH-prepends, isolates PORTAL_STATE_DIR and PORTAL_HOOKS_FILE, seeds N=2 sessions (alpha + beta), registers `touch <sentinelFile>` against beta's structural key, runs production-wired Orchestrator (RestoreAdapter + auto-defaulted real EagerSignalCore), verifies both sessions live post-Run, asserts sentinel appears within 2s using restoretest.WaitForFileExists. //go:build integration tag and testing.Short() skip honoured.

TESTS:
- Status: Adequate
- Coverage:
  - N>=2 saved sessions: covered.
  - Hook on non-attached session: covered.
  - Hook stdout/effect observable: covered via touch sentinel.
  - Path-agnostic between eager-signal-success and timeout fall-through: covered. 2s poll budget intentionally below the helper's 3s timeout.
  - Non-vacuity guard: both sessions live post-bootstrap.
  - Failure diagnostic: defer dumpPortalLogOnFailure.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel. Uses restoretest shared helpers.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Good.
- Readability: Excellent. Header comment block explains path-agnostic contract, non-vacuity guards, CI parallelism caveats.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] CI parallelism caveat (lines 49-56) — flake risk under default parallelism due to 500ms WriteFIFOSignal retry budget squeezed by concurrent go build. Mitigation documented but not enforced.
- [idea] Test relies on buildIntegrationOrchestrator's auto-default — a regression flipping this default would surface as confusing 2s timeout here rather than at the unit test. Cross-reference comment to Phase 4 task 4-2 would aid future debugging.
- [idea] Test asserts hook fires within 2s but does not assert eager-signal-success path was specifically taken vs timeout fall-through. Spec calls this "path-agnostic" by design.
