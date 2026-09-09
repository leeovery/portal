TASK: resume-hooks-silently-lost-8-5 — A Failed @portal-restoring Read Is Reported To The User As Restore In Progress

ACCEPTANCE CRITERIA:
- A failed `@portal-restoring` read renders a phrase naming the read failure on both surfaces, and never asserts a restore.
- A marker that is genuinely set still renders the restore phrase on both surfaces.
- The cycle still stands down on a failed read — no key is judged and nothing is written.
- The completeness guards over `skipReasons` and both phrase maps cover the new reason.

STATUS: complete

SPEC CONTEXT:
The specification's 2026-09-04 corrigendum (`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md:568`) ratifies exactly this split: the sweep declines for **six** reasons, not five, the sixth being `marker-read-failed` when the `@portal-restoring` read itself fails. It states the posture is shared with a set marker (cycle stands down, diagnosis reports not-evaluable) but the reason and phrase are the branch's own — `could not read the restore marker` on both the `--fix` line and the read-only diagnosis. The corrigendum names the same rationale the task does: `hook` and `doctor` are bootstrap-exempt and start no server, so a read-only `portal doctor` with the server down is the ordinary path into that branch, and a phrase asserting a restore would be a fresh false statement on the one command that exists to report honestly on a broken install. The corrigendum is authoritative over the spec body, and this task is what satisfies it.

Note on location: task 8-5 landed in `cmd/run_hook_stale_cleanup.go` (commit 88c028cf). Phase 9's task 9-12 (commit a4898f41) subsequently moved the whole cycle to `internal/hooksweep`, renaming `skipReason*` → the typed `hooksweep.Reason` constants and leaving the copy tables in `cmd/doctor.go`. Verification below is against the delivered state of the tree, tracking the substance across that move.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooksweep/reason.go:20` — `ReasonMarkerReadFailed Reason = "marker-read-failed"`, declared beside the other five.
  - `internal/hooksweep/reason.go:35-42` — `Reasons` enumerates it (the successor to `skipReasons`).
  - `internal/hooksweep/sweep.go:71-80` — `StalenessStandDown` keeps the fail-safe fold (`state.RestoreWindowActive(state.IsRestoringSet(reader))`) and picks the reason from the read: `err != nil` declines under `ReasonMarkerReadFailed` carrying the `error` attr at DEBUG; a clean read reporting the marker set declines under `ReasonRestoring`. Both are `declineDebug`, so the level is unchanged from before the split.
  - `cmd/doctor.go:221-222` — `restoreStandDownPhrase = "restore in progress"` and `markerReadStandDownPhrase = "could not read the restore marker"` declared as separate consts.
  - `cmd/doctor.go:238` and `cmd/doctor.go:249` — the new reason has its own entry in `skippedPrunePhrases` and `notEvaluableDetails`; neither composes `restoreStandDownPhrase`.
  - `cmd/doctor.go:375-377` — the read-only diagnosis takes `StalenessStandDown` before `JudgeAgainstLivePanes`, so a failed read reports not-evaluable without enumerating.
  - `internal/hooksweep/sweep.go:155-157` — `Run` takes the stand-down before `store.CleanStale`, so nothing is read or written on a failed marker read.
- Notes:
  - Do-item 4 is honoured: commit 88c028cf touches four files (`cmd/doctor_stand_down_copy_test.go`, `cmd/doctor_test.go`, `cmd/run_hook_stale_cleanup.go`, `cmd/run_hook_stale_cleanup_test.go`) and none in `internal/state` — `RestoreWindowActive`'s fail-safe semantics (`internal/state/markers.go:108-110`) are untouched, and `standDownAttrs` (`internal/hooksweep/standdown.go:14-16`) still stamps `op`/`via` identically for every reason.
  - The `notEvaluableDetails` entry carries no `(not evaluable)` suffix, matching the two other read-failure reasons (`ReasonStoreReadFailed`, `ReasonPaneReadFailed`); the suffix is reserved for reasons describing a state rather than a failed read. The `checkNotEvaluable` status glyph carries that half of the message. Consistent, not drift.
  - The `reason` log attr is an existing key in the closed vocabulary; only a new value was added, so no spec amendment to the logging taxonomy was required.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1 (read failure renders its own phrase, never a restore): `cmd/doctor_test.go:1074-1083` ("it reports a failed marker read as its own reason") drives the diagnosis with `restoringErr` set and asserts through `assertMarkerReadFailedResult` (`cmd/doctor_test.go:1174-1182`), which pins the detail to `phraseFor(notEvaluableDetails, hooksweep.ReasonMarkerReadFailed)`. `cmd/doctor_test.go:1107-1119` covers the real-world trigger — the server down, both reads failing — and asserts the same result. The `--fix` surface is pinned by the `restore marker unreadable` row in `cmd/doctor_stand_down_copy_test.go:69-79`, whose `skippedLine`/`notEvaluableLine` are the whole rendered user lines (`Skipped stale hook prune: could not read the restore marker` / `  · stale hooks: could not read the restore marker`), driven through `assertStandDownRepair` (`:313-336`). `TestStandDownCopy`'s "it renders a distinct phrase for each of the six reasons on both surfaces" (`:364-379`) reads the *rendered* phrases and would fail if the marker-read branch borrowed the restore wording.
  - Criterion 2 (a genuinely set marker still says restore): `cmd/doctor_test.go:1064-1070` via `assertRestoreWindowResult` (`:1162-1170`), plus the `restore window` copy row (`cmd/doctor_stand_down_copy_test.go:58-68`) pinning both rendered lines.
  - Criterion 3 (still stands down; nothing judged, nothing written): `internal/hooksweep/sweep_test.go:272-287` asserts `DeclineReason == ReasonMarkerReadFailed` and `lister.calls == 0`, so the enumeration is never reached. `assertStandDownSweep` (`cmd/doctor_stand_down_copy_test.go:289-309`) additionally pins `Removed` empty and the hooks.json bytes byte-identical before/after for every reason including this one. `cmd/doctor_test.go:1080-1082` pins zero enumerations on the diagnosis path.
  - Criterion 4 (completeness guards cover the new reason): `cmd/doctor_stand_down_phrase_guard_test.go:47-93` ranges over `hooksweep.Reasons` for both maps, in both directions (no missing phrase, no undeclared key), and proves its own rule catches an omission. `internal/hooksweep/reason_enumeration_guard_test.go:22-57` reads the AST and fails a const declared with the `Reason` type that `Reasons` omits — so the new reason could not have been added without enrolment. `cmd/doctor_stand_down_copy_test.go:342-358` fails any declared reason with no copy row.
  - The named test `"it names a phrase for every stand-down reason on both surfaces"` exists verbatim at `cmd/doctor_stand_down_copy_test.go:381-392` and drives every row through the sweep, the repair and the rendered not-evaluable line.
- Notes:
  - `internal/hooksweep/sweep_move_test.go:59-63` also covers the reason, but its subject is the emitted level/attrs across all six reasons rather than the ordering `sweep_test.go:272` pins; the overlap is a different question, not a duplicate assertion.
  - The stand-down log line is pinned at DEBUG with the `error` attr exactly equal to the read failure (`standDownErrorAttrExactly("no server running")`, `cmd/doctor_stand_down_copy_test.go:75`), which is what the task asked to leave unchanged.

CODE QUALITY:
- Project conventions: Followed. The `hooks` component binding stays inside `internal/hooksweep` (`sweep.go:14`), the copy tables stay in `cmd` beside the renderers that print them, and both match the architecture table in CLAUDE.md. No new log component or attr key was invented. All new tests are unit-lane and hermetic (stub readers, `hookstest.StageStore` temp dirs), correctly untagged.
- SOLID principles: Good. The reason vocabulary is a closed type owned by the cycle; the rendering vocabularies are owned by the surface. `StalenessStandDown` decides one thing and reports it.
- Complexity: Low. The change is a two-arm switch replacing a single `if`.
- Modern idioms: Yes — typed `Reason` string constants, tagless switch, map-driven rendering with a fall-through that renders nothing rather than a slug.
- Readability: Good. `sweep.go:62-70`'s doc comment states why a failed read must not claim a restore; `cmd/doctor.go:214-219` states why the two phrases are separate consts. Both hold true against the code.
- Issues: None reaching the bar. The `RestoreWindowActive` fold at `internal/hooksweep/sweep.go:72` is now only consulted on the `err == nil` arm, so its `restoring || err != nil` collapse is no longer observable at this call site — but the task explicitly directed keeping the fold, the helper has three other production callers (`cmd/state_commit_now.go:107`, `cmd/state_daemon.go:175`, `cmd/state_daemon.go:340`), and nothing about the site is incorrect or misdescribed. Simplification here would be a preference, so it is not reported as a finding.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
