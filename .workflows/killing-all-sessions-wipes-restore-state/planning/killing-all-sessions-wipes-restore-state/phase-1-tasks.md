# Phase 1: The save path never acts on a session view it cannot confirm — 8 tasks

## killing-all-sessions-wipes-restore-state-1-1

### Task 1.1: Failed session listing stands the commit cycle down

**Problem**: Every save reads the session list through the one listing all readers share, and that listing swallows a failed `list-sessions` and returns an empty list (`rg -n 'Swallowed deliberately' internal/tmux/tmux.go` → 1 hit, in `ListSessions`). When tmux begins exiting while a save is mid-capture, the save sees zero sessions and commits an empty index. The housekeeping pass inside that commit then deletes every scrollback file, including the files of sessions still alive. At a reboot, the save most likely to be in flight is the daemon's SIGTERM flush, and a hook-run `commit-now` can hit the same window. The existing test of a failed listing passes only because its fake returns an error the production client never delivers.

**Solution**: A failed `list-sessions` becomes an error to the shared commit cycle. The daemon's tick, its shutdown flush and `portal state commit-now` all run that cycle, so each one stands down instead of reading the failure as zero sessions. The change lands on the committing path only. The shared listing keeps reading a failure as "no server, so no sessions" for the picker, the resolver, shell completion and restore.

**Outcome**: With the production tmux client, a failed `list-sessions` ends every committer's cycle with nothing written, deleted or emptied. Every other reader of the session list, restore above all, behaves exactly as it does today.

**Acceptance Criteria**:
- [ ] The saved state holds sessions and their scrollback files. The production client's `list-sessions` fails while the shared commit cycle runs. The cycle returns an error, `sessions.json` is unchanged, and every scrollback file is still present with its content. (§2.1, §2.5, §6.1)
- [ ] With that saved state and the restore-in-progress marker reading as unset, the production client's `list-sessions` fails during the daemon's tick, during its shutdown flush and during `commit-now`. In each case `sessions.json` and every scrollback file are unchanged afterwards. (§2.1, §6.1)
- [ ] A `list-sessions` succeeds and names only Portal's own underscore-prefixed sessions. That is not a failure: the capture yields an empty index with no error. (§5.1)
- [ ] The session listing fails while bootstrap's restore reads it. Restore rebuilds every session in the saved state, as it does today, and no commit of an empty index follows it. (§2.1, §5.1, §6.1)
- [ ] The session listing fails. The picker lists no sessions, the resolver matches no session, and shell completion offers no session names. None of them reports an error, exactly as today. (§2.1, §5.1)

**Do**:
- Leave the shared listing swallowing its failure. That is `ListSessions` in `internal/tmux/tmux.go`, and `ListSessionNames`, which reads through it. The change lands on the committing path only (§2.1).
- `ListSessionsProbe` (`internal/tmux/tmux.go`) is the existing variant of the listing that returns the failure instead. Capture's listing read is in `captureStructure` (`internal/state/capture.go`), which does not use it today (`rg -c 'ListSessionsProbe' internal/state` → no matches) (§2.1).
- Restore keeps its current listing read (`snapshotLiveSessions`, `internal/restore/restore.go`) (§2.1).
- Rework the "it returns an error when ListSessionNames fails and does not call show-environment" subtest of `TestCaptureStructurePreLoopFailFatal` (`internal/state/capture_test.go`). It must exercise the listing the committing path actually uses, instead of a fake that returns an error the production client never delivers (§6.6).
- Keep the "returns empty slice when tmux server is not running" case of `TestListSessions` (`internal/tmux/tmux_test.go`) for the picker. It must no longer describe the save path (§6.6).
- Leave the three empty-save contract tests listed in §5.1 as they are.

**Context**:
> All three committers run one shared commit cycle (`state.RunCommitCycle`, `internal/state/commit_cycle.go`). It runs capture, then moves waiting panes' transcripts to their token-named paths, then runs the daemon's scrollback dump, then the commit and its housekeeping pass (`Commit` / `gcOrphanScrollback`, `internal/state/commit.go`). Everything in §2 applies to that cycle, so it applies to all three committers alike.
>
> The shared listing must not change because restore reads the same listing as capture (`rg -n 'ListSessionNames\(\)' internal/state/capture.go internal/restore/restore.go` → `capture.go:88`, `restore.go:99`). Restore already skips every session when that listing returns an error (`rg -n 'list-sessions failed' internal/restore/restore.go` → 1 hit, in `snapshotLiveSessions`). If the shared listing began returning its failures, one transient failure at bootstrap would make restore bring back nothing. The daemon's next tick would then truthfully commit an empty index over the full saved state. That is the same wipe, reached without anyone killing anything (§2.1).
>
> A stand-down writes no commit and runs no housekeeping pass. The cycle then ends through each committer's existing failure route (§2.5). The back-off line that route logs is Task 1.4.
>
> The prior `v1` specification defined "no server running means zero sessions" for the picker's session discovery. The picker, the resolver, completion and restore keep that reading (§7).

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.2, §2, §2.1, §2.5, §5.1, §6.1, §6.6, §7

## killing-all-sessions-wipes-restore-state-1-2

### Task 1.2: A refused confirmation read stands the commit cycle down

**Problem**: A failed listing is only one of the answers tmux gives while it shuts down, and the others are invisible to any error check. Once tmux begins exiting during a read, it can answer an in-flight `list-sessions` or `list-panes` with exit status 0 and no output. An empty `list-sessions` commits an empty index. An empty `list-panes` next to an environment read that still succeeds records a session with no windows. That deletes the session's scrollback files while its name stays in `sessions.json`. `tmux kill-server` survives today only by accident. Each save's first read is the restore-in-progress marker, and against an exiting server that read fails and is taken to mean "a restore is in progress, do not commit". A server that begins exiting a few milliseconds after that first read gets no such protection.

**Solution**: Before the shared commit cycle writes a commit, it confirms that tmux is still answering. It does this with a tmux read sent strictly after the last read the captured index is built from. The confirmation counts as answered when tmux returns exit status 0, whatever the output. Anything else is a refusal, and a refused confirmation stands the cycle down: no commit and no housekeeping pass.

**Outcome**: Every commit any of the three committers writes rests on proof that every capture read was answered before tmux began exiting. A capture holding a shutdown answer is never committed. A genuinely empty session list, such as the one left once the last user session is killed, still commits.

**Acceptance Criteria**:
- [ ] The saved state holds sessions and their scrollback files. A cycle's capture reads are all answered, then its confirmation read is refused. The cycle returns an error, `sessions.json` is unchanged, and every scrollback file is still present with its content. This holds whether the cycle is the daemon's tick, its shutdown flush or `commit-now`. (§2.2, §2.5, §6.1)
- [ ] A capture's `list-sessions` comes back with exit status 0 and no output, and the server then refuses connections. The cycle writes nothing and deletes no scrollback file. (§2.2, §6.1)
- [ ] A capture's `list-panes` comes back with exit status 0 and no output, the environment reads after it still succeed, and the server then refuses connections. No session is recorded without its windows, `sessions.json` is unchanged, and no scrollback file is deleted. (§2.2, §6.1)
- [ ] The server begins exiting just after one of a capture's reads and refuses every connection after it. Whichever read that is (the session listing, the pane listing, or any one session's environment read), the cycle writes nothing and deletes no scrollback file. (§2.2)
- [ ] The confirmation read is sent strictly after the last capture read and comes back with exit status 0 and no output. The cycle writes its commit. (§2.2, §6.1)
- [ ] A capture's `list-sessions` names only Portal's own sessions, because the last user session has been killed, and the confirmation is answered. The cycle commits the empty index, and its housekeeping pass removes the scrollback files the index no longer names. (§5.1)
- [ ] In every cycle that writes a commit, the confirmation read is sent after the last of the capture's reads. (§2.2)

**Do**:
- The confirmation belongs to the shared commit cycle every committer runs (`state.RunCommitCycle`, `internal/state/commit_cycle.go`), so it applies to the daemon's tick, its shutdown flush and `commit-now` alike (§2).
- A confirmation counts as answered when tmux returns exit status 0, whatever the output. Anything else is refused (§2.2).
- A refused confirmation writes no commit and runs no housekeeping pass (`Commit` / `gcOrphanScrollback`, `internal/state/commit.go`). The cycle ends as a failed cycle (§2.5).

**Context**:
> The rule rests on two properties of tmux 3.7c, taken from its source and borne out in the sandbox. Once the server begins exiting, it refuses every new client connection (`server.c`, `server_accept`). Once it has started exiting, it never stops (`server_exit` is set once, in the SIGTERM handler, before any session is destroyed). So an answered confirmation proves that every capture read before it was answered before the exit began. Even tmux's own shutdown answer to the confirmation (exit 0, no output) proves it, because any client tmux accepted at all was accepted before the exit began (§2.2).
>
> A `list-sessions` or `list-panes` that returned exit 0 with no output because tmux began exiting during the read was answered after the exit began. So the confirmation sent after it is refused. Under `tmux kill-server`, no committer can pass the confirmation, whichever of its reads the exit lands after. Survival under `kill-server` then no longer depends on which read happens to fail first (§2.2).
>
> Confirming before any waiting pane's transcript is moved (`refilePendingScrollback`, `internal/state/scrollback.go`) does not by itself keep `sessions.json` naming only files that exist. That guarantee, for every way a cycle can end uncommitted after the move, is Task 1.6 (§2.3).
>
> The rule that only the committer's own server can answer the confirmation is Task 1.3. Each committer's back-off line is Task 1.4. The dump's handling of an empty `capture-pane` is Task 1.5.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.2, §2, §2.2, §2.3, §2.5, §5.1, §6.1

## killing-all-sessions-wipes-restore-state-1-3

### Task 1.3: The confirmation counts only from the committer's own server

**Problem**: An answered confirmation proves nothing unless it was answered by the server the save belongs to. After a tmux server exits, a new one can be started on the same socket before its restore has run, and it holds none of the user's sessions. A save whose capture reads or confirmation reach that new server could pass the confirmation and commit an empty or partial view over the full saved state. The daemon's own case needs the opposite answer. When `portal uninstall` kills `_portal-saver` on a running server, the daemon's pane is gone but its server is not, and the shutdown flush that the kill triggers must still commit.

**Solution**: The confirmation counts only when it is answered by the tmux server the committer belongs to, and only when that same server answered every capture read the commit is built from. For the daemon, that is the server its `_portal-saver` pane runs in, and it stays the daemon's own server after that pane is destroyed. For `commit-now`, it is the server whose `session-closed` hook ran it. A save whose capture reads or confirmation reach any other server stands down.

**Outcome**: No committer commits a view that a different server, started on the same socket, captured or confirmed. The daemon's shutdown flush after `portal uninstall` on a running server still commits.

**Acceptance Criteria**:
- [ ] The daemon runs in `_portal-saver` on a server holding the user's sessions, and `portal uninstall` kills `_portal-saver` while the server keeps running. The daemon's shutdown flush commits, and its `shutdown` line reports `flush_completed=true`. (§2.2, §6.1)
- [ ] The daemon's tick, with its own server answering every capture read and the confirmation, commits as before. (§2.2)
- [ ] The committer's own server exits, and a new server is started on the same socket before its restore has run. A save whose capture reads and confirmation are all answered by the new server writes nothing: `sessions.json` and every scrollback file are unchanged. (§2.2, §6.1)
- [ ] A save's first capture reads are answered by its own server, which then exits. Its remaining capture reads, or only its confirmation, are answered by a new server started on the same socket. Nothing is written: `sessions.json` and every scrollback file are unchanged. (§2.2, §6.1)
- [ ] A server's `session-closed` hook runs `commit-now`, and that server answers its capture reads and its confirmation. The commit is written as before. (§2.2)
- [ ] A server's `session-closed` hook runs `commit-now`, and the hook's server exits before `commit-now` reads. Its reads reach a new server started on the same socket. Nothing is written. (§2.2)

**Do**:
- The daemon's own server is the server its `_portal-saver` pane runs in. It stays the daemon's own server after that pane is destroyed (§2.2). The daemon's committers are `tick` and `defaultShutdownFlush`, both through `captureAndCommit` in `cmd/state_daemon.go`.
- `commit-now`'s own server is the server whose `session-closed` hook ran it (§2.2). The command is `cmd/state_commit_now.go`. The hook runs it through `run-shell` (`commitNowCommand`, `internal/tmux/hooks_register.go`).
- A confirmation answered by any other server, or one following capture reads that any other server answered, is refused and stands the cycle down (§2.2, §2.5).

**Context**:
> A new server started on the same socket after the committer's own server exited, before its restore has run, holds none of the user's sessions. Its answers confirm nothing, and a save that reaches it stands down (§2.2).
>
> The save that runs when `portal uninstall` kills `_portal-saver` on a running server still commits, because the daemon's own server outlives the pane (§2.2). `portal uninstall` kills `_portal-saver`, which delivers SIGHUP to the daemon and triggers its shutdown flush (`cmd/uninstall.go`).
>
> This task narrows the confirmation Task 1.2 introduced. Task 1.4 reports each stand-down through the committer's failure route. The dump's confirmation of an empty capture (Task 1.5) uses the same own-server rule (§2.4).

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2.2, §2.4, §2.5, §6.1

## killing-all-sessions-wipes-restore-state-1-4

### Task 1.4: Each committer reports a stand-down through its existing failure route with a back-off line

**Problem**: Today a wipe is silent at the default log level. The only trace is `capture: tick complete sessions=0` or bare `process: start … state commit-now` lines. Once a cycle can stand down on a failed listing or a refused confirmation, each committer has to end that cycle as a failed one, through the route it already has, and say why. Otherwise a stood-down save is never retried, and nothing at the default level records that a save backed off because tmux stopped answering. The kill path depends on that retry: when a kill's `commit-now` stands down, only the daemon's next tick commits the kill.

**Solution**: Each committer reports a stand-down through its existing failure route, with a line at INFO or above saying it backed off because tmux stopped answering and the cause in `error`. The daemon's tick logs the failure and re-touches `save.requested`, so its next tick retries. `commit-now` logs the failure, touches `save.requested` and exits non-zero. The shutdown flush logs the failure and reports `flush_completed=false`. The lines each committer already logs for a failed restore-marker read stay exactly as they are.

**Outcome**: Every stand-down leaves a default-level line saying the save backed off because tmux stopped answering. The daemon's next tick commits whatever a stood-down save did not, including a kill whose `commit-now` stood down, and saves never stall.

**Acceptance Criteria**:
- [ ] The daemon's tick stands down, once because its session listing fails and once because its confirmation is refused. Each time, the log holds a line at INFO or above saying the save backed off because tmux stopped answering, with the cause in `error`, and `save.requested` is present afterwards. (§2.5, §4.2, §6.3)
- [ ] `commit-now` stands down, once because its session listing fails and once because its confirmation is refused. Each time, the log holds a line at INFO or above saying the save backed off because tmux stopped answering, with the cause in `error`. `save.requested` is touched and the command exits non-zero. (§2.5, §4.2, §6.3)
- [ ] The daemon's shutdown flush stands down, once because its session listing fails and once because its confirmation is refused. Each time, the log holds a line at INFO or above saying the save backed off because tmux stopped answering, with the cause in `error`, and the `shutdown` line reports `flush_completed=false`. (§2.5, §4.2, §6.3)
- [ ] One tick stands down on a transient listing failure on a healthy server, and the next tick's reads are all answered. That next tick commits, so the save is delayed by one tick. (§2.5)
- [ ] A live session is killed, and the `commit-now` its `session-closed` hook runs stands down. The daemon's next tick, with tmux answering, removes the killed session from `sessions.json` and deletes its scrollback files. (§2.5, §5.1, §5.2)
- [ ] The restore-in-progress marker read fails in the tick, in the flush and in `commit-now`. Each logs its existing WARN line for that failure, unchanged, with the cause in `error`. (§4.2)
- [ ] The restore-in-progress marker reads as set in the tick, in the flush and in `commit-now`. None of them logs a line it does not log today. (§4.2)
- [ ] Every back-off line uses an existing log component and existing attribute keys only. No new log component or attribute key is introduced. (§4.2)

**Do**:
- Report through each committer's existing failure route (§2.5). For the daemon's tick, that is the `tick failed` WARN followed by `state.TouchSaveRequested` (`tick`, `cmd/state_daemon.go`; `rg -n 'TouchSaveRequested' cmd/state_daemon.go` → 1 hit, after `tick failed`). For `commit-now`, it is `failCommitNow` (`cmd/state_commit_now.go`). For the shutdown flush, it is the `final flush failed` WARN and the `shutdown` line carrying `flush_completed` (`defaultShutdownFlush`, `cmd/state_daemon.go`).
- Carry the cause in the `error` attribute key, which Portal's closed log vocabulary already defines (§4.2).
- Leave the three restore-marker WARN lines as they are (`rg -n 'read @portal-restoring|isRestoring query failed' cmd/state_daemon.go cmd/state_commit_now.go` → 3 hits) (§4.2).

**Context**:
> A stand-down writes no commit and runs no housekeeping pass, so `sessions.json` stays as it was and no scrollback file is deleted or emptied. A transient failure on a healthy server therefore delays a save by one tick, and saves never stall (§2.5).
>
> tmux's `run-shell` can surface `commit-now`'s non-zero exit to an attached client. More WARN and ERROR lines at teardown are expected and accepted (§2.5).
>
> A save whose first read, of the restore-in-progress marker, fails has backed off too. All three committers already log that at WARN with the cause in `error`, and those lines stay. A marker that reads as set means a restore is in progress, which is not a back-off, and logs nothing new (§4.2).
>
> After the fix, back-off lines in a reboot's log mean a save was in flight as tmux went down. A teardown in which everything is hard-killed runs no committer and leaves no line at all (§4.3).
>
> The dump's refused-empty-write line is Task 1.5. The per-session drop line is Phase 2.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2.5, §4, §4.2, §4.3, §5.1, §5.2, §6.3

## killing-all-sessions-wipes-restore-state-1-5

### Task 1.5: The daemon's dump never writes an unconfirmed empty capture over a saved transcript

**Problem**: The daemon's scrollback dump runs in its tick and its shutdown flush. It writes a pane's scrollback read whenever that read differs from the saved file (`WriteScrollbackIfChanged`, called from the dump in `cmd/state_daemon.go`), so an empty read overwrites a saved transcript with zero bytes. That loss involves no housekeeping pass and no change to the session list, so neither the stand-down nor the commit's confirmation prevents it. The sandbox never saw tmux's shutdown answer (exit 0, no output) on `capture-pane`. But `capture-pane` goes through the same client path as the listings that did show it.

**Solution**: An empty capture may replace a saved non-empty transcript only once the §2.2 rule confirms it: a tmux read sent strictly after that capture has been answered by the server the daemon belongs to, the same server that answered the capture. An unconfirmed empty capture is not written. The saved transcript stands, and the refused write is logged with the pane named in `pane_key`.

**Outcome**: No tick or shutdown flush empties a saved transcript on a `capture-pane` answer it cannot confirm, and every refusal leaves a default-level line naming the pane. A confirmed empty capture is still written.

**Acceptance Criteria**:
- [ ] A pane has a non-empty saved transcript. The daemon's dump captures it empty, and the tmux read sent after that capture is refused. The saved transcript is byte-for-byte unchanged, and the log holds a line at INFO or above naming the pane in `pane_key`. This holds in the daemon's tick and in its shutdown flush. (§2.4, §4.2, §6.1, §6.3)
- [ ] A pane has a non-empty saved transcript. The dump captures it empty, and the tmux read sent after that capture is answered by a different server started on the same socket. The saved transcript is unchanged, and the refused write is logged naming the pane in `pane_key`. (§2.4)
- [ ] A pane has a non-empty saved transcript. The dump captures it empty, and the daemon's own server, the same server that answered the capture, answers the tmux read sent after it with exit status 0, whatever its output. The empty capture replaces the saved transcript. (§2.2, §2.4)
- [ ] The refused-write line uses an existing log component and existing attribute keys only. No new log component or attribute key is introduced. (§4.2)

**Do**:
- The dump is `scrollbackDump` in `cmd/state_daemon.go` (`run` and `dumpPane`), writing through `state.WriteScrollbackIfChanged` (`internal/state/scrollback.go`). `commit-now` passes no dump and dumps nothing (§2.4).
- Confirm an empty capture by the §2.2 rule: a tmux read sent strictly after that capture, answered by the daemon's own server (Task 1.3), the same server that answered the capture (§2.4).
- Log the refused write with the pane in `pane_key`, at INFO or above (§4.2).

**Context**:
> An empty capture may replace a saved non-empty transcript only once it is confirmed. An unconfirmed empty capture is not written, the saved transcript stands, and the refused write is logged (§2.4).
>
> `pane_key` is an attribute key Portal's closed log vocabulary already defines (`` rg -n '^\| `(session|pane_key|error)` \|' .workflows/portal-observability-layer/specification/portal-observability-layer/specification.md `` → 3 hits), so the line needs no new log component or attribute key (§4.2).

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2.2, §2.4, §4.2, §6.1, §6.3

## killing-all-sessions-wipes-restore-state-1-6

### Task 1.6: A cycle that ends uncommitted never leaves sessions.json naming a missing scrollback file

**Problem**: The capture cycle renames a newly waiting pane's transcript from its positional path to its token-named path (`refilePendingScrollback` in `internal/state/scrollback.go`). It does this expecting the cycle to commit the record that points at the new path. A cycle can end without that commit after the rename in three ways. It can stand down (Tasks 1.1 to 1.3). The daemon's tick can be cancelled mid-dump by the shutdown signal, and the shutdown flush that follows can itself stand down. Or its `sessions.json` write can fail. Each way leaves `sessions.json` still naming the vacated positional path. A later committing cycle would repair the record, because its re-file adopts the token-named file once the positional one is gone. At shutdown, though, no later cycle may commit, so the next restore would find no transcript at the path the record names.

**Solution**: However a cycle ends, it never leaves `sessions.json` naming a scrollback file that is not on disk. The guarantee covers every way a cycle can end uncommitted after the rename: a stand-down, a daemon tick cancelled mid-dump followed by a shutdown flush that stands down, and a failed `sessions.json` write.

**Outcome**: After any cycle that ends without committing, every scrollback path `sessions.json` names is present on disk. At the next restore, a waiting pane's transcript is at the path its record names.

**Acceptance Criteria**:
- [ ] A newly waiting pane's saved record names its positional scrollback path. A cycle moves the pane's transcript to its token-named path, and a stand-down is injected after the move. Afterwards every scrollback path `sessions.json` names is present on disk. This holds whichever committer runs the cycle. (§2.3, §6.1)
- [ ] A newly waiting pane's saved record names its positional scrollback path. The daemon's tick moves the pane's transcript to its token-named path and is then cancelled mid-dump by the shutdown signal. The shutdown flush that follows stands down. Afterwards every scrollback path `sessions.json` names is present on disk. (§2.3, §6.1)
- [ ] A newly waiting pane's saved record names its positional scrollback path. A cycle moves the pane's transcript to its token-named path, and its `sessions.json` write then fails. Afterwards every scrollback path `sessions.json` names is present on disk. (§2.3, §6.1)
- [ ] One of those cycles ends uncommitted at shutdown, and no later cycle commits. At the next restore, the path the waiting pane's record names holds that pane's transcript. (§2.3)

**Do**:
- The move is `refilePendingScrollback` (`internal/state/scrollback.go`), which `captureAndRefile` runs inside the shared commit cycle (`state.RunCommitCycle`, `internal/state/commit_cycle.go`) (§2.3).
- The cancelled tick is `errCycleCancelled` (`cmd/state_daemon.go`), returned after the renames and before the commit (§2.3). The failed write is `Commit`'s `sessions.json` write (`internal/state/commit.go`).

**Context**:
> How the guarantee is met is open. Confirming (§2.2) before any file is moved covers a stand-down, but not a cancellation or a failed write that comes after the move, so it is not enough on its own (§2.3).
>
> A later committing cycle repairs a record left naming the vacated positional path, because its re-file adopts the token-named file once the positional one is gone. At shutdown no later cycle may commit. The shutdown flush that would otherwise repair the record after a cancelled tick is itself a cycle that can now stand down (§2.3).
>
> A stand-down writes no commit and runs no housekeeping pass (§2.5).

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2, §2.3, §2.5, §6.1

## killing-all-sessions-wipes-restore-state-1-7

### Task 1.7: Kill path stays final under the hardened save path

**Problem**: Every kill's `commit-now` now has to pass the listing check and the confirmation before it commits, so the hardened save path sits directly on the kill path. A kill must stay final. The `session-closed` hook's synchronous `commit-now` removes the killed session's record and scrollback at that kill, and the hook-staleness sweep then reaps its resume hooks. Killing every session must still end with an empty restore state. That state is committed when the last user session closes while tmux keeps running for Portal's own `_portal-saver` and `_portal-bootstrap`. A regression here would quietly bring killed sessions back at the next restore.

**Solution**: Regression coverage of the kill path under the hardened save path. Live sessions are killed one at a time, and each removal, the empty end state and the reaping of the killed sessions' resume hooks are checked. The three tests that pin the empty-save contract stay green and unchanged.

**Outcome**: Killing sessions one at a time removes each one and its scrollback at its own kill. The last kill leaves zero sessions and zero scrollback files, and the next hook-staleness sweep reaps the killed sessions' resume hooks. The empty-save contract still holds.

**Acceptance Criteria**:
- [ ] A live runtime holds several user sessions, each with scrollback and a resume hook. The sessions are killed one at a time. After each kill, that session is gone from `sessions.json` and its scrollback files are deleted, committed at that kill. (§1.1, §5.1, §6.4)
- [ ] The last user session is killed while tmux keeps running for `_portal-saver` and `_portal-bootstrap`. `sessions.json` then holds zero sessions, and no scrollback file remains. (§1.1, §5.1, §6.4)
- [ ] The next hook-staleness sweep after the kills removes the killed sessions' resume hooks from `hooks.json`. (§1.1, §6.4)
- [ ] A capture whose session listing names only Portal's own sessions, and a capture in which every session vanishes mid-capture, each yield an empty index with no error. `commit-now` with zero live sessions writes a `sessions.json` holding zero sessions. The three tests listed in §5.1 that pin these pass with their assertions unchanged. (§5.1, §6.4)

**Do**:
- The `session-closed` hook still runs `commit-now` synchronously, and the kill path is unchanged (§5.1).
- Leave these tests as they are (§5.1): the "it returns an empty index with nil error when keep is empty after filtering" subtest of `TestCaptureStructurePreLoopFailFatal`, the "it proceeds with empty index when every session is natural churn" subtest of `TestCaptureStructurePerSessionLogAndContinue` (both `internal/state/capture_test.go`), and `TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions` (`cmd/state_commit_now_test.go`).

**Context**:
> A kill is intentional and names the session: the picker's kill (`k`, then `y`), `tmux kill-session` (including a key the user binds to it), or the user ending the session themselves by exiting its last program. A kill removes the session's record from `sessions.json`, deletes its scrollback files, and (through the hook-staleness sweep) removes its resume hooks. Killing every session is the same rule applied to each one, and it correctly ends with an empty restore state (§1.1).
>
> The prior `killed-session-resurrects-within-tick-window` specification made `session-closed` commit synchronously on every kill path. This fix keeps that rule unchanged (§7).
>
> A kill whose `commit-now` stands down is committed by the daemon's next tick, which Task 1.4 covers. A kill that no save commits before tmux exits is accepted residue (§5.2).

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.1, §5.1, §5.2, §6.4, §7

## killing-all-sessions-wipes-restore-state-1-8

### Task 1.8: Shutdown orderings against real tmux preserve the full saved state

**Problem**: The wipe depends on timing that only real tmux reproduces. In the sandbox, SIGTERMing the daemon 10–30ms before the tmux server wiped the whole saved state. Five 10ms trials at the production default log level all wiped it, while five with debug logging on all kept it, because debug logging shifts the timing. `tmux kill-server` survives today only because of which read happens to fail first. Unit tests with fakes cannot show that the fix holds against these orderings.

**Solution**: Integration-lane tests on real tmux, with an isolated socket, drive a live Portal runtime through the two shutdown orderings this phase must survive. In the first, the daemon is SIGTERMed 10–30ms before the server, repeated across many trials. In the second, `tmux kill-server` is run. Every trial runs at the production default log level with nothing wrapping `tmux`.

**Outcome**: On real tmux, both orderings leave the full saved state intact, at the timing that wiped it before the fix.

**Acceptance Criteria**:
- [ ] A live runtime holds saved sessions and their scrollback. The daemon is SIGTERMed, and the tmux server is SIGTERMed 10–30ms later. Across many trials, every trial ends with `sessions.json` naming every session and every scrollback file present and non-empty. (§2.2, §6.5)
- [ ] A live runtime holds saved sessions and their scrollback, and `tmux kill-server` is run. Afterwards `sessions.json` and every scrollback file are unchanged. (§2.2, §6.5)
- [ ] `tmux kill-server` is run while the daemon has a save in flight, repeated across trials so that the exit lands at different points in the save. Every trial leaves `sessions.json` and every scrollback file unchanged. (§1.2, §2.2, §6.5)
- [ ] Every trial runs at the production default log level, with nothing wrapping `tmux`. (§6.5)

**Do**:
- Run in the integration lane (`//go:build integration`), on real tmux with an isolated socket (§6.5).
- Run the trials at the production default log level, with nothing wrapping `tmux` (§6.5).

**Context**:
> Debug logging and a logging shim both shift the timing. In the sandbox, before the fix, five 10ms trials with debug logging on all kept the full state, while five at the default level all wiped it (§6.5).
>
> Under `tmux kill-server`, no committer can pass the confirmation, whichever of its reads the exit lands after, so survival no longer depends on which read happens to fail first (§2.2). Today it survives only because each save's first read, of the restore-in-progress marker, fails against an exiting server. A server that begins exiting a few milliseconds after that read gets no such protection (§1.2).
>
> The third ordering in §6.5 (the pane programs and the daemon signalled, then the server) depends on Phase 2's hardened panes and is covered there.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.2, §2.2, §6.5
