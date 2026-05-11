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

**Trigger mechanism still to be confirmed by code analysis.** The inbox report attributed the kill-respawn cycle to a mixed `release/dev` `portal` binary alternation, but the user has clarified they do **not** have two binaries on PATH. So the version-mismatch trigger (if that is even the trigger) fires for a different reason — possibly the `dev/dev` or empty-stored cases in `portalSaverVersionMismatch`, or via a path unrelated to version mismatch entirely.

What is reproducible regardless of trigger:

1. Whenever `EnsurePortalSaverVersion` decides to recycle the saver, it calls `KillSession(_portal-saver)` then immediately `BootstrapPortalSaver` — no aliveness barrier.
2. If the killed daemon is mid-`tick()` (1.5–4 s on the observed scrollback profile), the old daemon stays alive while the new one is already running.
3. Over enough invocations, daemons accumulate.

**Reproducibility:** Single observation so far (2026-05-09). Conditions for accumulation are structural (the bootstrap race), so any repeat of the trigger reproduces.

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
3. **Some path is firing the saver-recycle cycle frequently enough that orphans accumulate.** The inbox file blamed mixed release/dev binaries on PATH but the user has none — so we need to identify what actually triggers the kill-respawn cycle. Candidates: empty stored version on disk, `dev`/empty current version embedded in the binary, parallel `portal` invocations stomping each other, or non-version-mismatch code paths that also call `KillSession(_portal-saver)`.

(1) and (2) are the structural defects — they make accumulation possible. (3) determines frequency. The fix focuses on (1)+(2); (3) is diagnostic context.

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
