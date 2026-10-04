# Review Tracking: killing-all-sessions-wipes-restore-state - Gap Analysis

## Findings

### 1. The confirmation must come from the server the save belongs to

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §2.2 tmux is confirmed still answering before a commit is written

**Problem**:
The promise that `tmux kill-server` leaves every session restorable depends on the confirmation proving that the capture's reads were answered before the server began exiting. That proof only works for a single server process: an exiting server refuses new connections and never stops exiting. After kill-server, a new tmux server can start on the same socket while a save begun under the old server is still running. `portal open` can start it, and so can a script that restarts tmux. Until the new server's restore runs, it holds none of the user's sessions.

Two saves pass the check as written:
- a save whose capture got the old server's shutdown answers (exit 0, zero sessions) and whose confirmation reaches the new server;
- a late shutdown flush whose reads all land on the new server before that server sets its restore marker.

Either save commits an empty index, and the housekeeping pass deletes every scrollback file. The new server's restore then brings back nothing. This is the same wipe the fix exists to stop, and it hits a user who restarts tmux and reopens straight away. The spec only says "tmux is still answering", so a builder who sends any read to the socket ships this hole.

**Proposal**:
The confirmation counts only when it is answered by the server the committer belongs to, and that same server answered every capture read the commit is built from. An answer from any other server counts as refused. Two things determine this: the confirmation's own proof, whose two premises hold only for a single server process, and the rule that kill-server leaves every session restorable. The alternative is to record "a save still running when a new server comes up on the same socket" as accepted residue. That leaves a full wipe open, which is the defect itself, so no informed user would choose it. How the server's identity is established is the builder's.

**Proposed Text**:
Append to §2.2, after "Survival under `kill-server` no longer depends on which read happens to fail first (§1.2).":

The confirmation counts only when it is answered by the tmux server the committer belongs to, and only when that same server answered every capture read the commit is built from. For the daemon, that is the server hosting its `_portal-saver` pane. For `commit-now`, it is the server whose `session-closed` hook ran it. After that server exits, a new one can be started on the same socket before its restore has run, and it holds none of the user's sessions. Its answers confirm nothing, and a save that reaches it stands down (§2.5).

**Resolution**: Routed
**Notes**: This session's call (what leaned: the confirmation's single-process proof and kill-server leaving everything restorable; residue alternative set aside). Landed first in the investigation (Fix Direction item 1, confirmation bullet), then applied to §2.2 as staged.

---

### 2. A save that backs off at its first read is not named as logged

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §4.2 Backed-off saves and refused empty writes are logged

**Problem**:
Against an exiting tmux server, a save's very first read, of the restore-in-progress marker, fails. That failure is taken to mean "a restore is in progress, do not commit", so the save backs off at that point. This is the back-off a reboot produces when tmux and the daemon receive SIGTERM together: the daemon's final flush stops at that read. So does a `commit-now` started after the exit began. The logging rule names only back-offs on a failed session listing or a refused confirmation, so a builder has no instruction to log this one. Yet the fix's scope promises that every save that backs off is logged. The reboot log the user reads after the fix would show no back-off line for that flush. It would read as though no save was in flight as tmux went down.

**Proposal**:
Settled by measurement: all three committers already log this stand-down at WARN with the cause in `error`, so nothing new is built. The specification records that the lines exist and stay, so the reboot-log reading counts them as back-off lines. A marker that reads as set is a real restore, not a back-off.

**Proposed Text**:
New paragraph in §4.2, after the paragraph naming the back-off and refused-write lines:

A save whose first read, of the restore-in-progress marker, fails has backed off too. All three committers already log that at WARN with the cause in `error` (`rg -n 'read @portal-restoring|isRestoring query failed' cmd/state_daemon.go cmd/state_commit_now.go` → 3 hits), and those lines stay. A marker that reads as set means a restore is in progress, which is not a back-off, and logs nothing new.

**Resolution**: Approved
**Notes**: Record's own answer by measurement: the three committers already log a failed marker read at WARN. Applied to §4.2 as rewritten at disposal.

---

### 3. A session killed just before tmux exits comes back

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §5.2 Accepted residue (and the kill rule in §5.1)

**Problem**:
After the fix, a kill's synchronous `commit-now` writes nothing in two cases: its session listing fails, or tmux begins exiting while it runs. The kill is then committed by the daemon's next tick instead. If tmux exits before any later save commits the kill, the killed session is still in the saved state, with its scrollback and resume hooks. It comes back at the next restore. This happens to a session killed moments before `tmux kill-server` or a reboot. The spec states that a kill is final the moment it is made and lists no exception. A user who kills a session just before restarting would see it return, and a tester holding the rule has no stated expectation for this case.

**Proposal**:
Record it as accepted residue. It follows directly from two decisions the spec makes: every save-path rule applies to `commit-now` too, and the kill path is unchanged. The alternative is for `commit-now` to remove the closed session from the saved state by name even when it stands down. That avoids the return, but it changes the kill path, which this fix leaves unchanged.

**Proposed Text**:
New bullet in §5.2:

- **A kill whose `commit-now` stands down.** The kill's `commit-now` writes nothing when its session listing fails (§2.1) or tmux begins exiting while it runs (§2.2). The daemon's next tick commits the kill instead (§2.5). If tmux exits before any later save commits the kill, the killed session is still in the saved state, with its scrollback and resume hooks, and it comes back at the next restore.

**Resolution**: Routed
**Notes**: This session's call (what leaned: the kill path stays unchanged; alternative of commit-now removing the session by name on stand-down set aside). Landed first in the investigation (Known residue, accepted), then applied to §5.2 as staged.

---

### 4. The stand-down guarantee promises more about scrollback files than the rest of the spec allows

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §2.5 What a stand-down does (colliding with §2.3 and §2.4)

**Problem**:
The stand-down rule promises that `sessions.json` and every scrollback file stay as they were. Two other rules in the spec let files change before a stand-down:
- The re-file rule for waiting panes accepts a stand-down after a transcript has already moved to its token-named path. It only asks that the saved state then name files that exist.
- The daemon's scrollback dump runs before the commit. It may already have rewritten pane transcripts with fresh captures when a confirmation sent after it is refused.

Read literally, the promise rules out the second ordering the re-file rule accepts, and it rules out dumping before the confirmation. A test asserting untouched scrollback after a stand-down would fail a build the rest of the spec allows. What a stand-down actually protects is narrower: no session dropped, no transcript deleted or emptied, and no record naming a missing file.

**Proposal**:
Narrow the promise to what skipping the commit and the housekeeping pass actually guarantees. This is settled by the re-file rule and the dump rule. Both allow file changes before a stand-down, and neither harms a saved transcript.

**Current**:
A cycle stands down on a failed session listing (§2.1) or a refused confirmation (§2.2). When it does, it writes no commit and runs no housekeeping pass, so `sessions.json` and every scrollback file stay as they were.

**Proposed Text**:
A cycle stands down on a failed session listing (§2.1) or a refused confirmation (§2.2). When it does, it writes no commit and runs no housekeeping pass. So `sessions.json` stays as it was, no scrollback file is deleted or emptied, and the saved state names only files that exist (§2.3).

**Resolution**: Approved
**Notes**: Determined by the spec's own re-file (§2.3) and dump (§2.4) rules. Applied to §2.5 as staged.

---

## Observations

- The drop comparison never defines what makes a session a rename rather than a drop. Any session identity the builder matches on serves, because drops at shutdown never coincide with new sessions.
- Between the panel's draw exec'ing into the waiter and the waiter installing its SIGTERM catch, there is a brief unprotected window. Closing it is the builder's.
- The hydrate helper that runs before a restored pane's chain is not hardened against SIGTERM, and it is not listed as accepted residue. Its only exposure is a reboot within the short replay window just after a restore.
- "The user's shell is zsh" in the accepted residue reads as one install's fact inside a product specification.
- A pane lost at SIGTERM in a session that survives is not logged, since only dropped sessions get a line.
- "No committer can pass the confirmation, whichever of its reads the exit lands after" overstates the case for a committer whose confirmation was answered before the exit began. That committer's index is valid, so this is harmless.
- On a healthy server, a transient `commit-now` stand-down after a kill can show tmux's run-shell failure message to the attached client. The spec only states that extra lines are accepted at teardown.
