TASK: session-tagging-and-grouping-3-3 — Mode-aware session re-render core on the model (dispatches to Phase 2 builders)

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: zero live sessions per mode; mode-unchanged idempotent; correct builder per mode; SessionsMsg refresh preserves active mode.

SPEC CONTEXT: spec § Build note — render-layer dispatch Flat→ToListItems, ByProject→buildByProject, ByTag→buildByTag, zero-tags ByTag → signpost; Flat byte-for-byte today (no-regression §15).

IMPLEMENTATION: Implemented.
- model.go:1035-1076 rebuildSessionList — four-arm switch (signpost/ByProject/ByTag/default Flat); helpers filteredSessions, applySessions, nextSessionListMode, anyTagsExist, resolveSessionDirs, sessionListTitleForMode. Single chokepoint: applySessions (SessionsMsg) and handleSwitchViewKey both route here; mode read from m.sessionListMode, never reset on refresh. Value-copy in resolveSessionDirs — m.sessions never mutated. Idempotency structural (pure builders, byTagSignpost recomputed). Title set inside core. Later accretion (5-1/5-2/6-1/7-1) present, not drift.

TESTS: Adequate. rebuild_session_list_test.go — correct builder per mode + Flat byte-for-byte; cached project records; zero live sessions all modes; idempotent; SessionsMsg preserves mode (real Update path); inside-tmux exclusion + WithInsideTmux routing. Behaviour-focused.

CODE QUALITY: Conventions followed (pointer-receiver mutating core, tea.Cmd propagation, no t.Parallel, nil-tolerant seams); SOLID good (single chokepoint, open for new modes); low complexity; slices.SortFunc/value-copy idiomatic; thorough comments. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
