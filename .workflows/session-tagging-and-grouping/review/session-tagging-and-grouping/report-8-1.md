TASK: session-tagging-and-grouping-8-1 — Return the canonical key from Index.Match so buildByProject stops double-canonicalising

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: Index.Match returns canonical key, no caller recomputes; buildByProject sets GroupKey from Match return; one EvalSymlinks per known-project session per render; all callers/tests updated to new signature; By-Project output unchanged.

SPEC CONTEXT: Cycle-4 analysis task partially undoing a regression against Cycle-2 project.Index optimisation (Index is single per-render canonicalisation site). Composes two independent computations of same canonical value into one.

IMPLEMENTATION: Implemented.
- index.go:49-53 Match signature → (Project, string, bool); computes key:=CanonicalDirKey(dirPath) once, returns (p,key,ok) on hit and miss. grouping.go:55/61-65 buildByProject uses matched,key,ok and GroupKey:key (second CanonicalDirKey gone, grep confirms). grouping.go:130 resolveSessionTags updated, discards key (By-Tag GroupKey is tag). model.go:178 only struct field, not a Match caller. Full caller set = two in grouping.go, both updated. Doc comments updated.

TESTS: Adequate. index_test.go — trailing-slash (differential oracle, key==CanonicalDirKey(input)); symlink edge (key==resolved form); miss returns canonicalised input; empty; collision last-wins + empty/nil updated to 3-value. grouping_test.go:60-82 GroupKey reuses idx.Match key (proves reuse not recompute). By-Project ordering unchanged. Syscall-count not asserted (correct restraint — GroupKey-reuse is right behavioural proxy).

CODE QUALITY: Conventions followed (multi-return, no t.Parallel); SOLID good (Match single-responsibility, tightens chokepoint); low complexity (one fewer syscall on hot path); idiomatic; clear returned-key-contract doc. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
