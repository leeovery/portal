# Multiple `portal state daemon` instances run concurrently and pin the tmux server

Reproduced today against a long-running tmux server (uptime ~10 days, 20 sessions per `tmux ls`, mostly one window/one pane each). The user-visible complaint that opened the investigation was severe sluggishness across the entire tmux server: TUI redraws, prefix keystrokes, and `tmux ls` itself were all multi-second slow. The reporter is running many Claude sessions concurrently inside a single tmux server. Several sessions have accumulated large scrollbacks measured directly via `list-panes -F '#{history_bytes}'` (top sessions: pigeon-ekSUL0 ~93 MB, fabric-Cja82m ~82 MB, fabric-lk26UG ~58 MB, codeintel-54Jd4X ~56 MB, evvi ~50 MB, several more in the 5–25 MB range; total across all sessions ~450 MB).

At investigation time, **seven `portal state daemon` processes were running simultaneously**, all parented to the tmux server (PID 94966):

```
25482 (14:20 elapsed)   35467 (12:00)   46188 (09:40)
72062 (06:01)           79560 (04:15)   82148 (03:58)
82962 (24:38)
```

Three of those (72062, 79560, 82148) were observed appearing *during* the investigation, after `SIGSTOP` had been sent to four of the older ones. The four that were SIGSTOP'd did not stay stopped — within seconds they were back in `S` state in `ps` rather than `T`. Pausing all four originals did not reduce tmux CPU load nor prevent the additional three from spawning. The mechanism that resumed the stopped daemons and that spawned the new ones was not directly observed.

While the daemons were running, `ps` snapshots taken once per second showed 4–7 concurrent `tmux capture-pane -e -p -S -` processes at every snapshot, each one a child of one of the daemons. Sampling the tmux server (PID 94966) for 5 seconds showed 100% of CPU time spent in `cmd_capture_pane_exec` → `grid_string_cells` → `grid_string_cells_add_code`. The sample showed the server pegged at 75–98% CPU continuously and the system load average was 5–10 throughout.

Side observations made during the same window, not directly tied to the load:

- Three `sh -c 'portal state hydrate …; exec $SHELL'` wrappers from a previous bootstrap (~20 hours old) were still alive (paneKeys: agentic-workflows-XXrJ3J__1.1, leeovery-Gi5NLG__1.1, leeovery-feqhpg__1.1). On inspection (`ps -axo pid,ppid,…` showed each of the three wrappers parents an interactive `/bin/zsh`) hydrate has already exited and the wrapper is parked waiting on its child interactive shell. The trailing `; exec $SHELL` in the wrapper is therefore unreachable in practice. Not load-causing — minor cruft, mentioned because the same paneKeys recur in `2026-05-08--killed-sessions-resurrect-on-restart.md`.

Impact while reproduced: every tmux operation across every session was sluggish; load average stayed elevated throughout.

## Addendum — what happened after SIGTERM

The reporter `kill -TERM`'d all seven daemons. Subsequent observations:

- Within ~9 minutes, two new `portal state daemon` processes existed (PIDs 22734, 48924), parented to the tmux server. The `_portal-saver` tmux session was no longer present in `tmux ls`. How and where the two new daemons were spawned was not directly observed.
- With those two daemons running, the tmux server **stayed pegged at 75–97% CPU**. A fresh 5-second sample showed the same call graph as the seven-daemon sample: 100% in `cmd_capture_pane_exec` → `grid_string_cells` → `grid_string_cells_add_code`.
- Some minutes later, both of those daemons had also exited (PIDs no longer present in `ps`). With zero daemons running, tmux server CPU dropped to 0–22% across a 3-second `top -l 3` sample, and `ps aux | grep capture-pane` showed zero capture-pane processes for 5 consecutive 1-second samples. Why those two exited was not directly observed.

The "two daemons → still pegged" data point indicates the load is not solely a function of daemon multiplicity. A single capture-pane call against a 50–90 MB scrollback is non-trivial work for tmux; the daemon is firing them per pane per tick (1s) with `-S -` (full scrollback, no size cap, no incremental delta). Estimated rate from configuration alone: 20 panes × 2 daemons × 1 tick/sec = 40 capture-pane invocations/sec, each formatting a full pane scrollback. The exact instantaneous rate was not measured directly; only the in-flight concurrency count (4–7 with seven daemons; 0–2 with two daemons; 0 with zero daemons).

Whether this remains a problem after a singleton-lock fix lands is unverified — it would need re-measuring against a single-daemon configuration with the same scrollback profile.

## Related bugs

Companion bugs that may share machinery (logged separately, not assumed to be the same defect):

- `.workflows/.inbox/bugs/2026-05-08--killed-sessions-resurrect-on-restart.md`
- `.workflows/daemon-merge-reintroduces-dead-sessions/` (currently in implementation)
