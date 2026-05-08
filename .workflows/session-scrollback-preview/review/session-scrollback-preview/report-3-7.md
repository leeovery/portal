TASK: session-scrollback-preview-3-7 — Chrome stability under focus changes (no mid-preview re-enumeration)

ACCEPTANCE CRITERIA:
- ] ] [ [ Tab Tab Tab sequence asserts exactly one ListWindowsAndPanesInSession call.
- chromeLine() after each keypress reflects open-time cached groups.
- Hermetic (no tmuxtest, no real tmux, constructor-injected).
- Future re-enumeration regression fails with clear "called N times, expected 1" message.
- Tail call count = 1 open + 7 cycles = 8.

STATUS: Complete

SPEC CONTEXT:
§ Multi-pane Rendering Shape > Chrome Floor (Chrome data source): "Chrome is computed once at preview-open and cycled in place; no live re-enumeration mid-preview." § Cross-cutting Seams > Externally-Killed Session During Preview reinforces stability. § Acceptance Criteria > Side-effect-free contract pins "exactly one TmuxEnumerator call".

IMPLEMENTATION:
- Status: Implemented (test-only task per plan)
- Location: internal/tui/pagepreview_chrome_stability_test.go (211 lines)
- Production code under guard: internal/tui/pagepreview.go — cycle handlers (lines 273–306) read cached m.groups only; only NewPreviewModel (line 74) calls enumerator. Invariant already holds.

TESTS:
- Status: Adequate
- Coverage: Four sub-tests:
  1. TestPreviewChromeStability_FullCycleSequenceProducesExactlyOneEnumerationCall — asserts enum.callCount == 1.
  2. TestPreviewChromeStability_ChromeLineAfterEachCycleReflectsOpenTimeCachedGroups — across all 7 steps asserts no "REENUMERATED" leak, both open-time window names reachable, "of 2" / "of 3" counter preservation; spot-checks per-step focused name through ] / [ wraparound and Tab pane cycling.
  3. TestPreviewChromeStability_ChromeLineNeverReflectsPostOpenEnumeratorStateChanges — arms secondErr; asserts callCount == 1 and no chrome leak (stricter variant).
  4. TestPreviewChromeStability_TailCallsPerCycleEqualOnePlusSeven — asserts len(reader.calls) == 8.
- Hermetic: confirmed — imports errors, strings, testing, bubbletea, internal/tmux only. No tmuxtest, no package-level seam mutation.
- Fixture is 2-window x 3-pane ensuring all 7 cycles are non-no-ops.

CODE QUALITY:
- Project conventions: Followed — no t.Parallel(); mirrors existing stubEnumerator/recordingReader pattern; constructor-injected; standard-library testing only.
- SOLID: Good — mock obeys TmuxEnumerator interface; driveCycleSequence helper is DRY across all four sub-tests.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Strong. File-top comment cites spec sections.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] driveCycleSequence is shared only within this file; if peer cycle-stability suites later land, could move to a shared _testhelpers file.
- [idea] chromeStabilityEnumerator dual-mode (secondErr vs second shape) is exercised cleanly; split is not warranted at current scope.
