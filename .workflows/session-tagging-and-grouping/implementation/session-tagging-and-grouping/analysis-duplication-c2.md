# Analysis — Duplication — Cycle 2

STATUS: findings
FINDINGS_COUNT: 2 (1 medium, 1 low)

## FINDING: Aliases and Tags modal field handling is duplicated across all four modal sub-functions
- SEVERITY: medium
- FILES: internal/tui/model.go (Down/Up cursor ~1729-1745, Runes x-remove+add ~1747-1780, Backspace ~1711-1726, renderEditProjectContent Aliases vs Tags blocks ~2228-2276)
- DESCRIPTION: The Tags field was implemented as a near-verbatim copy of the Aliases field across the whole modal surface. Four key-handler arms each contain a paired Aliases-block + Tags-block differing only in field name (editAliasCursor/editTagCursor, editAliases/editTags, editNewAlias/editNewTag, editRemoved/editRemovedTags) and the editFieldAliases/editFieldTags discriminator. renderEditProjectContent repeats the same ~22-line "indicator → label → (none)/[x]-list → Add input" block twice. The spec-mandated behaviour parity ("Tags behaves exactly like Aliases") justifies the behaviour but not the literal duplication; drift risk is real (a fix to alias cursor-clamping/remove semantics must be hand-mirrored).
- RECOMMENDATION: Extract a small editable-string-set field helper: (1) renderEditListField(b, focused, label, entries, cursor, addInput); (2) a key-handler helper applying shared Down/Up/Backspace/x-remove/type-add transitions given pointers to (entries, cursor, newInput, removed). Keep editFocus dispatch and divergent persistence (RemoveTag/AddTag vs DeleteAndSave/SetAndSave) at call sites. NOTE: cycle 1's duplication agent recommended leaving this as-is for v1 given diverging persistence semantics; this is a judgment call.

## FINDING: @portal-dir best-effort stamp logic duplicated between eager-create and lazy-fallback paths
- SEVERITY: low
- FILES: internal/session/create.go:90-97, internal/session/dirstamp.go:62-69
- DESCRIPTION: Both CreateFromDir (eager) and ResolveAndStampDir (lazy) perform `_ = SetSessionOption(session, PortalDirOption, resolvedDir)` with a near-identical multi-line best-effort-swallow rationale comment. One-liner body below rule-of-three; the identical rationale comment is duplicated verbatim. Early-stage drift watch-point, not high-impact.
- RECOMMENDATION: Optional/low. If a third stamp site appears, extract stampPortalDir(setter, session, dir). For two sites this is borderline premature — drift watch-point, not a mandated extraction. NO ACTION recommended this round.

SUMMARY: One medium consolidation (modal Tags/Aliases literal duplication — judgment call, cycle 1 said leave it). One low drift watch-point (@portal-dir stamp, no action). Grouping/Phase-1 helpers (assembleGroups, sessionItemsToList, CanonicalDirKey, NormaliseTag, ResolveSessionDir, prefs/store, configFilePath wrappers) are well-factored, no extractable duplication.
