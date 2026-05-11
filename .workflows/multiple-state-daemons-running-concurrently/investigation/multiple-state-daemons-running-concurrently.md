# Investigation: Multiple State Daemons Running Concurrently

## Symptoms

### Problem Description

**Expected behavior:**
Exactly one `portal state daemon` process runs per tmux server lifetime. Bootstrap-driven kill+respawn cycles (e.g. version mismatch upgrades) produce a clean handover: the old daemon exits before the new one is allowed to start writing to shared state. Per-tick CPU cost stays bounded so the daemon does not pin the tmux server.

**Actual behavior:**
Up to **seven** `portal state daemon` processes were observed simultaneously, all parented to the tmux server (PID 94966). Only one was inside the `_portal-saver` session; the other six were past-lifecycle orphans whose owning session had already been killed but whose `cancel()` had not yet been observed by their in-flight `tick()`. While 2+ daemons ran, the tmux server stayed pegged at 75–98% CPU; `sample` showed all time in `cmd_capture_pane_exec` → `grid_string_cells` → `grid_string_cells_add_code`.

### Manifestation

- Severe sluggishness across the entire tmux server: TUI redraws, prefix keystrokes, and `tmux ls` itself were multi-second slow.
- Load average sustained at 5–10 during the observation window.
- `ps` snapshots once per second showed 4–7 concurrent `tmux capture-pane -e -p -S -` child processes (one per running daemon mid-sweep).
- With zero daemons running, tmux server CPU dropped to 0–22% and capture-pane processes dropped to zero.

### Reproduction Steps

Indirectly reproducible — requires the version-mismatch trigger to fire repeatedly during a busy window. Concrete trigger:

1. Have both `/opt/homebrew/bin/portal` (released, e.g. v0.3.1) and `/Users/leeovery/Code/portal/portal` (dev build) on PATH.
2. Run `portal` commands that alternate between the two binaries (shell aliases, completions, hooks, manual invocations).
3. Each alternation triggers `EnsurePortalSaverVersion` → `KillSession(_portal-saver)` → immediate `BootstrapPortalSaver` → new daemon spawn while the killed daemon is still mid-sweep (1.5–4 s window).
4. Over a 10-day uptime with frequent invocations and large scrollbacks, daemons stack.

**Reproducibility:** Reliably reproducible in conditions matching (1)+(2); the 7-daemon snapshot was the natural outcome of those conditions over real usage.

### Environment

- **Affected environments:** Any tmux server lifetime where the version-mismatch path is taken repeatedly. Empirically observed on macOS (`darwin`), but the root cause is platform-independent.
- **User conditions:** Mixed `release/dev` binaries on PATH; long-running tmux server; many panes with substantial scrollback.

### Impact

- **Severity:** High. The whole tmux server (all sessions, not just portal-managed ones) becomes unusable for sustained periods.
- **Scope:** Any user running portal long enough to accumulate orphan daemons; worst with mixed release/dev binaries.
- **Business impact:** N/A (developer tool); user-experience degradation severe.

### References

- Inbox file (archived): `.workflows/.inbox/.archived/bugs/2026-05-09--multiple-state-daemons-running-concurrently.md`
- Related (separate work units, may share machinery but not assumed identical):
  - `.workflows/killed-sessions-resurrect-on-restart/` (active, in implementation)
  - `.workflows/daemon-merge-reintroduces-dead-sessions/` (completed)

---

## Analysis

### Initial Hypotheses

1. **Bootstrap does not wait for the killed daemon to exit before spawning a replacement.** Read from `portal_saver.go:106-114` — `KillSession` then immediate `BootstrapPortalSaver` with no aliveness barrier.
2. **No singleton lock.** `state_daemon.go:226` writes `daemon.pid` informationally only; `BootstrapAliveCheck` (`portal_saver.go:37`) signal-0-probes only the *current* `daemon.pid` and cannot see prior orphans whose PID has been overwritten.
3. **Version-mismatch trigger fires too readily.** `portalSaverVersionMismatch` (`portal_saver.go:120-131`) returns true for any of: read error, empty stored, empty current, `dev` on either side, or any string mismatch. Mixed release/dev binaries on PATH cause every alternation to trip the kill-respawn cycle.

To validate against current code.

### Code Trace

To be filled in during Step 5 (Code Analysis). Initial entry points to trace:

- `portal_saver.go` — `BootstrapPortalSaver`, `EnsurePortalSaverVersion`, `BootstrapAliveCheck`, `portalSaverVersionMismatch`.
- `cmd/state_daemon.go` — daemon main loop (`defaultDaemonRun`, `tick`, signal handler, pidfile write).
- `internal/state/capture.go` (or equivalent) — `captureAndCommit`, `CaptureAndHashPane`, `WriteScrollbackIfChanged`.
- `internal/tmux/tmux.go` — `CapturePane` to confirm the `-S -` unbounded scrollback call site.

### Root Cause

To be confirmed after Step 5. Working hypothesis is the conjunction of:

- **(a)** Bootstrap's non-synchronous kill+respawn allows the old daemon to live for the rest of its current `tick()` while a new daemon is already running.
- **(b)** No singleton lock — nothing prevents N daemons from coexisting once (a) has happened.
- **(c)** Topology-churn regime caused by bootstrap itself drives back-to-back sweeps because the cold full-scrollback sweep duration (3.9 s) exceeds the 1 s tick interval, so daemons in this regime never reach the `ctx.Done()` check between sweeps.
- **(d)** Unbounded `capture-pane -S -` makes each sweep expensive in proportion to total cell-grid size, multiplying the cost of (b)+(c).

### Contributing Factors

- Mixed release/dev `portal` binaries on PATH trip the version-mismatch path frequently.
- Long-running tmux server with many large-scrollback panes (cumulative ~28 MB rendered text across 24 panes; top pane `history_bytes` 82 MB).
- Ticker drops missed ticks (Go semantics) so the daemon fires back-to-back when a sweep overruns the interval, never returning to the fast-path idle check.

### Why It Wasn't Caught

To investigate — likely:
- No integration test that runs two bootstrap cycles back-to-back and asserts a single live daemon.
- No load test against a realistic scrollback profile (24 panes, tens of MB rendered).
- Singleton invariant exists only as documentation comment (`portal_saver.go:32`), not as a runtime invariant.

### Blast Radius

**Directly affected:**
- All tmux operations on any session running on the same tmux server (not just portal-managed sessions).
- `_portal-saver` session lifecycle and the resurrection capture loop.

**Potentially affected:**
- Anything that shares the daemon's write path: scrollback `.bin` files, `state.json` atomic commits, FIFO sweep. Concurrent daemons writing the same files could race on atomic-rename semantics (to verify in Step 5).

---

## Fix Direction

To be filled in during Step 6 (Root Cause Synthesis) and Step 8 (Findings Review).

Initial fix surface candidates (from inbox file, to validate):

1. **Singleton flock** at daemon startup; bootstrap acquires-or-fails-fast.
2. **Synchronous KillSession** — poll `BootstrapAliveCheck` against the prior `daemon.pid` until dead, bounded timeout.
3. **Bound `capture-pane -S -<N>`** to cap per-tick cost (measured 13× speedup, 130× less data with `-S -100`).
4. **Tighten `portalSaverVersionMismatch`** so dev/dev and empty/empty cases don't always re-spawn.
5. **Cheaper change-detection** before `capture-pane` (e.g. `display-message -p '#{cursor_x},#{cursor_y},#{history_size}'`) so the hash dedup avoids the expensive call, not just the file write.

(1)+(2) close the structural multiplication. (3) is the high-leverage performance lever even at N=1. (4) reduces frequency. (5) helps further.

---

## Notes

- The bug report explicitly self-corrected several earlier claims (history_bytes ≠ rendered text; daemons not all in `_portal-saver`; hooks were registered the whole time; dirty flag only set on topology events). Trust the corrected version.
- Companion side-observation: 3 stale `sh -c 'portal state hydrate …; exec $SHELL'` wrappers from a ~20-hour-old bootstrap. PaneKeys recur with `killed-sessions-resurrect-on-restart`. Trailing `; exec $SHELL` is unreachable because hydrate has exited and the wrapper is parked on the child shell. Not load-causing — log as separate observation, not part of this fix.
