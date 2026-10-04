# Review Tracking: killing-all-sessions-wipes-restore-state - Input Review

## Findings

### 1. The shutdown-race trials must run without debug logging or a tmux wrapper

**Source**: investigation/killing-all-sessions-wipes-restore-state.md — Analysis → Hypotheses, H4 ("13 of the 22 ran with debug logging or a logging shim, which shift the timing"); Experiments, E5 ("With debug logging on, 10ms ×5 kept 5/5 (logging shifts the timing)") and E7 (the shimmed trials wiped at 15–30ms)
**Category**: Enhancement to existing topic
**Move**: settled
**Affects**: §6.5 Shutdown orderings against real tmux

**Problem**:
The integration test that is supposed to prove a reboot-time SIGTERM can no longer wipe the saved state can pass on unfixed code. The sandbox showed that debug logging and a logging wrapper around `tmux` both move the few-millisecond window in which the wipe happens. With debug logging on, five 10ms trials all kept the full state, while at the default level five 10ms trials all wiped it. An implementer who turns on debug logging to diagnose a flaky run, or wraps `tmux` to check that the confirmation read happened, ends up with a test that cannot fail. The confirmation could then regress without anyone noticing, and users would first see it as sessions and scrollback missing after a reboot.

**Proposal**:
Add the recorded timing caveat to the first shutdown-ordering trial: the trials run at the production default log level, with nothing wrapping `tmux`. The source states both facts: debug logging and a logging shim shift the timing, and debug-logged 10ms trials kept everything where default-level ones wiped everything. Carrying that across is not a new decision.

**Current**:
- The daemon SIGTERMed 10–30ms before the server, repeated across many trials, preserves the full state. Before the fix, this window wiped the whole state in the sandbox.

**Proposed Text**:
- The daemon SIGTERMed 10–30ms before the server, repeated across many trials, preserves the full state. Before the fix, this window wiped the whole state in the sandbox. The trials run at the production default log level, with nothing wrapping `tmux`, because debug logging and a logging shim both shift the timing. In the sandbox, five 10ms trials with debug logging on all kept the full state, while five at the default level all wiped it.

**Resolution**: Approved
**Notes**: The investigation states both facts (H4, E5, E7). Applied to §6.5 as staged.

---

## Observations

- The hydrate helper is Portal's own pane process in every restored pane until it execs the eager or lazy chain. It is not a shell, and nothing hardens it against SIGTERM, so a reboot during that window would remove those sessions. The sources list the helper among the non-shell processes and never consider whether it is exposed. The window is normally sub-second at restore.
- The source says a daemon that replaces itself when its binary changes would narrow the first-reboot residue, and calls it new machinery beyond the fix's three parts. The specification does not say this is out of scope.
- The source names `portal kill <name>` as a kill Portal makes itself. The specification's list of kill routes does not include it. The kill path is unchanged, so nothing depends on the omission.
