TASK: session-tagging-and-grouping-6-1 — Route WithInsideTmux through the rebuildSessionList chokepoint

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: WithInsideTmux delegates to rebuildSessionList (preferred) or asserts m.sessions empty; no longer bypasses resolution/grouping/mode; construction-ordering assumption removed; test with pre-populated grouped mode asserts grouped/resolved; inside-tmux exclusion composes with grouping.

SPEC CONTEXT: Analysis Cycle 2 remediation. rebuildSessionList is the genuine chokepoint (filtered view + mode dispatch + dir resolution + mode-aware title) but WithInsideTmux bypassed with direct SetItems push, relying on construction-ordering luck.

IMPLEMENTATION: Implemented (preferred delegation path).
- model.go:464-480 WithInsideTmux sets insideTmux/currentSession then (&m).rebuildSessionList(); :916-928 filteredSessions excludes current in every mode arm; :1066 title set inside chokepoint. Prior panic-guard removed entirely (routing handles any m.sessions state). Returned cmd discarded (construction-time, m.sessions empty, documented). Value-receiver + (&m) correct. Backward compatible (NewModelWithSessions → ModeFlat zero value).

TESTS: Adequate. rebuild_session_list_test.go:221-288 TestWithInsideTmuxRoutesThroughRebuild — populated grouped list grouped + current excluded (load-bearing, "chokepoint bypassed?" failure msg); empty-at-construction yields empty list + mode-aware title; flat-mode exclusion. Existing inside-tmux suite green. "guard trips" case N/A (preferred path has no guard). Would-fail-if-broken holds.

CODE QUALITY: Conventions followed (With-option pattern, value-receiver+pointer-method, no t.Parallel); SOLID good (collapses duplicated render path, DRY, single reason to change); low complexity (three statements); documented discarded-cmd invariant. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
