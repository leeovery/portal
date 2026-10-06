# Specification: Killing All Sessions Wipes Restore State

## Specification

### 1. Problem and Governing Rule

#### 1.1 The rule

A session the user kills never comes back. A session that stops any other way does come back. The cases this fix still leaves short of the rule are accepted residue, recorded in §5.2.

- **Killing is final.** A kill is intentional and names the session: the picker's kill (`k`, then `y`), `tmux kill-session` (including a key the user binds to it), or the user ending the session themselves by exiting its last program. A kill removes the session's record from `sessions.json`, deletes its scrollback files, and (through the hook-staleness sweep) removes its resume hooks. Killing every session is the same rule applied to each one, and it correctly ends with an empty restore state.
- **Everything else is restorable.** Detaching, `tmux kill-server` and a reboot of the Mac all leave every session restorable, with `sessions.json`, its scrollback files and its resume hooks intact. No Portal shutdown step is needed first. The user reboots and kills tmux normally and never has to shut Portal down beforehand.

#### 1.2 The defect

Every save treats the session list tmux reports at that instant as complete and authoritative, and acts on it irreversibly in the same step. A session the capture does not see is dropped from `sessions.json`. The housekeeping pass inside every commit then deletes every scrollback file the new index no longer names. A pane whose scrollback read comes back empty has its saved file overwritten with zero bytes. Nothing in the pipeline tells the user ending a session apart from tmux or the machine going away. So sessions that end without a kill, or only appear to end, are removed as finally as killed ones. Two ordinary shutdown situations hand a save that kind of view:

1. **tmux begins exiting while a save is mid-capture.** The session listing then reads as zero sessions, in two ways. A failed `list-sessions` is swallowed and returned as an empty list. And tmux itself, while shutting down, can answer an in-flight `list-sessions` or `list-panes` with exit status 0 and no output. The save's earlier reads have already succeeded, so nothing stands it down. It commits an empty index and deletes every scrollback file, including those of sessions still alive. At reboot, the save most likely to be in flight is the daemon's final flush, which SIGTERM triggers. A hook-run `commit-now` can hit the same window.
2. **Sessions die before tmux does.** The shutdown SIGTERM reaches the programs in panes independently of the tmux server. A session whose programs exit on it closes while the server still answers. Two saves then remove that session and its scrollback: the `commit-now` that the `session-closed` hook runs synchronously, and the daemon's SIGTERM flush. Interactive shells ignore SIGTERM. Portal's own restored resume-hook panes, eager and lazy, do not. So the sessions most exposed are the ones carrying resume hooks.

After either situation, the hook-staleness sweep on the next start deletes the resume hook of every pane that did not come back. The user loses the resume commands of sessions they never killed. The scrollback loss is the irreversible part: `sessions.json` can be rebuilt by reopening sessions, but deleted scrollback files cannot.

`tmux kill-server` survives today only by accident. Each save's first read is the restore-in-progress marker. Against an exiting server that read fails, and the failure is taken to mean "a restore is in progress, do not commit". A server that begins exiting a few milliseconds after that first read gets no such protection.

#### 1.3 Scope

The fix has three parts:

- the save path stops trusting a session list it cannot confirm (§2);
- Portal's own panes outlast the shutdown signal (§3);
- every dropped session and every save that backs off is logged (§4).

The kill path is unchanged, and the accepted leftovers are recorded in §5.

### 2. The Save Path Stops Trusting an Unconfirmed Session List

Three committers all run one shared commit cycle:

- the daemon's tick;
- the daemon's shutdown flush, on SIGHUP or SIGTERM;
- `portal state commit-now`, which the `session-closed` hook runs synchronously.

The cycle runs in this order: capture, move waiting panes' transcripts to their token-named paths, the daemon's scrollback dump, then the commit and its housekeeping pass. Everything in this section applies to that cycle, so it applies to all three committers alike.

#### 2.1 A failed session listing stops the cycle

A failed `list-sessions` is an error to the commit cycle. The cycle stands down (§2.5) instead of reading the failure as zero sessions.

Today every reader shares one session listing that swallows the failure and returns an empty list (`rg -n 'Swallowed deliberately' internal/tmux/tmux.go` → 1 hit, in `ListSessions`), and capture reads through it. A variant that returns the failure instead, `ListSessionsProbe`, already exists. Capture does not use it (`rg -c 'ListSessionsProbe' internal/state` → no matches).

The change lands on the committing path only, never in the shared listing method. The picker, the resolver, shell completion and restore keep reading a failed listing as "no server, so no sessions". Restore matters most:

- It reads the same listing as capture (`rg -n 'ListSessionNames\(\)' internal/state/capture.go internal/restore/restore.go` → `capture.go:88`, `restore.go:99`).
- It already skips every session when that listing returns an error (`rg -n 'list-sessions failed' internal/restore/restore.go` → 1 hit, in `snapshotLiveSessions`).

If the shared listing began returning its failures, one transient failure at bootstrap would make restore bring back nothing. The daemon's next tick would then truthfully commit an empty index over the full saved state. That is the same wipe, reached without anyone killing anything.

#### 2.2 tmux is confirmed still answering before a commit is written

Before the cycle writes a commit, it confirms that tmux is still answering. It does this with a tmux read sent strictly after the last read the captured index is built from. The rule rests on two properties of tmux 3.7c, taken from its source and borne out in the sandbox:

- once the server begins exiting, it refuses every new client connection (`server.c`, `server_accept`);
- once it has started exiting, it never stops (`server_exit` is set once, in the SIGTERM handler, before any session is destroyed).

So an answered confirmation proves that every capture read before it was answered before the exit began. A refused confirmation stands the cycle down (§2.5).

The confirmation counts as answered only when tmux returns exit status 0 with an answer that names the server that answered it. Any client tmux accepted at all was accepted before the exit began, so an answer from the committer's own server proves that every earlier read was answered in full. tmux's own shutdown answer to the confirmation (exit 0, no output) names no server. It cannot meet the own-server rule below, so it stands the cycle down.

The confirmation also catches the shutdown answers that no error check can see: a `list-sessions` or `list-panes` that returned exit 0 with no output because tmux began exiting during the read. A read like that was answered after the exit began, so the confirmation sent after it is refused. Without this check, the bad answers do damage in two ways:

- an empty `list-sessions` commits an empty index;
- an empty `list-panes` next to an environment read that still succeeds records a session with no windows. That deletes the session's scrollback files while its name stays in `sessions.json`.

Under `tmux kill-server`, no committer can pass the confirmation, whichever of its reads the exit lands after. Survival under `kill-server` no longer depends on which read happens to fail first (§1.2).

The confirmation counts only when it is answered by the tmux server the committer belongs to, and only when that same server answered every capture read the commit is built from. For the daemon, that is the server its `_portal-saver` pane runs in. It stays the daemon's own server after that pane is destroyed, so the save that runs when `portal uninstall` kills `_portal-saver` on a running server still commits. For `commit-now`, it is the server whose `session-closed` hook ran it. After that server exits, a new one can be started on the same socket before its restore has run, and it holds none of the user's sessions. Its answers confirm nothing, and a save that reaches it stands down (§2.5).

#### 2.3 A cycle that ends uncommitted never leaves the saved state naming a missing file

The capture cycle re-files a newly waiting pane's transcript from its positional path to its token-named path (`refilePendingScrollback` in `internal/state/scrollback.go`). The re-file is a hard link (`link(2)`) that never overwrites an existing token-named file: the positional name stays on disk until the housekeeping pass of a commit naming the token-named path removes it. So a cycle that ends without that commit leaves `sessions.json` naming the positional path, and the file is still there. A later committing cycle files the pane under its token-named path alone.

A cycle can end without committing after the re-file in three ways:

- it stands down (§2.5);
- the daemon's tick is cancelled mid-dump by the shutdown signal (`errCycleCancelled` in `cmd/state_daemon.go`, returned after the re-file and before the commit), and the shutdown flush that follows can itself stand down;
- its `sessions.json` write fails.

However a cycle ends, it never leaves `sessions.json` naming a scrollback file that is not on disk. Confirming (§2.2) before the re-file covers a stand-down, but not a cancellation or a failed write that comes after it; leaving the positional name in place until a commit's housekeeping pass covers all three.

#### 2.4 The scrollback dump never zeroes a saved transcript on an unconfirmed read

The daemon's scrollback dump runs in its tick and its shutdown flush; `commit-now` dumps nothing. The dump writes a pane's scrollback read whenever it differs from the saved file (`WriteScrollbackIfChanged`, reached from the dump in `cmd/state_daemon.go` only through the scrollback writer the commit cycle hands it). So an empty read overwrites a saved transcript with zero bytes. That loss involves no housekeeping pass and no change to the session list.

The sandbox never saw tmux's shutdown answer (exit 0, no output) on `capture-pane`. But `capture-pane` goes through the same client path as the listings that did show it.

An empty capture may replace a saved non-empty transcript only once it is confirmed by the rule in §2.2: a tmux read sent strictly after that capture has been answered by the server the committer belongs to, the same server that answered the capture. An unconfirmed empty capture is not written. The saved transcript stands, and the refused write is logged (§4).

A pane's saved transcript is the file its last committed record names. For a lazy pane that has just been answered, that is its token-named file, not its positional path. From the answer until a dump writes the pane's new capture with confirmed bytes, every commit names the token-named transcript and leaves it on disk, whether the pane's capture comes back empty, is refused, or no dump runs at all (`commit-now`). Only the housekeeping pass of a commit naming a positional file that holds the pane's capture removes the token-named file, short of the pane being killed.

The same holds for a pane now at a different address from the one its last committed record names: a pane restore brought back one window lower because its session's window indices had a gap, and a pane carrying a token that tmux renumbered or whose session was renamed. Until a dump writes such a pane's capture with confirmed bytes, every commit names the file its last committed record names and leaves it on disk. Every pane restore recreates carries an identity tying it to the record it was built from, so this holds for a restored pane whether or not it carries a resume hook.

#### 2.5 What a stand-down does

A cycle stands down on a failed session listing (§2.1) or a refused confirmation (§2.2). When it does, it writes no commit and runs no housekeeping pass. So `sessions.json` stays as it was, no scrollback file is deleted or emptied, and the saved state names only files that exist (§2.3). The cycle then ends as a failed cycle, through each committer's existing failure route:

- the daemon's tick logs its failure and re-touches `save.requested` (`rg -n 'TouchSaveRequested' cmd/state_daemon.go` → 1 hit, after `tick failed`), so its next tick retries;
- `commit-now` logs its failure, touches `save.requested` and exits non-zero (`failCommitNow`);
- the shutdown flush logs its failure and reports `flush_completed=false`.

A transient failure on a healthy server therefore delays a save by one tick, and saves never stall. tmux's `run-shell` can surface `commit-now`'s non-zero exit to an attached client. More WARN and ERROR lines at teardown are expected and accepted.

### 3. Portal's Own Panes Outlast the Shutdown Signal

#### 3.1 The restored resume-hook panes survive SIGTERM

Portal starts two kinds of pane as a non-interactive shell, and today SIGTERM kills both:

- the eager resume pane, `sh -c '<hook>; exec <shell>'` (`hookExecArgs` in `cmd/state_resume_chain.go`);
- the lazy waiting pane's parked chain, `sh -c 'trap : INT QUIT; <draw>; <recover>; <backstop>'` (`parkedChainTrap` / `execResumeChainAndExit` in `cmd/state_hydrate.go`). Its trap does not cover TERM.

Both now survive SIGTERM, the way an interactive shell already does. Their sessions then stay up until tmux itself exits, and by then no committer can reach the server (§2.2). Lazy is the shipped default resume mode, so after a restore every unanswered pane runs as the parked chain.

A lazy pane the user has answered runs its hook through that same eager shell, `sh -c '<hook>; exec <shell>'`, beneath its parked chain. That shell survives SIGTERM there too. So an answered pane whose hook program is still running keeps its session, and the pane goes on to the user's shell when the hook program ends.

The handling must be a caught trap, never an ignored disposition. That is already the parked chain's rule: an ignored signal stays ignored across `exec`, so the hook program and the user's shell would inherit it. The hook program and the user's shell keep default SIGTERM handling.

These two are the only non-interactive-shell panes Portal creates (`rg -n '"sh", "-c"' --type go -g '!*_test.go' cmd internal | wc -l` → 2). Session trees run `$SHELL -ic`, which is interactive (`BuildShellCommand` in `internal/session/create.go`). Every other pane process Portal starts is either not a shell (the hydrate helper, the saver's daemon) or an interactive shell (`_portal-bootstrap`, created with no command by `StartServer` in `internal/tmux/tmux.go`).

#### 3.2 A waiting pane stays waiting through the shutdown signal

Trapping TERM in the parked shell is not enough on its own. While a pane waits, its parked shell runs the panel's draw, which hands off to the waiter (`portal state resume-draw` → `portal state resume-wait`). Today the waiter handles only SIGWINCH (`rg -n 'signal.Notify' cmd/state_resume_wait.go` → 1 hit, `SIGWINCH`), so a reboot SIGTERM would end it. The parked shell would then run its recovery tail, and `resume-recover` would clear `@portal-resume-pending` while tmux is still answering. Three things would follow:

1. A capture builds the pane a fresh record naming its positional scrollback path. That name is gone: the first commit after the pane went waiting named its token-named path, and that commit's housekeeping pass removed the positional name.
2. A commit with no scrollback dump writes that record. One such commit is the `commit-now` that fires when `_portal-saver` itself closes after the daemon's flush; `commit-now` passes no dump.
3. The housekeeping pass deletes the token-named transcript.

The session survives, but its scrollback is silently lost.

So every process the waiting pane runs while it waits outlasts SIGTERM: the panel's draw, and the waiter it becomes. The panel stays up and the marker stays set. The pane is saved still waiting, with its transcript. After the reboot it comes back still asking, which is what lazy resume already does for an unanswered pane.

The draw and the waiter outlast SIGTERM by catching it, never by ignoring it, under the same rule as the parked chain (§3.1). An ignored signal stays ignored across `exec`, and an answered pane goes on to run its hook program and then the user's shell, which keep default SIGTERM handling.

That holds from the moment each process starts. A catch is not inherited: the parked shell's trap is reset to default in the draw it starts, the draw's catch is reset when it execs into the waiter, and the same happens at every redraw, when the waiter execs back into the draw. A SIGTERM landing before the new process has installed its catch still leaves the pane waiting, with its marker set and no recovery tail run. How that window is closed is the implementer's, within the same rule: caught, never ignored.

When tmux finally exits, it closes the waiting pane's pty the same way a kill does (§3.3). The SIGHUP ends the parked shell, and the waiter with it, before the recovery tail can start (measured: `sh -c 'trap : INT QUIT TERM; sleep 3; exit 7'` run as a pty's session leader through `python3`'s `pty.fork()`, master closed after 0.5s → killed by signal 1, SIGHUP). No marker clear is attempted, so the saved record keeps the pane waiting.

#### 3.3 A kill still ends the pane

A kill is unaffected. tmux ends a killed pane by closing its pty, which delivers SIGHUP; tmux 3.7c sends no signal to a pane's process itself. Neither the trap (§3.1) nor the waiting pane's handling (§3.2) catches SIGHUP. So a killed pane, eager or waiting, still dies at once, and the kill path (§1.1) is unchanged.

### 4. Dropped Sessions and Backed-Off Saves Are Logged

Today a wipe is silent at the default log level. The only trace is `capture: tick complete sessions=0` or bare `process: start … state commit-now` lines. `Commit` itself logs nothing below WARN, and only for failed housekeeping (`rg -n 'logger\.(Info|Warn|Debug|Error)' internal/state/commit.go` → 2 hits, both `Warn`). This section closes that gap.

#### 4.1 Every dropped session is logged by name

Every commit that drops a session logs each dropped session by name at INFO.

"Dropped" is measured against the prior on-disk index, which `Commit` already reads to decide whether anything changed (`structuralChange` in `internal/state/commit.go`). It is not measured against the daemon's in-memory previous index. That index does not see `commit-now`'s writes, so it would log a session the user just killed a second time.

A commit that drops nothing logs nothing new. That includes the daemon tick that follows a `commit-now` kill, which logs no second drop line. A renamed session is not a drop and gets no drop line.

The saved index names a session by its name alone, so telling a rename from a drop needs an identity the commit can compare. Which identity is the implementer's. At shutdown no new session appears beside a dropped one, so the drop lines a reboot's log is read for (§4.3) come out the same whichever identity is used.

#### 4.2 Backed-off saves and refused empty writes are logged

A committer that backs off because tmux stopped answering mid-save (§2.1, §2.2) logs a line saying so, through its existing failure route (§2.5). A dump that refuses to write an empty capture over a saved transcript (§2.4) logs a line naming the pane.

A save whose first read, of the restore-in-progress marker, fails has backed off too. All three committers already log that at WARN with the cause in `error` (`rg -n 'read @portal-restoring|isRestoring query failed' cmd/state_daemon.go cmd/state_commit_now.go` → 3 hits), and those lines stay. A marker that reads as set means a restore is in progress, which is not a back-off, and logs nothing new.

Every line in this section is recorded at the production default level, INFO or above. Each carries its data in attribute keys Portal's closed log vocabulary already defines: the dropped session's name in `session`, the refused pane in `pane_key`, the cause in `error` (`` rg -n '^\| `(session|pane_key|error)` \|' .workflows/portal-observability-layer/specification/portal-observability-layer/specification.md `` → 3 hits). The logging therefore needs no new log component or attribute key.

#### 4.3 What the lines show after a reboot

After the fix, these lines give a reboot's log partial evidence of how macOS ended tmux and its panes:

- drop lines mean sessions closed while tmux was still answering;
- back-off lines mean a save was in flight as tmux went down.

A teardown in which everything is hard-killed (SIGKILL) runs no committer and leaves no line at all. So a log with no such lines does not, by itself, show which way macOS ended things. The instrumented reboot (§5) remains the definitive measurement.

### 5. Unchanged Behaviour, Accepted Residue and Deferred Work

#### 5.1 Unchanged by design

- **A kill is final as soon as a save commits it (§1.1).** The `session-closed` hook still runs `commit-now` synchronously, and it removes the killed session, its scrollback and, through the hook-staleness sweep, its resume hooks. When that `commit-now` stands down (§2.5), the daemon's next tick commits the kill instead. A kill that no save commits before tmux exits is accepted residue (§5.2). Killing every user session still ends with an empty restore state. That empty state is committed when the last user session closes while tmux keeps running for Portal's own `_portal-saver` and `_portal-bootstrap`.
- **The daemon's shutdown flush still runs on SIGHUP and SIGTERM.** §2 is what makes it safe.
- **The picker, the resolver, shell completion and restore keep their reading of a failed session listing (§2.1).**
- **The tests pinning the empty-save contract stay as they are:**
  - the "returns an empty index with nil error when keep is empty after filtering" subtest of `TestCaptureStructurePreLoopFailFatal` (`internal/state/capture_test.go`);
  - the "proceeds with empty index when every session is natural churn" subtest of `TestCaptureStructurePerSessionLogAndContinue` (`internal/state/capture_test.go`);
  - `TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions` (`cmd/state_commit_now_test.go`).

#### 5.2 Accepted residue

These shutdown losses remain after the fix and are accepted:

- **A pane whose top program was started directly rather than inside a shell.** SIGTERM ends it while tmux still answers. When no pane in its session survives, the session is removed with its scrollback.
- **A reboot in which macOS hard-kills pane programs before tmux.** Only the hold (§5.3) covers this.
- **A pane with no resume hook moved after restore.** A pane carrying no token that tmux renumbers, or whose session is renamed, after restore has no identity tying it to its last committed record. If tmux goes down before the pane's next save completes, its saved transcript is lost.
- **A user whose interactive shell exits on SIGTERM.** Unlike zsh and bash, fish installs a SIGTERM handler that exits. Both hardened panes (§3.1) hand over to `$SHELL`, so once a fish user's hook ends, their pane is exposed again. The user's shell is zsh.
- **A kill whose `commit-now` stands down.** The kill's `commit-now` writes nothing when its session listing fails (§2.1) or tmux begins exiting while it runs (§2.2). The daemon's next tick commits the kill instead (§2.5). If tmux exits before any later save commits the kill, the killed session is still in the saved state, with its scrollback and resume hooks, and it comes back at the next restore.
- **The first reboot after installing the fix.** The daemon and Portal's resume-hook panes keep running the version they were started with. The daemon moves to the new version only when Portal next bootstraps, and the resume-hook panes only when the next restore rebuilds them. `commit-now` moves at once, because each `session-closed` hook starts it afresh. A reboot taken straight after upgrading, such as one right after `brew upgrade`, keeps the exposure from before the fix. If no bootstrap has replaced the daemon, its final flush can still wipe the saved state. Until a restore has rebuilt them, Portal's resume-hook panes can still lose their sessions and scrollback, even after a bootstrap has replaced the daemon. Running `portal uninstall` before that reboot, and running no Portal command that bootstraps until after it, covers it. Those commands are `portal open` (and the `x` shell function), `portal list`, `portal kill` and `portal completion` (`rg -n 'rootCmd\.AddCommand\(' cmd -g '!*_test.go'` → 11 commands, plus the `completion` command cobra generates because nothing disables it (`rg -n 'CompletionOptions|DisableDefaultCmd' cmd` → no matches); of these, `open`, `list`, `kill` and `completion` sit outside the 10-entry bootstrap-exempt `skipTmuxCheck` set in `cmd/root.go`). Each one revives the save daemon, and after an upgrade it also re-registers the `session-closed` hook. The log of that reboot is not the evidence the hold (§5.3) and the instrumented reboot (§5.4) wait on. That evidence comes from a reboot taken after a restore on the fixed version.

#### 5.3 Deferred: hold removals until tmux outlives them

This is out of scope for this fix. Under the hold, a session that disappears would be kept (its record, scrollback and resume hooks) and removed for good only once tmux has kept running for a window after it. If tmux died inside that window, the held sessions would restore. That would cover every signal ordering, including hard kills.

It becomes the follow-up if the log of a reboot taken after a restore on the fixed version (§5.2) shows sessions dropped during shutdown (§4.3). Adding it later builds on this fix rather than reworking it.

#### 5.4 Deferred: an instrumented reboot

Nobody has measured how macOS ends the detached tmux tree at a real reboot. If it sends SIGTERM to everything at once, this fix covers it. If it hard-kills processes in some order, only the hold (§5.3) does.

The measurement is not part of this fix. After the fix, the dropped-session logging (§4) makes the next ordinary reboot taken after a restore on the fixed version (§5.2) show whether anything was dropped during shutdown. A definitive answer can come whenever convenient, from a throwaway tmux and Portal setup running beside the real one. It would have its own socket and state directory, and its panes would log the signals they receive.

### 6. Testing

#### 6.1 Save path (§2)

- A failed `list-sessions` through the production client's listing makes the committing cycle error, with nothing written and no scrollback deleted. This holds for the daemon's tick, its shutdown flush and `commit-now`.
- A commit whose confirmation (§2.2) is refused writes nothing.
- A capture whose session or pane listing came back empty from a server that then refuses connections writes nothing.
- A confirmation whose own read returns exit 0 with no output names no server, and the cycle writes nothing.
- The daemon's shutdown flush after `_portal-saver` is killed on a running server, as `portal uninstall` does, still commits and reports `flush_completed=true`. A save whose capture reads or confirmation reach a different server started on the same socket writes nothing.
- A stand-down injected after the capture cycle's re-file leaves `sessions.json` naming only files that exist. So does a daemon tick cancelled after the re-file and followed by a shutdown flush that stands down, and a cycle whose `sessions.json` write fails after it.
- The daemon's dump does not overwrite a non-empty saved transcript with an empty capture it cannot confirm.
- Restore with a failed session listing behaves exactly as today: it rebuilds from the saved state, and no empty commit follows it.

#### 6.2 Panes (§3)

- Restored eager and lazy resume-hook panes survive a SIGTERM to the pane's top process. The hook program and the user's shell still receive SIGTERM with default handling, so the trap is not inherited as an ignore.
- A lazy pane answered on its panel, with its hook program still running, keeps its session when SIGTERM reaches every process in the pane. Its parked chain and the shell running its hook both survive, and the pane goes on to the user's shell.
- A lazy pane whose waiter caught a SIGTERM, and which the user then answers on its panel, runs its hook program and the user's shell with default SIGTERM handling.
- Lazy waiting pane, SIGTERM during the wait. The whole process tree is signalled alongside the daemon, the server later, and a `commit-now` with no dump lands in between (the `_portal-saver` close). The pane is still waiting with its marker set, and its token-named transcript is still referenced and present at the next restore. That restore brings the pane back still asking.
- A SIGTERM landing while the panel is still being drawn leaves the pane waiting, the same as one landing on the waiter. So does one landing as the draw or the waiter starts, before it has installed its catch, at restore or at a redraw.
- A waiting pane whose pty closes because tmux has begun exiting: the parked chain ends on SIGHUP before its recovery tail starts, no marker clear is attempted, and the saved record keeps the pane waiting.
- A killed pane, waiting or eager, still dies, because the kill path's SIGHUP is not caught.

#### 6.3 Logging (§4)

- A commit that drops sessions logs each one by name at INFO.
- A commit that drops none logs nothing new. That includes the daemon tick that follows a `commit-now` kill (no duplicate drop line) and a rename (no drop line).
- A back-off on a non-answering tmux and a refused empty scrollback write each log a line.

#### 6.4 Kill-path regressions (§5.1)

- The empty-save contract tests listed in §5.1 stay green.
- Killing each session in turn still removes it and its scrollback at that kill, the last kill leaving zero sessions and zero scrollback files. The next hook-staleness sweep still reaps the killed sessions' resume hooks.

#### 6.5 Shutdown orderings against real tmux

These run in the integration lane, on real tmux with an isolated socket:

- The daemon SIGTERMed 10–30ms before the server, repeated across many trials, preserves the full state. Before the fix, this window wiped the whole state in the sandbox. The trials run at the production default log level, with nothing wrapping `tmux`, because debug logging and a logging shim both shift the timing. In the sandbox, five 10ms trials with debug logging on all kept the full state, while five at the default level all wiped it.
- `tmux kill-server` on a live runtime preserves the full state.
- Signal the pane programs and the daemon, then the server. Every session whose panes are interactive shells or Portal's hardened panes (§3.1) is preserved.

#### 6.6 Existing tests to revisit

- The "returns an error when ListSessionNames fails and does not call show-environment" subtest of `TestCaptureStructurePreLoopFailFatal` (`internal/state/capture_test.go`) asserts an error the production client never delivers. It gets that error through a fake that can return one. It should exercise the listing the committing path actually uses (§2.1).
- The "returns empty slice when tmux server is not running" case of `TestListSessions` (`internal/tmux/tmux_test.go`) stays, for the picker, but must no longer describe the save path.

### 7. Prior Specifications This Fix Touches

- **`killed-session-resurrects-within-tick-window`.** It made `session-closed` commit synchronously on every kill path, so a killed session is never resurrected. This fix keeps that rule unchanged (§5.1).
- **`built-in-session-resurrection`.** It defined the housekeeping pass as self-healing by construction, and the daemon's SIGHUP/SIGTERM final flush, which treated an atomic write as a safe one. §2 removes both assumptions from the save path: an unreferenced scrollback file is no longer proof of an orphan when the session list cannot be confirmed, and the flush now stands down against a server that stops answering.
- **`resume-hooks-silently-lost`.** Its hook-staleness sweep stands down on an empty or failed pane read. §2 gives the session commit path the matching posture.
- **`lazy-resume-on-attach`.** It defined the waiting pane's parked chain and waiter. §3 makes both outlast SIGTERM while the pane waits.
- **`v1` (portal).** It defined "no server running means zero sessions" for the picker's session discovery. The picker, resolver, completion and restore keep that reading (§2.1).

---

## Working Notes

## Corrigenda

> **Corrigendum 2026-10-05** (from `planning/killing-all-sessions-wipes-restore-state`): §2.3 left open cycles that end uncommitted after the transcript rename for a reason other than a stand-down — "Either of two orderings is acceptable … Whichever is used, no stand-down may leave `sessions.json` naming a scrollback file that is not on disk." — corrected: the guarantee covers every cycle that ends without committing after the rename (a stand-down, the daemon tick cancelled mid-dump by the shutdown signal, a failed `sessions.json` write), confirming before any move is not enough on its own, and §6.1 tests the cancelled-tick and failed-write cases — settled by: §2.3's own stated purpose (at shutdown no later cycle repairs the record, so the next restore must find the transcript at the path it names) read against the tree — `errCycleCancelled` (`cmd/state_daemon.go`) returns after `captureAndRefile`'s renames and before `Commit`, `Commit` returns before housekeeping when `sessions.json`'s atomic write fails, and the shutdown flush that today repairs the record through the re-file's adoption of the token-named file is itself a cycle that can now stand down.

> **Corrigendum 2026-10-05** (from `planning/killing-all-sessions-wipes-restore-state`): §3.2 required the draw and the waiter to outlast SIGTERM but left open the moment each starts, before it has installed its catch (the parked shell's trap and each process's catch reset to default across fork and exec, including every redraw) — corrected: a SIGTERM landing in that window still leaves the pane waiting, with its marker set and no recovery tail run, closed by the implementer within the caught-never-ignored rule, and §6.2 tests it — settled by: the specification review's recorded observation (`review-gap-analysis-tracking-c1.md`, Observations) that closing this window is the builder's, read with §5.2, which does not list it as accepted residue.

> **Corrigendum 2026-10-05** (from `planning/killing-all-sessions-wipes-restore-state`): §4.1 said "A renamed session is not a drop and gets no drop line" with no identity the commit could compare, since the saved index names a session by its name alone — corrected: which identity tells a rename from a drop is the implementer's, and at shutdown, where no new session appears beside a dropped one, the drop lines come out the same under any identity — settled by: the specification review's recorded observation (`review-gap-analysis-tracking-c1.md`, Observations) that any session identity the builder matches on serves, because drops at shutdown never coincide with new sessions.

> **Corrigendum 2026-10-05** (from `implementation/killing-all-sessions-wipes-restore-state`): §2.2 said "The confirmation counts as answered when tmux returns exit status 0, whatever the output … So even tmux's own shutdown answer to the confirmation (exit 0, no output) still proves that every earlier read was answered in full", and §6.1 said "The confirmation is safe even when its own read returns exit 0 with no output" — corrected: the confirmation counts only when its exit-0 answer names the server that answered it; an empty answer names no server, cannot meet §2.2's own-server rule, and stands the cycle down, and §6.1 tests that — settled by: §2.2's own-server paragraph, which an answer naming no server cannot satisfy, as landed in `confirmOwnServer` (`internal/state/scrollback.go`); the cost is one stood-down save when the committer's own server begins exiting during the confirmation itself, with the previous saved state kept.

> **Corrigendum 2026-10-06** (from `implementation/killing-all-sessions-wipes-restore-state`): §2.3 said "The capture cycle renames a newly waiting pane's transcript from its positional path to its token-named path … `sessions.json` still names the vacated positional path" and "How that is met is open", §3.2 said the positional file "was renamed away when the pane first went waiting", and §6.1 spoke of "the capture cycle's renames" — corrected: the re-file is a hard link (`link(2)`) that never overwrites an existing token-named file, the positional name stays on disk until the housekeeping pass of a commit naming the token-named path removes it, so a cycle that ends uncommitted leaves `sessions.json` naming a file still present, and §3.2's fresh record names a positional path that commit's housekeeping already removed — settled by: the landed change in `refilePendingPane` / `linkStoredScrollback` (`internal/state/scrollback.go`), with `rename_noreplace_{darwin,linux}.go` deleted and `internal/state/commit_cycle_uncommitted_test.go` asserting the positional name goes only once a committing cycle's housekeeping has run.

> **Corrigendum 2026-10-06** (from `implementation/killing-all-sessions-wipes-restore-state`): §2.4 said the dump writes through "`WriteScrollbackIfChanged`, called from the dump in `cmd/state_daemon.go`" — corrected: the dump reaches `WriteScrollbackIfChanged` only through the `ScrollbackWriter` the commit cycle hands it, which applies the empty-capture confirmation before the write — settled by: the landed change in `ScrollbackWriter.Write` (`internal/state/commit_cycle.go`) and the commit guard (`internal/state/commit_guard_test.go`) refusing `WriteScrollbackIfChanged` in any non-test file outside `internal/state`.

> **Corrigendum 2026-10-06** (from `implementation/killing-all-sessions-wipes-restore-state`): §2.4 judged a pane's saved transcript only as "a saved non-empty transcript", which the landed check read as the pane's positional file, and §2.3 and §5.2 left open a lazy pane answered shortly before shutdown, whose fresh record names a positional path the first commit after it went waiting had removed — corrected: a pane's saved transcript is the file its last committed record names; from the answer until a dump writes the pane's new capture with confirmed bytes, every commit names the token-named transcript and leaves it on disk, whether the capture comes back empty, is refused, or no dump runs (`commit-now`) — settled by: the user's decision at the implementation phase 3 consolidation walk to close the path, on §1.1's rule that every session not killed stays restorable with its scrollback intact.

> **Corrigendum 2026-10-06** (from `implementation/killing-all-sessions-wipes-restore-state`): §2.4 named the file a pane's last committed record names as its saved transcript but held it only for a lazy pane on its token-named file, and §5.2 did not list a pane that restore, a renumber or a rename moves to a different address — corrected: a pane at a different address from the one its last committed record names keeps that file named and on disk until a dump writes its capture with confirmed bytes; every pane restore recreates carries an identity tying it to the record it was built from, so a restored pane is held whether or not it carries a resume hook; a pane with no token moved later by a renumber or rename is accepted residue in §5.2 — settled by: the user's decision at the implementation phase 7 consolidation walk that every restored pane keeps its transcript through the first save after restore, on §1.1's rule, with the shifted-run loss for tokened panes measured against HEAD (`consolidation-findings-p7.md`, F1).
