TASK: session-tagging-and-grouping-9-2 — Delete orphaned MatchProjectByDir public API and inline its differential-test oracle

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: tree-wide grep for MatchProjectByDir returns zero after; no production caller existed at delete time; CanonicalDirKey + Index.Match unchanged; index_test.go + dirresolve_test.go oracles rewritten to inline CanonicalDirKey + map-membership preserving assertions; symlinked/non-existent/exact/canonicalised cases covered; go test ./... passes.

SPEC CONTEXT: Cycle-5 cleanup. MatchProjectByDir added Phase 1 (T1-4), superseded by cached project.Index within same feature (Index.Match sole production path). Orphaned wrapper carried per-call EvalSymlinks cost the Index eliminated.

IMPLEMENTATION: Implemented (correct).
- pathkey.go MatchProjectByDir deleted; CanonicalDirKey:24-42 unchanged. index.go Index.Match:49-53 + NewIndex unchanged. Tree-wide grep over *.go returns zero MatchProjectByDir (remaining hits only in .workflows/ docs + .tick history). STOP precondition held (no production .go caller ever — all paths use Index.Match).

TESTS: Adequate. index_test.go TestIndexMatch rewritten to inline differential oracle (wantKey:=CanonicalDirKey(dir), scan projects for member, assert (Project,key,ok)); covers canonicalised/trailing-slash, symlink, no-match, empty-dir; collision + nil/empty retained. dirresolve_test.go (internal/session) former use → inline map-membership loop preserving assertion intent. Independent recomputation (correct oracle shape, not circular). All required edge cases.

CODE QUALITY: Conventions followed (no t.Parallel, table/subtest); SOLID good (tightens public surface to single lookup path); low complexity; oracles commented as independent cross-checks. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None. (Task text named dirresolve_test.go without package path — it lives in internal/session/; correct file edited, no defect.)
