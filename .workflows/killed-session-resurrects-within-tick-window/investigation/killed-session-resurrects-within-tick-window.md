# Investigation: Killed Session Resurrects Within Tick Window

## Symptoms

### Problem Description

**Expected behavior:**
After a tmux session is killed (via tmux's `M-q kill-session` keymap, the user's `Option-Q` binding, or Portal's TUI `K` confirm flow), a subsequent `portal` invocation must not show that session in the Sessions list and must not reconstruct it as a skeleton pane in tmux. The kill should be authoritative and immediate from the user's point of view.

**Actual behavior:**
For roughly 2–5 seconds after the kill, a subsequent `portal` invocation:
- Still lists the killed session in the TUI Sessions page.
- Triggers bootstrap step 5 `Restore` to reconstruct the session in tmux as a skeleton pane (`pane_start_command` shows `portal state hydrate ...`, matching `internal/restore/session.go` / `internal/restore/restore.go`).

After ~5 seconds the session disappears from both the list and tmux and stays gone — i.e. eventual consistency on the order of one daemon tick.

### Manifestation

- Session appears in `portal` Sessions list after the user explicitly killed it.
- Same session is observable as a freshly-created skeleton pane in tmux via `tmux list-panes -a -F '#{pane_start_command}'` showing `portal state hydrate ...`.
- Window/pane geometry of the resurrected session matches the pre-kill saved skeleton, not whatever the user had open at kill time.
- After ~5s the same `portal` invocation produces a session list without the killed session and tmux is quiet.

### Reproduction Steps

1. Have at least one Portal-managed tmux session attached.
2. Kill it via any of the three paths:
   - `Option-Q` (user keybind),
   - tmux's `M-q` binding to `kill-session`,
   - Portal TUI: select session, press `K`, confirm.
3. Within ~2 seconds, run `portal` (or `x`).
4. Observe the killed session present in the Sessions list and reconstructed in tmux as a skeleton pane.
5. Wait until at least ~5 seconds have elapsed from the kill, run `portal` again.
6. Observe the session gone from the list and absent from tmux.

**Reproducibility:** Always, given the timing window.

### Environment

- **Affected environments:** Local — Portal 0.5.0 on the user's primary development machine.
- **Browser/platform:** N/A (CLI/TUI). macOS, tmux backend.
- **User conditions:** Single-user-per-machine. State directory has clean `@portal-skeleton-*` markers (verified — `tmux show-options -s | grep @portal-skeleton` is empty), and previously-affected sessions have fresh scrollback `.bin` files. Sibling, not regression, of `daemon-merge-reintroduces-dead-sessions` and `killed-sessions-resurrect-on-restart`, which both targeted the stale-marker class — that class is observably resolved here.

### Impact

- **Severity:** High (trust-tier). User-visible "I killed this, and Portal brought it back" symptom on the same surface as two recent resurrection-class bugfixes.
- **Scope:** Any user who kills a session and reopens Portal within the race window. Single-user product so no multi-user concurrency angle.
- **Business impact:** Trust regression against recently-shipped resurrection fixes.

### References

- Recently-shipped sibling fixes: `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart`.
- Implicated paths called out by the user:
  - tmux global hook `session-closed` → `portal state notify` (`cmd/state_notify.go`).
  - Daemon tick loop owning `sessions.json` rewrites (`cmd/state_daemon.go`).
  - Bootstrap step 5 `Restore` reading `sessions.json` on every Portal invocation (`internal/restore/restore.go`, `internal/restore/session.go`).

---

## Analysis

### Initial Hypotheses

- The kill-side path (`session-closed` hook → `portal state notify`) is fire-and-forget against the daemon. `sessions.json` is rewritten on the daemon's tick, not synchronously with the kill — so any Portal invocation between kill and the next tick observes a `sessions.json` that still lists the dead session, and `Restore` faithfully reconstructs it.
- The user has explicitly stated that the fix direction must be **synchronous at the kill-side path** (commit the persistence change before returning), not timeouts / tick-rate adjustments / retry tuning.

### Code Trace

_(to be filled during Step 5)_

### Root Cause

_(to be filled during Step 6)_

---

## Fix Direction

_(to be filled during Step 8)_

---

## Notes

- User directional preference recorded up front: synchronous kill-side persistence, not eventual consistency. Avoid timeouts / retry tuning / tick-rate as mitigation.
