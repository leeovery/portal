# Analysis — Architecture — Cycle 5

STATUS: findings
FINDINGS_COUNT: 1 (1 low)

## FINDING: MatchProjectByDir is an orphaned public API — introduced then immediately superseded by Index.Match
- SEVERITY: low
- FILES: internal/project/pathkey.go:44-60 (MatchProjectByDir), internal/project/index.go:49-53 (Index.Match)
- DESCRIPTION: MatchProjectByDir was added by this feature (T1-4) as the per-render dir→project lookup, then superseded within the same feature by the cached project.Index (Index.Match), which is the sole production lookup path — buildByProject, buildByTag, and resolveSessionTags all route through idx.Match. MatchProjectByDir now has ZERO production callers; it survives only as a differential-test oracle (index_test.go cross-checks Index.Match against it; dirresolve_test.go uses it once). This leaves an exported internal/project function whose linear O(projects) per-call EvalSymlinks cost is exactly what Index was built to eliminate, reachable as if it were a supported lookup — a future caller could pick the slow (syscall-per-stored-project) entry point. Its underlying primitive CanonicalDirKey remains the legitimate shared canonicaliser; only the linear-scan wrapper is dead. NOTE: this was raised in cycle 3 (duplication) and discarded then as "keep as Index.Match oracle"; re-raised here with a cleaner fix that removes the dead public surface without losing the differential assertion.
- RECOMMENDATION: Either delete MatchProjectByDir and rewrite the two test oracles to compare Index.Match against an inline CanonicalDirKey equality (preserving the differential assertion without the public surface), or unexport / doc-mark it as test-oracle-only so it is not mistaken for a supported lookup path. Low priority — correct and harmless today, purely dead-public-surface cleanup.

SUMMARY: Architecture is essentially clean at this convergence stage — single rebuildSessionList chokepoint, setProjects as the sole index writer, gated lazy resolution, typed SessionItem, pure composable builders (assembleGroups/appendCatchAll shared tail), leaf prefs package, sound remove/re-add tag reconciliation, correctly-guarded typed-nil persister, thorough seam-level + cmd-integration test coverage. The only residual issue is the orphaned MatchProjectByDir lookup the feature introduced and then superseded with the cached Index.
