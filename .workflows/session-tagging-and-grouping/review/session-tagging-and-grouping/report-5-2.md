TASK: session-tagging-and-grouping-5-2 — Re-group on ProjectsLoadedMsg so grouped first paint is independent of startup message order

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: SessionsMsg before ProjectsLoadedMsg; setItemsCmd batched not substituted; no spurious re-group in flat mode or before sessions load.

SPEC CONTEXT: grouping fed by cached project records (m.projects from ProjectsLoadedMsg); fetchSessions/loadProjects dispatched concurrently → non-deterministic order; if SessionsMsg first in grouped mode every session falls to catch-all. AC11 persisted-mode first-paint requires correction once records arrive.

IMPLEMENTATION: Implemented.
- model.go:1344-1375 ProjectsLoadedMsg handler — setProjects (rebuilds projectIndex via NewIndex), populates project list, conditional re-group; gate `grouped := ByProject||ByTag; if grouped && len(m.sessions)>0` (excludes Flat + no-sessions); tea.Batch(setItemsCmd, (&m).rebuildSessionList()) preserves project-list SetItems. Reuses rebuildSessionList chokepoint.

TESTS: Adequate. projects_loaded_regroup_test.go — re-groups By Project / By Tag after sessions (does NOT pre-seed projects, genuine empty start); batches SetItems+rebuild; does NOT re-group Flat; rebuilds index per message (stale-index guard); does NOT re-group when no sessions (still caches). 1:1 edge-case coverage. Behaviour-focused.

CODE QUALITY: Conventions followed (tea.Batch idiom, single chokepoint, value-copy/pointer-call discipline); SOLID good (delegates grouping + index construction); low complexity; exemplary 10-line rationale comment. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
