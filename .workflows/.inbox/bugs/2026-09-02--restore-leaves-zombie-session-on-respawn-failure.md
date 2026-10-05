# Restore leaves a dead zombie session behind when respawn-pane fails during the arm phase

On the 2026-08-21 boot restore, one of 46 saved sessions (`fab-apian-n8n`) failed to arm. The log line was:

```
WARN restore: restore session failed session=fab-apian-n8n error=session "fab-apian-n8n": arm pane fab-apian-n8n:1.1: failed to respawn-pane "fab-apian-n8n:1.1": tmux respawn-pane -k -t fab-apian-n8n:1.1 portal state hydrate --fifo '…' --file '…' --hook-key 'oeozVu:1.1': exit 1: respawn pane failed: fork failed: Device not configured
```

Two seconds later bootstrap reported `orchestration complete steps=10 warnings=0`, so nothing reached the user. The skeleton session had already been created before the arm phase ran, and it was left on the server as-is: one pane whose original shell had been killed by `respawn-pane -k` and whose replacement never started. tmux described the pane as `pane_pid=-1`, `pane_dead=0`, empty `pane_current_path`, `pane_current_command=portal`, with `pane_start_command` still holding the full hydrate argv. No hydrate process existed, and by the time it was examined twelve days later the FIFO and the `@portal-skeleton-*` marker for it were both gone.

The symptom the user saw was `portal open fab-apian-n8n` attaching to a blank, inert session — no shell, no scrollback, no resume hook fired — with no indication anything was wrong. The session appeared normal in the picker and in `tmux list-sessions`.

The knock-on cost is worse than the blank pane. The daemon kept capturing the dead session on every tick as a live one: `sessions.json` records it with `cwd: ""` and `current_command: "portal"`, and `scrollback/fab-apian-n8n__1.1.bin` was rewritten down to 52 bytes. The scrollback that existed before the reboot is gone. The resume hook entry in `hooks.json` and the Claude transcript survive, so the Claude session itself is recoverable, but the tmux-side history is not.

This is the only `fork failed` in the logs from 2026-08-03 onward, so the trigger is rare, but the handling is what matters: `restoreOne` in `internal/restore/restore.go` logs the warning and returns `false`, and `armPanes` in `internal/restore/session.go` returns the error with the session still standing. There is no cleanup of the half-armed skeleton, no retry, no surfacing through the bootstrap warning channel, and no protection against the daemon overwriting the good saved state for that session with the empty one.

A separate observation from the same session: running `tmux respawn-pane -k` manually against that `pane_pid=-1` pane on 2026-09-02 brought the whole tmux server down (all 48 sessions, the daemon, and every Claude process in them). That may be a tmux 3.7c matter rather than Portal's, but it means a session left in this state is actively dangerous to touch, not merely broken.

## Recurrence — 2026-10-05

Third recorded occurrence: 2026-08-21 (one session, above), 2026-09-02 (three sessions missing from that boot's restore, noted in the `killing-all-sessions-wipes-restore-state` investigation), and now 2026-10-05, the first boot restore under 0.12.1 with lazy resume. Two of 44 sessions failed to arm with the identical `fork failed: Device not configured`: `laravel-workflow-PGR3jA` and `switchboard-4dthH2`, both carrying resume hooks. Bootstrap again finished `warnings=0`; the only trace was the two `restore session failed` WARN lines. The other 42 sessions armed and went into the lazy waiting panel normally.

Both panes were left `pane_pid=-1` with their `@portal-pane-id` tokens stamped and no `@portal-skeleton-*` marker. Within the daemon's first save both scrollback files were rewritten to 24 bytes of newlines; the pre-reboot transcripts (4.5 KB and 3.3 KB) were recoverable only from a Time Machine backup. In the picker's preview the sessions showed no scrollback, and in tmux they were inert panes with no resume panel. The hooks and Claude transcripts survived.

The respawn hazard reproduced as well. `tmux respawn-pane -k -t =switchboard-4dthH2:1.1 '<the hydrate argv restore uses>'` against the `pane_pid=-1` pane — with its skeleton marker set and FIFO created beforehand — made the server exit at once (`server exited unexpectedly`, no crash report under DiagnosticReports), taking all 44 sessions and every Claude process in them down. Portal's saved state survived that exit: the daemon's SIGHUP flush logged `read @portal-restoring at shutdown failed; skipping final flush`, and the next `x` restored all 44 sessions, both previously failed ones included, with no arm failure.
