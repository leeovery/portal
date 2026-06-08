TASK: session-tagging-and-grouping-1-3 — Per-project tag set add/remove (normalised, deduped, persisted)

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: add canonical (trim+lower) deduped persisted; remove persisted. Edge: duplicate-after-normalisation no-op; removing absent tag no-op; blank/whitespace add rejected; project path not found.

SPEC CONTEXT: Tags live on Project record. v1 validation = trim + lower + reject empty + per-project dedup as set; internal whitespace preserved. Unknown path is addressing error (not no-op).

IMPLEMENTATION: Implemented.
- internal/project/tags.go: AddTag (:53-77), RemoveTag (:87-114), findByPath (:38-43), saveTagMutation (:120-128), NormaliseTag (:26-32), ErrProjectNotFound (:15).
- Branch precedence correct: Load → findByPath (ErrProjectNotFound, no Save) → NormaliseTag (blank → no-op) → dedup/absent check (no-op) → mutate + save. Canonical form sourced solely from NormaliseTag. Breadcrumb attrs mirror Upsert/Rename; no-op emits nothing.

TESTS: Adequate. tags_store_test.go
- AddTag: normalised add+persist; dedup no-op (content + mtime); blank no-op; unknown path → ErrProjectNotFound (no file created). All four edge cases.
- RemoveTag: case-insensitive removal+persist; absent no-op; unknown path → ErrProjectNotFound.
- mtime-based "Save skipped" assertions verify no-op at behaviour level. Not over-tested.

CODE QUALITY: Conventions followed (reuses Store/AtomicWrite/audit shape/ClassifyWriteError); SOLID good (findByPath + saveTagMutation extraction); low complexity; modern idioms (slices.IndexFunc/Contains/DeleteFunc). No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES:
- [quickfix] tags_store_test.go:135 — add a RemoveTag-with-blank-rawTag no-op subtest to cover RemoveTag's NormaliseTag !ok branch directly.
- [do-now] tags_store_test.go:12 — add assertion that internal whitespace is preserved through the store (e.g. "Code Review" → "code review") at the store boundary.
