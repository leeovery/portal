## Attempt 1

ISSUES:
- /Users/leeovery/Code/portal/CLAUDE.md:61 (`state` row, the new stand-down sentence). The text says a stand-down "ends through each committer's existing failure route (the tick's `tick failed` WARN …)". The code says otherwise. `cmd/state_daemon.go:204` logs `cycleFailureMessage("tick", err)`, and `cycleFailureMessage` (`cmd/state_daemon.go:244-249`) returns `tick backed off: tmux stopped answering` for anything wrapping `ErrTmuxStoppedAnswering`. `failCommitNow` (`cmd/state_commit_now.go:152`) and the flush (`cmd/state_daemon.go:391`) work the same way. So the WARN a stand-down actually emits is the back-off line, not `tick failed`. The sentence just before this one in the same paragraph correctly says `ErrTmuxStoppedAnswering` is what "the committers' back-off lines key on", so the paragraph now contradicts itself. The risk: an agent bringing the code in line with this text could fold the stand-down back into `<stage> failed`. That would remove the back-off line the spec requires, which is what a post-reboot log is read for. Nothing would flag it, because no test reads CLAUDE.md.
  FIX: Replace the OLD text verbatim with the NEW text.
    OLD: and ends through each committer's existing failure route (the tick's `tick failed` WARN and `save.requested` re-touch, `failCommitNow`, the flush's `flush_completed=false`).
    NEW: and ends through each committer's existing failure route (the tick's WARN and `save.requested` re-touch, `failCommitNow`, the flush's `flush_completed=false`), its line reading `<stage> backed off: tmux stopped answering` where any other failure reads `<stage> failed`.
  CONFIDENCE: high
- /Users/leeovery/Code/portal/CLAUDE.md:61 (`state` row, the sentence after the inserted block). The new text went in between the failed-capture sentence and "No committer calls it directly: every committing cycle runs through `RunCommitCycle`". The sentence directly before "it" is now the one about the daemon's scrollback dump logging a refusal naming the pane. So "it" now reads as "the dump" or "the pane", not `captureAndRefile`. The only statement that `captureAndRefile` is reachable solely through the locked `RunCommitCycle` now depends on a referent about ten sentences back. An agent reading it as "the dump" loses the rule that no committer captures outside the commit lock.
  FIX: Name the referent. Replace the OLD text with the NEW text (it occurs once in the file).
    OLD: No committer calls it directly:
    NEW: No committer calls `captureAndRefile` directly:
  CONFIDENCE: high

NOTES:
- CLAUDE.md:60 (`tmux` row) says `ServerPIDFromEnv` "is how a committer run by a tmux hook knows its own server". That is true but incomplete. The daemon gets its `OwnServer` the same way: `cmd/state_daemon.go:453` calls `ownTmuxServer()` → `ServerPIDFromEnv(os.Getenv("TMUX"))`, reading the `_portal-saver` pane's environment. A more accurate wording, which can be folded into the same fix round: "is how each committer knows its own server — the daemon from its `_portal-saver` pane's `TMUX`, `commit-now` from the `session-closed` hook's". This does not block.
- An older sentence in the same row was not touched by this diff and is out of scope. It says a carry-name capture failure retries through "the daemon's `tick failed` WARN". That still holds while the own server is healthy. Since Task 3-2, the same failure on a server that has stopped answering logs as a back-off.
- I did not run `go test ./...` independently, because no Go source or test file changed.
- Nothing arrived in the dispatch beyond the enumerated inputs.
