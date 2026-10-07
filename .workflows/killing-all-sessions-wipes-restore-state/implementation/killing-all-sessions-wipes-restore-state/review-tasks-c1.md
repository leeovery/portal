# Review Tasks: killing-all-sessions-wipes-restore-state (Cycle 1)

## Task 1: The Dump Never Replaces A Scrollback Name Another Pane's Saved Record Still Names
severity: high
sources: 1-6-1, 7-1-1, 7-2-1

**Problem**: `ScrollbackWriter.Write` (`internal/state/commit_cycle.go:74-95`) writes every pane's capture at that pane's live positional name through `WriteScrollbackIfChanged` (`internal/state/scrollback.go:80-92`). It does this even when `sessions.json` still names that file on a different pane's record.

Scenario: restore brings back a session whose window indices had a gap, one window lower.
- Lazy pane X was saved at `work:2.0` with record `work__2.0.bin`. It comes back waiting at `work:1.0`.
- Pane Y was saved at `work:3.0`. It now sits at `work:2.0`.

On the first tick:
1. X's record is merged on its token and keeps `work__2.0.bin`.
2. The re-file links `work__2.0.bin` to `pane-<X>.bin`.
3. The dump writes Y's capture over the name `work__2.0.bin`.

The cycle can then end uncommitted in two ways: the tick is cancelled at the per-pane context check (`cmd/state_daemon.go:310-312`) and the shutdown flush that follows stands down, or the `sessions.json` write fails. Either way, `sessions.json` still records X naming `work__2.0.bin`, which now holds Y's bytes. At the next restore X replays Y's transcript. The re-file then adopts the surviving `pane-<X>.bin` on EEXIST, so X's own bytes last only until X's next capture replaces them, and that capture carries Y's replayed history.

The same replacement reaches any pane, held or not, tokened or not, waiting or not, whose live positional name the committed index names on another pane's record. That includes a shifted run of non-waiting moved panes and a new pane at a vacated address. Task 1-6's fourth criterion and §2.4 fail for this case. The phase-7 proposal noted it as a residual, but §5.2 does not list it, and no suite case covers a moved pane, a second pane at its old address and an uncommitted end.

The nearest case, `TestRunCommitCycleEndingUncommittedAfterALinkKeepsTheMovedPanesTranscript` (`internal/state/commit_cycle_shifted_test.go:234`), drives this shape: a moved pane, a new pane written at its old address, then a failed dump or a failed `sessions.json` write. It asserts only that each named file exists and that the next committing cycle adopts the token-named file. It never reads what X's named file holds after the uncommitted end, so it passes over the loss.

The `held` field doc (`commit_cycle.go:60-61`) is also stale. It says held is "the file its last committed record named, which its record still names". That is false in the cycle that links a moved pane: there `keepAnsweredTranscripts` has already moved the record onto `PendingScrollbackFile(token)` (`:205-206`), and `heldTranscripts` maps the key to that file (`:237-251`).

**Solution**: Close the case rather than record it as §5.2 residue. Four parts of the record require it, and closing it costs the user nothing:
- §1.1 says every case the fix leaves short of its rule is recorded in §5.2, and this case is not.
- §2.3's stated purpose is that the next restore finds the transcript at the path the record names.
- Task 1-6's fourth criterion requires the moved pane's transcript at that path.
- At the phase-7 walk the user decided that every restored pane keeps its transcript through the first save after restore.

**When a name is contested.** The writer treats a pane's positional name as contested when the committed index names it on a record carrying a non-empty token other than the pane's own. The committed index is `sessions.json` as read under the commit lock (`committed` in `RunCommitCycle`), not the caller's `LoadPrev` fallback. With no readable `sessions.json`, nothing is contested. A record with no token is matched by address as today and never contests, because it may be this same pane, stamped since by `hook set`.

The protected set is every pane the dump writes, not only held panes. The hazard is defined by the committed index, and a guard on held panes alone leaves the new-pane-at-a-vacated-address and failed-link variants open.

**A contested pane whose token the pane-token rule accepts** (`PendingScrollbackPath`):
- The writer writes its capture under its token-named transcript (`PendingScrollbackFile(token)`) instead of the positional name.
- Its record names that file in this cycle's commit, and `fileAtPositional` does not file it at its positional name.
- In the first cycle after that commit lands, the name is no longer contested. The existing hold then files the pane at its positional name, and that commit's housekeeping removes the token-named file.

This beats the two alternatives:
- Writing to another name and renaming it into place at commit fails either way. A rename before the `sessions.json` write reopens the gap on a failed write. A rename after it leaves the pane's committed record naming the other pane's bytes until the rename lands.
- Deferring every contested write would drop the pane's fresh capture for a cycle, and at a shutdown flush that is the last capture the pane gets.

Writing under the token-named path avoids both problems:
- It is already the pane's held name in the existing machinery.
- Only this pane's record ever names it.
- `AtomicWrite` replaces the name rather than the file, so a positional file linked under that name keeps its inode.

**A contested pane with no token, or with a token the rule refuses**:
- Nothing is written this cycle and its dedup entry is left alone. The write is deferred until a commit lands, and once this cycle's commit has re-pointed the other record, the next cycle finds the name uncontested.
- Its record is left as an unwritten capture leaves it today.

A tokenless pane has no second name and no identity across a move (§5.2), so deferral is the only form open to it.

**Unchanged rules and doc updates:**
- The empty-capture confirmation still applies before any write, whichever name the write lands on.
- The `held` field doc becomes: "held maps a pane key to the file keepAnsweredTranscripts held it on — its token-named transcript, or the file its last committed record named — which its record names in this cycle and its empty capture is judged against." This is 7-2-1's wording, chosen over 7-1-1's because a token-named file adopted on EEXIST need not hold the bytes the last committed record named.
- The `ScrollbackWriter` struct doc and the `Write` doc describe the contested write and which file the record then names.

**Guard conditions:**
- The alternate-name write stays inside `internal/state`'s `ScrollbackWriter` and is never reached by calling `WriteScrollbackIfChanged` from `cmd` (`TestNoProductionCommitOutsideState`).
- A real-tmux covering test calls `IsolateStateForTest` and `RegisterStateDirTeardownGuard`, both before `tmuxtest.New` (`TestTeardownGuardCoversEveryServerHostingFixture`).
- Any line the change emits uses the existing log components and attr keys.

**Outcome**: The first cycle after a move can end three ways: it commits; it is cancelled mid-dump and the shutdown flush that follows stands down; or its `sessions.json` write fails. In every case, each tokened pane's record in `sessions.json` names a file holding that pane's own transcript, and the next restore replays it. No dump writes a capture over a positional name the committed index still names on another tokened pane's record.

**Acceptance Criteria**:
- [ ] Session `work` is saved with tokened pane X at `work:2.0` (record `work__2.0.bin`) and tokened pane Y at `work:3.0` (record `work__3.0.bin`). It comes back one window lower: X at `work:1.0`, waiting or not, and Y at `work:2.0`. The first cycle's dump writes Y's capture and the cycle commits. Y's capture is in Y's token-named transcript and Y's record names that file. Nothing was written at `work__2.0.bin` in that cycle. X's record names X's token-named transcript, holding X's saved bytes. No file sits on two records. (§2.3, §2.4)
- [ ] Same layout, but the cycle ends uncommitted after Y's capture is written: either the daemon's tick is cancelled at the per-pane check on a pane after Y and the shutdown flush that follows stands down, or the cycle's `sessions.json` write fails. `sessions.json` is unchanged. `work__2.0.bin`, which X's record names, holds X's saved transcript. `work__3.0.bin`, which Y's record names, holds Y's saved transcript. Every scrollback path `sessions.json` names is on disk. (§2.3, §6.1)
- [ ] Once the commit that named Y's capture under its token has landed, the next cycle that writes Y's capture writes it at `work__2.0.bin`. Y's record names `work__2.0.bin`, and that commit's housekeeping removes Y's token-named transcript. (§2.4)
- [ ] Two tokened panes swap addresses: X moves `work:1.0` → `work:2.0` and Y moves `work:2.0` → `work:1.0`. A capture written for either pane, in either order, lands in that pane's own token-named transcript. Neither `work__1.0.bin` nor `work__2.0.bin` is written in that cycle. If the cycle ends uncommitted, each pane's record still names a file holding its own saved transcript. (§2.3, §2.4)
- [ ] Tokened X moves `work:2.0` → `work:1.0`, and in the same cycle a new pane with no token appears at `work:2.0`, while `sessions.json` still names `work__2.0.bin` on X's record. The dump's write of the new pane's capture writes nothing and leaves the new pane's dedup entry as it was. If the cycle commits, the new pane's record is left as an unwritten capture leaves it, and the next cycle writes the new pane's capture at `work__2.0.bin` without touching X's bytes. If the cycle ends uncommitted, `work__2.0.bin` still holds X's saved transcript. A contested pane whose token the pane-token rule refuses is deferred the same way. (§2.3, §5.2)
- [ ] Nothing is contested, and every write lands at its positional name as today, in three cases: `sessions.json` is absent or unreadable, whatever the caller's fallback index names; the committed record naming the pane's positional name carries no token; or that record carries the pane's own token. (§2.4)
- [ ] A contested tokened pane takes an empty capture over a saved non-empty transcript, and the committer's own server does not confirm it. Nothing is written at either name, and the write is refused as today. (§2.4)

**Do**:
- The change lives in `internal/state/commit_cycle.go`: `ScrollbackWriter` and the writer `RunCommitCycle` builds. The writer judges contest against `committed`, which is `sessions.json` as read under the commit lock. It never judges against the `LoadPrev` fallback. When `committed` is nil, nothing is contested.
- A pane's positional name is contested when the committed index names it on a record carrying a non-empty token other than the pane's own. A committed record with no token never contests. The protected set is every pane the dump writes, held or not.
- A contested pane whose token `PendingScrollbackPath` accepts:
  - its capture is written under `PendingScrollbackFile(token)`;
  - its record names that file in this cycle's commit;
  - `fileAtPositional` does not file it at its positional name.
  
  The following cycle needs no new handling. The existing hold (`keepAnsweredTranscripts` / `heldTranscripts`) files the pane at its positional name once that name is uncontested, and that commit's housekeeping removes the token-named file.
- A contested pane with no token, or with a token the rule refuses: nothing is written, its dedup entry is left alone, and its record is left as an unwritten capture leaves it.
- `confirmEmptyCapture` still runs before any write, whichever name the write lands on.
- Replace the `held` field doc (`commit_cycle.go:60-61`) with: "held maps a pane key to the file keepAnsweredTranscripts held it on — its token-named transcript, or the file its last committed record named — which its record names in this cycle and its empty capture is judged against." The `ScrollbackWriter` struct doc and the `Write` doc describe the contested write and which file the record then names.
- Guard conditions:
  - The alternate-name write stays inside `ScrollbackWriter`. `cmd` never reaches `WriteScrollbackIfChanged` (`TestNoProductionCommitOutsideState`).
  - Any real-tmux test calls `portaltest.IsolateStateForTest` and `portaltest.RegisterStateDirTeardownGuard` before `tmuxtest.New` (`TestTeardownGuardCoversEveryServerHostingFixture`).
  - Any emitted line uses the existing log components and attr keys.
- The cancelled-tick ending is the daemon's. `scrollbackDump.run` returns `errCycleCancelled` at its per-pane context check (`cmd/state_daemon.go:310-312`), and `defaultShutdownFlush` follows.
- These existing cases assert a write at a contested name and move to the new behaviour. All are in `internal/state/commit_cycle_shifted_test.go`:
  - the "the pane moved 3→2 is written first" rows of `TestRunCommitCycleKeepsEveryShiftedPanesTranscriptOverAnUnconfirmedEmptyCapture` (:66), which assert B at `work__2.0.bin`;
  - both rows of `TestRunCommitCycleKeepsEachSwappedPanesOwnTranscript` (:150), which assert Y at `work__1.0.bin`;
  - the "in the same cycle" row of `TestRunCommitCycleKeepsAMovedPanesTranscriptBesideANewPaneAtItsSavedAddress` (:188);
  - `TestRunCommitCycleEndingUncommittedAfterALinkKeepsTheMovedPanesTranscript` (:234), whose dump fails the test unless the new pane's write lands at `work__2.0.bin`.
