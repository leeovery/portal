TASK: Fix T6-4 Scrollback-Stability Harness Silent-Pass On Empty Baseline And Missing Dir (T11-2)

ACCEPTANCE CRITERIA:
- len(baseline) > 0 asserted with plan-specified message.
- Walker/caller distinguishes ENOENT-at-root from empty-set, fails with "scrollback dir does not exist".
- Test still passes against working pipeline.
- Docstring no longer claims empty-set is a valid baseline.

STATUS: Complete

SPEC CONTEXT:
Composite end-to-end scrollback-stability test (spec § "Composite End-to-End Verification" bullet 6) must catch Component E capture-pipeline regressions. Pre-fix, harness silently green-lit (a) empty baseline despite seeded `while sleep 0.1; do echo "hello $RANDOM"; done` guaranteeing per-tick output, and (b) ENOENT on scrollback root dir collapsed via filepath.SkipDir into empty path-set. Both gaps invalidated the test as regression guard.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/bootstrap/composition_e2e_scrollback_stability_integration_test.go
- Specifics:
  - Lines 124-132: baseline snapshot returns `(paths, dirExists)`; ENOENT fails with `"scrollback dir does not exist: %s"`; empty-but-existing fails with plan-verbatim `"scrollback baseline empty after first post-bootstrap tick — capture pipeline may be broken or seed activity insufficient"`.
  - Lines 167-201: `snapshotScrollbackPaths` stats root first, returns `(nil, false)` on `os.IsNotExist`; non-ENOENT stat errors `t.Fatalf`; otherwise walks and returns `(paths, true)`. Docstring (lines 143-166) documents three-shape contract; no claim that empty-set is valid baseline.
  - Lines 111-118: post-bootstrap-buffer comment updated noting seeded shell loop guarantees ≥1 .bin file by baseline tick and empty baseline IS a regression signal.
  - Lines 30-40 (file header): edge-case bullets explicitly state empty-baseline-fails and ENOENT-distinguished-via-bool-return.

TESTS:
- Status: Adequate
- Coverage: Test under modification IS the regression guard. Two failure paths (empty baseline; missing dir) are now load-bearing with distinct diagnostics. Happy path passes per task description (commit f0baaa18).
- Notes: Plan-suggested optional unit test for `snapshotScrollbackPaths` (missing-dir vs empty-existing-dir) not implemented — plan marks it "recommended but not load-bearing".

CODE QUALITY:
- Project conventions: Followed — no t.Parallel, build-tagged integration, polled os.ReadDir/WalkDir, modern Go idioms (maps.Keys, slices.Sorted).
- SOLID: Good — `snapshotScrollbackPaths` has single well-documented responsibility with clear return contract.
- Complexity: Low — linear control flow; bool-return discriminant avoids sentinel-error gymnastics.
- Modern idioms: Yes — `os.IsNotExist` correct; `fs.DirEntry.Type()` lstat semantics documented.
- Readability: Good — three-shape contract documented verbatim (lines 147-156); file header edge-case section names regression each assertion guards.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Line 138 `observation, _ := snapshotScrollbackPaths(...)` discards `dirExists`. If dir is deleted mid-window, observation is nil and `assertPathSetEqual` reports it as "removed paths" — correct behavior, but explicit `if !dirExists { t.Fatalf("scrollback dir disappeared at observation %d", i) }` would yield sharper diagnostic. Optional refinement.
- [idea] Plan-suggested optional unit test pinning new `snapshotScrollbackPaths` contract (missing-dir vs empty-existing-dir) would lock contract against future drift. Not in acceptance criteria; plan marks it recommended but not load-bearing.
