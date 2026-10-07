# Consolidation Findings: killing-all-sessions-wipes-restore-state (Phase 9)

## Findings

### F1: A deferred contested pane is committed naming another pane's transcript, and that commit deletes the deferred pane's own
- **Class**: behaviour
- **Failure**: The phase defers the write of a contested pane that has no usable token: no token, or one the pane-token rule refuses. That is right for a cycle that ends uncommitted. When the cycle commits, though, the deferred pane's record stays at its fresh positional name, and that file holds the bytes of the tokened pane whose committed record contested it. That pane's record moves to its token-named link in the same commit, and its inode is still the old one. If the deferred pane was itself moved, the same commit's housekeeping deletes its previous file, because no record names it any more.
  - **What persists**: until the next committing cycle that runs a dump, sessions.json names a neighbour's transcript on the deferred pane, and the deferred pane's own transcript is on no file at all. The next such cycle comes at the next `save.requested` touch or the 30s `MaxGap` (cmd/state_daemon.go:186-188, :462).
  - **Who hits it**: a user with `renumber-windows on` who closes a window ahead of a pane created after restore without a hook, when the window between them carries a token. For example, kill window 1 of restored windows 1 and 2 with a new window 3: B goes 2→1 and A goes 3→2, and A is contested by B's committed record. A new hook-less pane opened at a vacated index before the next commit hits it too.
  - **How it is noticed**: tmux goes down in that window, at a reboot whose shutdown flush stands down (the case this whole fix exists for). At the next restore the deferred pane replays the neighbour's history, and its own is gone.
  - **Before this phase**: that same commit wrote the pane's live capture at the positional name.
  - **Measured**: an overlay probe on HEAD (B tokened, saved at work:2.0 and live at 1.0; A tokenless, saved at 3.0 and live at 2.0; both written; the cycle commits) gives A's record `scrollback/work__2.0.bin` holding B's saved bytes, with `work__3.0.bin` deleted. The same probe on the pre-phase tree (`git archive 3b67f0809~1`) gives A's record holding `a-capture`.
  - **Spec**: this also breaks §5.2's bound, which says the residue for a hook-less moved pane lasts only "before the pane's next save completes" (see S1). A brand-new pane is not covered by §5.2 at all.
- **Evidence**:
  - internal/state/commit_cycle.go:96-98 and :118-122: a contested pane goes to `writeUnderToken`, which returns `(false, nil)` when `PendingScrollbackPath` refuses the token, and leaves `filed` unset.
  - internal/state/commit_cycle.go:192: `fileWritten` leaves the deferred record at its fresh positional name. :231 and :287: `keepAnsweredTranscripts` and `heldTranscripts` skip a tokenless pane. :194: the commit and its housekeeping follow regardless.
  - internal/state/commit_cycle.go:244-251 and :263-276 (`keepAnsweredTranscripts` via `linkHeldTranscript`): the contesting tokened pane's record moves onto its token-named link of the same inode.
  - Tests that now assert the contaminated committed state:
    - internal/state/commit_cycle_shifted_test.go:229-236, the "in the same cycle" row: after the cycle commits, `work__2.0.bin`, which the new pane's record names, must hold the moved pane's saved bytes.
    - internal/state/commit_cycle_contested_test.go:259-262: the deferred pane's committed record names `occ.pane.stored()`, which holds X's bytes.
    - cmd/state_daemon_run_test.go:640-655 with :635: after the skeleton tick commits, `work__2.0.bin` is named by the tokenless pane at work:2.0 and holds `y-saved`. That pane's own `work__3.0.bin` (`x-saved`) is unnamed and collected.
- **Proposed shape**:
  - **Primary: write a deferred capture once its commit has landed, in the same locked cycle.**
    - `ScrollbackWriter` keeps each deferred pane's `(data, hash)` instead of dropping it, after running `confirmEmptyCapture` on it as for any write.
    - `commitOver` reports whether it actually wrote sessions.json. Today it returns nil without writing when nothing changed.
    - Once sessions.json has been written, `RunCommitCycle` writes each kept capture at its positional name through `WriteScrollbackIfChanged`, updating the dedup entry. It skips any name that the index it just committed still names on a record carrying another pane's token. That record may not have moved, for example after a failed skeleton link.
    - A cycle whose commit does not land writes none of them, so the uncommitted-end guarantee this phase bought is unchanged.
    - The committed record then names the pane's own capture. The exposure shrinks to the instant between the sessions.json rename and that write.
  - **Cheaper fallback: retry on the next tick.** Surface "a pane was deferred" from the cycle so the daemon re-touches `save.requested`. That bounds the window to one tick but does not close it.
  - **Either way:** the three test sites above move. The deferral row asserts the deferred pane's record holds its own capture after the commit, and a case is added for a moved tokenless pane whose previous file is collected by that commit.
  - **If the user keeps the deferral as it is:** this becomes S1's spec amendment instead.

## Spec Defects

### S1: §5.2's bound on the hook-less moved-pane loss no longer holds
- **Claim**: §5.2, "Accepted residue": "**A pane with no resume hook moved after restore.** A pane carrying no token that tmux renumbers, or whose session is renamed, after restore has no identity tying it to its last committed record. If tmux goes down before the pane's next save completes, its saved transcript is lost."
- **Observed**: When such a pane lands on an address that the committed index names on another tokened pane's record, its next save completes without its capture. The write is deferred (internal/state/commit_cycle.go:118-122), and that save's housekeeping deletes its previous file. Its record names the other pane's bytes until a later dump cycle commits. That was measured by the F1 probe, and cmd/state_daemon_run_test.go:640-655 asserts it. So the loss outlasts "the pane's next save". It adds a replay of a neighbour's transcript, which §5.2 does not mention. The same commit also gives a brand-new hook-less pane at a vacated address a neighbour's transcript, and that is not a moved pane, so §5.2 does not cover it.
- **Read**: Genuinely open, and it turns on F1. If F1's primary shape is taken, the code is wrong and §5.2 stands as written. If the deferral is kept, the spec is stale: §5.2 needs the bound extended to the save after next, plus the neighbour-replay, for both the moved pane and the new pane at a vacated address.
