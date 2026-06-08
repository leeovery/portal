TASK: session-tagging-and-grouping-4-5 — Persist tag additions/removals on confirm via ProjectEditor AddTag/RemoveTag seam

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: persist failure sets editError and aborts close; no-op when tags unchanged; additions and removals in one confirm; normalisation/dedup owned by Phase 1 store.

SPEC CONTEXT: spec 263-291 tags in projects modal, TUI-only, store owns canonicalisation (line 113), modal must not re-normalise; AC6.

IMPLEMENTATION: Implemented (5-3 reconciliation folded in).
- model.go:92-96 ProjectEditor seam (AddTag/RemoveTag(path,rawTag), store hardcodes via=cli); :1866-1906 confirm path — removals loop (1871) then additions loop (1894), then close + loadProjects(). No-op when unchanged: additions diffed against editProject.Tags, removals against final editTags → zero seam calls if untouched. Both in one confirm: separate ordered loops. Persist failure: editError set + return (modal stays open, no refresh). Normalisation: raw buffer values verbatim to store. 5-3: removal skipped if still in final editTags (remove-then-re-add). Consistent with alias confirm (per-op writes).

TESTS: Adequate. model_test.go:5311-5548 TestEditProjectTagPersistence — addition; removal; both-in-one; unchanged→zero calls; AddTag failure→nil cmd+error+open; RemoveTag failure; raw verbatim no re-normalise; remove-then-re-add survives. Store-level dedup/normalise in tags_store_test.go. Real Update loop + key sequence. Behaviour-focused.

CODE QUALITY: Conventions followed (ISP seam, store-chokepoint breadcrumb, no t.Parallel); SOLID good (normalisation pushed to single owner); low complexity (two flat loops); slices.Contains/IndexFunc/DeleteFunc idiomatic; rationale comments. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None. (Per-op writes mean removals-succeed-then-addition-fails leaves partial commit while modal reports failure — inherent to per-op design, identical to pre-existing alias confirm; editError aborts close. Accepted pattern, not a finding.)
