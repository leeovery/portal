# Multiple `portal state daemon` instances run concurrently and pin the tmux server

Reproduced today against a long-running tmux server (uptime ~10 days, ~20 sessions, several panes each). The user-visible complaint that opened the investigation was severe sluggishness across the entire tmux server: TUI redraws, prefix keystrokes, and `tmux ls` itself were all multi-second slow. The reporter is running many Claude sessions concurrently inside a single tmux server, so the pane count is high and several sessions have accumulated large scrollbacks (top sessions: pigeon-ekSUL0 ~93 MB, fabric-Cja82m ~82 MB, fabric-lk26UG ~58 MB, codeintel-54Jd4X ~56 MB, evvi ~50 MB, several more in the 5–25 MB range).

At investigation time, **seven `portal state daemon` processes were running simultaneously**, all parented to the same tmux server (PID 94966):

```
25482 (14:20 elapsed)   35467 (12:00)   46188 (09:40)
72062 (06:01)           79560 (04:15)   82148 (03:58)
82962 (24:38)
```

Three of those (72062, 79560, 82148) appeared *during* the investigation, after the older four had been targeted. Whatever startup/recovery path produces them is still active — sending `SIGSTOP` to one of the daemons did not keep it stopped (within seconds it returned to the `S` state in `ps`), and pausing all four original daemons together did not reduce the tmux CPU load nor stop new daemons from being created.

While the daemons were running, `ps` showed 4–7 live `tmux capture-pane -e -p -S -` invocations every single second, each one a child of one of the daemons. Sampling the tmux server (PID 94966) for 5 seconds showed 100% of CPU time spent in `cmd_capture_pane_exec` → `grid_string_cells` → `grid_string_cells_add_code`. With 7 daemons each running their own per-tick capture loop, the tmux server saw roughly 7× the capture load it would see from a single daemon. Across ~20 panes that totalled in the order of 140 `capture-pane` invocations per second against full-scrollback panes; the sample showed the server pegged at 75–98% CPU continuously and the system load average was 5–10 throughout.

The reporter also observed parallel side effects worth noting alongside this:

- All seven daemons appear in the same `_portal-saver` tmux session (the user noticed only one `_portal-saver` session in `tmux ls`, yet seven daemon processes are children of the server).
- Three `sh -c 'portal state hydrate …; exec $SHELL'` wrappers from a previous bootstrap (~20 hours old) are still alive. On closer inspection they are not stuck hydrate processes: hydrate has long since exited and the wrapper is parked waiting on its child interactive shell. The trailing `; exec $SHELL` is therefore dead code in practice (unreachable while the user's shell stays alive). Mentioned only as a pattern observed during the same investigation; minor cruft, not load-causing.

Impact while reproduced: every tmux operation across every session was sluggish; load average stayed elevated throughout. Reporter is the only user but the pattern (long-running tmux server, many panes, accumulated scrollback) is plausibly the common case for heavy Portal users over time.

Companion bugs that may share machinery (logged separately, not assumed to be the same defect):

- `.workflows/.inbox/bugs/2026-05-08--killed-sessions-resurrect-on-restart.md`
- `.workflows/daemon-merge-reintroduces-dead-sessions/` (currently in implementation)
