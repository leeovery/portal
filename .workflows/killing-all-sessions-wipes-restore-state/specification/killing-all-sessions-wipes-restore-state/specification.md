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

---

## Working Notes
