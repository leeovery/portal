TASK: session-tagging-and-grouping-6-2 — Pre-canonicalise stored project paths once per project-load instead of per grouped render

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: paths canonicalised once per load (not per render/session); symlinked path resolves; index rebuilt after add/remove/edit; grouped output byte-identical; collision last-write-wins; CanonicalDirKey sole key form.

SPEC CONTEXT: spec §111-113 render-time lookup key must match stored Project.Path; mismatch silently drops session. Analysis Cycle 2 optimisation: per-render scan re-ran CanonicalDirKey (EvalSymlinks) O(sessions×projects); replaced by map[canonicalKey]Project built once per load.

IMPLEMENTATION: Implemented.
- index.go:14-53 Index + NewIndex + Match; model.go:992-995 setProjects (single seam mutating m.projects AND m.projectIndex together); :1052/1054 grouped arms consume m.projectIndex; grouping.go:55/130 consume idx.Match. NewIndex canonicalises each path once; both build+lookup route through CanonicalDirKey. Only m.projects= is in setProjects (rebuilds index atomically); ProjectsLoadedMsg routes through it. Empty-dir guard retained. Match returns key for reuse.

TESTS: Adequate. index_test.go — TestIndexMatch (differential oracle, symlink resolves, miss returns zero+key, empty); TestNewIndexCollisionLastWins; TestNewIndexEmpty. grouping_test.go:60-82 GroupKey reuses idx.Match key. projects_loaded_regroup_test.go:126-168 index rebuilt across messages. All 5 edge cases.

CODE QUALITY: Conventions followed (pure leaf data structure, pre-sized map, no t.Parallel); SOLID good (derived cache, single mutation seam); low complexity; multi-return idiomatic; thorough why-comments. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
