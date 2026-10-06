# Consolidation Findings: Killing All Sessions Wipes Restore State (Phase 7)

## Findings

### F1: The hold drops every moved tokened pane whose saved file another live pane's fresh record names, which covers every pane in a shifted run except the last
- **Class**: behaviour
- **Failure**: Restore recreates windows in saved order with no index (`internal/restore/session.go:98-110`), so a gap in a session's window indices compresses every later window by one. When two or more windows follow the gap, each shifted pane's saved file is the positional file of the next shifted pane's live address. That pane's fresh record names it, so `keepAnsweredTranscripts` drops the hold (`named[file] == 0` fails) for every tokened pane in the run except the last window's. In the first save after hydration, each excluded pane loses its saved transcript in one of three ways:
  - an empty capture during shutdown is judged at the pane's absent live positional file and written with no confirmation read;
  - a refused `capture-pane` writes nothing but still moves the record off the saved file;
  - a dump-less `commit-now` also moves the record off the saved file.

  In every case the commit names no record on the saved file, so housekeeping deletes it. The next pane's confirmed write lands on that same path anyway. After the reboot the pane comes back with empty scrollback. Nothing is logged, because the `empty capture not confirmed` WARN (`cmd/state_daemon.go:345`) fires only on a refused confirmation, and none was sent. This is the loss the phase set out to close, and CLAUDE.md's "an unconfirmed one is not written, the saved transcript stands" stays false for these panes.

  The same exclusion produces two more layouts:
  - **Swapped windows:** each pane is judged at a file holding the other pane's bytes, so the refused pane's committed record keeps the other pane's transcript, and that pane restores showing the wrong scrollback.
  - **A new pane at the vacated address:** a new pane occupies the address the moved pane left.

  `renumber-windows on` gives the same consecutive shift live, when a middle window closes ahead of two or more others.

  Reproduced against HEAD in a scratch copy (`git archive`, tests in the scratch tree only):
  - **Shifted run:** saved tokened panes at `work:0.0`, `work:2.0` and `work:3.0` are live at `0.0`, `1.0` and `2.0`. An empty capture of A (2→1) with the later confirmation refused returned `Write = (true, nil)`. The commit named `work__1.0.bin` for A and `work__3.0.bin` for B, and `work__2.0.bin` was deleted. A dump-less `commit-now` over the same layout also deleted `work__2.0.bin`.
  - **Swap:** X (1→2) and Y (2→1). X's empty capture was refused, and its committed record named `work__2.0.bin` holding Y's saved transcript. Y's write replaced X's bytes at `work__1.0.bin`.
- **Evidence**:
  - `internal/state/commit_cycle.go:164` (`named` taken from the capture's fresh records), `:177-182` (candidate and claim), `:186-190` (the `named[file] == 0` exclusion)
  - `internal/state/commit_cycle.go:77` (an unheld pane is judged at `ScrollbackFile(w.dir, paneKey)`)
  - `internal/state/scrollback.go:180-231`: `linkMovedSkeletonScrollback` / `linkMovedPane` answer the same shared-file condition (`:196`) for skeleton panes by linking to the token-named path. The hold answers it by dropping protection.
  - `internal/state/scrollback.go:148` (`linkStoredScrollback`, which adopts an existing token-named file on `EEXIST`)
  - `internal/state/commit_cycle_moved_test.go:324-372`: the case at `:334`, "another moved pane is live at the saved address", asserts the lossy outcome as expected (B held on `work__3.0.bin`, A written unconfirmed at its own positional file). The case at `:333` asserts it for the new-occupant layout.
  - `internal/restore/session.go:98-110`
- **Proposed shape**:
  - **The change:** in `keepAnsweredTranscripts`, when a candidate's file is named by another capture record and its token passes `PendingScrollbackPath`, hard-link that file to `PendingScrollbackFile(token)` through `linkStoredScrollback`. This is the primitive `linkMovedPane` and `refilePendingPane` already share. Point the record at the token-named path and let the existing unconditional token-named hold carry it. `heldTranscripts`, `ScrollbackWriter.Write` and `fileAtPositional` stay as they are.
  - **Why the link survives:** the link runs before the writer is built, so the other pane's positional write replaces its name atomically and leaves the moved pane's inode under the token name.
  - **Link failure:** keep today's judgement and emit one WARN with the existing `pane_key` / `path` / `error` attrs.
  - **Signature:** `keepAnsweredTranscripts` gains the state dir and logger.
  - **Claims case:** keep the exclusion for two candidates claiming one file, because which pane's bytes the file holds is unknowable.
  - **Wider option, worth the same decision:** link every candidate, not only shared ones. This also removes a fragility: a pane held on another address's positional file loses its hold in the cycle a new pane appears at that address, and that pane's write then replaces the held file.
  - **Test impact:** the `:334` case flips. A is held on `pane-<token>.bin`, and its refused empty capture returns `ErrUnconfirmedEmptyCapture`.
  - **Residual (exists today, not made worse):** a cycle that writes the next pane's capture and then ends uncommitted still leaves `sessions.json` naming the overwritten positional file for A. The next cycle adopts the token-named file linked before that dump, but a reboot straight after restores A with the other pane's bytes.
  - **Needs the user's decision:** this contradicts task 7-1's fourth acceptance criterion ("judged at its own positional file as today").
- **Bank**: reviewer, 7-1: "When restore closes a window-index gap with two or more windows after it, the hold protects only the last shifted pane". Reviewer, 7-1: "When a restore shifts two or more consecutive windows, every shifted pane except the last in the run is still unprotected".

## Spec Defects

### S1: A pane with no token that restore places at another address loses its saved transcript, and the spec neither prevents nor accepts it
- **Claim**: §2.4 (spec lines 96 and 98): "An unconfirmed empty capture is not written. The saved transcript stands" and "A pane's saved transcript is the file its last committed record names". §6.1 (line 224): "The daemon's dump does not overwrite a non-empty saved transcript with an empty capture it cannot confirm". §5.2 (Accepted residue) does not list this case.
- **Observed**: Portal stamps a token only on `portal hook set`, so most panes carry none. Restore recreates windows in sequence (`internal/restore/session.go:98-110`), so in any saved session with a window-index gap a pane with no token comes back at a lower address. Neither the hold nor `takePrevRecord`'s by-address fallback (`internal/state/capture.go:440-452`) can find its last committed record. Its empty capture is judged at its absent live positional file and written with no confirmation read, and the commit names that file. Nothing names the saved file, so that commit's housekeeping removes it. This phase pins the behaviour as expected in `TestRunCommitCycleJudgesATokenlessPaneRestoredOneWindowLowerAtItsOwnPositionalFile` (`internal/state/commit_cycle_moved_test.go:374-399`, task 7-1's fifth criterion).
- **Read**: genuinely open. The spec is stale if the loss is accepted: §5.2 would need to say that a pane with no token has no identity across an address change. The code is wrong if it is not accepted: restore's ordinal pairing, which recreates windows in saved order, is an identity the cycle could use for the restore case, though not for a renumber or a rename. Most panes carry no token, so this is the user's decision.
