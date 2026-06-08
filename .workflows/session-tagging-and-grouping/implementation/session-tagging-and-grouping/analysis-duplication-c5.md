# Analysis — Duplication — Cycle 5

STATUS: findings
FINDINGS_COUNT: 1 (1 low)

## FINDING: renderEditProjectContent Aliases/Tags field blocks are structurally near-identical
- SEVERITY: low
- FILES: internal/tui/model.go (renderEditProjectContent — Aliases block ~2252-2274, Tags block ~2278-2300)
- DESCRIPTION: The Tags field render block was authored as a line-for-line mirror of the Aliases field render block within renderEditProjectContent. Both repeat the same four-part structure: focus indicator ("> " vs "  ") keyed on m.editFocus; a "(none)" empty-state branch; a per-entry list with a "  > "/"    " cursor marker and "[x] %s" row; and a trailing "Add: %s" input row whose marker is set when the cursor sits at len(entries). The only differences are the field label ("Aliases"/"Tags") and the four state fields (editAliases/editTags, editAliasCursor/editTagCursor, editNewAlias/editNewTag). This is a RENDER-layer mirror, DISTINCT from the already-accepted persistence/update-handler mirror — it carries no spec-sanctioned diverging behaviour (the two renders are meant to look identical), so it is pure copy-paste that will drift if one field's presentation changes (e.g. a count badge or richer empty hint added to one but not the other).
- RECOMMENDATION: Extract a single helper, e.g. `renderEditListField(b *strings.Builder, label string, focused bool, entries []string, cursor int, addInput string)`, called twice (Aliases, Tags). Per-field state passed as plain args — no new Model abstraction — collapsing ~23 duplicated lines to one definition + two call sites so a future presentation tweak lands in both fields automatically.

SUMMARY: One new low-severity duplication: the Aliases and Tags field RENDER blocks in renderEditProjectContent are a ~20-line structural copy-paste (distinct from the already-accepted persistence mirror) and should collapse to a single renderEditListField helper. All other tag/grouping/resolution code is already well-factored via the cycle-4 extractions (findByPath, assembleGroups, sessionItemsToList, CanonicalDirKey/Index). Did NOT re-flag: the modal persistence mirror, the two SetSessionOption stamp calls, loadPrefsStore/prefsFilePath mirror, MatchProjectByDir-vs-Index.Match, findByPath, the grouping skeleton.
