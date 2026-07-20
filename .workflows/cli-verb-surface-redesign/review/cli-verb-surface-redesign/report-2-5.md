TASK: cli-verb-surface-redesign-2-5 — `-f/--filter` mutual exclusivity extended to all pin flags

ACCEPTANCE CRITERIA (task edge cases from the Phase 2 table):
- `-f` + `-s`/`-p`/`-z`/`-a` (each) → usage error
- `-f` + positional already rejected in Phase 1 (regression)
- `-f` + `-e`/`--` command NOT an exclusivity violation (allowed — routed to the command-picker task)
- `-f` alone still opens the picker (regression)
- multiple pins alongside `-f` still error

Phase 2 AC line covered: "`-f` is mutually exclusive with every pin flag as well as positionals (usage error otherwise)."

STATUS: Complete

SPEC CONTEXT:
Spec § "-f/--filter is the sole non-composing flag": `-f` is not a target — it is a "skip resolution, open the picker pre-filtered" redirect, mutually exclusive with positional targets and with every other pin flag (usage error otherwise). Plain `-f <text>` opens the Sessions-page picker pre-filled; the command variant `-f <text> -e <cmd>` is the stated exception (filtered Projects picker, routed to Task 2-7). Spec § Target-set composition reiterates `-f` as "the sole non-composing flag (picker redirect; exclusive with all targets and pins)."

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open.go:206-217 (the guard); flag registration cmd/open.go:997-998; help text cmd/open.go:168-169.
- Notes: The guard is hand-rolled at the top of openCmd.RunE, positioned AFTER the `--ack` validation (191-195) but BEFORE both the multi-target routing gate (230-233) and the single-pin dispatch table (259-272). Ordering is correct and load-bearing:
  * `if cmd.Flags().Changed("filter")` gates the whole block, so `-f ""` (empty but Changed) is still caught.
  * `anyPin` = session||path||zoxide||alias Changed → covers all four pins and, by OR, the multiple-pins case in one predicate.
  * `destination != "" || anyPin` → the single usage error, so positional-conflict and pin-conflict share one message. Placing the block before the burst gate means `-f -s a -s b` is rejected rather than bursting; placing it before pin dispatch means no resolution/attach/mint ever fires on a conflict.
  * Empty-value check (`filterVal == ""`) runs only after the exclusivity check, so `-f "" -s api` reports the exclusivity error — a reasonable precedence.
  * The command carve-out is implicit and correct: `command` (from parseCommandArgs) is not a pin and not a positional destination, so `-f web -e claude` and `-f web -- claude` both fall through to `openTUIFunc(cmd, filterVal, command, …)`. Verified via parseCommandArgs (cmd/open.go:464-504): `-- claude` yields dest="" (dashIdx==0), command=["claude"], so the guard's `destination != ""` is false.
- No `MarkFlagsMutuallyExclusive` is used; the hand-rolled guard is the right call here because cobra's built-in would not express the `-f`+command carve-out, could not cover the positional (non-flag) case, and would not single-source the message. Consistent with the codebase's RunE-guard convention.

TESTS:
- Status: Adequate
- Coverage (all in cmd/open_test.go, unit lane, mocked deps — no daemon/tmux):
  * TestOpenCommand_Filter_WithPin_UsageError (2521) — table-driven across all four pins (-s/-p/-z/-a); asserts exact message, *UsageError (exit 2), and that neither the picker, the resolver (lister.called), nor any attach/mint outcome (openSessionFunc/openPathFunc) fires. Directly covers edge case 1.
  * TestOpenCommand_Filter_WithPositionalTarget_UsageError (2474) — `-f blog api`; covers edge case 2 (Phase 1 regression) with same assertions.
  * TestOpenCommand_Filter_WithMultiplePins_UsageError (2603) — `-f blog -s api -p ~/Code/new`; covers edge case 5.
  * TestOpenCommand_Filter_OpensPickerPrefilteredAndSkipsResolution (2426) + TestOpenCommand_NoArgs_NoFilter_LaunchesPicker (2683) — cover edge case 4 (`-f` alone opens picker; resolver never consulted; no-arg regression).
  * TestOpenCommand_Filter_ThreadsCommandToPicker (2853) — `-f web -e claude` is NOT a usage error and threads the command to the picker; covers edge case 3 for the `-e` spelling.
  * TestOpenCommand_Filter_EmptyValue_UsageError (2649) — the empty-value branch.
  Each test asserts the exact error string and `errors.As(err, &usageErr)`, and negatively asserts the picker/resolver/outcome funcs did NOT run — so a broken guard (wrong routing, resolver consulted, outcome fired) would fail. Tests are focused, one-per-edge-case, no redundant assertions or excessive mocking.
- Gap (minor): edge case 3 names both `-e` and `--`, but only the `-e` spelling of the `-f`+command carve-out is exercised (TestOpenCommand_Filter_ThreadsCommandToPicker). No test drives `-f <text> -- <cmd>`. The `--` path is exercised indirectly (parseCommandArgs handles it identically and TestOpenCommand_CommandNoTarget_DashDash_OpensProjectsPicker covers `--` without `-f`), so this is a coverage gap, not a defect.

CODE QUALITY:
- Project conventions: Followed. Package-level `*Deps` mock injection + t.Cleanup restore pattern; unit-lane placement (no integration tag, no daemon); UsageError → exit 2 contract honoured; single-sourced error message.
- SOLID principles: Good. Guard has a single responsibility; the OR-combined `anyPin` keeps "add a future pin" localized (though see note — it is a second enumeration of the pin set).
- Complexity: Low. One `if` with a compound predicate; clear early-return.
- Modern idioms: Yes.
- Readability: Good — the guard carries an accurate multi-line comment (197-205) explaining placement and the command carve-out.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/open_test.go:2853 — add a `-f <text> -- <cmd>` case (dash-dash spelling) alongside the existing `-e` case in TestOpenCommand_Filter_ThreadsCommandToPicker (or a sibling test), so edge case 3's "`-e`/`--`" is covered for both spellings rather than only `-e`.
- [idea] cmd/open.go:208-209 — the pin set (session/path/zoxide/alias) is enumerated here as `anyPin` and again as the `pinDispatch` table at 259-267. A future fifth pin must be added in two places or the exclusivity guard silently misses it. Consider deriving `anyPin` from a single pin-name list shared with the dispatch table (decide whether the churn is worth it for a 4→5 pin change).
