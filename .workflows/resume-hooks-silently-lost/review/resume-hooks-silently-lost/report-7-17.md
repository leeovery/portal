TASK: resume-hooks-silently-lost-7-17 — "The hooks.json Byte-Identity Assertion Has Three Homes Across Two Packages" (consolidation; promote the byte-identity assertion into `internal/hookstest`, delegate `cmd`'s copy, convert the open-coded forms, and stop splitting the hooks/projects read pairs down the middle)

ACCEPTANCE CRITERIA:
- One byte-identity assertion exists, in `internal/hookstest`, reachable from both `cmd` and `internal/hooks`.
- No suite open-codes `bytes.Equal(before, after)` over a hooks.json read.
- The optional context argument still replaces the default failure wording.
- An absent file before and after still compares equal rather than fatalling.
- The three hooks/projects pairs both read through helpers — neither half is left raw.
- `go test ./...` passes with no assertion weakened.

STATUS: complete

SPEC CONTEXT: The specification does not govern this task directly (phase 7 is an implementation-analysis consolidation, judged against its own body). It is nonetheless the assertion the spec's own acceptance rows are measured with: the spec requires that a lock timeout leaves `hooks.json` "byte-identical" on `hook set`/`hook rm` and that the sweep deletes nothing (specification.md:505), and that removing nothing is non-zero "both leaving `hooks.json` byte-identical" (:506). Consolidating the assertion means those spec-mandated properties are proved by one implementation rather than by four near-copies with drifting wording.

IMPLEMENTATION:
- Status: Implemented (with one deliberate, sound divergence from the Do list — see Notes)
- Location:
  - `internal/hookstest/hooks.go:109-122` — the single `AssertHooksFileUnchanged(t harnesstest.TestingT, path string, before []byte, context ...string)`, carrying the optional context and the original failure body verbatim (`hooks.json %s:\nbefore %s\nafter  %s`).
  - `internal/hookstest/hooks.go:93-107` — `HooksFileBytes`, the ENOENT-tolerant read the assertion's after-read goes through; `HooksJSONBytes` (`:85-91`) now delegates to it rather than restating the read.
  - `cmd/testhelpers_test.go:228-233` — `cmd`'s `assertHooksFileUnchanged` reduced to a naming shim over the shared assertion, so no `cmd` call site changed.
  - `cmd/testhelpers_test.go:126-133` — `cmd`'s `readFileBytes` reduced to a shim over `hookstest.HooksFileBytes`, preserving the prior nil-on-ENOENT semantics exactly.
  - Converted call sites: `internal/hooks/store_test.go:381,414,461,476,1034`, `internal/hooks/store_shape_test.go:36,102`, `internal/hooks/lock_write_test.go:44,65,159`, `internal/hooks/cleanstale_snapshot_test.go:137`, and the third form at `cmd/doctor_fix_transient_listpanes_shared_integration_test.go:74`. `internal/hooksweep/sweep_test.go:33,57,247,402` and `internal/hooksweep/lock_timeout_test.go:39` (later tasks) reach the same assertion, so a third package consumes it.
  - The three read pairs now read both halves through a helper: `cmd/doctor_test.go:845/853` (`assertStalePrunesApplied`), `:911/913` (`assertDownServerDeferral`), `:1561/1562` + `:1576/1577` (`TestDoctorStaleChecksAreReadOnly`).
- Notes:
  - Do-list item 4 asked for a `projects.json` read counterpart in `hookstest`. The implementation declined it and instead routed the projects half through the already path-generic `HooksFileBytes` via `cmd`'s `readFileBytes` shim, documenting the choice at `cmd/testhelpers_test.go:126-129`. The criterion's substance ("neither half is left raw") is met at all three pairs, `internal/hooks` never reads projects.json, and a `hookstest.ProjectsFileBytes` would have been mis-homed. Judged a sound divergence, not a loss.
  - The plan text located two of the three pairs at `cmd/run_hook_stale_cleanup_test.go:305,448`; those two sites are single fatal-on-absent after-reads with substring assertions, not before/after pairs, and the three actual pairs all live in `cmd/doctor_test.go`. All three were converted. (That file was subsequently deleted by task 9-12 when the sweep moved to `internal/hooksweep`.)
  - The ENOENT tolerance the conversion introduced at the doctor after-reads is fenced against going vacuous: `cmd/doctor_test.go:846-848` and `:914-916` fail explicitly on an absent-or-empty file, and `TestDoctorStaleChecksAreReadOnly` is protected by the two `checkFail` fatals at `:1570-1574` that a missing fixture file would trip first.

TESTS:
- Status: Adequate
- Coverage: `internal/hookstest/hooks_test.go:91-178` pins the promoted assertion across all five named cases — byte-identical passes (`:92`), a single changed byte fails once and carries the after bytes (`:104`), the caller's context replaces the default wording and the default is gone (`:125`), an absent file before and after neither fatals nor errors (`:160`), plus a sixth beyond the plan: a non-ENOENT read failure fatals and names the path (`:146`). Each drives the assertion through `harnesstest.Recorder`, so the helper's own failure paths are observed rather than inferred. `cmd/testhelpers_test.go:138-154` pins the read rule over `projects.json` (nil when absent, exact bytes when present), which is what makes the shared read legitimate for the non-hooks half of each pair.
- Notes: Not over-tested — six focused subtests for a helper with three behaviours (equal, unequal, absent) plus the context argument and the fatal path. No assertion was weakened in conversion: every converted site kept its own context wording (`"rewritten on a no-op removal"`, `"changed on a timed-out Set"`, `"mutated by diagnosis (read-only violated)"`, etc.), and the `modTime` companion checks that distinguish "not rewritten" from "rewritten identically" (`internal/hooks/store_test.go:382-384`, `:414-416`) are untouched.

CODE QUALITY:
- Project conventions: Followed. `hookstest` remains test-only and outside `_test.go` so both packages can import it; the assertion takes `harnesstest.TestingT` (the single declaration of that subset per CLAUDE.md) rather than `*testing.T`, which is what makes its own failure path testable. No lane rule touched — the unit-lane files stay unit-lane, and the one integration site (`cmd/doctor_fix_transient_listpanes_shared_integration_test.go`) keeps its tag.
- SOLID principles: Good. One rule, one home; the two `cmd` shims add naming and nothing else, which is what kept ~20 call sites unedited.
- Complexity: Low.
- Modern idioms: Yes — `errors.Is(err, os.ErrNotExist)` replaces the older `os.IsNotExist` in the promoted read.
- Readability: Good. `internal/hookstest/doc.go:3-5` was amended so the package doc names the assertion it now owns.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
