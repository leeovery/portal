TASK: resume-hooks-silently-lost-8-24 — "The General-Purpose TestingT Subset Has Two Homes, And Four Packages Each Carry Their Own Recording Fataller" (tick-72c5f6)

ACCEPTANCE CRITERIA:
- One `TestingT` declaration in the tree; `logtest`, `sourceguardtest`, `restoretest` and `hookstest` reference it.
- `internal/hookstest` no longer imports `internal/logtest`.
- One importable recording fataller; the four copies are gone and the broken recover with them.
- The new package's transitive deps are stdlib-only, pinned by its guard.

STATUS: complete

SPEC CONTEXT: None. This is a phase-8 implementation-analysis (duplication) task; the specification for the work unit says nothing about test-harness scaffolding (grep for `TestingT` / `harnesstest` / `stand-in` in the spec returns no hits). Per the shared verifier context, a phase 6–10 task's authority is its own body, which is what I judged it against.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - New package: `internal/harnesstest/doc.go`, `internal/harnesstest/recorder.go` (`TestingT` at :8, `NamingT` at :16, `Recorder` at :32, the private `fatalSentinel` at :42, `Fatalf` at :58, `Run` at :67), `internal/harnesstest/leaf_guard_test.go`, `internal/harnesstest/recorder_test.go`.
  - Re-pointed: `internal/logtest/capture.go:35,56,69,85,99,157` and `internal/logtest/assert.go:25,50`; `internal/sourceguardtest/packagedeps.go:152,162`, `assertdepswithin.go:24`, `parsesources.go:41,56,71`, `reposources.go:53,66,87`; `internal/restoretest/marker_count.go:43,62,82,100`, `restore_marker.go:40`, `restoretest.go:73`, `waitfor_file_exists.go:18`; `internal/hookstest/hooks.go:97,113` and `hooks_lock.go:108`; plus `internal/tmuxtest/socket.go:124` and `internal/commandertest/scripted.go:47`, which the task did not name but which held two more copies of the same subset.
  - Commit: `700a700a` (34 files, +523/−494).
- Notes:
  - AC 1 holds exactly: `grep -rn "TestingT" --include="*.go"` across the tree finds no second declaration — every other occurrence is a reference to `harnesstest.TestingT`. The four packages the criterion names all reference it (paths above). The now-dead `logtest.TestingT`, `sourceguardtest.TestingT`, `sourceguardtest.AssertingT`, `commandertest.TestingT`, `tmuxtest.TestingT`, `restoretest.markerReporter` and `restoretest.fataller` have no residual references anywhere.
  - AC 2 is met in substance, not in letter, and the divergence is sound. `internal/hookstest/hooks.go` — the byte helpers the task's Problem statement indicts — no longer imports `internal/logtest`. The package still imports it at `internal/hookstest/hooks_lock.go:12`, but for `logtest.Sink`-typed lock-WARN/degraded-read breadcrumb assertions (`UnlockedRecords` at :98, `AssertLockWarn` at :108, `AssertDegradedRead` at :135) that were authored earlier (tasks 6-18/6-20, present at `700a700a^`) and whose subject genuinely *is* captured log records. That is the dependency `logtest` exists to provide, not the category error the task set out to remove; deleting it would have meant deleting unrelated working helpers. The literal criterion was unachievable as written from the moment it was authored.
  - AC 3 holds: all four named copies are gone (`fakeT`/`captureFailure`, `recordingReporter`/`fatalSentinel`, `fakeFataller`, `recordingT`/`captureAssert` — the broken blanket-`recover` one). A repo-wide scan for `func (…) Fatalf(` outside `internal/harnesstest` returns exactly two stand-ins: `internal/sourceguardtest/packagedeps_test.go:57` and `internal/commandertest/scripted_test.go:27`. Both are deliberate, documented retentions and both are justified by their subject: each asserts on the value the helper *returns after* fatalling (`packagedeps_test.go:38` `deps != nil`; `scripted_test.go:212` the `(out, err)` pair after strict `RunRaw` fatals), which a stand-in that stops execution structurally cannot observe. Neither was among the four the task listed, and both predate it.
  - AC 4 holds: `internal/harnesstest`'s non-test sources import only `fmt` and `time`; `leaf_guard_test.go:16-19` pins that with `AssertDepsWithin(t, harnessTestPkg, nil, ForbiddingThirdParty(), lane)` across both lanes. The guard file lives in `package harnesstest_test`, so its own `sourceguardtest` import (which imports `harnesstest` back) is not a cycle and, being a test-only dep, is correctly outside what `go list -deps` judges.
  - Two additions beyond the Do list, both sound: `NamingT` (`recorder.go:16`), because `restoretest`'s deleted `fataller` required `Name()` and three helpers still take it (`waitfor_file_exists.go:18`, `restoretest.go:73`, `restore_marker.go:40`); and the `tmuxtest`/`commandertest` re-points, which removed two further copies of the same subset the task had not enumerated. Both extend the task's stated outcome rather than diverging from it.
  - `internal/tui/pagepreview_surface_audit_test.go:191` adds `harnesstest` to the pre-existing-package set that audit enumerates — required for the new directory, and the established pattern for every package added since that guard shipped.
  - No production (non-`*test`) package imports `harnesstest`: every non-`_test.go` importer is itself a test-helper package.

TESTS:
- Status: Adequate
- Coverage: `internal/harnesstest/recorder_test.go` carries all three test names the plan asked for verbatim — "it records the fatal message and stops the helper" (:20), "it records an Errorf without stopping" (:42), "it reports no failure for a passing helper" (:62) — and the leaf guard carries "it confines internal/harnesstest to the standard library" (`leaf_guard_test.go:16`). Three further cases cover behaviour the plan did not name but the implementation introduced: a non-sentinel panic is re-raised (:72), `Report()` carries both what was recorded (:88), `Name()` answers for `NamingT` (:102), and a `Fatalf` escaping `Run` renders its message rather than crashing anonymously (:110). Each pins a distinct property of the code as written; none is redundant.
- Notes:
  - The stop-the-helper case is the load-bearing one and it does bite: `fatalThenErrorf` (:13) writes an `Errorf` after the `Fatalf`, and the test asserts `len(rec.Errors) == 0` (:31) — so a `Fatalf` that recorded without panicking would fail rather than pass.
  - Re-pointed suites keep their assertions, and several were tightened in the move: the guard suites went from a `fataled bool` to an exact `len(Fatals) != 1` with the message checked out of `Fatals[0]` (`internal/tmux/target_composition_guard_test.go`, `internal/restoretest/orchestrator_literal_guard_test.go`, `session_restorer_literal_guard_test.go`, `internal/sourceguardtest/assertdepswithin_test.go`). No assertion was dropped.
  - The two capture shims that changed shape preserve their old semantics: `internal/logtest/capture_test.go`'s `captureFailure` still reports "failed" for a `Fatalf` alone (the old `fakeT.failed` was likewise set only by `Fatalf`), and `cmd/seam_guard_test.go:372` still reports "failed" for either verb (the old `seamGuardT` set `failed` in both) — its `msg` is now `Report()` rather than the last message, and every caller matches with `strings.Contains`, so no assertion is weakened.
  - Adoption is the strongest evidence the home is the right one: 59 `harnesstest.Recorder{}` construction sites across 22 files now, most added by later tasks.

CODE QUALITY:
- Project conventions: Followed. Stdlib-only, untagged, `<subject>test` naming, test-only doc boundary, leaf guard modelled on `internal/nanoid/leaf_guard_test.go` as the Do list asked, and the CLAUDE.md architecture table gained an accurate `harnesstest` row in the same commit.
- SOLID principles: Good. `TestingT` is the minimal union the two former declarations required; `NamingT` composes rather than widens it, so a helper needing `Name()` says so in its signature.
- Complexity: Low. `Run`'s deferred recover discriminates on a private sentinel type and re-panics anything else — the one piece of subtlety, and it is both commented (`recorder.go:64-66`) and covered (`recorder_test.go:72`).
- Modern idioms: Yes. `slices.ContainsFunc` in the replacement `errored` helper; value-receiver `String()` on the sentinel so the runtime renders it when a `Fatalf` escapes `Run`.
- Readability: Good. Comments state why rather than what, and the ones that would have gone stale were removed with their types — no surviving comment in the tree still describes a non-stopping stand-in for code that now uses the stopping one.
- Issues: None reaching the bar. Two observations recorded for completeness, neither reported as a finding: `stagedHydrateExe` and `assertRestoringSet` take `NamingT` without calling `Name()` (uniform with the sibling that does, and nothing breaks), and `doc.go` describes the stand-in family without naming `NamingT` by hand.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
