TASK: session-tagging-and-grouping-8-3 — Extract a findByPath helper for the new AddTag/RemoveTag lookup duplication

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: not-found → ErrProjectNotFound preserved for both AddTag and RemoveTag; store.go Upsert/Rename/Remove left untouched; inline slices.IndexFunc + idx<0 guard removed from both new sites.

SPEC CONTEXT: Pure-refactor analysis-cycle task. Phase 1 invariant: tag mutation valid only against known project; unknown path is addressing error → ErrProjectNotFound with no Save. Must not alter observable behaviour.

IMPLEMENTATION: Implemented (clean).
- tags.go:34-43 findByPath helper (slices.IndexFunc on p.Path==path, returns (idx, idx>=0)); consumed at :59 (AddTag) and :93 (RemoveTag) via idx,ok:=findByPath(...) + if !ok return ErrProjectNotFound. Inline guard gone from both. ErrProjectNotFound returned before any Save (identical). store.go Upsert/Rename/Remove untouched (own loops with different not-found contracts — Rename/Remove no-op on absent, correctly not folded in). Minimal scoped refactor.

TESTS: Adequate. tags_test.go:69-99 TestFindByPath (first/middle/last hit + not-found (-1,false)); tags_store_test.go AddTag/RemoveTag not-found → ErrProjectNotFound + no file created (public API, both routes). Would fail if helper inverted or call site dropped error. findByPath unit test justified (explicit deliverable), not bloat.

CODE QUALITY: Conventions followed ((idx,bool) mirrors NormaliseTag (string,bool), doc comment); SOLID good (SRP, DRY win); low complexity; slices.IndexFunc idiomatic. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
