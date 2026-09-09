TASK: resume-hooks-silently-lost-8-39 — CLAUDE.md's logtest Row Describes A Surface That No Longer Exists (documentation-only correction of the `logtest` architecture row)

ACCEPTANCE CRITERIA:
- Every identifier the row names exists and is exported.
- The row names both return values of `NewCaptureLogger`.
- The row names no wrapper that has been deleted.
- No code changes — documentation only.

STATUS: complete

SPEC CONTEXT: This is a phase-8 implementation-analysis task, so its authority is its own body rather than the specification (per the shared verifier context: phases 6–9 are consolidation/quality tasks the implementation generated). The task's subject is CLAUDE.md's `logtest` architecture row, which had drifted from `internal/logtest`'s shipped surface during the logtest consolidation earlier in the same plan. The task body itself flags that its Do list must re-check each clause before editing, because earlier tasks in the plan had already rewritten parts of the row, and that a later task (8-42) may rewrite the row again.

IMPLEMENTATION:
- Status: Implemented
- Location: `CLAUDE.md:84` (the `logtest` table row); commit `a87427e9` — one file, one line, `+1/-1`.
- Notes:
  - Criterion 1 (identifiers exist and are exported): at the commit's parent the row already named the exported `Sink` query methods `Records` / `RecordsAtExactLevel` / `RecordsAtOrAboveLevel` / `RecordsWith` / `RecordsWithMessage` / `RecordsAtExactLevelWith` / `RecordsAtExactLevelWithMessage`, and all seven were exported in `internal/logtest/capture.go` at that revision (verified against `git show a87427e9:internal/logtest/capture.go`). The finding's original complaint — the row naming the then-unexported `atExactLevel`/`atOrAboveLevel`/`withMessage`/`matching` — had already been resolved by an earlier task's rewrite, so leaving that clause untouched is the correct application of the task's own Do step 1 ("correct only what is still wrong").
  - Criterion 2 (both return values named): this is the substantive edit. The row now reads "`NewCaptureLogger(t)` hands back a standalone `*slog.Logger` over one, returning that `*Sink` alongside it", matching `func NewCaptureLogger(t *testing.T) (*slog.Logger, *Sink)` (`internal/logtest/capture.go:257`). The clause survives verbatim at HEAD (`CLAUDE.md:84`), so the task's outcome was not undone by the later 8-42 rewrite.
  - Criterion 3 (no deleted wrapper named): `internal/restore/logging_capture_test.go` was already absent at the commit (`git cat-file -e a87427e9:internal/restore/logging_capture_test.go` fails), and the row names it nowhere — its closing clause names the removed embedded-field wrappers only as gone. The two non-twin handlers it names existed at the commit and still do: `warnBypassHandler` (`cmd/open_test.go:2859`) and `orchestrationSeqHandler` (`cmd/bootstrap/latch_test.go:35`).
  - The edit also added the phrase "beyond the ready-to-use zero value", a new claim not requested by the criteria. It holds: `Sink`'s doc comment states it (`internal/logtest/capture.go:176`), the struct is zero-value-safe (`sync.Mutex`, nil slices, nil `shared` handled by `owner()` at `internal/logtest/capture.go:185`), and real consumers construct it that way (`cmd/open_test.go:2865`, `cmd/bootstrap/orphan_sweep_test.go:105`).
  - Criterion 4 (documentation only): `git show --stat a87427e9` reports `CLAUDE.md | 2 +-` and nothing else. No Go source was touched.
  - HEAD divergence, expected and not a drift finding: the row at `CLAUDE.md:84` today is commit `abe3132a`'s (task 8-42) rewrite to the chainable exported filters `AtExactLevel` / `AtOrAboveLevel` / `WithMessage` / `Matching`. That is the successor rewrite this task's closing note anticipated, and every identifier in the current text resolves against `internal/logtest/capture.go` (lines 130, 135, 141, 149, 157, 233, 241, 250) and `internal/logtest/install.go:11`. Judging the current row's other clauses belongs to 8-42's review, not this one.

TESTS:
- Status: Adequate (no test change is the correct outcome)
- Coverage: The task is documentation-only and the commit contains no Go change, so no test could observe it. `internal/logtest`'s suite is unchanged, exactly as the task's Tests section states.
- Notes: Nothing in the tree asserts on CLAUDE.md prose, so a claim in this row is verified by reading rather than by a guard — consistent with how every other architecture row is treated. No under- or over-testing to report.

CODE QUALITY:
- Project conventions: N/A for Go conventions (no code). The edit follows CLAUDE.md's own row conventions — single table row, backticked identifiers, no process-artifact references (no task ids, phase numbers or spec sections leaked into the prose).
- SOLID principles: N/A
- Complexity: N/A
- Modern idioms: N/A
- Readability: Good — the added clause is one sub-sentence appended where the return shape is described, rather than a restatement elsewhere in the row.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
