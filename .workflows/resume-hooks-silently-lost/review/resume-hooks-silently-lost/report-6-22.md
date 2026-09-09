TASK: resume-hooks-silently-lost-6-22 — Retire The Three Subsumed Test Cases (tick-1edef2)

ACCEPTANCE CRITERIA:
- Each of the three is deleted or differentiated; none remains as a strict subset.
- No assertion present today is lost: for each deletion, name the surviving test that makes it.
- `cmd/doctor_test.go:1132` (the "it reads the marker before the empty-live-set branch" sibling) is untouched.
- Tests: the surviving suites passing, with the case-count delta stated.

STATUS: complete

SPEC CONTEXT:
Phase 6 is an implementation-analysis cycle, so this task's authority is its own body rather than the
specification. The subject matter sits on the spec's stale-hook-judgement surface: `checkStaleHooks`
(cmd/doctor.go:363) must consult the `@portal-restoring` marker before it enumerates live pane tokens,
because a skeleton's panes carry no token yet and a count taken inside a restore window would report every
token-shaped key on the machine as lost. That ordering is what the differentiated case now pins.

IMPLEMENTATION:
- Status: Implemented (commit 2ad00331; parent 0eb1d55d)
- Location:
  - Deletion (a): `TestDoctorFixPrunedHookOutput` removed from `cmd/doctor_test.go` (was at :1596-1612 pre-commit).
    Gone repo-wide — the only surviving mentions are `.workflows/` analysis artifacts.
  - Differentiation (b): `cmd/doctor_test.go:1095-1105` — "it reads the marker before counting" now asserts
    `lister.calls != 0` → fail, above an explanatory comment at :1092-1094.
  - Deletion (c): `cmd/hooks_test.go` — "it errors when TMUX_PANE is unset for set" removed from
    `TestHooksSetCommand` (was at :495-515 pre-commit). Gone repo-wide.
- Notes:
  - Criterion 1 holds for all three. (b) is a real differentiation, not a cosmetic one: `checkStaleHooks`
    (cmd/doctor.go:374-381) calls `hooksweep.StalenessStandDown` before `hooksweep.JudgeAgainstLivePanes`,
    and the latter is the sole caller of `reader.ListAllPaneHookKeys` on the read-only path
    (internal/hooksweep/sweep.go:86). Under this fixture (one reapable persisted key, a non-empty live set)
    an inverted ordering would still render `checkNotEvaluable` + the ReasonRestoring phrase — the comment's
    stated reason — but would leave `lister.calls == 1`, so the new assertion is the only thing that would
    catch the regression. `stubStaleSweepReader.calls` is incremented in `ListAllPaneHookKeys`
    (cmd/hookkey_vocabulary_test.go:102-108), and `runDoctorDiagnosis` (cmd/doctor.go:311) is the read-only
    path, so no other check contributes to the count.
  - Criterion 2 holds; naming the survivors, as the criterion requires:
    - (a) `TestDoctorFixPrunesStaleEntriesThenRediagnosesClean` (cmd/doctor_test.go:1375). At the commit it
      drove the identical fixture (`seedStalePruneFixture(t, t.TempDir(), staleHookLister())`) through the
      identical driver (`runDoctorFixCmd`) and made the identical `assertStalePrunesApplied` call plus four
      further assertions. The intent the deleted comment carried — that `--fix` stdout names the key and
      not the reaped command — is preserved by the exact-equality pruned-line assertion inside
      `assertStalePrunesApplied` (cmd/doctor_test.go:869-873).
    - (c) `cmd/hooks_test.go:328` "returns error when TMUX_PANE is not set" (in `TestHooksSetCommand`). Same
      fixture, same argv, and it makes both of the deleted case's assertions (non-nil error; error text
      contains "must be run from inside a tmux pane") plus the `os.Stat(hooksFile)` not-created check at
      :347. The same-named case at `cmd/hooks_test.go:478` is the `hooks rm` variant, a different command.
  - Criterion 3 holds: the diff's only doctor_test.go hunks are the added assertion/comment and the deleted
    function. "it reads the marker before the empty-live-set branch" (pre-commit :1095, HEAD :1085) is
    byte-identical across the commit.
  - No orphaned helper: `runDoctorFixCmd` still had 25 other call sites at that commit (it was later renamed
    by task 7-18), so removing its caller left no unused declaration for `golangci-lint`'s `unused` to flag.
  - The deletion in `cmd/hooks_test.go` also removed the stray blank line that preceded the closing brace, so
    the region stays gofmt-clean.

TESTS:
- Status: Adequate
- Coverage: This task *is* a test change, so the assessment is what the suite still holds. Nothing observable
  was lost: both deletions are strict subsets of a named survivor (verified assertion-by-assertion against the
  pre-commit bodies), and the differentiation adds one assertion that no other case in the file makes for the
  counting branch. The parallel ordering pin for the failed-marker-read branch
  (cmd/doctor_test.go:1074-1083) uses the same `lister.calls` route with matching wording, so the two read as
  a pair.
- Notes:
  - The commit records the delta as required by Do item 4: functions 747 → 746, subtests 854 → 852, total
    1601 → 1598. I could not run the suite, but the deltas are corroborated by source counts across the
    commit (`func Test` in `cmd/*_test.go`: -1; top-level `t.Run(` : -2), and the absolute figures are
    internally consistent (747+854=1601, 746+852=1598).
  - Unit-lane, `cmd` package, no `t.Parallel()`, no real tmux — the reader is a stub, so the lane and
    isolation invariants are untouched.

CODE QUALITY:
- Project conventions: Followed. Test-only change in the unit lane; no seam assigned directly (the fixture
  goes through `staleDeps`/`runDoctorDiagnosis`), no tmux or filesystem reach beyond `t.TempDir()`.
- SOLID principles: N/A (test edit).
- Complexity: Low.
- Modern idioms: Yes. The added failure message follows the file's `got = X; want Y` convention and carries
  the reason in parentheses, matching its sibling verbatim in shape.
- Readability: Good. The added comment states why the rendered result is insufficient evidence for the
  ordering — a claim I verified against `cmd/doctor.go:374-381` and `internal/hooksweep/sweep.go:86`, and it
  holds. It names no task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
