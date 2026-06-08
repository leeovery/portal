TASK: session-tagging-and-grouping-5-3 — Reconcile remove-then-re-add of the same tag within one edit-modal session

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: remove-then-re-add survives save (no silent drop); remove-only still removes; brand-new tag still persists.

SPEC CONTEXT: Analysis Cycle 1 fix for silent-drop bug. Confirm path tracked removals (editRemovedTags) and additions independently; tag removed via x then re-added before save → RemoveTag fired while addition loop skipped it (in original set) → silent drop. Fix reconciles against final desired state.

IMPLEMENTATION: Implemented.
- model.go:1866-1901 handleEditProjectConfirm; buffers :290-291; seeding :1662-1663. Removal loop :1871-1886 `if slices.Contains(editTags, removed) { continue }` skips queued removals still in final buffer. Addition loop :1894-1901 diffs editTags vs original editProject.Tags. All buffers hold canonical values (NormaliseTag at add, removed pulled from editTags, original canonical from Phase 1) → exact slices.Contains comparison. Removals-then-additions ordering documented.

TESTS: Adequate. model_test.go TestEditProjectTagPersistence — remove-then-re-add (zero RemoveTag AND zero AddTag); remove-only (RemoveTag fires); brand-new (AddTag fires); plus both-in-one, unchanged→zero, failure paths. Asserts on seam call records. Real regression tripwire (removing guard fails test).

CODE QUALITY: Conventions followed (Deps/seam DI, no t.Parallel, slices.Contains/Clone); SOLID good (reconciliation in confirm chokepoint, store owns normalisation); low complexity (two linear passes); documented rationale. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES:
- [idea] model.go:1871-1886 — editRemovedTags can accumulate duplicates on repeated remove/re-add/remove, causing multiple RemoveTag calls at save. Harmless (store treats absent-tag removal as no-op), redundant not buggy. Optionally dedup editRemovedTags; otherwise rely on store idempotency.
