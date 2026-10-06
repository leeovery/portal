## Attempt 1

ISSUES:
- internal/state/commit_cycle.go:182,187 — the multi-claimant guard (`claims[record.ScrollbackFile]++` / `claims[file] == 1`) has no test. This branch keeps the commit from naming one file on two records when two tokened, writable panes' last committed records name the same non-token file and no capture record names it. With the condition reduced to `named[file] == 0`, all of `go test ./internal/state/` still passes. If a later edit drops it, both panes are held on one file; the next write by either pane then replaces the bytes restore would replay into the other, which shows up only as one pane restoring another pane's scrollback after a reboot. The state can be reached: a skeleton link that failed, or a non-token-shaped token, commits two records on one file (`scrollback.go:180-231`), and both panes can later move off that address.
  FIX: Add a subtest to `TestRunCommitCycleJudgesAMovedPaneAtItsOwnPositionalFileWhenAnotherRecordNamesItsSavedFile` (`commit_cycle_moved_test.go:324`), or a sibling test. Commit an index by hand in which two records name `scrollback/work__2.0.bin`, with that file on disk holding bytes: `waitingPaneToken` saved at `work:2.0`; `otherMovedToken` saved at `work:3.0`. `seedMoved` writes each pane on its own positional file, so either build the `state.Index` inline before `state.Commit`, or give `movedPane` an optional stored-file override. Make both panes live away from `work:2` (for example `work:1.0` and `work:4.0`), with no live pane at `work:2.0`. Dump an empty capture for each, with `later` refused. Assert: both writes succeed with `laterReads == 0`; each committed record names its own live positional file; `assertNoFileOnTwoRecords` passes on the committed index.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- internal/state/commit_cycle.go:158-161 — "is left to that record" is false in the claims case: when two such panes claim one file, neither is held, and no record is left holding it.
  OLD: // housekeeping would delete it once no record named it. A file another record
// in the capture names, or another such pane claims, is left to that record,
// so no commit names one file on two records; a token-named transcript is
// always held.
  NEW: // housekeeping would delete it once no record named it. No such pane is held
// on a file another record in the capture names or another such pane claims,
// so no commit names one file on two records; a token-named transcript is
// always held.
- internal/state/commit_cycle_moved_test.go:78-79 — refers to a `body` parameter the helper does not have; the bytes are derived per pane.
  OLD: // seedMoved commits an index holding each saved pane on its positional file,
// with that file on disk holding body.
  NEW: // seedMoved commits an index holding each saved pane on its positional file,
// with that file on disk holding the pane's saved bytes.

NOTES:
- `keepAnsweredTranscripts` now also holds moved panes that were never answered. The name, read at the call site (`commit_cycle.go:131`), understates its scope. The task asked only for its doc comment to be widened.
- The unconditional token-named branch (`file == PendingScrollbackFile(p.PortalPaneID) ||`) differs from the general rule only in degenerate states (two live panes sharing a token; a carried session's token-stripped record still naming `pane-T.bin`), where it can put `pane-T.bin` on two records. Pre-existing behaviour the task explicitly keeps; noted only.
- The reviewer banked a separate defect for the phase boundary: in a multi-window shift only the last shifted pane is held.
