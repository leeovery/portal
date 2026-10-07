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

**Outcome**: The first cycle after a move can end three ways: it commits; it is cancelled mid-dump and the shutdown flush that follows stands down; or its `sessions.json` write fails. In every case, each tokened pane's record in `sessions.json` names a file holding that pane's own transcript, and the next restore replays it.
