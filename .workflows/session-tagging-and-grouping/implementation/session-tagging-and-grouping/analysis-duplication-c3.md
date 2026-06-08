AGENT: duplication
FINDINGS:
- FINDING: MatchProjectByDir is a now-dead near-duplicate of Index.Match
  SEVERITY: low
  FILES: internal/project/pathkey.go:50-60, internal/project/index.go:43-46
  DESCRIPTION: MatchProjectByDir and Index.Match implement the same
    canonical-key directory→project lookup with the same (Project{}, false)
    miss contract. Index.Match was introduced specifically to replace the
    O(sessions × projects) EvalSymlinks scan that MatchProjectByDir performs,
    and all production grouping callers (buildByProject, buildByTag,
    resolveSessionTags) now route exclusively through idx.Match. A grep of
    non-test production code shows MatchProjectByDir has zero callers — it
    survives only in pathkey_test.go / index_test.go / dirresolve_test.go.
    Two functions encoding the same matching semantics is drift risk: a future
    canonicalisation tweak to one (e.g. handling a new path edge case) would
    silently diverge from the other, and the dead one carries a quadratic-cost
    implementation a contributor might re-adopt by reaching for the
    package-level function over the Index.
  RECOMMENDATION: Remove MatchProjectByDir (and its dedicated tests), or, if a
    no-Index convenience entry point is genuinely wanted, reduce it to a thin
    one-line wrapper that builds a transient Index and delegates to Match —
    keeping CanonicalDirKey-based matching defined exactly once. Index.Match is
    the single source of truth; pathkey.go should not carry a parallel scan.
SUMMARY: The feature is well-factored — canonical helpers (CanonicalDirKey,
  NormaliseTag, ExpandTilde) and the grouping-assembly tail (assembleGroups,
  appendCatchAll, sessionItemsToList) are each defined once and reused. The
  only genuine duplication is the superseded MatchProjectByDir, which now
  duplicates Index.Match's semantics with no production callers.
