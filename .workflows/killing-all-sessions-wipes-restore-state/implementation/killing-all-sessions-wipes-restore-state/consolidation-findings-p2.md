# Consolidation Findings: Killing All Sessions Wipes Restore State (Phase 2)

## Findings

None. The phase's production surface is small and already consolidated:
- Every hook hand-off goes through `hookExecArgs`, so the eager pane, the answered lazy pane and the recovered pane share one composition.
- The two shell gates on the pending marker share `paneStillPending`.
- The draw and the waiter install one `catchSIGTERM`.
- `Commit` reads the prior index once (`readPriorIndex`), and both the change test and the drop log use that one read.
- The new test harnesses keep every tmux call on a PATH stub or a `tmuxtest` socket, and signal only processes walked down from their own panes. A local run of the process-level suites (`-count=3`) passed.

## Comment Corrections

- `CLAUDE.md:192` (project guide prose, Resume hooks): this sentence still quotes the hook shell as it was before Task 2-1. Folds the bank entries from 2-1 (two), 2-3 and 2-4.
  OLD: exec's `sh -c '<HOOK>; exec $SHELL'` as it always has, and a pane with no registration a bare `$SHELL`;
  NEW: exec's `sh -c 'trap : TERM; <HOOK>; exec $SHELL'` — the trap keeps the pane through a shutdown's SIGTERM while the hook runs, and the same shell runs the hook of a lazy pane answered on its panel — and a pane with no registration a bare `$SHELL`;

- `CLAUDE.md:192` (project guide prose, Resume hooks): three things are now out of date here.
  - Task 2-3 added TERM to the parked chain's trap.
  - Task 2-4 wrapped the draw in a loop that redraws after a SIGTERM.
  - Task 2-2 made the draw and the waiter catch SIGTERM.

  None of this appears. A reader who takes the documented argv as the shape to keep would drop the handling that keeps a waiting pane's transcript through a reboot. Folds the same bank entries.
  OLD: and the helper then exec's `/bin/sh -c 'trap : INT QUIT; <draw argv>; <recover argv>; <backstop>'`, a parked shell running the panel's draw followed by the chain's tail — the trap keeps the parked shell alive through a group interrupt or quit so the tail still runs.
  NEW: and the helper then exec's `/bin/sh -c 'trap : INT QUIT TERM; while <draw argv>; [ $? -eq 143 ] && <pane still pending>; do :; done; <recover argv>; <backstop>'`, a parked shell running the panel's draw followed by the chain's tail — the trap keeps the parked shell alive through a group interrupt or quit so the tail still runs, and through a shutdown's SIGTERM so the pane keeps its session until tmux itself exits. The draw and the waiter it becomes catch SIGTERM too (`catchSIGTERM`), so a waiting pane is saved still waiting; a catch does not survive `exec`, so a draw or waiter that a SIGTERM ends before it has installed its catch is started again while the pane's `@portal-resume-pending` reads set (a failed read counts as set). Each of these is a caught trap, never an ignore, so the hook program and the user's shell start with the default SIGTERM; none catches SIGHUP, so a kill or tmux's own exit still ends the pane before the tail can start.
