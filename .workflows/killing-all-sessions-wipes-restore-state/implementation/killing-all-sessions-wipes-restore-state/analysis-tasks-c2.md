# Analysis Tasks: Killing All Sessions Wipes Restore State (Cycle 2)

## Task 1: The Answered-Pane Hold Is Decided From The Last Committed Index
severity: low
sources: architecture

**Problem**: Phase 3 Task 1 added a hold that keeps an answered lazy pane on its token-named transcript. The hold is decided from the caller's `LoadPrev` (`keepAnsweredTranscripts`, called at `internal/state/capture.go:157-159` and defined at `:365`). For the daemon, `LoadPrev` is its in-memory index (`cmd/state_daemon.go:271`). That index changes only when the daemon itself commits, so it does not see a `commit-now` that filed the pane under its token. `holdTokenFiledTranscripts` (`internal/state/scrollback.go:374-397`) covers that lag by checking whether files exist, so it helps only when the positional file named by the pane's fresh record is absent.

The loss is reachable inside one daemon commit gap:
- a pane waits;
- a `session-closed` `commit-now` files it under its token-named file, and that commit's housekeeping removes the positional name;
- the user answers the pane;
- the pane's live address now names a positional file that is already on disk, for example after a sibling pane was killed and the window renumbered;
- as tmux goes down, the pane's capture comes back empty and unconfirmed, or is refused.

Neither pass holds the pane. The commit names the positional file, and `gcOrphanScrollback` deletes the token-named transcript. After the next reboot the pane comes back with another pane's old scrollback, or with none, and the bytes cannot be recovered.

§2.4 states the measure the hold misses: a pane's saved transcript is the file its last committed record names. `RunCommitCycle` already reads that index under the same lock, inside `Commit` (`internal/state/commit.go:37`). The test at `internal/state/commit_cycle_answered_test.go:332` pins that a positional file on disk wins over the token-named transcript whenever the caller's previous index names the positional file.

**Solution**:
- `RunCommitCycle` reads `sessions.json` once under the commit lock, and the hold is decided from that index. A pane that is neither skipped nor waiting, and whose last committed record (matched on its token) names its token-named transcript, keeps naming that transcript until a dump writes the pane's new capture. The writer's `held` map and the `fileAtPositional` hand-back stay as they are.
- The commit measures change and drops against that same read, so `sessions.json` is not decoded a second time.
- `holdTokenFiledTranscripts` is deleted.
- `LoadPrev` still feeds the skeleton, frozen and carry merges, unchanged. When `sessions.json` cannot be read, the hold falls back to `LoadPrev`, as it uses it today.

This is derived from the record:
- §2.4 names the last committed record as the measure.
- §4.1 measures drops against the on-disk index for the same reason: the daemon's in-memory index does not see `commit-now`'s writes.
- The phase 3 fix round decided against pointing the daemon's whole `LoadPrev` at disk, because of its cost to those merges and the extra decode. This proposal reads the disk for the hold alone and reuses the read `Commit` already makes.

**Outcome**: Whether an answered pane keeps its transcript no longer depends on which committer filed it under its token, or on what its new positional path holds. From the answer until a dump writes the pane's new capture with confirmed bytes, every commit names the token-named transcript the last commit named. That includes the case where the pane's new address names a positional file another pane left on disk.

## Task 2: A Session Listing Tmux Answered But Portal Could Not Parse Is Classified By The Confirmation
severity: low
sources: architecture

**Problem**:
- `captureStructure` wraps every `ListSessionNamesProbe` error in `ErrTmuxStoppedAnswering` (`internal/state/capture.go:95-98`).
- That probe returns `parseSessionList`'s errors through the same return as a failed `list-sessions` (`internal/tmux/tmux.go:163-169`, `:185-197`).
- `classifyFailedCapture` returns early on the sentinel (`internal/state/scrollback.go:408-411`).

So a listing that tmux answered in full, but that Portal could not parse, is labelled a back-off without any confirmation being sent.

A session name containing `|` triggers this every time. tmux accepts the name, and neither `ValidateSessionName` nor `SanitiseProjectName` refuses it. The `|` shifts the `|`-delimited fields of `listSessionsArgs`, so the window count fails to parse. As long as that session exists:
- every daemon tick logs `tick backed off: tmux stopped answering` at WARN;
- every session close logs `commit cycle backed off: tmux stopped answering` at ERROR.

The log blames tmux going down when the cause is the session list. These are also the lines that §4.3's reboot reading counts as saves in flight while tmux went down, and the deferred hold (§5.3) waits on that evidence.

The pane listing already separates the two cases: its read is wrapped (`capture.go:104-107`), and its parse (`parsePaneRows`) is not. Phase 1 Task 1 settled that failures which are not refused reads stay `… failed`.

**Solution**:
- Only a failed `list-sessions` read is wrapped in `ErrTmuxStoppedAnswering`.
- The tmux client marks a failure to parse the session listing with a sentinel in `internal/tmuxerr`, and `captureStructure` returns that error unwrapped. `classifyFailedCapture`'s confirmation then classifies it the way it classifies a pane-row parse failure: `… failed` when the committer's own server answers, a back-off when it does not.
- Refused reads keep their unconditional wrap and still send no confirmation after them.
- `ErrTmuxStoppedAnswering`'s doc names the session listing's read rather than the listing as a whole.
- Writes, retries, `save.requested` and `flush_completed` do not change. Only the logged line moves.

This is derived from the record:
- phase 1 Task 1: failures that are not refused reads stay `… failed`;
- cycle 1 Task 2: a failure that is not already a refused read is classified by the confirmation;
- §4.2: a back-off line is for a committer that backed off because tmux stopped answering.

`tmuxerr` is the leaf package `internal/state` already uses to tell tmux error classes apart without importing the client. Marking the parse failure there mirrors the pane listing, whose read is wrapped and whose parse is not.

**Outcome**: While the committer's own server answers, a session list Portal cannot parse logs `tick failed` and `commit cycle failed` with the parse error. Every capture read tmux refuses still logs its committer's back-off line.
