TASK: resume-hooks-silently-lost-7-33 — internal/portalbintest Compiles The Portal Binary In The Unit Lane
(tick-5154eb). Move `internal/portalbintest/build_test.go` behind `//go:build integration` and widen
CLAUDE.md's lane rule so a test that *builds* the CLI is as much an integration test as one that execs it.

ACCEPTANCE CRITERIA:
1. `go test ./...` compiles no portal binary; `internal/portalbintest`'s build test runs only under `-tags integration`.
2. `go test -tags integration -p 1 ./...` still covers the build helper.
3. `ProjectRoot` keeps unit-lane coverage, since unit-lane source guards depend on it.
4. CLAUDE.md's lane rule names builds alongside daemon spawns and binary execs.
5. The unit lane's wall time is measurably lower and the figure is in the commit message.

STATUS: complete

SPEC CONTEXT: This is a phase-7 implementation-analysis task, so its authority is its own body rather than
the specification — but the two are aligned rather than merely non-conflicting. The specification's §9.1
lane statement (`specification.md:479`) now reads with three clauses ("every test that builds a `portal`
binary, spawns a `portal state daemon`, or execs a built `portal` binary carries `//go:build integration`"),
and Corrigendum 2026-09-01 (`specification.md:562`) records the correction explicitly, noting no test
placement the specification prescribes is invalidated by the wider rule. The task's outcome and the spec's
corrected text therefore say the same thing.

IMPLEMENTATION:
- Status: Implemented (with one acceptance criterion deliberately and defensibly not met — see Notes)
- Location:
  - `internal/portalbintest/build_test.go:1-4` — `//go:build integration` plus the note explaining why
    ("This file builds the portal CLI, so it lives in the integration lane: the unit lane compiles no
    portal binary"). `TestStagePortalBinary` (`:18`) is unchanged in substance.
  - `internal/portalbintest/project_root_test.go:1-32` — new untagged file holding `TestProjectRoot`,
    split out of the tagged file so the guards' shared primitive keeps its unit-lane coverage.
  - `CLAUDE.md:15` — the lane sentence now reads "Every test that builds a `portal` binary, spawns a
    `portal state daemon`, or execs a built `portal` binary lives behind `-tags integration`", followed by
    the "Building counts, not just running" paragraph.
  - `CLAUDE.md:112` — "Lane rule first" restated with the **builds** clause and the lane-purity (not speed)
    justification.
  - `CLAUDE.md:9` — the unit-lane comment reads "no portal binary built or run".
  - `CLAUDE.md` architecture-table row for `portalbintest` — records that its own build test is
    integration-tagged while `ProjectRoot` keeps its unit-lane test.
- Notes:
  - Criterion 1 verified structurally: `internal/portalbintest/build.go:67` is the repo's only
    `exec.Command("go", "build", …)`; the only other Go-toolchain shell-outs in the tree are
    `go list` (`internal/sourceguardtest/packagedeps.go`, `internal/tmux/target_composition_guard_test.go`)
    and `go env` (`cmd/testmain_isolation_test.go`), none of which produce a binary. Every file that calls
    a build helper (`BuildPortalBinary`, `StagePortalBinary`, `BuildPortalBinaryDir`,
    `BuildPortalBinaryStable`) carries `//go:build integration` on line 1 — I enumerated them: 28 `_test.go`
    files plus `internal/restoretest/restoretest.go`, all tagged. The two untagged files in
    `internal/portalbintest` that mention those names — `lane_guard_test.go:20-25` (a `[]string` vocabulary)
    and `lane_guard_rule_test.go:15,27` (calls inside fixture *string literals*) — make no such call.
  - Criterion 3 verified: `project_root_test.go` is untagged, and `internal/sourceguardtest/reposources.go:56`
    is the one production-side caller of `portalbintest.ProjectRoot`, through which the repo's untagged
    source guards reach it. Following the task's step 1 literally (tagging the file `TestProjectRoot` lived
    in) would have stranded that primitive's direct coverage outside the lane that runs its consumers; the
    implementer's split is the correct reading of intent over letter.
  - Criterion 5 is not met and the commit message says so in plain terms, with the measurements that
    disprove its premise: three independent runs found the unit lane unchanged within noise (medians
    42.54s → 42.66s) because the package sits off a critical path set by `internal/tui`. The Do list's stated
    purpose for the measurement — "so the change's whole justification is checkable" — is met: the figures
    are in the commit message (warm relink ~0.2s; first integration-tagged build ~1.1-1.6s and ~13 MB of
    cache; the package alone 0.70s → 0.37s), and both CLAUDE.md sites were rewritten to rest the change on
    lane purity rather than on a speed claim the measurement destroyed. This is a sound divergence, not a
    loss: substituting an honest justification for a false one is strictly better than satisfying the
    criterion's words.
  - Two recorded fix rounds (`fix-tracking-resume-hooks-silently-lost-7-33.md`) removed three falsifiable
    claims from CLAUDE.md ("a full compile of the whole program on every run", "the fast lane shells out to
    no toolchain", "every consumer of `portalbintest`", "a ~42s critical-path package"). I re-read the
    current text: all four corrections are present, and the surviving claims hold — "the fast lane builds no
    portal binary" and "every consumer of `portalbintest`'s **build helpers** already lives in the
    integration lane" are both true against the tree as enumerated above.

TESTS:
- Status: Adequate
- Coverage: `TestStagePortalBinary` keeps its whole subject in the integration lane — the binary exists at
  `binDir/portal` (`build_test.go:27-30`), `PATH` is prepended rather than replaced (`:34-41`, holding the
  "a system-installed portal must not shadow the freshly built one" ordering property), and `exec.LookPath`
  resolves under the staged dir with `EvalSymlinks` on both sides so a symlinked `$TMPDIR` cannot
  false-negative (`:43-58`). `BuildPortalBinary` stays covered transitively through `StagePortalBinary`,
  exactly as before the move. `TestProjectRoot` keeps its two assertions — a `go.mod` under the returned root
  and the module path inside it, the second guarding against a stray parent `go.mod`.
- Notes: no coverage was added or removed by this task, only relocated, which is what the task asked for.
  The rule the task established later acquired a structural guard
  (`internal/portalbintest/lane_guard_test.go`, `TestBuildHelpersStayInTheIntegrationLane`), so the placement
  this task made by hand is now enforced by a test rather than by discipline — that guard belongs to a later
  phase and is not scored here.

CODE QUALITY:
- Project conventions: Followed. The lane rule this task edits is the convention, and the tree now matches
  the rule as written. `t.Setenv` (not `t.Parallel`) keeps the moved test compatible with the no-parallel
  house rule and with `-p 1`. No isolation concern: the test spawns no daemon and touches nothing outside a
  `t.TempDir` and its own restored `PATH`.
- SOLID principles: N/A (a build-constraint move and a file split).
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. The explanatory comment between `//go:build` and `package` in `build_test.go:3-4` was
  challenged during the fix rounds and confirmed as house style — `cmd/reattach_integration_test.go:3-6`,
  `cmd/state_daemon_self_supervision_integration_test.go:3-5` and
  `cmd/state_daemon_hysteresis_measurement_test.go:3-8` do the same. Both new comments state why each file
  sits in its lane, which is the one thing a reader of a build constraint needs.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
