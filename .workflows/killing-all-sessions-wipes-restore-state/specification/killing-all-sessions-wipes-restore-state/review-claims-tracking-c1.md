# Review Tracking: Killing All Sessions Wipes Restore State - Claims Verification

## Findings

### 1. At tmux exit a waiting pane's recovery tail never runs, because SIGHUP ends the parked shell first

**Source**: Tree measurement — a `python3` `pty.fork()` of the parked-chain shape `sh -c 'trap : INT QUIT[ TERM]; sleep 3; exit 7'` with the master closed after 0.5s, plus tmux 3.7c `window.c` `window_pane_destroy` (lines 1090–1104)
**Category**: Source defect
**Move**: route
**Affects**: §3.2 A Waiting Pane Stays Waiting Through the Shutdown Signal (final paragraph); §6.2 Panes (fourth bullet)

**Problem**:
The specification says that when tmux finally exits and closes a waiting pane's terminal, the pane's recovery tail runs, tries to clear the waiting marker, and fails because tmux is refusing connections. It also prescribes a test asserting that failed clear. That is not what happens. Closing the pane's terminal sends SIGHUP to the parked shell. That shell catches only INT and QUIT, plus TERM once this fix lands, so it dies immediately. The tail after the draw never starts, and no clear is attempted.

What the user gets still holds: the pane is saved still waiting and comes back still asking. But it holds because of the same uncaught SIGHUP that ends a killed pane (the kill section's own mechanism), not because a tmux read is refused. As written, the test can only pass if the parked chain catches SIGHUP so the tail runs. That is the one change that would stop a killed waiting pane from dying at once, which breaks the rule that a kill is final the moment it is made.

**Evidence**:
Claims, verbatim:
- §3.2: "When tmux finally exits and the waiter's pty closes, the recovery tail tries to clear the marker. That cannot reach a server that is refusing connections (§2.2), so the saved record keeps the pane waiting."
- §6.2: "A waiting pane whose pty closes because tmux has begun exiting: the recovery tail's marker clear fails against the refusing server, and the saved record keeps the pane waiting."
- These conflict with §3.3: "tmux ends a killed pane by closing its pty, which delivers SIGHUP … Neither the trap (§3.1) nor the waiting pane's handling (§3.2) catches SIGHUP. So a killed pane, eager or waiting, still dies at once". tmux exit closes the pane's pty the same way a kill does.

Measurement. `python3`, which writes no files: `pty.fork()` makes the child the pty's session leader, as tmux's pane child is. The child runs `execv('/bin/sh', ['sh','-c', SCRIPT])`. The parent sleeps 0.5s, closes the master fd and calls `waitpid`. If the tail ran, the child would exit with status 7.
```
trap INT QUIT      : killed by signal 1 (SIGHUP)     # SCRIPT = trap : INT QUIT; sleep 3; exit 7
trap INT QUIT TERM : killed by signal 1 (SIGHUP)     # SCRIPT = trap : INT QUIT TERM; sleep 3; exit 7
```
Today's parked chain is `const parkedChainTrap = "trap : INT QUIT; "` (`cmd/state_hydrate.go:280`), and the draw runs as its foreground child (`parkedResumeChain`, `cmd/state_hydrate.go:324-328`).

tmux 3.7c tears panes down at server exit through the kill path. The SIGTERM handler runs `server_exit = 1; server_send_exit();` (`server.c:439-440`). `server_send_exit` runs `session_destroy` for every session, and `session_destroy` calls `winlink_remove` for each window. From there it is `window_remove_ref` → `window_destroy` → `window_destroy_panes` → `window_pane_destroy`, which ends in `close(wp->fd)` (`window.c:1104`). Across the 3.7c sources, no `kill()` targets a pane process other than SIGCONT to a stopped child (`server_child_stopped`).

Source: the investigation carries both statements. One is in Fix Direction → Chosen Approach, item 2, sub-bullet "A waiting pane stays waiting through the shutdown signal", final sentence: "When tmux finally exits and the waiter's pty closes, its recovery tail's marker clear cannot reach a server that is refusing connections, so the saved record keeps the pane waiting." The other is in Testing Recommendations: "A waiting pane whose pty closes because tmux has begun exiting: the recovery tail's marker clear fails against the refusing server and the saved record keeps the pane waiting."

**Proposed Text**:

**Resolution**: Pending
**Notes**:

---

## Observations

- §2.1's recorded result for `rg -n 'ListSessionNames\(\)' internal/state/capture.go internal/restore/restore.go` lists two hits. The command returns three, because `capture.go:20` (the `CaptureClient` interface declaration) also matches.
- §2.1 says "every reader shares one session listing that swallows the failure". In fact the search form's count already reads `ListSessionsProbe` (`cmd/open_search.go:178`). The picker, resolver, completion and restore do read the swallowing listing, as stated.
- §2.2's second damage path (an empty `list-panes` beside an environment read that still succeeds) cannot come from tmux's shutdown answer. In tmux 3.7c, `CLIENT_EXIT_SHUTDOWN` is set only in `server_send_exit`, which is called only right after `server_exit = 1`. So the `show-environment` reads that follow are refused. They are classified anomalous, because the classifier matches only "no such session" or "can't find session", and the all-sessions-failed guard then errors. The pane-listing half of §6.1's third bullet therefore already passes on today's code. The investigation carries the same claim (H6, reach (a)).
- §6.6 says the capture subtest "asserts an error the production client never delivers". Production `ListSessionNames` can return a parse error (`parseSessionList`, "unexpected session format"). It never returns a failed-command error.
