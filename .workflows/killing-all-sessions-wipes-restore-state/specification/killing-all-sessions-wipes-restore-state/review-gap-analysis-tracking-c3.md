# Review Tracking: killing-all-sessions-wipes-restore-state - Gap Analysis

## Findings

### 1. The scrollback dump's confirmation leaves out the same-server condition

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §2.4 The scrollback dump never zeroes a saved transcript on an unconfirmed read (colliding with §2.5 What a stand-down does, and the server condition in §2.2)

**Problem**:
An empty scrollback read may overwrite a saved transcript only once a later tmux read has been answered. The scrollback-dump section restates that rule but drops one condition: the answer has to come from the committer's own server. For commits, that condition is what stops a new tmux server, started on the same socket, from confirming a save. The dump section never states it.

A builder who follows the dump section's wording ships this sequence:
- tmux begins exiting while the daemon's dump is capturing a pane, and the old server answers that capture with nothing (exit 0, no output);
- the user restarts at once (`tmux kill-server`, then `x`), and the new server answers the dump's confirming read;
- the dump writes zero bytes over the pane's saved transcript;
- the commit's own confirmation also reaches the new server, so it counts as refused and the cycle stands down, as it should.

`sessions.json` still holds the session, but its transcript is now empty. The next restore brings the session back with its scrollback gone. The stand-down rule promises that a stand-down leaves no scrollback file emptied. This build breaks that promise while following every rule as worded.

**Proposal**:
The dump's confirmation takes the same server condition as the commit's. Two things determine this. The stand-down rule promises that no scrollback file is emptied. And the confirmation rule the dump section already cites rests on a proof that holds only for one server process, so an answer from a different server says nothing about the capture the old server answered. The alternative is to keep the dump's wording as it is. That leaves open the scrollback loss the server condition was added to close, so no informed user would choose it. How the server is identified is the builder's call.

**Current**:
An empty capture may replace a saved non-empty transcript only once it is confirmed by the rule in §2.2: a tmux read sent strictly after that capture has been answered.

**Proposed Text**:
An empty capture may replace a saved non-empty transcript only once it is confirmed by the rule in §2.2: a tmux read sent strictly after that capture has been answered by the server the committer belongs to, the same server that answered the capture.

**Resolution**: Pending
**Notes**:

---

### 2. The first-reboot residue ties the resume-hook panes' exposure to the daemon's bootstrap

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §5.2 Accepted residue (the first-reboot-after-installing bullet)

**Problem**:
This residue note tells the user when they still need to run `portal uninstall` before rebooting. It first says the daemon moves to the fixed version at the next bootstrap, and Portal's resume-hook panes only when the next restore rebuilds them. The next sentence then makes both exposures depend on one condition: "If no bootstrap has replaced the daemon, its final flush can still wipe the saved state, and Portal's resume-hook panes can still lose their sessions and scrollback."

Suppose the user has run `portal open` since upgrading. The bootstrap has replaced the daemon, so they read that sentence as saying the reboot is now safe, and they skip the uninstall. But their resume-hook panes are still the old ones without the SIGTERM trap, because a bootstrap on a running server does not rebuild live panes. At the reboot those panes die on SIGTERM while tmux is still answering. The fixed `commit-now` then commits their sessions as closed and deletes their scrollback. On the next start, the hook-staleness sweep reaps their resume hooks.

**Proposal**:
Split the sentence so that the panes stay exposed until a restore has rebuilt them, whether or not a bootstrap has run. The note's own earlier sentence determines this: the panes move to the new version only when the next restore rebuilds them. No other reading is consistent with that sentence.

**Current**:
If no bootstrap has replaced the daemon, its final flush can still wipe the saved state, and Portal's resume-hook panes can still lose their sessions and scrollback.

**Proposed Text**:
If no bootstrap has replaced the daemon, its final flush can still wipe the saved state. Until a restore has rebuilt them, Portal's resume-hook panes can still lose their sessions and scrollback, even after a bootstrap has replaced the daemon.

**Resolution**: Pending
**Notes**:

---

### 3. The deferred-work sections point at the reboot the residue note rules out as evidence

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §5.3 Deferred: hold removals until tmux outlives them; §5.4 Deferred: an instrumented reboot (colliding with the first-reboot bullet in §5.2)

**Problem**:
The two deferred-work sections disagree with the residue note about which reboot decides the next step:
- The hold section says the hold becomes the follow-up if "a reboot's log after the fix" shows sessions dropped during shutdown.
- The instrumented-reboot section says the dropped-session logging makes "the next ordinary reboot" show whether anything was dropped.
- The residue note says the log of the first reboot after installing is not that evidence. The evidence comes from a reboot taken after a restore on the fixed version.

So the deferred sections, read on their own, send the user to the very reboot the residue note rules out. That reboot will show drop lines whenever Portal's old resume-hook panes die on SIGTERM, because the fixed `commit-now` logs them. If those lines are read as the trigger, the hold gets built as a follow-up on evidence that measured only the pre-fix panes.

**Proposal**:
Name the qualifying reboot in both deferred sections. The residue note's own statement of which reboot carries the evidence determines this.

**Current**:
§5.3: It becomes the follow-up if a reboot's log after the fix shows sessions dropped during shutdown (§4.3).

§5.4: After the fix, the dropped-session logging (§4) makes the next ordinary reboot show whether anything was dropped during shutdown.

**Proposed Text**:
§5.3: It becomes the follow-up if the log of a reboot taken after a restore on the fixed version (§5.2) shows sessions dropped during shutdown (§4.3).

§5.4: After the fix, the dropped-session logging (§4) makes the next ordinary reboot taken after a restore on the fixed version (§5.2) show whether anything was dropped during shutdown.

**Resolution**: Pending
**Notes**:

---

## Observations

- The level of the refused-empty-write line is given only as "INFO or above". INFO and WARN would both serve the reboot-log reading.
- The closing paragraph of the backed-off-saves subsection says "every line in this section". It also covers the dropped-session line, which belongs to the preceding subsection.
- The drop line's message and component are not named. Any existing component would serve someone grepping the log after a reboot.
