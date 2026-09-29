# Analysis Tasks: Lazy Resume On Attach (Cycle 6)

## Task 1: A Waiting Pane's Transcript Survives Two Commits That Overlap
severity: medium
sources: architecture

**Problem**: The saver's tick and `portal state commit-now` each run a whole committing cycle: capture, re-file, commit, then the housekeeping pass. Nothing orders one cycle against the other. Before this feature capture was a pure read and every record named a positional file the daemon rewrites from the live pane, so when one committer's housekeeping deleted a file only the other's view named, the loss could be re-captured. The feature changed two things:
- `CaptureAndRefile` now changes the filesystem inside the capture (`internal/state/scrollback.go:275-288`). The pending re-file moves a pane's positional file onto `pane-<token>.bin` (`refilePendingPane`, `:120-135`).
- Which path a record names now depends on markers that flip during the restore-to-wait hand-over: skeleton first, then pending.

A token-named file is written once, by that move, and a frozen pane is never captured again. `gcOrphanScrollback` deletes every `.bin` its own committed index does not name (`internal/state/commit.go:77-111`). `Commit` runs that pass whenever its index differs from the one on disk (`:22-44`).

Traced against the tree:
- Pane X has a lazy registration and was saved at `scrollback/K.bin`. It was not waiting before the reboot and is restored at its saved address.
- The daemon's tick captures while X is still skeleton-marked, so its index names `K.bin` for X (`cmd/state_daemon.go:244`). The tick then spends its dump phase capture-paning the panes that have finished hydrating (`:258-297`).
- During that phase X's helper marks X pending and clears its skeleton marker.
- A session closes, which fires `commit-now` through the `session-closed` hook (`internal/tmux/hooks_register.go:24`, `:84`). The user culling finished sessions in the picker that has just appeared is enough.
- `commit-now` reads the on-disk index (`cmd/state_commit_now.go:118`) and sees X pending. It moves `K.bin` to `pane-<token>.bin` and commits (`:121-126`).
- The daemon then commits its older index, which still names `K.bin`. Its housekeeping pass removes `pane-<token>.bin` as unreferenced.
- On the daemon's next tick the re-file finds `K.bin` gone. A missing source counts as adoption (`placeStoredScrollback`, `internal/state/scrollback.go:143-152`), so the record is pointed at a token file that no longer exists.

The loss is silent. The helper already replayed, so this boot looks right. From then on the picker preview shows the placeholder, and if the pane is still waiting at the next reboot the helper takes the file-missing tail and the panel comes up over an empty pane.

The mirror order loses the file the same way: the daemon re-files, then a `commit-now` whose capture predates the pending mark commits and collects after it. Two overlapping `commit-now` runs from killing several sessions quickly can also do it.

The no-clobber re-file (review cycle 1) closes the overwrite routes but not this deletion route. No test runs two committers across a skeleton-to-pending transition.

**Solution**: Serialise the committing cycle. The direction comes from two sources: the tree's own rule for a shared file, where each writer holds one exclusive `flock` across its whole load-mutate-save (the `hooks.json.lock` sidecar), and the specification's rule that a waiting pane is restored with its original content on every reboot it waits through.
- **The lock.** One exclusive `flock` on a sidecar in the state directory. It is held from the capture through the re-file, the caller's dump and the commit to the end of the housekeeping pass.
- **Where it lives.** `internal/state` owns the lock. The cycle has one locked entry point whose only variable part is the caller's dump, building on phase 4's composite, which already leaves each caller owning only whether it dumps. The daemon's tick, its shutdown flush and `commit-now` can then neither capture, re-file nor collect outside the hold.
- **Reading the previous index.** `commit-now` reads `sessions.json` after the lock is taken, not before. The daemon's in-memory previous index can still lag a `commit-now` commit made between two of its ticks. That stays safe: the re-file adopts a source that has already moved, and the token-named file it points at is on disk because the other commit's housekeeping named it.
- **A bounded acquire.** Bounded like the hooks store's lock, whose source records that an unbounded acquire would park the daemon's tick loop behind a holder that is alive but stuck (`internal/hooks/lock.go:17-21`). A timeout uses routes that already exist:
  - A daemon tick that cannot take the lock returns through its existing `tick failed` WARN with `save.requested` left in place, so the next tick retries.
  - A `commit-now` that cannot take it fails through `failCommitNow`, which touches `save.requested` so the daemon's next tick commits the closed session's removal.
- **A crashed holder.** `flock` is released when its process exits, so a holder that crashes never wedges the other committer.
- **What stays as it is.** The no-clobber re-file and the skeleton-stage link are unchanged. They still cover the crash, retry and in-process routes that review cycle 1 and phase 10 settled.

Rejected: having the housekeeping pass spare token-named files. A leftover token file from an earlier wait would then always survive, and the no-clobber re-file would adopt it over the pane's fresh positional capture at its next freeze. The residual that review cycle 1 accepted as needing repeated failed deletes would become the ordinary path, and the pane would restore its old transcript instead of its current one.

**Outcome**: A housekeeping pass deletes a waiting pane's token-named transcript only when its own index was captured after that file was filed. However the daemon's ticks and session-close `commit-now` runs interleave across a pane's restore-to-wait hand-over, the file survives, the picker previews it, and the pane restores with it at the next reboot.

## Task 2: Corrections
severity: corrections
sources: standards

**Problem**: `CLAUDE.md:61`, the `state` row, still states the address-match rule that phase 11 replaced: "A skeleton-marked pane carrying no token still takes the whole previous record at its own address."
- The tree no longer works that way. `indexPrevPanes` (`internal/state/capture.go:235-250`) leaves a record whose token a live pane in the same capture carries out of the address map. A tokenless pane at that record's address therefore keeps its fresh record. Both `mergeSkippedPanes` (`:151-175`) and `mergeFrozenPanes` (`:183-206`) inherit that rule.
- The row also says nothing of a tokenless pending pane, which takes `CWD`, `CurrentCommand` and `ScrollbackFile` by address under the same exclusion.

Every agent session loads CLAUDE.md as the description of the capture cycle. The next planned work on this lookup, the parked `durable-pane-identity` migration, would start from the unconditional rule. It would drop the exclusion and let a commit hold one token on two records.

**Solution**:
- `CLAUDE.md:61`: replace "A skeleton-marked pane carrying no token still takes the whole previous record at its own address." with "A skeleton-marked or pending pane carrying no token takes the previous record at its own address — the whole record for a skeleton-marked pane, its `CWD`, `CurrentCommand` and `ScrollbackFile` for a pending one — unless a live pane in the same capture carries that record's token: that record belongs to the pane answering to the token, and the tokenless pane keeps the fresh record the capture built, so no commit holds one token on two records."
