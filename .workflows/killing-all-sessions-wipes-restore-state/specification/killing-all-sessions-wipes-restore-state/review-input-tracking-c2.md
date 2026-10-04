# Review Tracking: Killing All Sessions Wipes Restore State - Input Review

## Findings

### 1. `portal uninstall`'s final save must still count as coming from the daemon's own server

**Source**: investigation `killing-all-sessions-wipes-restore-state.md`. Symptoms → References: the verified safe-reboot procedure is "`portal uninstall` (kills `_portal-saver`, whose SIGHUP makes the daemon flush a final atomic state and exit …) → confirm `flush_completed=true` in `portal.log`". Fix Direction → Chosen Approach, part 1, confirmation bullet: "the confirmation counts only when it is answered by the committer's own server — the one hosting the daemon's `_portal-saver` pane … an answer from any other server counts as refused". Fix Direction: the fix changes no part of the SIGHUP flush beyond the save-path rules.
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §2.2 tmux is confirmed still answering before a commit is written; §6.1 Save path

**Problem**:
After the fix, a save is written only when tmux answers from the server the save belongs to. For the daemon, the specification defines that as the server "hosting" its `_portal-saver` pane. `portal uninstall` kills `_portal-saver` while the server keeps running. That kill sends the daemon the SIGHUP that triggers its final save, and the source's verified safe-reboot procedure depends on that save, checked by `flush_completed=true` in the log. But when that save runs, the pane has already been destroyed, so no server hosts it any more. The daemon already has a probe that answers exactly "am I still the `_portal-saver` pane of this server?". A builder who reuses it as the ownership check makes every such save stand down. A user who runs `portal uninstall` would then lose the output that has reached every session since the daemon's last save, which can be up to 30 seconds. They would also see `flush_completed=false` in the log, where the procedure tells them to look for `true`.

**Proposal**:
The daemon's own server is the tmux server process its `_portal-saver` pane runs in, and it stays the daemon's own server after that pane is destroyed. Only a different server process, such as a new one started on the same socket, is foreign. Two things in the record decide this. First, the source has the uninstall SIGHUP flush commit a final state on a running server, and no part of the fix changes that. Second, the rule exists to turn away a fresh server holding none of the user's sessions, and a server that has just lost one of its own sessions is not that. The other reading, where a destroyed pane means a foreign server, makes every save triggered by a SIGHUP on a running server stand down. It loses output that no shutdown put at risk, and no informed user would pick it. How the server's identity is established stays the builder's. Add a test that pins both sides of the rule.

**Current**:
§2.2:
> The confirmation counts only when it is answered by the tmux server the committer belongs to, and only when that same server answered every capture read the commit is built from. For the daemon, that is the server hosting its `_portal-saver` pane. For `commit-now`, it is the server whose `session-closed` hook ran it. After that server exits, a new one can be started on the same socket before its restore has run, and it holds none of the user's sessions. Its answers confirm nothing, and a save that reaches it stands down (§2.5).

**Proposed Text**:
§2.2:
> The confirmation counts only when it is answered by the tmux server the committer belongs to, and only when that same server answered every capture read the commit is built from. For the daemon, that is the server its `_portal-saver` pane runs in. It stays the daemon's own server after that pane is destroyed, so the save that runs when `portal uninstall` kills `_portal-saver` on a running server still commits. For `commit-now`, it is the server whose `session-closed` hook ran it. After that server exits, a new one can be started on the same socket before its restore has run, and it holds none of the user's sessions. Its answers confirm nothing, and a save that reaches it stands down (§2.5).

§6.1, new bullet after "The confirmation is safe even when its own read returns exit 0 with no output, as long as it is sent strictly after the last capture read.":
> - The daemon's shutdown flush after `_portal-saver` is killed on a running server, as `portal uninstall` does, still commits and reports `flush_completed=true`. A save whose capture reads or confirmation reach a different server started on the same socket writes nothing.

**Resolution**: Pending
**Notes**:

---

## Observations

- The source names `portal kill <name>` among the kills Portal makes, but the specification's list of kills leaves it out. It goes through the same kill path, so nothing would be built differently.
