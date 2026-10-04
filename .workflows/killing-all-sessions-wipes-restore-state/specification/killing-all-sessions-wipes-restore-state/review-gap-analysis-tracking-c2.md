# Review Tracking: killing-all-sessions-wipes-restore-state - Gap Analysis

## Findings

### 1. The unchanged-by-design list says a kill is final at once, but the accepted residue lets a killed session come back

**Source**: Specification analysis
**Category**: Contradiction
**Move**: settled
**Priority**: Important
**Affects**: §5.1 Unchanged by design (colliding with §5.2 Accepted residue and the §2 lead-in)

**Problem**:
The list of things the fix leaves unchanged says two things about a kill: it is final the moment it is made, and the synchronous save the `session-closed` hook runs removes the killed session, its scrollback and its resume hooks. The accepted-residue list says otherwise for one case. When that save backs off, because its session listing fails or tmux starts exiting while it runs, nothing removes the kill until the daemon's next tick. If tmux exits before then, the killed session is still in the saved state and comes back at the next restore. The save-path section also says every new back-off rule applies to all three committers, the kill's save included.

A builder or tester who reads the unchanged-by-design line as the stronger promise has two ways to make it true. Both hurt the user:
- They could exempt the kill's save from the new back-off rules. A kill save that runs as tmux exits, with its listing failed, then commits an empty saved state and deletes every scrollback file. That is the wipe this fix exists to stop.
- They could have a backed-off kill save remove the closed session by name anyway. That changes the kill path, which the fix leaves alone.

A tester holding the unchanged-by-design line would also expect a kill to be gone from the saved state at once, which a transient back-off on a healthy server legitimately delays by one tick.

**Proposal**:
Reword the kill bullet in the unchanged-by-design list so it names the one-tick delay and the one exception. Two decisions the specification already makes settle this. Every save-path rule applies to the kill's save. A kill whose save backs off just before tmux exits is accepted residue. Only one wording is open: dropping or narrowing the residue would reopen a decision already settled, and leaving the bullet as it is keeps the two readings above available. The rest of the bullet, about killing every session, stays as written.

**Current**:
- **A kill is final the moment it is made (§1.1).** The `session-closed` hook still runs `commit-now` synchronously, and it removes the killed session, its scrollback and, through the hook-staleness sweep, its resume hooks. Killing every user session still ends with an empty restore state. That empty state is committed when the last user session closes while tmux keeps running for Portal's own `_portal-saver` and `_portal-bootstrap`.

**Proposed Text**:
- **A kill is final as soon as a save commits it (§1.1).** The `session-closed` hook still runs `commit-now` synchronously, and it removes the killed session, its scrollback and, through the hook-staleness sweep, its resume hooks. When that `commit-now` stands down (§2.5), the daemon's next tick commits the kill instead. A kill that no save commits before tmux exits is accepted residue (§5.2). Killing every user session still ends with an empty restore state. That empty state is committed when the last user session closes while tmux keeps running for Portal's own `_portal-saver` and `_portal-bootstrap`.

**Resolution**: Approved
**Notes**: Determined by the spec's own decisions (save-path rules apply to the kill's save; the kill-stand-down residue in §5.2). Applied to §5.1 as staged.

---

## Observations

- If a confirmation gets tmux's shutdown answer (exit 0, no output), its output cannot show which server answered it. The rule that only the committer's own server counts then leaves the builder to choose between committing and standing down. Either choice leaves the user served: the difference is at most one tick of freshness, and a kill's save is already covered by the accepted residue.
- The accepted-residue list opens by calling its items "shutdown losses". The item about a kill whose save stands down is neither a loss nor limited to shutdown.
- A resume hook whose command is itself a shell construct, such as a retry loop or builtins, runs inside the hardened shell. Those shell-level parts catch SIGTERM rather than taking the default. At shutdown this only keeps the session alive, which is the intended outcome.
- An eager pane's hand-over to the user's shell leaves a brief window. The exec has reset the caught SIGTERM to default, and the interactive shell has not yet started ignoring it.
