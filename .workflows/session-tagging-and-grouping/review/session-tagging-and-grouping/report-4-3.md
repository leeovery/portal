TASK: session-tagging-and-grouping-4-3 — Add-tag-on-Enter (field-scoped add, blank/dup no-op)

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: blank/whitespace-only Enter no-op; "  Work " stored as "work"; duplicate-after-normalisation no-op; Enter while Name/Aliases focused still confirms; Tags-focused empty-input Enter is no-op not confirm.

SPEC CONTEXT: spec lines 58-63 tag value rules; line 276 Enter field-scoped (add) not confirm while Tags focused with non-empty input; AC6.

IMPLEMENTATION: Implemented.
- model.go:1712-1733 KeyEnter arm — when editFieldTags: NormaliseTag(editNewTag), blank (ok=false) → early return no confirm, duplicate (slices.Contains) → no append no confirm (clears editNewTag), else append canonical + clear editNewTag + clear editError; Name/Aliases focus → falls to handleEditProjectConfirm. Normalisation delegated to project.NormaliseTag (no re-implementation).

TESTS: Adequate. edit_modal_add_tag_test.go — AppendsNormalisedTag; NormalisesWhitespaceAndCase; BlankIsNoOp (no append + modal stays); DuplicateAfterNormalisationIsNoOp; DoesNotCloseModal; ClearsError; Enter Name/Aliases focused confirms. Real updateEditProjectModal + KeyEnter. No-op tests assert both non-mutation AND modal unchanged.

CODE QUALITY: Conventions followed (no t.Parallel, seam stubs, single NormaliseTag chokepoint); SOLID good (dispatch on focus, delegated normalisation); low complexity; slices.Contains + (value,ok) idiomatic. Good comments. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None. (Note: spec line 276 describes Enter field-scoped for both Tags and Aliases, but task edge case states Aliases Enter confirms — pre-existing alias behaviour, out of scope for 4-3.)
