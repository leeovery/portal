# Consolidation Findings: Killing All Sessions Wipes Restore State (Phase 1)

## Findings

### F1: The back-off classification covers two of the cycle's tmux reads; the others refused by an exiting tmux log as plain failures
- **Class**: behaviour
- **Failure**: An exiting tmux refuses every read sent after its exit begins. The cycle's reads run in this order: skeleton markers, `list-sessions`, `list-panes`, `show-environment` for each session, then the confirmation. Only two of those failures are wrapped in `ErrTmuxStoppedAnswering`: the session listing (Task 1-1) and the confirmation (Tasks 1-2 and 1-3). `cycleFailureMessage` (Task 1-4) reads that sentinel, so only those two give the `tick / final flush / commit cycle backed off: tmux stopped answering` line. The same shutdown can land just before the skeleton-marker read or the `list-panes` read instead. Then the stand-down is logged as `tick failed`, `final flush failed` or `commit cycle failed`. Those are the words used for a real fault, such as a parse error or `errCarryNameTaken`. At a reboot the daemon and tmux are SIGTERMed together, so the final flush can be refused at any of these points. The user reads that log for evidence of how macOS ended tmux (§4.3, §5.4). The lines are noticed when grepping for "backed off" undercounts the saves that were in flight as tmux went down, and the matching "failed" lines look like genuine faults.
- **Evidence**:
  - `internal/state/capture.go:92-95` wraps the listing failure in `ErrTmuxStoppedAnswering`.
  - `internal/state/capture.go:101-104` returns a `list-panes` failure unwrapped.
  - `internal/state/scrollback.go:350-353` returns a skeleton-marker read failure as plain `list skeleton markers: %w`.
  - `internal/state/scrollback.go:359-361` wraps the confirmation refusal.
  - `cmd/state_daemon.go:244-249` (`cycleFailureMessage`) classifies on the sentinel alone. It is consumed at `cmd/state_daemon.go:204` (tick), `cmd/state_daemon.go:391` (final flush) and `cmd/state_commit_now.go:152` (`failCommitNow`).
  - `internal/state/scrollback.go:250-253`: the `ErrTmuxStoppedAnswering` doc lists only the listing and the confirmation.
  - Tests that pin today's `tick failed` for a marker-read failure: `cmd/state_daemon_run_test.go:705` and `:765`.
- **Proposed shape**: Wrap the tmux-read failures of `ListSkeletonMarkers` (`scrollback.go:350`) and `ListAllPanesWithFormat` (`capture.go:101`) in `ErrTmuxStoppedAnswering`, the same way `capture.go:94` wraps the listing. Then every capture read tmux refuses is classified alike. Non-read failures stay `failed`: `parsePaneRows`, `errCarryNameTaken` and the all-sessions-anomalous aggregate. Widen the sentinel's doc to "a capture read failed or the confirmation was refused", and move the marker-read tests' expected message to the back-off line. Writes and retries do not change; only the message moves.

## Comment Corrections

- `internal/state/scrollback.go:141-146` — every re-file runs under the exclusive commit lock, so there is no concurrent re-file for link(2)'s atomicity to guard against. The phase rewrote this sentence and kept the stale claim.
  OLD: // A record naming no file is treated as a missing source: the bytes are already
// wherever they are, and joining an empty path onto dir would name the state
// directory itself. An existing token-named file is adopted rather than
// replaced: the positional file may by now hold another pane's capture, and
// link(2) refuses an existing name atomically so a concurrent re-file cannot
// slip between check and placement.
  NEW: // A record naming no file is treated as a missing source: the bytes are already
// wherever they are, and joining an empty path onto dir would name the state
// directory itself. An existing token-named file is adopted rather than
// replaced: the positional file may by now hold another pane's capture.

- `internal/state/scrollback.go:240-245` — the reason after the colon comes from Task 1-2 and repeats `confirmOwnServer`'s doc. Its "only a refused read" is narrower than the landed own-server rule: an answer naming another server, or none, also stands the cycle down.
  OLD: // AnsweringConfirmer confirms tmux is still answering. ConfirmAnswering must
// return a nil error only for a read tmux answered with exit status 0, with the
// pid of the server that answered it, or 0 for an answer naming none: an exiting
// tmux can answer a read already in flight with exit status 0 and no output, and
// only a refused read sent after it tells that answer apart from an empty
// server.
  NEW: // AnsweringConfirmer confirms tmux is still answering. ConfirmAnswering must
// return a nil error only for a read tmux answered with exit status 0, with the
// pid of the server that answered it, or 0 for an answer naming none.

- `CLAUDE.md:194` (project guide prose) — Task 1-6 deleted `rename_noreplace_{darwin,linux}.go`. The re-file is now a plain `link(2)`, and the positional name stays until the committing housekeeping pass removes it.
  OLD: The waiting pane's scrollback re-file onto its token-named path goes through a no-replace rename (`internal/state/rename_noreplace_{darwin,linux}.go`, falling back to `link(2)` where the filesystem rejects the flag), so it never overwrites an existing token-named file.
  NEW: The waiting pane's scrollback re-file onto its token-named path is a `link(2)`, so it never overwrites an existing token-named file, and the positional name stays on disk until the housekeeping pass of the commit naming the token — a cycle that ends uncommitted leaves `sessions.json` naming a file still present.

- `CLAUDE.md:61` (project guide prose, `state` row) — the re-file no longer vacates the positional path; it links, and the positional name stays until the commit's housekeeping pass.
  OLD: so no committing caller can leave out the skeleton set or commit an index that still names a waiting pane's vacated positional path.
  NEW: so no committing caller can leave out the skeleton set or commit an index that still names a waiting pane's positional path.

## Spec Defects

### S1: The specification still describes the waiting pane's re-file as a rename
- **Claim**: §2.3 says "The capture cycle renames a newly waiting pane's transcript from its positional path to its token-named path (`refilePendingScrollback` in `internal/state/scrollback.go`)". §3.2 item 1 says "A capture builds the pane a fresh record naming its positional scrollback path. That file was renamed away when the pane first went waiting."
- **Observed**: Task 1-6 replaced the no-clobber rename with a link. `refilePendingPane` calls `linkStoredScrollback`, which calls `os.Link` (`internal/state/scrollback.go:133`, `:147-156`). The doc at `scrollback.go:89-102` says the positional name is left for the housekeeping pass of the commit naming the token. `rename_noreplace_{darwin,linux}.go` are deleted. `internal/state/commit_cycle_uncommitted_test.go:167-169` asserts that the positional file is gone only once a committing cycle's housekeeping has run.
- **Read**: Spec stale, in the wording of the mechanism only. §3.2's chain of consequences still holds: the first committed cycle's housekeeping removes the positional file, so a later fresh record names a missing file. But Phase 2 builds on §3.2, and "renamed away" points its executors at a rename the tree no longer performs.
