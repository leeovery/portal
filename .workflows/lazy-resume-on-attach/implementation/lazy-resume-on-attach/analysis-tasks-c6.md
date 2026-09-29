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

**Acceptance Criteria**:
In each scenario pane X has a lazy registration and a token, was saved at `scrollback/K.bin` while not waiting, and is restored at its saved address.
- [ ] A daemon tick captures while X is skeleton-marked. During that tick's dump X is marked pending and its skeleton marker cleared, and a `commit-now` starts before the tick commits. The `commit-now` takes no capture until the tick has committed and finished its housekeeping pass. It then re-files `K.bin` onto `pane-<token>.bin` and commits: `pane-<token>.bin` holds X's transcript and `sessions.json` names it for X.
- [ ] Continuing that run, the daemon's next tick, whose in-memory previous index still names `K.bin` for X, commits X's record on `pane-<token>.bin`, and the file is on disk after that tick's housekeeping pass.
- [ ] The mirror order: a tick that is re-filing X onto `pane-<token>.bin` holds the cycle when a `commit-now` starts. The `commit-now` captures only after the tick's housekeeping pass, sees X pending, and leaves `pane-<token>.bin` on disk and named for X.
- [ ] Two `commit-now` runs started back to back across X's pending mark, as from closing two sessions in quick succession, run one after the other. The second takes as its previous index the `sessions.json` the first committed, and `pane-<token>.bin` survives both housekeeping passes.
- [ ] While another process holds the lock past the bound, a daemon tick logs its existing `tick failed` WARN having captured nothing, moved no scrollback file, written no `sessions.json` and deleted no `.bin`. `save.requested` stays in place, and a tick after the lock frees runs the cycle. A shutdown flush in the same position logs its existing `final flush failed` WARN and its `shutdown` line reports `flush_completed=false`.
- [ ] While another process holds the lock past the bound, `commit-now` exits non-zero with stderr silent through its existing failure route, its ERROR in `portal.log` and `save.requested` touched, having captured, moved, written and deleted nothing.
- [ ] A committer killed while it holds the lock does not hold up the next one: the next tick or `commit-now` takes the lock without waiting out the bound, and commits.
- [ ] With no other committer running, the tick, the shutdown flush and `commit-now` each commit what they commit today. The no-clobber re-file and the skeleton-stage link are unchanged, and their existing tests stay green.

**Do**:
- **The lock.** One exclusive `flock` on a sidecar file of its own in the state directory, owned by `internal/state`. It is not `daemon.lock`, which the daemon holds for its whole life (`cmd/state_daemon.go:92-95`). The hold runs from the capture (the skeleton-marker read that opens `CaptureAndRefile`) through the re-file, the caller's dump and `Commit` to the end of `gcOrphanScrollback` (`internal/state/commit.go:39`, `:77-112`).
- **The entry point.** One locked entry point in `internal/state`, built on `CaptureAndRefile` (`internal/state/scrollback.go:275-288`), whose only variable part is the caller's dump. `commit-now` dumps nothing.
- **The complete set it replaces.** `rg -n --type go -g '!*_test.go' 'CaptureAndRefile\b|\bCommit\(|state\.Commit\b'` returns 11 lines. Three are in `internal/state`: the two declarations and `CaptureAndRefile`'s doc comment. The other eight are the two committing sites, and both convert:
  - `cmd/state_daemon.go:244` and `:299`: `captureAndCommit`, reached from both `tick` (`:191`) and `defaultShutdownFlush` (`:359`).
  - `cmd/state_commit_now.go:36`, `:63`, `:64`, `:67`, `:121` and `:126`: the `CommitNowDeps` capture and commit seams, and the `RunE` that calls them.
  - Afterwards neither file captures, re-files, dumps or commits outside the entry point.
- **The fixtures.** The two integration fixtures that reproduce those cycles by hand enter through the entry point too, so they keep taking the route production takes: `captureRound` (`internal/restore/lazy_resume_panel_integration_test.go:377-419`) and `commitNowRound` (`internal/restore/lazy_resume_renumbered_restore_integration_test.go:169-184`).
- **The previous index.** `commit-now` reads `sessions.json` (`loadPrevIndex`, `cmd/state_commit_now.go:118`) after the lock is taken. The daemon keeps its in-memory `PrevIndex` (`cmd/state_daemon.go:303`).
- **The bound.** The acquire is bounded, as the hooks store's is (`internal/hooks/lock.go:17-21`, `:62-84`). A timeout leaves through the existing routes: the tick's `tick failed` WARN (`cmd/state_daemon.go:191-194`), which returns before `save.requested` is removed, and `failCommitNow` (`cmd/state_commit_now.go:144-150`), which touches it.
- **Unchanged.** The no-clobber re-file (`refilePendingPane`, `placeStoredScrollback` and `moveNoClobber`, `internal/state/scrollback.go:120-173`) and the skeleton-stage link (`linkMovedSkeletonScrollback`, `:196-247`). `gcOrphanScrollback` keeps deleting every `.bin` its own index does not name, token-named files included.

## Task 2: Corrections
severity: corrections
sources: standards

**Problem**: `CLAUDE.md:61`, the `state` row, still states the address-match rule that phase 11 replaced: "A skeleton-marked pane carrying no token still takes the whole previous record at its own address."
- The tree no longer works that way. `indexPrevPanes` (`internal/state/capture.go:235-250`) leaves a record whose token a live pane in the same capture carries out of the address map. A tokenless pane at that record's address therefore keeps its fresh record. Both `mergeSkippedPanes` (`:151-175`) and `mergeFrozenPanes` (`:183-206`) inherit that rule.
- The row also says nothing of a tokenless pending pane, which takes `CWD`, `CurrentCommand` and `ScrollbackFile` by address under the same exclusion.

Every agent session loads CLAUDE.md as the description of the capture cycle. The next planned work on this lookup, the parked `durable-pane-identity` migration, would start from the unconditional rule. It would drop the exclusion and let a commit hold one token on two records.

**Solution**:
- `CLAUDE.md:61`: replace "A skeleton-marked pane carrying no token still takes the whole previous record at its own address." with "A skeleton-marked or pending pane carrying no token takes the previous record at its own address — the whole record for a skeleton-marked pane, its `CWD`, `CurrentCommand` and `ScrollbackFile` for a pending one — unless a live pane in the same capture carries that record's token: that record belongs to the pane answering to the token, and the tokenless pane keeps the fresh record the capture built, so no commit holds one token on two records."

**Outcome**: The `state` row of `CLAUDE.md` states the address match as `indexPrevPanes` runs it: a tokenless skeleton-marked or pending pane takes the record at its own address unless a live pane in the same capture carries that record's token. An agent reading the row builds from the rule the tree enforces.

**Acceptance Criteria**:
- [ ] `CLAUDE.md:61` no longer contains "A skeleton-marked pane carrying no token still takes the whole previous record at its own address." It carries the Solution's replacement sentence verbatim in that place, between "…survives the housekeeping pass." and "The saver's per-pane scrollback skip is mid-restore".
- [ ] No other text in `CLAUDE.md` changes, no source or test file changes, and the existing tests stay green.

**Do**:
- Apply the one edit the Solution lists, in the `state` row at `CLAUDE.md:61`.
