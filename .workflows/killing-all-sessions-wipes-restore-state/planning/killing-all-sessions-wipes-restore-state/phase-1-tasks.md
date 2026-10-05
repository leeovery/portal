# Phase 1: The save path never acts on a session view it cannot confirm — 8 tasks

## killing-all-sessions-wipes-restore-state-1-1

### Task 1.1: Failed session listing stands the commit cycle down

**Problem**: Every save's capture reads the session list through the one listing every reader shares, and that listing swallows a failed `list-sessions` and returns an empty list (`rg -n 'Swallowed deliberately' internal/tmux/tmux.go` → 1 hit, in `ListSessions`). When tmux begins exiting while a save is mid-capture, the save therefore sees zero sessions and commits an empty index. The housekeeping pass inside that commit then deletes every scrollback file, including those of sessions still alive. The daemon's SIGTERM flush is the save most likely to be in flight at a reboot, and a hook-run `commit-now` can hit the same window. The existing failure-path subtest only passes because its fake returns an error the production client never delivers.

**Solution**: A failed `list-sessions` becomes an error to the shared commit cycle that the daemon's tick, its shutdown flush and `portal state commit-now` all run, so the cycle stands down instead of reading the failure as zero sessions. The change lands on the committing path only. The shared listing keeps reading a failure as "no server, so no sessions" for the picker, the resolver, shell completion and restore. Changing it there would make one transient failure at bootstrap restore nothing, after which the daemon's next tick would truthfully commit an empty index over the full saved state.

**Outcome**: With the production tmux client, a failed `list-sessions` ends every committer's cycle with an error and nothing written, deleted or emptied. Every other reader of the session list, restore included, behaves exactly as it does today.

**Acceptance Criteria**:
- [ ] The saved state holds sessions and their scrollback files, and the production client's `list-sessions` fails while the shared commit cycle runs. The cycle returns an error, `sessions.json` is unchanged, and every scrollback file is still present with its content. (§2.1, §2.5, §6.1)
- [ ] With the same saved state and the restore-in-progress marker reading as unset, the production client's `list-sessions` fails during the daemon's tick, during its shutdown flush and during `commit-now`. In each case, `sessions.json` and every scrollback file are unchanged afterwards. (§2.1, §6.1)
- [ ] On a running server whose `list-sessions` answers with only Portal's own underscore-prefixed sessions, the capture still yields an empty index with no error. (§5.1)
- [ ] The session listing fails while bootstrap restores. Restore rebuilds every session in the saved state, exactly as it does today, and no commit of an empty index follows it. (§2.1, §5.1, §6.1)
- [ ] With the session listing failing, the picker lists no sessions, the resolver finds no exact session match, and shell completion offers no session names, each without reporting an error, exactly as today. (§2.1, §5.1)

**Do**:
- Leave the shared listing on `tmux.Client` (`ListSessions`, and `ListSessionNames` that reads through it, in `internal/tmux/tmux.go`) swallowing its failure. The change lands on the committing path only (§2.1).
- `ListSessionsProbe` (`internal/tmux/tmux.go`) is the existing variant of the listing that returns the failure instead. Capture's listing read is in `captureStructure` (`internal/state/capture.go`), which does not use it today (`rg -c 'ListSessionsProbe' internal/state` → no matches).
- Restore keeps its current listing read (`snapshotLiveSessions` in `internal/restore/restore.go`, which reads the same listing as capture).
- Revisit the "returns an error when ListSessionNames fails and does not call show-environment" subtest of `TestCaptureStructurePreLoopFailFatal` (`internal/state/capture_test.go`). It must exercise the listing the committing path actually uses, through the production client, instead of a fake that returns an error the production client never delivers (§6.6).
- Keep the "returns empty slice when tmux server is not running" case of `TestListSessions` (`internal/tmux/tmux_test.go`) for the picker. It must no longer describe the save path (§6.6).
- Leave unchanged the "returns an empty index with nil error when keep is empty after filtering" subtest of `TestCaptureStructurePreLoopFailFatal`, the "proceeds with empty index when every session is natural churn" subtest of `TestCaptureStructurePerSessionLogAndContinue` (both `internal/state/capture_test.go`), and `TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions` (`cmd/state_commit_now_test.go`) (§5.1).

**Context**:
> All three committers run one shared commit cycle (`state.RunCommitCycle`, `internal/state/commit_cycle.go`): capture, the move of waiting panes' transcripts to their token-named paths, the daemon's scrollback dump, then the commit and its housekeeping pass (`Commit` / `gcOrphanScrollback`, `internal/state/commit.go`). Everything in spec §2 applies to that cycle, and so to all three committers alike.
>
> A stand-down (§2.5) writes no commit and runs no housekeeping pass. The cycle ends as a failed cycle through each committer's existing failure route. The committers already route a cycle error there. Reporting the stand-down with a back-off line is Task 1.4.
>
> Restore already skips every session when its listing returns an error (`rg -n 'list-sessions failed' internal/restore/restore.go` → 1 hit). That is why the failure must not surface through the shared listing: restore would then bring back nothing, and the next tick would commit the empty result over the full saved state.
>
> The spec records "no server running means zero sessions" as the picker's session-discovery contract, defined by the prior `v1` specification. The picker, the resolver, completion and restore keep that reading (§7).

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.2, §2, §2.1, §2.5, §5.1, §6.1, §6.6, §7

## killing-all-sessions-wipes-restore-state-1-2

### Task 1.2: A refused confirmation read stands the commit cycle down

**Problem**: A failed listing is only one of the answers tmux gives while it shuts down. Once it begins exiting during a read, tmux can answer an in-flight `list-sessions` or `list-panes` with exit status 0 and no output, and no error check can see that. An empty `list-sessions` commits an empty index. An empty `list-panes` next to an environment read that still succeeds records a session with no windows, which deletes the session's scrollback files while its name stays in `sessions.json`. `tmux kill-server` survives today only by accident: each save's first read is the restore-in-progress marker, and against an exiting server that read fails and is taken to mean "a restore is in progress, do not commit". A server that begins exiting a few milliseconds after that first read gets no such protection.

**Solution**: Before the shared commit cycle writes a commit, it confirms that tmux is still answering. It does this with a tmux read sent strictly after the last read the captured index is built from. The confirmation counts as answered when tmux returns exit status 0, whatever the output. A refused confirmation stands the cycle down: no commit and no housekeeping pass.

**Outcome**: Every commit any of the three committers writes rests on proof that every capture read was answered before tmux began exiting. A capture holding a shutdown answer is never committed, and a genuinely empty session list, such as the one left when the last user session is killed, still commits.

**Acceptance Criteria**:
- [ ] The saved state holds sessions and their scrollback files. A cycle's capture reads are all answered, then its confirmation read is refused. The cycle returns an error, `sessions.json` is unchanged, and every scrollback file is still present with its content. This holds whether the cycle is the daemon's tick, its shutdown flush or `commit-now`. (§2.2, §2.5, §6.1)
- [ ] A capture's `list-sessions` comes back with exit status 0 and no output from a server that then refuses connections. The cycle writes nothing and deletes no scrollback file. (§2.2, §6.1)
- [ ] A capture's `list-panes` comes back with exit status 0 and no output, the environment reads after it still succeed, and the server then refuses connections. No session is recorded without its windows, `sessions.json` is unchanged, and no scrollback file is deleted. (§2.2, §6.1)
- [ ] The confirmation read is sent strictly after the last capture read and comes back with exit status 0 and no output. The cycle writes its commit. (§2.2, §6.1)
- [ ] The last user session is killed on a server that keeps running for `_portal-saver` and `_portal-bootstrap`, and the confirmation is answered. The cycle commits the empty index, and the housekeeping pass removes the killed session's scrollback files. (§5.1)
- [ ] In every cycle that writes a commit, the confirmation read is sent after every read the captured index is built from: the session listing, the pane listing and each session's environment read. (§2.2)

**Do**:
- The confirmation belongs to the shared commit cycle every committer runs (`state.RunCommitCycle`, `internal/state/commit_cycle.go`), so it applies to the daemon's tick, its shutdown flush and `commit-now` alike (§2).
- A confirmation counts as answered when tmux returns exit status 0, whatever the output. Anything else is refused (§2.2).
- A refused confirmation writes no commit and runs no housekeeping pass (`Commit` / `gcOrphanScrollback`, `internal/state/commit.go`). The cycle ends as a failed cycle (§2.5).

**Context**:
> The rule rests on two properties of tmux 3.7c, taken from its source and borne out in the sandbox. Once the server begins exiting, it refuses every new client connection (`server.c`, `server_accept`). Once it has started exiting, it never stops (`server_exit` is set once, in the SIGTERM handler, before any session is destroyed). So an answered confirmation proves that every capture read before it was answered before the exit began. Even tmux's own shutdown answer to the confirmation (exit 0, no output) proves it: any client tmux accepted at all was accepted before the exit began.
>
> A `list-sessions` or `list-panes` that returned exit 0 with no output because tmux began exiting during the read was answered after the exit began, so the confirmation sent after it is refused. Under `tmux kill-server`, no committer can pass the confirmation, whichever of its reads the exit lands after. Survival under `kill-server` then no longer depends on which read happens to fail first.
>
> Where the confirmation sits relative to the move of waiting panes' transcripts (`refilePendingScrollback`, `internal/state/scrollback.go`) is open. §2.3 accepts confirming before any file is moved, or a stand-down after a move that leaves the saved state naming only files that exist. Task 1.6 owns that guarantee.
>
> The rule that the confirmation counts only from the committer's own server is Task 1.3. Each committer's back-off line is Task 1.4. The dump's handling of an empty `capture-pane` is Task 1.5.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.2, §2, §2.2, §2.3, §2.5, §5.1, §6.1

## killing-all-sessions-wipes-restore-state-1-3

### Task 1.3: The confirmation counts only from the committer's own server

**Problem**: An answered confirmation proves nothing unless the server that answered it is the one the save belongs to. After a tmux server exits, a new one can be started on the same socket before its restore has run, and that server holds none of the user's sessions. A save whose capture reads or confirmation reach it would see an empty or partial session list, pass the confirmation and commit that view over the full saved state. The daemon's own case needs the opposite answer. When `portal uninstall` kills `_portal-saver` on a running server, the daemon's pane is gone but its server is not, and the shutdown flush that kill triggers must still commit.

**Solution**: The confirmation counts only when the tmux server the committer belongs to answers it, and only when that same server answered every capture read the commit is built from. For the daemon, that is the server its `_portal-saver` pane runs in, and it stays the daemon's own server after that pane is destroyed. For `commit-now`, it is the server whose `session-closed` hook ran it. A save whose reads or confirmation reach any other server stands down.

**Outcome**: No committer commits a view captured or confirmed by a different server started on the same socket, and the daemon's flush after `portal uninstall` on a running server still commits.

**Acceptance Criteria**:
- [ ] The daemon runs in `_portal-saver` on a server holding the user's sessions, and `portal uninstall` kills `_portal-saver` while the server keeps running. The daemon's shutdown flush commits, and its `shutdown` line reports `flush_completed=true`. (§2.2, §6.1)
- [ ] The committer's server exits and a new server is started on the same socket before its restore has run. The save's capture reads and its confirmation are both answered by the new server. Nothing is written: `sessions.json` and every scrollback file are unchanged. (§2.2, §6.1)
- [ ] A save's capture reads are answered by its own server, which then exits, and its confirmation is answered by a new server started on the same socket. Nothing is written: `sessions.json` and every scrollback file are unchanged. (§2.2, §6.1)
- [ ] A `commit-now` run by a server's `session-closed` hook, with that server answering its capture reads and its confirmation, commits as before. (§2.2)
- [ ] A `commit-now` run by a server's `session-closed` hook whose reads reach a different server, started on the same socket after the hook's server exited, writes nothing. (§2.2)
- [ ] The daemon's tick, with its own server answering every capture read and the confirmation, commits as before. (§2.2)

**Do**:
- The daemon's own server is the server its `_portal-saver` pane runs in. It stays the daemon's own server after that pane is destroyed (§2.2). The daemon's committers are `tick` and `defaultShutdownFlush` in `cmd/state_daemon.go`.
- `commit-now`'s own server is the server whose `session-closed` hook ran it (§2.2). The command is `cmd/state_commit_now.go`, and the hook runs it as `run-shell` (`commitNowCommand`, `internal/tmux/hooks_register.go`).
- A confirmation answered by any other server, or one sent after capture reads another server answered, counts as refused and stands the cycle down (§2.2, §2.5).

**Context**:
> The confirmation counts only when it is answered by the tmux server the committer belongs to, and only when that same server answered every capture read the commit is built from. A server started on the same socket after the committer's own server exited, before its restore has run, holds none of the user's sessions, so its answers confirm nothing (§2.2).
>
> `portal uninstall` kills `_portal-saver`, which delivers SIGHUP to the daemon and triggers its shutdown flush (`cmd/uninstall.go`). That flush runs against a server that is still running, so it must still commit.
>
> This task narrows the confirmation Task 1.2 introduced. Reporting each stand-down through the committer's failure route is Task 1.4. The dump's confirmation of an empty capture (Task 1.5) uses the same own-server rule.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2.2, §2.5, §6.1

## killing-all-sessions-wipes-restore-state-1-4

### Task 1.4: Each committer reports a stand-down through its existing failure route with a back-off line

**Problem**: Today a wipe is silent at the default log level. The only trace is `capture: tick complete sessions=0` or bare `process: start … state commit-now` lines. Once the cycle stands down on a failed listing or a refused confirmation (Tasks 1.1 to 1.3), each committer must end it as a failed cycle through the route it already has. Otherwise a stood-down save is never retried, and nothing at the default level records that a save backed off because tmux stopped answering. The kill path depends on that retry too. A kill whose `commit-now` stands down is committed only by the daemon's next tick.

**Solution**: Each committer reports a stand-down through its existing failure route and logs a line, at INFO or above, saying it backed off because tmux stopped answering, with the cause in `error`. The daemon's tick logs the failure and re-touches `save.requested`, so its next tick retries. `commit-now` logs the failure, touches `save.requested` and exits non-zero. The shutdown flush logs the failure and reports `flush_completed=false`. The lines each committer already writes for a failed restore-marker read stay exactly as they are.

**Outcome**: Every stand-down leaves a default-level line saying the save backed off because tmux stopped answering. The daemon's next tick commits whatever a stood-down save did not, including a kill whose `commit-now` stood down, and saves never stall.

**Acceptance Criteria**:
- [ ] The daemon's tick stands down because its session listing fails, and again because its confirmation is refused. Each time, the log holds a line at INFO or above saying the save backed off because tmux stopped answering, with the cause in `error`. `save.requested` is present afterwards. (§2.5, §4.2, §6.3)
- [ ] One tick stands down on a transient listing failure on a healthy server, and the next tick's reads are all answered. That next tick commits, so the save is delayed by one tick. (§2.5)
- [ ] `commit-now` stands down because its session listing fails, and again because its confirmation is refused. Each time, the log holds a line at INFO or above saying the save backed off because tmux stopped answering, with the cause in `error`. `save.requested` is touched and the command exits non-zero. (§2.5, §4.2, §6.3)
- [ ] A live session is killed and the `commit-now` its `session-closed` hook runs stands down. The daemon's next tick, with tmux answering, removes the killed session from `sessions.json` and deletes its scrollback files. (§2.5, §5.1, §5.2)
- [ ] The daemon's shutdown flush stands down because its session listing fails, and again because its confirmation is refused. Each time, the log holds a line at INFO or above saying the save backed off because tmux stopped answering, with the cause in `error`, and the `shutdown` line reports `flush_completed=false`. (§2.5, §4.2, §6.3)
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
> A save whose first read, of the restore-in-progress marker, fails has backed off too. All three committers already log that at WARN with the cause in `error`, and those lines stay. A marker that reads as set means a restore is in progress, which is not a back-off (§4.2).
>
> After the fix, back-off lines in a reboot's log mean a save was in flight as tmux went down. A teardown in which everything is hard-killed runs no committer and leaves no line at all (§4.3).
>
> The dump's refused-empty-write line is Task 1.5. The per-session drop line is Phase 2.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2.5, §4, §4.2, §4.3, §5.1, §5.2, §6.3

## killing-all-sessions-wipes-restore-state-1-5

### Task 1.5: The daemon's dump never writes an unconfirmed empty capture over a saved transcript

**Problem**: The daemon's scrollback dump runs in its tick and its shutdown flush, and writes a pane's scrollback read whenever it differs from the saved file (`WriteScrollbackIfChanged`, called from the dump in `cmd/state_daemon.go`). So an empty read overwrites a saved transcript with zero bytes. That loss involves no housekeeping pass and no change to the session list, so neither the stand-down nor the confirmation of the commit prevents it. The sandbox never saw tmux's shutdown answer (exit 0, no output) on `capture-pane`, but `capture-pane` goes through the same client path as the listings that did show it.

**Solution**: An empty capture may replace a saved non-empty transcript only once the §2.2 rule confirms it: a tmux read sent strictly after that capture has been answered by the server the daemon belongs to, the same server that answered the capture. An unconfirmed empty capture is not written. The saved transcript stands, and the refused write is logged with the pane named in `pane_key`.

**Outcome**: No tick or shutdown flush empties a saved transcript on a `capture-pane` answer it cannot confirm, every refusal leaves a default-level line naming the pane, and a confirmed empty capture is still written.

**Acceptance Criteria**:
- [ ] A pane has a non-empty saved transcript. The daemon's dump captures it empty, and the tmux read sent after that capture is refused. The saved transcript is byte-for-byte unchanged, and the log holds a line at INFO or above naming the pane in `pane_key`. This holds in the daemon's tick and in its shutdown flush. (§2.4, §4.2, §6.1, §6.3)
- [ ] A pane has a non-empty saved transcript. The dump captures it empty, and the tmux read sent after that capture is answered by a different server started on the same socket. The saved transcript is unchanged, and the refused write is logged naming the pane in `pane_key`. (§2.4)
- [ ] A pane has a non-empty saved transcript. The dump captures it empty, and the daemon's own server, the same server that answered the capture, answers the tmux read sent after it. The empty capture replaces the saved transcript. (§2.4)
- [ ] The refused-write line uses an existing log component and existing attribute keys only. No new log component or attribute key is introduced. (§4.2)

**Do**:
- The dump is `scrollbackDump` in `cmd/state_daemon.go` (`dumpPane`, writing through `state.WriteScrollbackIfChanged` in `internal/state/scrollback.go`). `commit-now` passes no dump and dumps nothing (§2.4).
- Confirm an empty capture by the §2.2 rule: a tmux read sent strictly after that capture, answered by the daemon's own server (Task 1.3), the same server that answered the capture (§2.4).
- Log the refused write with the pane in `pane_key`, at INFO or above (§4.2).

**Context**:
> An empty capture may replace a saved non-empty transcript only once it is confirmed. An unconfirmed empty capture is not written, the saved transcript stands, and the refused write is logged (§2.4).
>
> `pane_key` is an attribute key Portal's closed log vocabulary already defines (`` rg -n '^\| `(session|pane_key|error)` \|' .workflows/portal-observability-layer/specification/portal-observability-layer/specification.md `` → 3 hits), so the line needs no new log component or attribute key (§4.2).

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2.2, §2.4, §4.2, §6.1, §6.3

## killing-all-sessions-wipes-restore-state-1-6

### Task 1.6: A stand-down never leaves sessions.json naming a missing scrollback file

**Problem**: The capture cycle renames a newly waiting pane's transcript from its positional path to its token-named path (`refilePendingScrollback` in `internal/state/scrollback.go`). It does this expecting the same cycle to commit the record that points at the new path. Now that a cycle can stand down (Tasks 1.1 to 1.3), a cycle that backs off after that rename would leave `sessions.json` still naming the vacated positional path. At shutdown no later cycle runs to repair it, so the next restore would find no transcript at the path the record names.

**Solution**: One of the two orderings the spec accepts. Either the confirmation comes before any file is moved, or a stand-down after a move leaves the saved state naming only files that exist. Whichever is used, no stand-down leaves `sessions.json` naming a scrollback file that is not on disk.

**Outcome**: After any stand-down, every scrollback path `sessions.json` names is present on disk, so a waiting pane's transcript is where its record says at the next restore.

**Acceptance Criteria**:
- [ ] A waiting pane's saved record names its positional scrollback path. A cycle moves the pane's transcript to its token-named path, and a stand-down is injected after the move. Afterwards every scrollback path `sessions.json` names is present on disk. This holds whichever committer runs the cycle. (§2.3, §6.1)
- [ ] A newly waiting pane's saved record names its positional scrollback path, and the daemon's shutdown flush stands down with no later cycle to follow. Every scrollback path `sessions.json` names is present on disk, and at the next restore the path the pane's record names holds its transcript. (§2.3)

**Do**:
- The move is `refilePendingScrollback` (`internal/state/scrollback.go`), which `captureAndRefile` runs inside the shared commit cycle (`state.RunCommitCycle`, `internal/state/commit_cycle.go`).
- Either ordering is acceptable: the confirmation (§2.2) comes before any file is moved, or a stand-down after a move leaves the saved state naming only files that exist (§2.3).

**Context**:
> The capture cycle renames a newly waiting pane's transcript expecting the cycle to commit the record that points at the new path. If a cycle backed off after that rename, `sessions.json` would still name the vacated positional path. At shutdown no later cycle runs to repair it, so the next restore would find no transcript at the path the record names (§2.3).
>
> A stand-down writes no commit and runs no housekeeping pass (§2.5), so a stand-down cannot point the record at the new path itself.
>
> Task 1.2 places the confirmation in the cycle without fixing its position relative to the move. This task makes the guarantee hold whichever position it took.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §2, §2.3, §2.5, §6.1

## killing-all-sessions-wipes-restore-state-1-7

### Task 1.7: Kill path stays final under the hardened save path

**Problem**: Every kill's `commit-now` now has to pass the new listing check and the confirmation before it commits, so the hardened save path sits directly on the kill path. A kill must stay final. The `session-closed` hook's synchronous `commit-now` removes the killed session's record and scrollback at that kill, and the hook-staleness sweep then reaps its resume hooks. Killing every session must still end with an empty restore state, committed when the last user session closes while tmux keeps running for Portal's own `_portal-saver` and `_portal-bootstrap`. A regression here would quietly bring killed sessions back at the next restore.

**Solution**: Regression coverage of the kill path under the hardened save path. Live sessions are killed one at a time on a live runtime, and each removal, the empty end state and the reaping of the resume hooks are checked. The three tests pinning the empty-save contract stay green and unchanged.

**Outcome**: Killing sessions one at a time removes each one and its scrollback at its own kill, the last kill leaves zero sessions and zero scrollback files, the next hook-staleness sweep reaps the killed sessions' resume hooks, and the empty-save contract still holds.

**Acceptance Criteria**:
- [ ] A live runtime holds several user sessions, each with scrollback and a resume hook. The sessions are killed one at a time. After each kill, that session is gone from `sessions.json` and its scrollback files are deleted, committed at that kill. (§1.1, §5.1, §6.4)
- [ ] The last user session is killed while tmux keeps running for `_portal-saver` and `_portal-bootstrap`. `sessions.json` then holds zero sessions and the scrollback directory holds zero scrollback files. (§1.1, §5.1, §6.4)
- [ ] The next hook-staleness sweep after the kills removes the killed sessions' resume hooks from `hooks.json`. (§1.1, §6.4)
- [ ] A capture whose session listing answers with only Portal's own sessions, and one whose every session vanishes mid-capture, each yield an empty index with no error. `commit-now` with zero live sessions writes a `sessions.json` holding zero sessions. The tests pinning these pass with their assertions unchanged. (§5.1, §6.4)

**Do**:
- The `session-closed` hook still runs `commit-now` synchronously, and the kill path is unchanged (§5.1).
- Leave these tests as they are (§5.1): the "returns an empty index with nil error when keep is empty after filtering" subtest of `TestCaptureStructurePreLoopFailFatal`, the "proceeds with empty index when every session is natural churn" subtest of `TestCaptureStructurePerSessionLogAndContinue` (both `internal/state/capture_test.go`), and `TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions` (`cmd/state_commit_now_test.go`).

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

**Solution**: Integration-lane tests on real tmux, with an isolated socket, drive a live Portal runtime through the two shutdown orderings this phase must survive. In the first, the daemon is SIGTERMed 10–30ms before the server, repeated across many trials. In the second, `tmux kill-server` is run. The trials run at the production default log level with nothing wrapping `tmux`.

**Outcome**: On real tmux, both orderings leave the full saved state intact at the timing that wiped it before the fix.

**Acceptance Criteria**:
- [ ] A live runtime holds saved sessions and their scrollback. The daemon is sent SIGTERM, and the tmux server is sent SIGTERM 10–30ms later. Across many trials, every trial ends with `sessions.json` naming every session and every scrollback file present and non-empty. (§2.2, §6.5)
- [ ] A live runtime holds saved sessions and their scrollback, and `tmux kill-server` is run. Afterwards `sessions.json` and every scrollback file are unchanged. (§2.2, §6.5)
- [ ] `tmux kill-server` is run while a save is in flight, after that save's restore-in-progress marker read has been answered. Afterwards `sessions.json` and every scrollback file are unchanged. (§1.2, §2.2, §6.5)
- [ ] Every trial runs at the production default log level, with nothing wrapping `tmux`. (§6.5)

**Do**:
- Run in the integration lane (`//go:build integration`), on real tmux with an isolated socket (§6.5).
- Run the trials at the production default log level, with nothing wrapping `tmux`. Debug logging and a logging shim both shift the timing (§6.5).

**Context**:
> In the sandbox, five 10ms trials with debug logging on all kept the full state, while five at the default level all wiped it. Before the fix, this window wiped the whole state (§6.5).
>
> Under `tmux kill-server`, no committer can pass the confirmation, whichever of its reads the exit lands after. Survival under `kill-server` no longer depends on which read happens to fail first (§2.2). Today it survives only because each save's first read, of the restore-in-progress marker, fails against an exiting server. A server that begins exiting a few milliseconds after that read gets no such protection (§1.2).
>
> The test signals only the daemon and the tmux server it started itself (CLAUDE.md's process-isolation invariant).
>
> The third ordering in §6.5 (the pane programs and the daemon signalled, then the server) depends on Phase 2's hardened panes and is covered there.

**Spec Reference**: `.workflows/killing-all-sessions-wipes-restore-state/specification/killing-all-sessions-wipes-restore-state/specification.md` — §1.2, §2.2, §6.5
