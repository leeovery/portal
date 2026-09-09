TASK: resume-hooks-silently-lost-2-8 — "Site The Row Vocabulary With The Seeds, And Close The Literal Bypasses" (severity: drift)

ACCEPTANCE CRITERIA:
- The row builders sit with the seed vocabulary they compose with
- No hand-written token literal survives at a fixture that stamps or seeds one
- No fixture named for liveness survives on unjudgeability
- Every test keeps its name, cases and expected counts
- Both lanes pass

STATUS: complete

SPEC CONTEXT:
The specification's §9.3 ("Existing tests to re-point or retire") already prescribes re-pointing the seeded keys in
the destructive integration suites at token shapes, naming `cmd/cleanstale_transient_listpanes_doctorfix_integration_test.go`,
`cmd/state_daemon_hook_cleanup_integration_test.go` and `cmd/rename_restore_cleanup_survival_integration_test.go`
specifically. §3.2 is the reason it matters: a hook key is token-shaped iff it is exactly the pane-token width drawn
from `nanoid.Alphabet`, and §5.2's staleness rule reaps only token-shaped-or-empty keys while retaining everything
else forever. A fixture that hand-writes a "token" therefore measures the reap only while that literal happens to
stay token-shaped — a width or alphabet move silently converts a reap fixture into a retention fixture with nothing
failing. This task is the consolidation of that re-point: one declaration of what a test hook key is, so the mint's
own constants carry every fixture along.

IMPLEMENTATION:
- Status: Implemented (and carried forward, strengthened, by later phases)
- Location (delivered change-set, commit f32e70d8):
  - `cmd/hookkey_vocabulary_test.go` (new) — row builders `tokenRows`/`unstampedRows`, `restoringOption` and the
    `recordingHookKeyLister` fake moved in beside the seed vocabulary
  - `cmd/bootstrap_production_test.go` — row builders removed from the bootstrap file they had no business in
  - `cmd/cleanstale_transient_listpanes_doctorfix_integration_test.go` — `liveKey = "livetk"` re-pointed at the vocabulary
  - `cmd/rename_restore_cleanup_survival_integration_test.go` — `renameRestoreToken = "tokrst"` re-pointed
  - `cmd/state_daemon_hook_cleanup_integration_test.go` — `liveHookToken = "livetk"` re-pointed
  - `cmd/run_hook_stale_cleanup_test.go` — the two `"live:0.0"` literals re-pointed at the unjudgeable seed
- Location (current head, after the phase 6–9 consolidations moved the code):
  - Row builders: `cmd/hookkey_vocabulary_test.go:41` (`tokenRows`) and `:49` (`unstampedRows`), under a header
    comment that states where the seed keys now live
  - Seed vocabulary: `internal/hookstest/hooks.go:167-206` (`ReapableSeedA..D`, `LiveSeedA..C`, `UnjudgeableSeedA..C`,
    `SubjectSeedA..D`), minted through `tokenShapedHookKey` (`internal/hookstest/hooks.go:127`), which reads the
    width off `nanoid.NewPaneTokenGenerator()` and panics if the result is not `nanoid.IsTokenShaped`
  - Re-pointed integration fixtures: `cmd/doctor_fix_transient_listpanes_integration_test.go:101,104-105`,
    `cmd/hook_prune_rename_survival_integration_test.go:40,52-53`,
    `cmd/state_daemon_hook_cleanup_integration_test.go:100,109-111`
  - Re-pointed unjudgeable fixtures: `internal/hooksweep/sweep_test.go:385` and `:445`
- Notes:
  - The three named literals are gone from the tree: a repo-wide grep for `livetk` and `tokrst` over `*.go` returns
    no matches. The surviving `"live:0.0"` strings are pane *locations* (a `session:window.pane` target handed to
    `StampPaneToken`, or the display-only half of an enumeration row) rather than hook keys — e.g.
    `cmd/doctor_fix_transient_listpanes_integration_test.go:101` and `cmd/state_daemon_hook_cleanup_test.go:23` —
    which is the correct use of that shape.
  - Later phases moved the seed keys out of `cmd` into `internal/hookstest` so `internal/hooks`, `internal/hooksweep`
    and `cmd` share one declaration. That is a divergence from the task's literal wording ("beside the seed
    vocabulary" then meant *inside* `cmd/hookkey_vocabulary_test.go`) and it is an improvement, not a loss: the
    Outcome the task states — "One place declares what a test hook key is" — is more fully met by a home three
    packages can reach than by one `cmd` file, and both `cmd/hookkey_vocabulary_test.go:1-7` and
    `internal/hooksweep/helpers_test.go:1-4` carry a header comment saying exactly where the keys live and why.
  - The seam fakes the task moved (`stubAllPaneLister`, `recordingHookKeyLister`) were subsequently renamed and
    consolidated into `stubStaleSweepReader` / `recordingPaneHookLister`; neither old name survives anywhere in the
    tree, so nothing was left stranded by the move.
  - Hand-written token literals do survive outside the task's scope — `cmd/bootstrap/reboot_roundtrip_test.go:78`
    (`savedHookKey = "alphaPaneToken"`) and `internal/restore/exit_closes_pane_integration_test.go:123`
    (`paneToken = "exitClosesPaneToken"`). Neither is a defect: both fixtures measure hook *firing* across a reboot,
    not staleness judging, so nothing in them depends on the value being token-shaped, and neither file is in the
    set the task named. Not reported.

TESTS:
- Status: Adequate — correctly, no new test
- Coverage: The task is a naming-and-siting change over existing fixtures, and its Tests section says so outright:
  "The existing sweep, doctor and daemon suites are the proof — unchanged in count and meaning after the re-point."
  Read against the commit diff, that holds. No test was renamed, added or removed; every subtest kept its expected
  counts:
  - "it does not fire the mass-deletion guard when no pane is stamped" swapped `"live:0.0"` for a second unjudgeable
    seed. Both keys remain unjudgeable, so the file-unchanged assertion (`internal/hooksweep/sweep_test.go:402`)
    measures the same thing it did before.
  - "it counts the rows, not the tokens, on the counts line" kept `panes = 4` against `unstampedRows(4)`
    (`internal/hooksweep/sweep_test.go:448,454-456`).
  - The three integration re-points kept a distinct stale/live pair per fixture — `ReapableSeedA` beside `LiveSeedA`
    at head — so no fixture accidentally collapsed its two keys onto one value.
- Notes: The vocabulary is now self-guarding, which is the task's stated Outcome ("A width or alphabet change fails
  loudly at every fixture instead of silently emptying three of them"). `internal/hookstest/hooks_test.go:37-74`
  sweeps every named seed and asserts token-shapedness, pane-token width, non-token-shapedness for the unjudgeable
  half, and pairwise distinctness — enumerating the seeds by name in a map so the sweep covers the whole vocabulary
  rather than a prefix of it. `tokenShapedHookKey` (`internal/hookstest/hooks.go:127-146`) panics rather than
  returning a degraded key, so an alphabet change fails at construction.

CODE QUALITY:
- Project conventions: Followed. Lane rules hold — the re-pointed integration fixtures keep `//go:build integration`
  (`cmd/hook_prune_rename_survival_integration_test.go:1`, `cmd/state_daemon_hook_cleanup_integration_test.go:1`)
  and the new vocabulary file is unit-lane and test-only. `internal/hookstest` remains a test-only helper package
  per the architecture table.
- SOLID principles: Good. The vocabulary file has one job (subjects the suites share) and the header comment draws
  the line against `testhelpers_test.go` (staging) explicitly, which is what keeps the split legible.
- Complexity: Low — pure fixture construction.
- Modern idioms: Yes. `for i := range n` in `unstampedRows`; `sync.OnceValue` for the minted width in `hookstest`.
- Readability: Good. Every seed name states its role (reapable / live / unjudgeable / subject), which is the
  property the task was buying.
- Issues: `hooksBody`, `tokenRows` and `unstampedRows` are now declared twice, byte-identically, in
  `cmd/hookkey_vocabulary_test.go:25,41,49` and `internal/hooksweep/helpers_test.go:24,39,47`. That duplication was
  created by the later `internal/hooksweep` extraction, not by this task, and it is a defensible accommodation of
  Go's package boundary (sharing them would require exporting `tmux.PaneHookRow` builders from `internal/hookstest`,
  pulling an `internal/tmux` edge into a package every hooks test imports). Recording it as an observation, not a
  finding — the seed *keys*, which is what the task's Outcome names, are declared once.

BLOCKING ISSUES:
- None.

FINDINGS:
- [in-scope] [contained] internal/hooksweep/sweep_test.go:384 — the entry's `on-resume` payload is still `"cmd-live"`
  while its key was re-pointed to `hookstest.UnjudgeableSeedB`; rename it to `"cmd-unjudgeable"` (and the sibling on
  :383 from `"cmd-old"` if a matching pair reads better). — FAILS: this is the same defect the task's own third
  acceptance criterion forbids, left at one of the two lines the task edited. The subtest's assertion is that the
  file is byte-unchanged, i.e. that BOTH entries are retained; both are retained because their keys are unjudgeable,
  and neither because a pane is live. Labelling one of them `live` tells a reader the fixture demonstrates the
  live-token preservation path, which it does not exercise at all — the `stubReader` is armed with
  `unstampedRows(3)`, so the live token set is empty. The two correctly-paired uses at :198 and :228 sit against
  `hookstest.LiveSeedA`, which is what makes the mislabelling at :384 read as a genuine one rather than a house
  convention.
- [in-scope] [contained] internal/hooksweep/sweep_test.go:445 — same payload, same fix: the sole seeded entry is
  keyed on `hookstest.UnjudgeableSeedB` and its command is `"cmd-live"`. — FAILS: the subtest asserts only that the
  DEBUG counts line reports `panes = 4`, and its own failure message says "the count is of live panes; none of these
  carries a token" — so the fixture's one entry is deliberately NOT live, and calling its payload `cmd-live`
  contradicts the message printed three lines below it.
