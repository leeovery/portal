TASK: session-tagging-and-grouping-3-7 — By-Tag zero-tags "No tags yet" signpost

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: zero tags anywhere → signpost; degrade-with-message not silent flatten; reopen persisted by-tag with zero tags shows signpost; one s advances to Flat; tags-exist-all-sessions-tagged does NOT trigger signpost.

SPEC CONTEXT: spec § Empty states — By Tag zero tags renders plain list with explicit "No tags yet" (degrade-with-signpost); gate is union of tags across all projects (directory-tag presence, independent of live sessions); signposted state IS by-tag for cycling.

IMPLEMENTATION: Implemented.
- model.go:979 anyTagsExist (union across projects, session-independent); :1045 gate m.byTagSignpost = ModeByTag && !anyTagsExist (recomputed every rebuild, self-clears); :1049-1050 signpost arm builds via ToListItems (plain flat, no Untagged heading) and precedes ModeByTag arm (no pane reads); :2313-2320 view inserts signpost row additively via insertRowBelowTitle; :2359 text points to projects page. nextSessionListMode cycles ByTag→Flat. Distinction from empty-Untagged-suppression correct (gate keys on directory-tag presence).

TESTS: Adequate. bytag_zero_tags_signpost_test.go — TestAnyTagsExist (nil/no-tags/empty-slice/tagged); signpost shown; plain flat list under signpost (no untaggedHeading); s advances to Flat (real Update); reopen persisted by-tag; does-not-show with tags / all-sessions-tagged (load-bearing distinction). Behaviour-focused.

CODE QUALITY: Conventions followed (TUI-layer rendering, immutable lipgloss style, no t.Parallel); SOLID good (pure anyTagsExist, DRY insertRowBelowTitle); low complexity; idiomatic. Excellent spec-citing comments. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
