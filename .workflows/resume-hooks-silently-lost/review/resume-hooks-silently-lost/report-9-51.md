TASK: resume-hooks-silently-lost-9-51 (tick-28c67a) — The Source-Reading Guards Certified Under `go test -overlay` Are Unverified

ACCEPTANCE CRITERIA:
- Every `sourceguardtest`-driven source-reading guard has been observed failing against a real violation introduced in a scratch copy.
- No guard's certification rests on a `go test -overlay` probe.
- The verification method and the reason an overlay cannot serve are recorded in `internal/sourceguardtest`'s package documentation.
- Any guard found not to bite is named and either fixed or raised.
- The working tree is left unmodified by the verification itself.

STATUS: issues_found (one non-blocking comment-accuracy finding; the deliverable itself is in place)

SPEC CONTEXT: This is a phase-9 implementation-analysis task, so its authority is its own body rather than the specification. Its subject is the ~20 unit-lane source guards driven by `internal/sourceguardtest` — the machinery CLAUDE.md describes in the `sourceguardtest` row (`GoSourceFiles`, `PackageGoFiles`, `ParsePackageSources`, `RepoSources`, `PackageDeps` …). The claim it rests on is verifiable in the tree: `ParseSources` calls `parser.ParseFile(fset, path, nil, ParseMode)` with a nil `src` (internal/sourceguardtest/parsesources.go:77), so every guard routed through `ParsePackageSources` (:49) or `RepoSources` reads the file from disk, and `ProjectRoot` walks up from `os.Getwd()` (internal/portalbintest/build.go:18) — the real package directory under `go test`. An overlay, which substitutes only the go command's build inputs, therefore cannot reach what these guards read. The task's argument is sound against the code.

IMPLEMENTATION:
- Status: Implemented (documentation deliverable), with one over-broad claim.
- Location: internal/sourceguardtest/doc.go:16-27 (added whole by commit 75bcd793, the task's only source change).
- Notes:
  - Criterion 3 is met: the paragraph records both the method (scratch copy, introduce the violation, run, discard — never edit the tree back) and the reason an overlay cannot serve (build-input substitution vs. a nil-`src` disk read, plus the mixed-source case where a guard compares parsed literals against sibling values from the compiled package). Each factual claim in it holds against the tree — I checked the nil `src` at parsesources.go:77 and the getwd-anchored root at build.go:18.
  - Criterion 5 is met: `git status` shows no source modification outside `.workflows/`, and the task's commit touches doc.go alone.
  - Criteria 1, 2 and 4 describe a verification exercise that by its own design ("the working tree is left unmodified by the verification itself") leaves no artifact. Nothing in the tree contradicts them: no `go test -overlay` probe, script or guidance survives anywhere in the Go sources, CLAUDE.md or `.claude/skills` (the only `overlay` hits are TUI modal prose and unrelated workflow-engine code). No guard was named as non-biting by this task; the one guard-family defect of this class in the work unit — a dependency guard's verdict served stale from the test cache — was raised and fixed separately as phase-10 task 10-1 (commit ac02d961, `internal/sourceguardtest/packagedeps.go` + `packagedeps_cacheinputs_test.go`), originating in task 9-19 rather than here.
  - The one substantive problem is that the recorded method is written as a universal. See FINDINGS.

TESTS:
- Status: Adequate (no new test expected or added).
- Coverage: The deliverable is a package doc comment plus a one-off verification exercise; the task's "Tests" entries are narrations of that exercise, not code tests. No test could assert "a human ran the guards in a scratch copy", and adding one would be over-testing.
- Notes: Where a guard's biting half *can* be pinned permanently, the tree already does it, and better than a scratch copy does — `internal/log/discard_guard_test.go:86` and `internal/restoretest/literal_guard_test.go:170` drive the same scan over a staged fixture tree via `sourceguardtest.Rooted`, with expected-findings cases (e.g. discard_guard_test.go:70) that fail if the guard stops biting. That route is durable where the scratch copy is a one-off, which is why the doc's omission of it matters (finding below).

CODE QUALITY:
- Project conventions: Followed. Package doc lives in `doc.go`; the comment is one continuous block ending at `package sourceguardtest`; backtick-quoted identifiers match the style of the package's sibling comments (e.g. reposources.go:50-52).
- SOLID principles: N/A (documentation-only change).
- Complexity: Low.
- Modern idioms: N/A.
- Readability: Good — the paragraph states the method, then the mechanism, then what an overlay does and does not prove, in that order.
- Issues: The method sentence claims exclusivity the package's own API contradicts (see FINDINGS).

BLOCKING ISSUES:
- None. The acceptance criteria that can be observed in the tree are met, and the single finding's entire remedy is comment text.

FINDINGS:
- [in-scope] [contained] internal/sourceguardtest/doc.go:16 — "Showing that a guard built on these primitives bites means copying the tree to a scratch location … never editing the tree back" states the scratch copy as the only route, but this package exposes `Rooted` for exactly the opposite purpose — its own doc at internal/sourceguardtest/reposources.go:42-45 says it exists "so a guard's own rule tests drive the same scan over a staged fixture", and two guards already prove their biting half that way as permanent tests (internal/log/discard_guard_test.go:86 with the expected-findings case at :70; internal/restoretest/literal_guard_test.go:170). Re-voice the sentence so the scratch copy is the fallback for a guard anchored at its own package directory — the shape that cannot be re-pointed, e.g. `ParsePackageSources(t, ".", false)` at cmd/hooks_pane_token_width_guard_test.go:24 — and name the fixture-driven rule test as the route to prefer where the guard's scan takes a root or a directory. — FAILS: the paragraph is the place the task set out to make "where the next round will look", so a contributor adding a guard whose scan does take a root follows it into a one-off manual ceremony and ships no regression test, where the package supports a permanent one; and as written the sentence is untrue of the guards that are already covered structurally.
