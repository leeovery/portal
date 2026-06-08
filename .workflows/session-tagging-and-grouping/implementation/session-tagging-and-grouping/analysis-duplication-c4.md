# Analysis — Duplication — Cycle 4

STATUS: findings
FINDINGS_COUNT: 1 (1 low)

## FINDING: AddTag / RemoveTag re-implement the find-project-by-path loop
- SEVERITY: low
- FILES: internal/project/tags.go:48-53, internal/project/tags.go:84-89
- DESCRIPTION: Both new tag-mutation methods open with the identical exact-path lookup `slices.IndexFunc(projects, func(p Project) bool { return p.Path == path })` followed by an `if idx < 0 { return ErrProjectNotFound }` guard. These two NEW instances are byte-identical to each other, and the same "locate the project record whose Path == path" idiom already recurs in pre-existing store.go (Upsert, Rename, Remove). The feature added two more copies of a project-lookup the project package now performs in six places. Each is small (~4 lines) so impact is modest, but the pattern is past Rule of Three; the existing inconsistency (IndexFunc vs hand-rolled range; returning index vs name vs bool) is the kind of drift that invites a future divergence bug.
- RECOMMENDATION: Extract a single private helper, e.g. `findByPath(projects []Project, path string) (int, bool)`, and route the two NEW tags.go call sites through it. The pre-existing store.go sites are out of plan scope and need not be modified; consolidating only the new tags.go callers removes the newly-introduced duplication and leaves a seam store.go can adopt later.

SUMMARY: One low-severity, genuinely-new duplication: the two new AddTag/RemoveTag methods each re-author the find-project-by-path lookup. Already-accepted intentional (not re-raised): the Tags/Aliases modal field mirror, the two best-effort SetSessionOption stamp calls, loadPrefsStore/prefsFilePath mirror, MatchProjectByDir-as-Index.Match-oracle. grouping.go is well-factored (assembleGroups/appendCatchAll/sessionItemsToList shared).
