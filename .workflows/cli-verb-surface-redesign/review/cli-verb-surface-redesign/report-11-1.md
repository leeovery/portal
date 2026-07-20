TASK: cli-verb-surface-redesign-11-1 — Correct doctor's host-terminal seam provenance comments (they claim shared-bundle single-sourcing that does not exist)

ACCEPTANCE CRITERIA:
- Comment-only change; resolveDoctorDeps' Detector/Resolve construction byte-for-byte unchanged (no behavioral/code change).
- No comment in cmd/doctor.go implies doctor reads from the shared buildProductionSpawnSeams bundle via "the SAME" seam/resolver.
- Comments explicitly state the detector+resolve pair is independently re-constructed in resolveDoctorDeps and name the deliberate deferred terminals.json read (behind the lazy Resolve closure) as the reason doctor does not adopt the eager bundle, kept in sync by hand.
- The optional route-through-buildProductionSpawnSeams refactor is explicitly NOT required.
- go build ./... succeeds and golangci-lint run clean on cmd/doctor.go.
- No new tests; existing doctor host-terminal suite (cmd/doctor_test.go) stays green.

STATUS: Complete

SPEC CONTEXT: The cli-verb-surface-redesign spec folds host-terminal detection into `portal doctor` as an informational (checkInfo) line at the end of the report that never drives the exit code. The line is computed from the same detection primitives (spawn.NewDetector + buildResolver().Resolve) the picker and multi-target open burst use. This analysis-cycle-5 finding corrects docstrings that overclaimed single-sourcing: they described doctor's seams as "the SAME" seam/resolver the picker/burst use, implying doctor reads from the shared buildProductionSpawnSeams bundle — but resolveDoctorDeps re-constructs the detector and (lazily) the resolver independently, precisely because doctor needs a deferred terminals.json read that the eager bundle would break.

IMPLEMENTATION:
- Status: Implemented (comment-only, exactly as scoped)
- Location: cmd/doctor.go — DoctorDeps.Detector doc (98-106), DoctorDeps.Resolve doc (107-115), resolveDoctorDeps inline comment (143-153), checkHostTerminal doc (374-378). Reference type: cmd/spawn_seams.go buildProductionSpawnSeams (44-61).
- Notes:
  - Verified against commit 5d8c3548: the diff touches only comment lines in doctor.go. The construction statements `Detector: spawn.NewDetector(client)` and `Resolve: func(id spawn.Identity) ... { return buildResolver().Resolve(id) }` appear as unchanged context (no +/- prefix) — construction is byte-for-byte unchanged.
  - Zero "SAME" occurrences remain in doctor.go (the two overclaiming "the SAME Detect() seam" / "the SAME config-aware resolver" phrases were removed).
  - All five buildProductionSpawnSeams references in doctor.go explicitly negate shared sourcing ("NOT via", "constructed INDEPENDENTLY", "NOT through", "does NOT adopt", "not routed through their shared").
  - Comments name the deferred read as the reason and the hand-sync obligation: "Resolve is deferred through a closure so buildResolver only reads terminals.json when the line is actually computed ... whereas buildProductionSpawnSeams reads terminals.json eagerly at construction" (152-153) and "the two must be kept in sync by hand" (147).
  - Factual accuracy confirmed: buildProductionSpawnSeams (spawn_seams.go:54) sets `Resolve: buildResolver().Resolve` — buildResolver() runs at construction → terminals.json loaded eagerly. Doctor (doctor.go:155-157) wraps buildResolver().Resolve in a closure → buildResolver() runs per-invocation, and checkHostTerminal (399) short-circuits on a NULL identity before calling resolve → terminals.json read only when a non-null identity computes the line. The "same detector primitive" claim holds: both sites use spawn.NewDetector(client). The optional refactor was correctly NOT performed.
  - No stale drift elsewhere: spawn_seams.go's own docstring (44-50) claims the bundle is "the single construction site the open burst and picker both read" — it does not (and now must not) claim doctor as a consumer; consistent with the correction.

TESTS:
- Status: Adequate (no new tests required; existing suite is comment-agnostic and remains valid)
- Coverage: cmd/doctor_test.go TestDoctorHostTerminalLine (three classifications: supported / recognised-but-undriven / NULL-remote) and TestDoctorHostTerminalNeverDrivesExit inject Detector/Resolve via DoctorDeps (fakeTerminalDetector + a fabricated Resolve closure) and assert on checkResult.status/detail strings — behaviour, not comment text. A comment-only change cannot affect them.
- Notes: Correctly no new tests. Adding a test for a docstring correction would be over-testing; the acceptance explicitly forbids new tests.

CODE QUALITY:
- Project conventions: Followed — matches the codebase's heavy-docstring convention and the DI-seam idiom; the comments now accurately describe the per-field nil-fallthrough / independent-construction pattern used across commitNowDeps/bootstrapDeps.
- SOLID principles: N/A (no code change).
- Complexity: N/A (no code change).
- Modern idioms: N/A.
- Readability: Improved — the docstrings now correctly distinguish "same primitive, independently re-built" from "shared single-sourced bundle", and document the eager-vs-lazy terminals.json distinction that motivates the independence. Intent is clear and technically accurate.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.

VERIFICATION NOTE: go build / golangci-lint were not executed (per verifier rules, shell use is limited to the output-file rename). Assessed by reading: the change is comment-only on a previously-green file with zero code delta, so build and lint status are unaffected.
