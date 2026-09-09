TASK: resume-hooks-silently-lost-7-34 — The `doctor --fix` stand-down copy reassigns a signed-off user-facing string

ACCEPTANCE CRITERIA:
- The string `could not read live panes` appears in no renderer.
- `pane-read-failed` renders wording naming a failed enumeration; `empty-pane-read` renders wording naming an empty result. The two are not interchangeable.
- All five reasons render a non-empty phrase in `skippedPrunePhrases` and in `notEvaluableDetails`, `lock-timeout` included.
- The exact rendered line for each reason is pinned by a test on both surfaces.
- The specification's Corrigenda carries the withdrawal, naming what replaced it and why.
- Neither the `--fix` exit code nor the read-only exit code changes: both stay driven by the post-repair diagnosis.

STATUS: complete

SPEC CONTEXT:
`specification.md:255-257` signs off three literal `Skipped stale hook prune: …` lines and assigns the third, `could not read live panes`, to the empty-live-set guard (a *successful* read that returned no panes). The 2026-08-30 corrigendum (`specification.md:548`) had already widened the reason set from three to five, and the implementation had moved the signed-off string onto the new `pane-read-failed` branch while inventing new words for the guard — the silent reassignment this task exists to settle. The 2026-09-01 corrigendum (`specification.md:552`) records the resolution: the line is **withdrawn**, not reassigned; the guard renders `live pane list came back empty`, the failed enumeration renders `could not enumerate live panes`, and `notEvaluableDetails` gains its missing `lock-timeout` entry. Corrigenda are authoritative over the §5.1 body, which still shows the withdrawn line — that residue is the corrigendum mechanism working as designed, not drift.

IMPLEMENTATION:
- Status: Implemented (and legitimately moved past the task's own wording by later phases — see Notes)
- Location:
  - `cmd/doctor.go:220-226` — the shared stand-down phrase consts, including `paneReadStandDownPhrase = "could not enumerate live panes"` and `lockStandDownPhrase = "hooks.json is locked"`.
  - `cmd/doctor.go:236-243` — `skippedPrunePhrases`: `pane-read-failed` → `could not enumerate live panes`, `empty-pane-read` → `live pane list came back empty`. The withdrawn string is present in neither.
  - `cmd/doctor.go:247-254` — `notEvaluableDetails`, carrying its `lock-timeout` entry (`hooks.json is locked (not evaluable)`).
  - `cmd/doctor.go:256-262` / `:266-268` / `:352-354` — the single `phraseFor` lookup and the two renderers (`reportSkippedPrune` for `--fix`, `staleHooksNotEvaluable` for the read-only check) that draw from it.
  - `.workflows/…/specification.md:552` — the corrigendum recording the withdrawal, the two replacement phrases, and why it was withdrawn rather than restored.
  - Task commit `bd0cfb5b`; the corrigendum landed in `be5d8316`.
- Notes:
  - Criterion 1 verified by repo-wide grep: outside `.workflows/` the string survives at exactly one site, `cmd/doctor_stand_down_copy_test.go:426`, where it is the `withdrawn` constant a guard asserts the *absence* of. No renderer holds it.
  - Criterion 3 says "five reasons"; the vocabulary now holds **six** (`internal/hooksweep/reason.go:19-25`) — `marker-read-failed` was added later by T8-5 and the sweep moved to `internal/hooksweep` at T9-12. Both maps cover all six, so the criterion is met a fortiori. Judged against intent, the count moving is later work, not this task's drift.
  - Criterion 6 verified structurally: `cmd/doctor.go:33-41` declares `checkNotEvaluable` outside the exit-driving statuses, and `cmd/doctor.go:147-172` drives both exits solely through `doctorUnhealthy(results)` / `doctorUnhealthy(postResults)`. No stand-down path touches the exit code.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/doctor_stand_down_copy_test.go:56-124` — one table row per reason carrying the *whole rendered line* for both surfaces (`skippedLine`, `notEvaluableLine`) alongside the log level/attrs and the untouched-file assertion.
  - `:373-392` — a completeness arm keyed on `hooksweep.Reasons`: a declared reason with no row fails, a row naming an undeclared reason fails, and a duplicated reason fails. No subtraction list.
  - `:396-411` — distinctness is asserted on the *rendered* phrases rather than the table's expectations, which is what actually catches one branch borrowing another's words.
  - `:413-422` — per reason: the real cycle (`hooksweep.Run`), the real `doctor --fix` run (`assertSkippedPruneLine`, exact line equality at `cmd/doctor_test.go:1598`), and the real report renderer for the not-evaluable line (`renderStaleHooksLine`, `:227-240`). Both surfaces, exact lines, no tautology — the literals are pinned, only the lookup is shared with production.
  - `:441-455` — the withdrawn-string guard over both maps for every reason.
  - `:459-489` — the exit-code arm: read-only and `--fix` both nil over a stand-down, and both `ErrDoctorUnhealthy` once a genuinely failing check is added under the same stand-down, which is what stops the assertion passing by hardwiring.
  - `cmd/doctor_stand_down_phrase_guard_test.go:46-70` — the declaration-level coverage guard (`missingPhrases` / `undeclaredKeys` against `hooksweep.Reasons`, with an empty-set tripwire), which is what makes "every reason renders a phrase" survive a new reason arriving.
- Notes:
  - The task's listed test names differ from the shipped ones (`…each of the six reasons on both surfaces` rather than `…five`; the lock-timeout detail is covered by its table row plus the coverage guard rather than a named test). The named behaviours are all pinned, so this is naming, not a coverage gap.
  - `lock-timeout`'s not-evaluable line is pinned through `renderDoctorReport` with the production map rather than through a live `doctor` run, because a lock acquisition failure degrades to an unlocked read and so cannot reach that path today (`internal/hooksweep/reason.go:32-36`, and the corrigendum says the same). The row's `postRepairNotEvaluable: false` records that honestly rather than asserting a line that cannot appear.
  - The task commit deleted `TestSkippedPrunePhrase`, which had covered the then-existing unmapped-reason fallback to the raw slug. That fallback was itself removed by later work (`phraseFor`, `cmd/doctor.go:260-262`, now returns the zero value) and replaced by the declaration-level guard, so no coverage is missing from the delivered code.
  - Not over-tested: each arm asks a different question (reported outcome, log line, file untouched, both rendered surfaces, exit code), and the per-reason exit-code arm's three `doctor` runs are in-process with injected seams.

CODE QUALITY:
- Project conventions: Followed. Injected `*DoctorDeps` seams throughout, no real tmux or real config touched (`hookstest.StageStore`, `t.TempDir`), unit-lane placement correct — nothing here builds or spawns a binary. `hooks.SetLockTimeoutForTest(t, lockBound)` is the sanctioned test-side bound.
- SOLID principles: Good. Copy is data (two maps) read through one lookup; the renderers hold no branch of their own, so a reason cannot reach a user through words written at a call site.
- Complexity: Low. `phraseFor` is a single map index; both renderers are one `Fprintf`.
- Modern idioms: Yes — typed `hooksweep.Reason` map keys, `slices.Contains` / `strings.SplitSeq` in the suite.
- Readability: Good. The shared consts are declared once and composed into both registers, and the comment at `cmd/doctor.go:213-219` states why a failed marker read is worded apart from a restore.
- Issues: None. The comments in the changed region hold against the code: the phrases both vocabularies share are in fact the consts at `:220-226`; `empty-pane-read` genuinely has no shared const because its two surfaces say different things; and `phraseFor`'s claim that exhaustiveness is a test's to enforce is true of `cmd/doctor_stand_down_phrase_guard_test.go`.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
