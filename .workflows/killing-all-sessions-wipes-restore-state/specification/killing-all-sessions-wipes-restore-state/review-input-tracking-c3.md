# Review Tracking: Killing All Sessions Wipes Restore State - Input Review

## Findings

### 1. The first reboot after installing the fix still runs the old daemon and the old resume-hook panes

**Source**: investigation `killing-all-sessions-wipes-restore-state.md`:
- Symptoms → Reproduction Steps: "The user's normal reboot — the real-world sequence a reproduction has to cover: 1. Close everything down first; often run `brew update` + `brew upgrade`. 2. Quit Ghostty … 3. Reboot through macOS."
- Analysis → Code Trace → Entry point: the daemon runs inside `_portal-saver`. The `session-closed` hook runs `command -v portal >/dev/null 2>&1 && portal state commit-now`, which starts a fresh process every time.
- Fix Direction → Chosen Approach, part 2: the hardened shapes are the restored resume-hook panes' chains, which start at restore.
- Fix Direction → Discussion: "Until the fix ships, the verified safe-reboot procedure (`portal uninstall` first) remains the safe path"; "the dropped-session logging makes the next ordinary reboot after the fix show whether anything was dropped during shutdown".
- Symptoms → References: the safe-reboot procedure is `portal uninstall`, then a reboot "without running `x` / `portal open`".
**Category**: Gap/Ambiguity
**Move**: settled
**Affects**: §5.2 Accepted residue (and the log-evidence reading in §4.3, §5.3 and §5.4)

**Problem**:
The fix lands in processes that are already running when it is installed. The daemon is started inside `_portal-saver` and keeps running the code it started with. Portal's resume-hook panes are shells started at the last restore, so they keep the old SIGTERM handling. Only `commit-now` changes straight away, because the `session-closed` hook starts a fresh `portal` from the PATH each time.

The user's normal routine is to run `brew upgrade` and then reboot. So the reboot straight after installing this release still runs the old pane chains. It also runs the old daemon, unless `portal open` ran in between and restarted it on the new version. That reboot can still wipe the whole saved state through the daemon's final flush. Every unanswered waiting pane, and every eager pane whose hook is still running, can still lose its session or its scrollback.

The specification promises that no Portal shutdown step is needed. So the user skips `portal uninstall` on the one reboot that still needs it, and finds out when sessions or scrollback are missing afterwards.

There is a second cost. The new `commit-now` logs a drop line for each old-chain session that closes. Read the way the specification reads a post-fix reboot's log, those lines look like macOS killing panes before tmux. They would send the user to the deferred hold for a loss this fix already closes.

**Proposal**:
Record it as accepted residue, and say which reboot's log counts as evidence. Two things in the record settle this:
- the fix's rules live in the daemon and in the restored panes' chains, which an upgrade does not replace, while `commit-now` starts fresh each time;
- the record already names `portal uninstall` first as the safe path until the fix is in effect.

First principles leave one answer for the panes. Portal does not run during `brew upgrade`, and nothing outside a running shell can give it a new trap, so no change can protect panes started before the upgrade.

For the daemon there are two alternatives:
- the daemon replaces itself when its binary changes. That covers only the daemon, and it is new machinery outside this fix's three parts;
- the residue is left unstated. That costs the user the reboot described above and a misread log.

Recording it as residue is accurate, and it asks the user to take care on one reboot.

**Proposed Text**:
New bullet at the end of §5.2:

> - **The first reboot after installing the fix.** The daemon and Portal's resume-hook panes keep running the version they were started with. The daemon moves to the new version only when Portal next bootstraps, and the resume-hook panes only when the next restore rebuilds them. `commit-now` moves at once, because each `session-closed` hook starts it afresh. A reboot taken straight after upgrading, such as one right after `brew upgrade`, keeps the exposure from before the fix. If no bootstrap has replaced the daemon, its final flush can still wipe the saved state, and Portal's resume-hook panes can still lose their sessions and scrollback. Running `portal uninstall` before that reboot, and not running `portal open` again until after it, covers it. The log of that reboot is not the evidence the hold (§5.3) and the instrumented reboot (§5.4) wait on. That evidence comes from a reboot taken after a restore on the fixed version.

**Resolution**: Routed
**Notes**: This session's call (what leaned: the fix lives in processes an upgrade does not replace; the record's uninstall-first procedure; daemon self-replacement set aside as new machinery that still leaves old panes exposed). Landed first in the investigation (Known residue, accepted), then applied to §5.2 as staged.

---

## Observations

- The source records that debug logging and the logging shim shift the race timing: 10ms ×5 kept 5/5 with debug logging on. So the integration test of the 10–30ms window could pass on pre-fix code if run with debug logging. Confirming that the test fails before the fix is the builder's job.
- The source's 2026-09-02 restore left 3 sessions unrestored because `respawn-pane` failed. After the fix, the next save still drops such sessions as ordinary drops, now logged by name, and the sweep reaps their resume hooks. That loss happens at restore, outside this fix's shutdown scope.
- No test mirrors E1b, the daemon tick committing the kill-all empty state with no hook. The rule that a stood-down kill is committed by the daemon's next tick depends on that path.
