# Specification: Killing All Sessions Wipes Restore State

## Specification

### 1. Problem and Governing Rule

#### 1.1 The rule

A session the user kills never comes back. A session that stops any other way does come back.

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

The confirmation counts as answered when tmux returns exit status 0, whatever the output. Any client tmux accepted at all was accepted before the exit began. So even tmux's own shutdown answer to the confirmation (exit 0, no output) still proves that every earlier read was answered in full.

The confirmation also catches the shutdown answers that no error check can see: a `list-sessions` or `list-panes` that returned exit 0 with no output because tmux began exiting during the read. A read like that was answered after the exit began, so the confirmation sent after it is refused. Without this check, the bad answers do damage in two ways:

- an empty `list-sessions` commits an empty index;
- an empty `list-panes` next to an environment read that still succeeds records a session with no windows. That deletes the session's scrollback files while its name stays in `sessions.json`.

Under `tmux kill-server`, no committer can pass the confirmation, whichever of its reads the exit lands after. Survival under `kill-server` no longer depends on which read happens to fail first (§1.2).

#### 2.3 A stand-down never leaves the saved state naming a missing file

The capture cycle renames a newly waiting pane's transcript from its positional path to its token-named path (`refilePendingScrollback` in `internal/state/scrollback.go`). It does this expecting the cycle to commit the record that points at the new path. If a cycle backed off after that rename, `sessions.json` would still name the vacated positional path. At shutdown no later cycle runs to repair it, so the next restore would find no transcript at the path the record names.

Either of two orderings is acceptable:

- the confirmation (§2.2) comes before any file is moved; or
- a stand-down after a move leaves the saved state naming only files that exist.

Whichever is used, no stand-down may leave `sessions.json` naming a scrollback file that is not on disk.

#### 2.4 The scrollback dump never zeroes a saved transcript on an unconfirmed read

The daemon's scrollback dump runs in its tick and its shutdown flush; `commit-now` dumps nothing. The dump writes a pane's scrollback read whenever it differs from the saved file (`WriteScrollbackIfChanged`, called from the dump in `cmd/state_daemon.go`). So an empty read overwrites a saved transcript with zero bytes. That loss involves no housekeeping pass and no change to the session list.

The sandbox never saw tmux's shutdown answer (exit 0, no output) on `capture-pane`. But `capture-pane` goes through the same client path as the listings that did show it.

An empty capture may replace a saved non-empty transcript only once it is confirmed by the rule in §2.2: a tmux read sent strictly after that capture has been answered. An unconfirmed empty capture is not written. The saved transcript stands, and the refused write is logged (§4).

#### 2.5 What a stand-down does

A cycle stands down on a failed session listing (§2.1) or a refused confirmation (§2.2). When it does, it writes no commit and runs no housekeeping pass, so `sessions.json` and every scrollback file stay as they were. The cycle then ends as a failed cycle, through each committer's existing failure route:

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

The handling must be a caught trap, never an ignored disposition. That is already the parked chain's rule: an ignored signal stays ignored across `exec`, so the hook program and the user's shell would inherit it. The hook program and the user's shell keep default SIGTERM handling.

These two are the only non-interactive-shell panes Portal creates (`rg -n '"sh", "-c"' --type go -g '!*_test.go' cmd internal | wc -l` → 2). Session trees run `$SHELL -ic`, which is interactive (`BuildShellCommand` in `internal/session/create.go`). Every other pane process Portal starts is either not a shell (the hydrate helper, the saver's daemon) or an interactive shell (`_portal-bootstrap`, created with no command by `StartServer` in `internal/tmux/tmux.go`).

#### 3.2 A waiting pane stays waiting through the shutdown signal

Trapping TERM in the parked shell is not enough on its own. While a pane waits, its parked shell runs the panel's draw, which hands off to the waiter (`portal state resume-draw` → `portal state resume-wait`). Today the waiter handles only SIGWINCH (`rg -n 'signal.Notify' cmd/state_resume_wait.go` → 1 hit, `SIGWINCH`), so a reboot SIGTERM would end it. The parked shell would then run its recovery tail, and `resume-recover` would clear `@portal-resume-pending` while tmux is still answering. Three things would follow:

1. A capture builds the pane a fresh record naming its positional scrollback path. That file was renamed away when the pane first went waiting.
2. A commit with no scrollback dump writes that record. One such commit is the `commit-now` that fires when `_portal-saver` itself closes after the daemon's flush; `commit-now` passes no dump.
3. The housekeeping pass deletes the token-named transcript.

The session survives, but its scrollback is silently lost.

So every process the waiting pane runs while it waits outlasts SIGTERM: the panel's draw, and the waiter it becomes. The panel stays up and the marker stays set. The pane is saved still waiting, with its transcript. After the reboot it comes back still asking, which is what lazy resume already does for an unanswered pane.

When tmux finally exits and the waiter's pty closes, the recovery tail tries to clear the marker. That cannot reach a server that is refusing connections (§2.2), so the saved record keeps the pane waiting.

#### 3.3 A kill still ends the pane

A kill is unaffected. tmux ends a killed pane by closing its pty, which delivers SIGHUP; tmux 3.7c sends no signal to a pane's process itself. Neither the trap (§3.1) nor the waiting pane's handling (§3.2) catches SIGHUP. So a killed pane, eager or waiting, still dies at once, and the kill path (§1.1) is unchanged.

---

## Working Notes
