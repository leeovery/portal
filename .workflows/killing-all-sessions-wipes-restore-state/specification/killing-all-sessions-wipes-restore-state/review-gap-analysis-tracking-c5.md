# Review Tracking: killing-all-sessions-wipes-restore-state - Gap Analysis

## Findings

### 1. A lazy pane the user has answered is left out of the SIGTERM hardening

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Move**: settled
**Priority**: Important
**Affects**: §3.1 The restored resume-hook panes survive SIGTERM; §6.2 Panes (§3)

**Problem**:
Lazy is the shipped default, so after a restore most resume-hook panes wait on their panel until the user answers them. When the user presses Enter, the panel hands the pane to its hook program through the same `sh -c '<hook>; exec <shell>'` shell the eager pane runs, and that shell sits beneath the parked chain. The hardening names only two cases: the eager pane, and the pane while it waits. Its tests cover only a SIGTERM sent to the pane's top process, and whether the trap is inherited. Nothing says an answered pane's hook shell must survive SIGTERM, and no test catches it if it doesn't.

A builder could harden the eager pane at the point where it is launched rather than in the shell both routes share. That leaves every answered lazy pane as exposed as it is today. At a reboot, SIGTERM ends that shell. The parked chain's tail then sees the pane was already answered and does nothing, so the chain ends and the pane closes while tmux is still answering. The `session-closed` commit removes the session and deletes its scrollback, and on the next start the hook-staleness sweep deletes its resume hook. These are the panes the user resumed and is actively working in, with the hook program still running. They are the sessions most worth keeping, and after the reboot they are simply gone.

**Proposal**:
State that an answered lazy pane's hook shell also survives SIGTERM, and add a test for it. The record settles this:
- The governing rule makes every session that wasn't killed restorable.
- The specification names resume-hook sessions as the most exposed.
- An answered pane runs the exact shell the specification hardens for the eager pane.

The alternative is to record answered lazy panes as accepted residue. That leaves the normal working state of the default mode's resume-hook panes exposed, so no informed user would choose it. Where the trap is installed is the builder's choice.

**Proposed Text**:
§3.1, new paragraph after "Lazy is the shipped default resume mode, so after a restore every unanswered pane runs as the parked chain.":

A lazy pane the user has answered runs its hook through that same eager shell, `sh -c '<hook>; exec <shell>'`, beneath its parked chain. That shell survives SIGTERM there too. So an answered pane whose hook program is still running keeps its session, and the pane goes on to the user's shell when the hook program ends.

§6.2, new bullet after "Restored eager and lazy resume-hook panes survive a SIGTERM to the pane's top process. …":

- A lazy pane answered on its panel, with its hook program still running, keeps its session when SIGTERM reaches every process in the pane. Its parked chain and the shell running its hook both survive, and the pane goes on to the user's shell.

**Resolution**: Approved
**Notes**: Record's own: the investigation hardens the eager shell shape, and an answered lazy pane runs that shape (measured: `handOffToHookOrShell` composes it from both `cmd/state_hydrate.go:251` and the waiter's answer path, `cmd/state_resume_wait.go:316`). Applied to §3.1 and §6.2 as staged.

---

## Observations

- `portal kill` is not named among the kill routes in the governing rule. It ends a session through tmux's own kill, so the same rule covers it.
- The shutdown-ordering trials are specified as "many", with no figure given.
- A commit whose prior on-disk index is missing or unreadable has nothing to measure drops against. Logging nothing in that case still serves the reading of a reboot log.
- The confirmation's proof rests on properties of tmux 3.7c. Nothing states what Portal assumes on another tmux version.
