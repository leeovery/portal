TASK: The Daemon's Dump Never Writes An Unconfirmed Empty Capture Over A Saved Transcript (killing-all-sessions-wipes-restore-state-1-5, tick-d5feb3)

ACCEPTANCE CRITERIA:
- A pane has a non-empty saved transcript; the daemon's dump captures it empty, and the tmux read sent after that capture is refused. The saved transcript is byte-for-byte unchanged, and the log holds a line at INFO or above naming the pane in `pane_key`. Holds in the daemon's tick and its shutdown flush.
- Same, with the read after the capture answered by a different server started on the same socket: the transcript is unchanged and the refused write is logged naming the pane in `pane_key`.
- Same, with the daemon's own server (the one that answered the capture) answering the read after it with exit status 0: the empty capture replaces the saved transcript.
- The refused-write line uses only an existing log component and existing attribute keys.

STATUS: complete

SPEC CONTEXT: §2.4 says an empty capture may replace a saved non-empty transcript only when the §2.2 rule confirms it: a tmux read sent strictly after the capture must be answered by the committer's own server. Otherwise nothing is written, the transcript stands, and the refusal is logged (§4.2: INFO or above, the pane in `pane_key`, closed vocabulary only). Two corrigenda bear on this task. The 2026-10-05 corrigendum to §2.2 says an exit-0 answer naming no server is a refusal. This supersedes the AC's "whatever its output" wording, which an answer that cannot name the own server could never satisfy anyway, so the code's stricter reading is the sound one. The 2026-10-06 corrigendum to §2.4 routes the dump to `WriteScrollbackIfChanged` only through the cycle's `ScrollbackWriter` and makes a pane's saved transcript the file its last committed record names.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/commit_cycle.go:74-95: `ScrollbackWriter.Write` judges against the stored (token-named) path for a held pane and the positional path otherwise. It calls `confirmEmptyCapture` before `WriteScrollbackIfChanged`.
  - internal/state/scrollback.go:295-303: `confirmEmptyCapture` checks only an empty capture over a file that may hold bytes. It sends a fresh `ConfirmAnswering` through `confirmOwnServer`, and a refusal wraps `ErrUnconfirmedEmptyCapture`.
  - internal/state/scrollback.go:306-312: `savedTranscriptMayHoldBytes` presumes an uninspectable file holds bytes.
  - internal/state/scrollback.go:267-282: `confirmOwnServer` refuses a confirmation answered by another pid, one naming no server, and one with the own server unknown.
  - internal/state/commit_cycle.go:135-148: the writer is built with the cycle's client and `OwnServer` and handed to `Dump`.
  - cmd/state_daemon.go:342-347: `dumpPane` logs `"empty capture not confirmed; saved transcript kept"` at WARN on `deps.Logger`, which is `daemonLogger`, component `daemon` (cmd/state_common.go:8). The attrs are `pane_key` and `error`.
  - The shutdown flush reaches the same dump through `captureAndCommit(context.Background(), deps)` (cmd/state_daemon.go:389).
- Notes:
  - The confirmation is a new `display-message` sent in `Write`, which runs after `CaptureAndHashPane` returns (cmd/state_daemon.go:331 then :342). It is strictly after the capture, not a reuse of the cycle's pre-dump confirmation.
  - An answer from the own pid also proves the capture reached that server, by the §2.2 argument recorded in `confirmOwnServer`'s comment.
  - The only production call to `WriteScrollbackIfChanged` is commit_cycle.go:90. `internal/state/commit_guard_test.go:175-178` guards against any non-test file outside `internal/state` naming it.
  - `commit-now` passes no `Dump`, so it writes no scrollback.
  - A refusal does not fail the cycle. The commit still lands on the already-confirmed index with the refused pane's record naming its kept file. This matches §2.4, which asks only that the write be withheld and logged.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_daemon_empty_capture_test.go:75-100 runs the tick and the shutdown flush against three unconfirmed answers: refused, another server, and naming no server. A dispatch hook flips the confirmation only after `capture-pane`, so the cycle's own pre-dump confirmation passes and only the post-capture read is refused. Each case asserts the transcript is byte-identical and exactly one `daemon` line at INFO or above, with keys exactly `[component pane_key error]` and `pane_key` = `work__0_0` (AC1, AC2, AC4).
  - Lines 102-122 cover the confirmed case on both paths. The transcript is emptied, no refusal line is logged, and exactly two `display-message` reads are sent (the cycle's and the empty capture's), which pins that the post-capture confirmation is actually sent (AC3).
  - Lines 124-172 pin the anomalous tally and keep the refusal line apart from `write scrollback failed`.
  - Lines 209-329 cover the held, token-named transcript case from the 2026-10-06 corrigendum.
  - internal/state/empty_capture_test.go:93-166 covers the writer:
    - no confirmation is sent when nothing can be lost (a non-empty capture, no saved file, or an empty saved file);
    - a dedup hit writes nothing;
    - an uninspectable saved file is refused and left intact.
  - The tests would fail if the feature broke. Removing the confirmation fails the refused cases, an always-refuse fails the confirmed case, and reusing the pre-dump confirmation fails the refused cases.
- Notes: The tests are focused and none are redundant. The table drives both daemon dump paths and does not duplicate bodies.

CODE QUALITY:
- Project conventions: Followed. The writer seam is created by the commit cycle and is the only route to the write, enforced by a guard. The log line uses the bound component logger and the closed vocabulary. No `t.Parallel`. Seams are injected through the existing fake commander.
- SOLID principles: Good. The refusal decision lives in `internal/state`, beside the confirmation rule it shares with the commit's stand-down. `cmd` owns only the log rendering.
- Complexity: Low
- Modern idioms: Yes. `%w: %w` multi-wrap and `errors.Is` discrimination are used, and the tests use Go 1.26 `new(expr)`, which matches `go 1.26.0` in go.mod.
- Readability: Good. The comments on `Write`, `confirmEmptyCapture` and `WriteScrollbackIfChanged` hold true against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
