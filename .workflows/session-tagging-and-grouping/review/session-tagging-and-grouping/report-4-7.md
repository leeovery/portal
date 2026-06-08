TASK: session-tagging-and-grouping-4-7 — Dispatch re-group sessions refresh on projects-edit → sessions-page transition

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: refresh respects active grouping mode; nil SessionLister tolerated; no refresh when not transitioning; command-pending guard preserved.

SPEC CONTEXT: spec §278-283 — on projects-edit → sessions-page transition dispatch sessions-list refresh that re-resolves project records and re-groups, mirroring preview-dismiss → sessions transition. No background watch for v1.

IMPLEMENTATION: Implemented (mirrors existing contract).
- model.go:1535-1558 s/x key arms — flip activePage=PageSessions, return refreshSessionsAfterPreviewCmd(""); :1126-1139 reused helper (nil when sessionLister==nil, else previewSessionsRefreshedMsg); :1439-1451 handler → applySessions; :938-941 applySessions → rebuildSessionList (mode-aware core, re-reads tags via m.projects/projectIndex). No-refresh-when-not-transitioning structural (only s/x arms). command-pending early-return both arms. Filter/modal interaction safe.

TESTS: Adequate. projects_transition_refresh_test.go — DispatchesRefresh (s+x, 1 ListSessions); RegroupsWithUpdatedTags (ByTag re-groups under work heading — mode-aware + live re-resolution); ToleratesNilLister; PreservesCommandPendingGuard; NonTransitionKeyDoesNotRefresh. Through top-level Update. Behaviour-focused.

CODE QUALITY: Conventions followed (reuses refreshSessionsAfterPreviewCmd seam + previewSessionsRefreshedMsg, no parallel path, nil tolerance, no t.Parallel); SOLID good (DRY single refresh helper, single rebuildSessionList chokepoint); low complexity; spec-citing comments. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None. (s/x arms near-identical but both delegate to shared helper; extracting for two trivial sibling arms would reduce readability, mirrors pre-existing d/e style.)
