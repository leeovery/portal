# Analysis — Architecture — Cycle 2

STATUS: findings
FINDINGS_COUNT: 4 (2 medium, 2 low)

## FINDING: SessionItem.Tag is a write-only (dead) field, redundant with GroupKey
- SEVERITY: low
- FILES: internal/tui/session_item.go:61-66, internal/tui/grouping.go:107-113
- DESCRIPTION: buildByTag sets Tag, GroupKey, and GroupHeading to the identical canonical-tag value for every By-Tag instance, but no production code reads SessionItem.Tag — boundary detection and counting read GroupKey, the heading reads GroupHeading, selection/attach/preview/kill key on Session.Name. The field's own doc concedes selection/attach key on Session.Name. Pure redundancy with GroupKey (the Untagged catch-all is distinguished by the CatchAll flag, not an empty Tag). Only consumers are test assertions using it as a convenience handle. A third parallel copy of the same value to keep in sync.
- RECOMMENDATION: Remove the Tag field; affected tests assert on GroupKey/GroupHeading instead. If a tag accessor is wanted for readability, derive it (method returning GroupKey when !CatchAll) rather than storing a third copy.

## FINDING: WithInsideTmux bypasses the rebuildSessionList resolution/grouping chokepoint; correctness depends on construction ordering
- SEVERITY: medium
- FILES: internal/tui/model.go:461-471 (WithInsideTmux), cmd/open.go:408-410
- DESCRIPTION: rebuildSessionList is designed as the single chokepoint over (sessions, projects, dir-resolution, mode). WithInsideTmux is the one production path that pushes session items directly via SetItems(ToListItems(filtered)) — skipping resolveSessionDirs, the grouping builders, and the mode switch. Today this is harmless only because in production WithInsideTmux runs immediately after tui.New (open.go:408) when m.sessions is still empty, so it pushes an empty list the first SessionsMsg→applySessions→rebuildSessionList overwrites. The chokepoint is true over its full input set only by accident of call ordering. The path's panic("unreachable") guard covers only the filter-applied case, not "sessions already populated + grouped mode" — if a future refactor populates sessions before WithInsideTmux (NewModelWithSessions already does this in tests), this path silently renders flat/un-resolved/un-grouped items until the next refresh, with no guard tripping.
- RECOMMENDATION: Route WithInsideTmux's item population through rebuildSessionList (already a pointer-receiver call elsewhere) so inside-tmux exclusion composes with mode/grouping/resolution through the single chokepoint. At minimum, if the direct push is kept as a deliberate "no sessions yet" fast path, replace the partial panic guard with one that also asserts m.sessions is empty, enforcing the construction-ordering assumption rather than leaving it implicit.

## FINDING: MatchProjectByDir re-canonicalises every stored Project.Path on every session lookup — O(sessions × projects) EvalSymlinks syscalls per grouped render
- SEVERITY: medium
- FILES: internal/project/pathkey.go:50-60, internal/tui/grouping.go:54/130, internal/tui/model.go:1028-1030
- DESCRIPTION: Each grouped render calls MatchProjectByDir once per session (buildByProject; per session via resolveSessionTags in buildByTag). MatchProjectByDir loops all projects and calls CanonicalDirKey(p.Path) for every project on every call. CanonicalDirKey performs filepath.EvalSymlinks — a filesystem syscall — so a render costs ~N_sessions × M_projects EvalSymlinks calls, recomputing the same stored-path keys repeatedly. The spec made grouping cheap (the @portal-dir stamp avoids per-session git rev-parse), but stored-side canonicalisation reintroduces a per-render filesystem cost scaling with the product of two unbounded sets. The session-side want key is computed once per call; only the project side is wastefully recomputed.
- RECOMMENDATION: Canonicalise stored project paths once when projects are cached (the ProjectsLoadedMsg handler that sets m.projects) — build a map[canonicalKey]Project once per project-load — and have builders look up by pre-canonicalised key. Collapses render cost from O(sessions × projects) syscalls to O(projects) once per project change + O(sessions) map lookups, keeping CanonicalDirKey as the single source of truth for the key form.

## FINDING: No end-to-end test for the @portal-dir stamp → ListSessions(Dir) round-trip seam
- SEVERITY: low
- FILES: internal/session/create.go:96, internal/tmux/tmux.go:198/239, internal/tmux/tmux_test.go:140
- DESCRIPTION: The two halves of the stamp seam are each unit-tested in isolation — CreateFromDir's SetSessionOption(@portal-dir) write, and ListSessions parsing #{@portal-dir} from a fabricated commander line — but no test exercises the round trip through a real tmux server (stamp a session, read it back via ListSessions, confirm Session.Dir matches the resolved git-root). Correctness hinges on the stamped value and the format-field read agreeing; a tmux quoting/escaping or format-string drift would pass both unit tests while breaking grouping in production. The package already has real-tmux integration fixtures (tmuxtest, portal_saver_integration_test) to model this on.
- RECOMMENDATION: Add one integration-tagged test that creates a session via the real path, calls ListSessions against the real socket, and asserts Session.Dir equals the canonical git-root.

SUMMARY: rebuildSessionList is a genuine chokepoint in steady state, but WithInsideTmux composes with it only by construction-ordering luck (medium), and the cheap-by-design grouped render silently reintroduces O(sessions×projects) filesystem syscalls via repeated stored-path canonicalisation (medium). A dead Tag field (low) and a missing stamp round-trip integration test (low) round out the findings.
