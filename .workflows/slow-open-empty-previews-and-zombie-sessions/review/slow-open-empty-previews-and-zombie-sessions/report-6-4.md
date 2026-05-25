TASK: 6-4 — Assert scrollback directory stability across 10x1 s observations post-bootstrap

STATUS: Issues Found (two non-blocking but notable plan-vs-implementation edge case inversions)

SPEC CONTEXT: Spec § Composite step 6 — "Scrollback directory stable across 10 consecutive 1 s observations — no .bin deletions or unexpected new files (A+B+E)". Plan explicitly required edge cases: empty baseline must FAIL with `"scrollback baseline empty after first post-bootstrap tick — capture pipeline may be broken or seed activity insufficient"`; missing scrollback dir must FAIL with `"scrollback dir does not exist"`.

IMPLEMENTATION:
- Status: Implemented with deviations
- Location: `cmd/bootstrap/composition_e2e_scrollback_stability_integration_test.go:85-123` `TestCompositeBootstrap_ScrollbackDirPathSetStableAcross10Observations`
- Constants at 67/73/79; helpers `snapshotScrollbackPaths` (139), `assertPathSetEqual` (186), `setDifference` (213)
- Bootstrap slice (`SweepOrphanDaemons` + `BootstrapPortalSaver`) via adapters, mirroring 6-3

Deviations from plan:
- Plan: buffer = `TickerPeriod + 500ms` (1.5s); impl: 1.0s exactly — daemon's first tick fires at ~1s; 500ms safety margin gone
- Plan: reuse `portaltest.Fingerprint`/`DiffFingerprints`; impl: parallel `snapshotScrollbackPaths` returning `map[string]struct{}` (path-set only). File header argues path-set is right shape
- **Plan edge case INVERTED**: empty baseline must FAIL with specific message; impl EXPLICITLY treats empty baseline as valid pass (file header 30-33; line 113)
- **Plan edge case INVERTED**: missing scrollback dir must FAIL with `"scrollback dir does not exist"`; impl silently treats ENOENT as empty path-set (lines 145-150)

TESTS:
- Status: Under-tested vs plan
- Plan enumerates 4 tests; only happy-path stability implemented
- "mtime-only update passes" — not explicitly tested (implicit via path-set ignoring mtime)
- "transient .bin deletion fails" — NOT TESTED; no failure-injection
- "unexpected new .bin fails" — NOT TESTED
- Per-observation diff against baseline (not previous observation) correctly satisfies "oscillation still counts as deletion" criterion

CODE QUALITY:
- Project conventions: Followed
- SOLID/Complexity: Good
- Modern idioms: `maps.Keys`/`slices.Sorted`; `filepath.WalkDir`/`filepath.ToSlash`
- Readability: Strong file-header rationale
- `snapshotScrollbackPaths` conflates ENOENT and "no files present" — root cause of empty-baseline silent-pass

BLOCKING ISSUES:
- None per spec text alone (spec says "stable"; empty staying empty is technically stable)

NON-BLOCKING NOTES:
- [bug] Empty-baseline silent-pass: harness seeds two sessions running `while sleep 0.1; do echo $RANDOM; done` so empty baseline IS the "E regressed, capture pipeline broken" signal this test is meant to catch. Fix: assert `len(baseline) > 0` after `baseline := snapshotScrollbackPaths(...)` with plan-specified message
- [bug] Missing-scrollback-dir silent-pass: distinguish ENOENT from empty set, fail with plan-specified message
- [idea] Fingerprint-helper reuse criterion unmet; refactor to use `portaltest.SnapshotStateDir` + `DiffFingerprints` OR document waiver
- [idea] Bump `stabilityPostBootstrapBufferTick` from 1s to 1500ms to match plan and reduce CI flake risk
- [idea] Add unit tests of `assertPathSetEqual` with synthetic baseline/observation maps to cover plan tests 2-4 cheaply
