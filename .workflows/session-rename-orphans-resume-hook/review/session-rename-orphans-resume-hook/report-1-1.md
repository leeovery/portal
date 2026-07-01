TASK: Add tmux.HookKey pure-Go hook-key formatter (session-rename-orphans-resume-hook-1-1 / tick-7ebd68)

ACCEPTANCE CRITERIA:
- HookKey("id-abc","my-project",0,1) == "id-abc:0.1" (non-empty portalID wins).
- HookKey("","my-project",2,3) == "my-project:2.3" (empty portalID -> name).
- HookKey("","",0,0) == ":0.0" (degenerate form, no panic).
- Multi-pane distinct suffixes under one id: id-abc:0.0 / id-abc:0.1 / id-abc:1.0.
- Base-index-1: HookKey("id-abc","x",1,1) == "id-abc:1.1".
- PaneTarget, PaneTargetExact, StructuralKeyFormat bodies unchanged.
- go build -o portal . succeeds; go test ./internal/tmux/... passes.

STATUS: Complete

SPEC CONTEXT:
Spec (Hook-Key Derivation) introduces two derivation primitives so every hook-key-producing site derives the identical key ("prefer @portal-id, else session_name", suffixed :window.pane). HookKey is the pure-Go mirror of the tmux conditional in HookKeyFormat, for the saved (sessions.json) path where values come from persisted state rather than a live tmux read — the Stage 3 baker (collectArmInfos) will consume it. The spec is explicit: token is opaque and name is used verbatim (no trim/sanitize/validate) so a saved-path key is byte-identical to the live HookKeyFormat read; the load-bearing "format stable across releases — changing it silently invalidates every hooks.json entry" invariant transfers to HookKey/HookKeyFormat. This task adds the primitive only (no consumer wired in Phase 1).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go:601-623 (HookKey, with doc-comment 601-617, body 618-623).
- Notes: Body matches the spec rule exactly — portalID != "" -> fmt.Sprintf("%s:%d.%d", portalID, window, pane); else the same with name. No trimming/sanitizing/validation, matching the spec's verbatim contract. Placed directly after PaneTarget/PaneTargetExact and near StructuralKeyFormat/HookKeyFormat, as required ("hook-key primitives live together"). Doc-comment carries the transferred stability invariant verbatim in intent and explicitly notes it was formerly on PaneTarget. Confirmed against git diff vs main: PaneTarget (tmux.go:579-581), PaneTargetExact (597-599), and the StructuralKeyFormat constant (829) bodies/values are UNCHANGED — only their doc-comments were revised (a separate Phase-1 task, T1-5). The only added `return fmt.Sprintf` lines in the diff are HookKey's own two return statements. No drift from plan.

TESTS:
- Status: Adequate
- Coverage: internal/tmux/hookkey_test.go (package tmux_test, no t.Parallel). Table-driven TestHookKey covers all five named cases from the plan's Tests block: non-empty portalID wins, empty->name fallback, empty+empty degenerate ":0.0" (implicitly asserts no panic — a panic would fail the run), base-index-1 verbatim, zero indices verbatim. TestHookKey_DistinctSuffixesUnderOneID covers the multi-pane distinct-suffix case (0.0/0.1/1.0 under one id) AND asserts uniqueness via a set — directly exercising the "independently addressable panes" concern. A bonus TestHookKeyFormatContainsPortalIDLiteral static tripwire guards the @portal-id literal in HookKeyFormat (byte-identity across the three embeddings); this belongs to the sibling HookKeyFormat concern but is a cheap, non-tmux-gated guard and is not redundant with the HookKey cases.
- Notes: Every acceptance-criteria value is asserted by exactly one case (no redundancy). Tests verify observable behavior (returned string), not implementation details. Mirrors TestPaneTarget's table-driven style (tmux_test.go:2862). Not over-tested: the 5+1 rows are each distinct. Not under-tested: all spec edge cases (empty portalID, empty+empty degenerate, multi-pane, base-index-1, zero indices) are present. Tests would fail if the formatter broke (wrong branch, wrong suffix, or a panic on empty inputs).

CODE QUALITY:
- Project conventions: Followed. Test file uses external test package (tmux_test), table-driven with descriptive "it ..." names, no t.Parallel (per CLAUDE.md and golang-testing skill). Formatter is the single canonical primitive (no hand-rolled Sprintf at call sites), consistent with PaneTarget's stated contract.
- SOLID principles: Good. Single tiny pure function, one responsibility.
- Complexity: Low. Single branch, two return paths; trivially clear.
- Modern idioms: Yes. Idiomatic Go early-return, standard fmt.Sprintf.
- Readability: Good. Self-documenting; doc-comment explains the saved-path role, the verbatim contract, and the stability invariant with its provenance.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
