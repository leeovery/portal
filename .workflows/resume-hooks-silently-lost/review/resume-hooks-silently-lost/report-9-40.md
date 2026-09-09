TASK: resume-hooks-silently-lost-9-40 — The internal/log Discard Guard Is Blind To Test Files And Hand-Rolls The Shared Walk (tick-86d18c)

ACCEPTANCE CRITERIA:
- [x] The guard scans test files as well as production files and fails on an open-coded `slog.NewTextHandler(io.Discard, nil)` in either.
- [x] `internal/log`'s in-package tests and `internal/prefs` are exempt for stated reasons, and no other package is.
- [x] The guard enumerates through `sourceguardtest.GoSourceFiles` and declares no walk of its own.
- [x] The two `internal/log` capture handlers are either folded onto `Sink` or left with the reach that prevented the move stated.
- [x] CLAUDE.md's `logtest` row matches whichever outcome lands.

STATUS: complete

SPEC CONTEXT: None governing. This is a phase-9 implementation-analysis task; the specification
(`.workflows/resume-hooks-silently-lost/specification/.../specification.md`) is silent on the discard guard and on
`logtest` (grep for "discard"/"logtest" returns only the unrelated restore re-stamp paragraph). Judged against the
task body, per the shared verifier context.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/log/discard_guard_test.go` (whole file rewritten by eb50db78), `CLAUDE.md:62` (`log` row),
  `CLAUDE.md:84` (`logtest` row).
- Notes:
  - Test files are now in scope: `scanForDiscardConstruction` (`internal/log/discard_guard_test.go:121-142`) reads
    `sourceguardtest.RepoSources(t, sourceguardtest.AllSources, …)`, replacing the previous `NonTestSources`
    selection (`git show eb50db78^:internal/log/discard_guard_test.go`). `TestNoDiscardLoggerConstruction`
    (`:16-22`) drives it over the real tree.
  - Exemptions are a single closed switch, `discardConstructionExempt` (`:151-163`): `internal/log` exempts its
    test files and `discard.go`; `internal/prefs` exempts its **non**-test files. No third directory. The stated
    reasons sit directly above it (`:144-150`).
  - The `internal/prefs` ruling is inverted relative to the task's Do list, which asked for prefs' *test* files to
    be exempt. The implemented ruling is the correct one and is verifiable: `internal/prefs/leaf_guard_test.go:16-20`
    asserts the leaf rule through `sourceguardtest.AssertDepsWithin`, whose `go list -deps` invocation
    (`internal/sourceguardtest/packagedeps.go:94-98`) carries no `-test`, so the rule binds production imports
    alone — and `internal/prefs/translation_saver_test.go:13-14` already imports `internal/log` and
    `internal/logtest`. Exempting prefs' test files as the plan's wording asked would have taken a package that
    demonstrably has the route out of the rule. Sound divergence, not a loss; the fourth rule case (`:64-71`) and
    its comment record it.
  - "Declares no walk of its own": already true before this commit (the prior guard also went through
    `RepoSources`); the task description's claim of a local `filepath.WalkDir` was stale. The criterion holds —
    `RepoSources` composes `GoSourceFiles` at `internal/sourceguardtest/reposources.go:95`, and the guard declares
    no walk, no exclusions and no root resolution of its own.
  - The two capture handlers were left in place with the reach stated: `recordingHandler`
    (`internal/log/log_test.go:111`, also consumed by `close_exit_test.go`) and `componentCapture`
    (`internal/log/rotate_test.go:15`). Both files are `package log` and both drive unexported machinery —
    `setHandler` (`log_test.go:73`) and `newRotatingSink` (`rotate_test.go:79`) — so the fold would mean either
    exporting that machinery or importing `logtest` (which imports `internal/log`) from `package log`. CLAUDE.md's
    `logtest` row now says exactly that, and every claim it makes checks out: `TestSetEmitsOpAsJSONField` exists
    (`internal/hooks/store_test.go:1255`) and installs a `slog.NewJSONHandler` over a buffer (`:1260`);
    `warnBypassHandler` is at `cmd/open_test.go:2859`; `orchestrationSeqHandler` at `cmd/bootstrap/latch_test.go:35`.
  - The widened guard passes over the tree as it stands: the only occurrences of the forbidden construction are
    `internal/log/discard.go:8` and the guard file's own const and fixture (`:14`, `:32`), all inside the exempted
    directory.

TESTS:
- Status: Adequate
- Coverage: `TestDiscardGuard_Rule` (`:24-96`) drives the same `scanForDiscardConstruction` over staged trees via
  `sourceguardtest.Rooted`, with all four named cases present — a finding in a test file, the `internal/log`
  exemption, the `internal/prefs` exemption, the shared-walk exclusions (vendor / dot-dir / node_modules) — plus
  the extra case pinning that prefs *test* files stay under the rule. Each case asserts `scanned` as well as
  `findings`, so an exempt or excluded file is proven skipped rather than merely non-matching.
  `TestDiscardGuard_FatalsWhenItScansNothing` (`:99-111`) pins the scanned-nothing tripwire through
  `harnesstest.Recorder`; the "stopped looking" substring it asserts is produced by `ParseSources`
  (`internal/sourceguardtest/parsesources.go:82`), so the assertion is anchored to real text.
- Notes: The rule is exercised through the same function the real scan uses, so the two cannot drift. Not
  over-tested — the six cases are disjoint. One thing left unasserted is the `t.Errorf` wording of a reported
  finding (`:20`); not worth a test.

CODE QUALITY:
- Project conventions: Followed. Unit lane, no build tag (correct — `sourceguardtest` is untagged and compiles no
  binary); reports through `harnesstest.TestingT` so its own fatal path is testable; `t.Fatalf` followed by a
  defensive `return`, matching the `sourceguardtest` house style; no `t.Parallel()`.
- SOLID principles: Good. Enumeration, exemption and reporting are three separate units; the exemption rule is one
  function with one switch.
- Complexity: Low.
- Modern idioms: Yes (`slices.Equal`, table-driven subtests, variadic `ScanOption` pass-through).
- Readability: Good. The exemption comment states why each directory is exempt rather than restating the switch.
- Issues: None. `RepoSources` parses every file and the guard then re-reads the bytes with `os.ReadFile`, so each
  source is touched twice — a preference about which primitive to compose (`GoSourceFiles` + `ProjectRoot` would
  avoid the parse), not a defect: the parse is what supplies root-relative paths and the scanned-nothing fatal the
  guard's own test depends on, and the cost is invisible against the unit lane.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
