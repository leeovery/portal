TASK: CLAUDE.md Describes The Save Path And Resume Shells As They Were Before This Fix (killing-all-sessions-wipes-restore-state-3-3, tick-8c440b)

ACCEPTANCE CRITERIA:
- CLAUDE.md's `state` row describes the waiting pane's re-file as a hard link (`link(2)`) that never overwrites an existing token-named file, leaving the positional name for the housekeeping pass of a commit naming the token-named path; the row no longer speaks of a vacated positional path. (§2.3)
- The `state` row states four rules: every committing cycle stands down unless the committer's own tmux server answers a confirmation sent after the last capture read; the committing path lists sessions through `ListSessionNamesProbe` while the shared listing keeps its "no server, no sessions" reading; `ErrTmuxStoppedAnswering` is the stand-down class the back-off lines key on; an empty capture over a saved transcript is written only once the own server confirms it. (§2.1, §2.2, §2.4, §2.5, §4.2)
- The `tmux` row names `ConfirmAnswering`, `ListSessionNamesProbe` and `ServerPIDFromEnv`. (§2.1, §2.2)
- The Resume hooks section quotes the eager shell as `trap : TERM; <HOOK>; exec $SHELL` and the parked chain's trap as `INT QUIT TERM`; states the chain re-runs the draw while the draw ends on SIGTERM and the marker still reads pending; states the draw and the waiter catch SIGTERM, each is a caught trap never an ignore, and SIGHUP stays uncaught. (§3.1, §3.2, §3.3)
- `rg -n 'rename_noreplace' CLAUDE.md` finds nothing. Every sentence in the touched passages holds whether or not Tasks 1 and 2 land. No source or test file changes, and `go test ./...` stays green.

STATUS: complete

SPEC CONTEXT: §2 hardens the shared commit cycle (daemon tick, shutdown flush, commit-now): a failed `list-sessions` stands the cycle down on the committing path only (the shared listing keeps swallowing failures for picker/resolver/completion/restore); a confirmation read sent after the last capture read must be answered by the committer's own server (pid from its TMUX) or the cycle stands down; the waiting pane's re-file is a no-clobber hard link whose positional name survives until a committing housekeeping pass; an empty capture over a saved transcript is written only on confirmation; a stand-down writes nothing and exits through each committer's existing failure route with a back-off line. §3 makes the eager hook shell and the parked chain trap TERM (caught, never ignored), has the draw and waiter catch SIGTERM with the chain closing the hand-off gap, and leaves SIGHUP uncaught so a kill still ends the pane.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - CLAUDE.md:60 (`tmux` row) — names `ListSessionNamesProbe` (beside `ListSessionNames`, contrasted with the shared listing), `ConfirmAnswering` (server lifecycle group) and `ServerPIDFromEnv` (package function, second-from-end TMUX field, daemon from the saver pane's TMUX, commit-now from the session-closed hook's).
  - CLAUDE.md:61 (`state` row) — the hard-link (`link(2)`) no-overwrite re-file with the positional name left for housekeeping; the own-server confirmation rule; `ListSessionNamesProbe` on the committing path vs the shared listing's "no server, no sessions" reading; `ErrTmuxStoppedAnswering` as the back-off class; the empty-capture confirmation rule with the refusal logged naming the pane. No "vacated" wording remains (grep for `vacat` finds nothing).
  - CLAUDE.md:192 (Resume hooks) — `sh -c 'trap : TERM; <HOOK>; exec $SHELL'` (`hookShellTrap`), `/bin/sh -c 'trap : INT QUIT TERM; <draw loop>; <recover argv>; <backstop>'` (`parkedChainTrap`), the `parkedChainDraw` re-run rule, `catchSIGTERM` on draw and waiter, caught-never-ignored, SIGHUP uncaught.
  - The `rename_noreplace` sentence is gone; `grep -n rename_noreplace CLAUDE.md` returns nothing, and both `internal/state/rename_noreplace_{darwin,linux}.go` are absent from the tree.
- Notes: Every claim in the touched passages was checked against the code and holds:
  - `ConfirmAnswering` — internal/tmux/tmux.go:92-105 (`display-message -p '#{pid}'`, 0 for empty answer, error otherwise).
  - `ListSessionNamesProbe` — internal/tmux/tmux.go:230-232 over `ListSessionsProbe` (:164-170); shared `ListSessions` swallows at :152-156; sole production caller is internal/state/capture.go:95, which wraps failure in `ErrTmuxStoppedAnswering` (:102).
  - `ServerPIDFromEnv` — internal/tmux/detect.go:17-27; consumed by `ownTmuxServer` (cmd/state_commit_now.go:128-131), used by the daemon (cmd/state_daemon.go:452) and commit-now (cmd/state_commit_now.go:111).
  - Own-server confirmation after the last capture read — internal/state/scrollback.go:356-375 (`confirmOwnServer` at :366 after `captureStructure`, before link/re-file), :267-282 (no-server and other-server answers refused).
  - Back-off line — `cycleFailureMessage` (cmd/state_daemon.go:244-249) keys on `ErrTmuxStoppedAnswering`, used by the tick (:204, with the `save.requested` re-touch), the flush (:391, `flush_completed`), and `failCommitNow` (cmd/state_commit_now.go:144).
  - Empty-capture rule — `confirmEmptyCapture` (internal/state/scrollback.go:295-303) via `ScrollbackWriter.Write` (internal/state/commit_cycle.go:87); daemon logs `empty capture not confirmed; saved transcript kept` with `pane_key` (cmd/state_daemon.go:343-346).
  - Hard-link re-file — `linkStoredScrollback` (internal/state/scrollback.go:148-157) uses `os.Link`, adopting an existing token file on EEXIST.
  - Resume shells — `hookShellTrap` (cmd/state_resume_chain.go:213) used by `hookExecArgs` (:219-221), which `handOffToHookOrShell` (:171-179) uses for answered panes too; `parkedChainTrap` (cmd/state_hydrate.go:281); `parkedChainDraw` (:341-344) loops on status 143 and the pending gate; `catchSIGTERM` (cmd/state_resume_chain.go:204-206) is `signal.Notify` on SIGTERM only and is wired into the draw (cmd/state_resume_draw.go:44, :185) and waiter (cmd/state_resume_wait.go:103, :502). No production `signal.Ignore` and no SIGHUP catch outside the daemon.
  - The touched text states rules, not error shapes ("ends the cycle", "ends it before anything is linked or re-filed"), and holds with Tasks 1 (writer-owned empty-capture confirmation) and 2 (back-off classification of every failed capture read) both landed.

TESTS:
- Status: Adequate
- Coverage: Documentation-only task; no tests are expected. The task commit (782b31a60) touches CLAUDE.md alone, and no Go file in the repository references CLAUDE.md, so the change cannot alter any test outcome.
- Notes: None

CODE QUALITY:
- Project conventions: Followed — prose states rules rather than call graphs and names no deleted file; no process-artifact references (task ids, spec section numbers) in CLAUDE.md.
- SOLID principles: Good (N/A — documentation)
- Complexity: Low
- Modern idioms: Yes (N/A)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` stays green." — needs a unit-lane run of `go test ./...` over the current tree. Reading settles that this task cannot affect the result (CLAUDE.md-only commit; no Go file reads CLAUDE.md), but whether the whole tree passes is only known by running it.
