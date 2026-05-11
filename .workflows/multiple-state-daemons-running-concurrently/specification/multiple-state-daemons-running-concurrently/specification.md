# Specification: Multiple State Daemons Running Concurrently

## Specification

## Problem Statement

### What's broken

Up to seven `portal state daemon` processes were observed running concurrently against the same tmux server, all parented to the tmux server PID. Only one was the daemon hosted inside the `_portal-saver` session; the other six were past-lifecycle orphans — their owning session had been killed, but their in-flight `tick()` had not yet observed the cancelled context.

Expected behaviour: **exactly one** `portal state daemon` process per tmux server lifetime. Bootstrap-driven kill+respawn cycles (e.g. version-mismatch upgrades) should produce a clean handover — the old daemon exits before the replacement starts writing to shared state.

### Impact

- **Severity: High.** When 2+ daemons run concurrently the tmux server pegs at 75–98% CPU. The cost is in `capture-pane -e -p -S -` (unbounded scrollback) called by each daemon's per-pane sweep — `sample` shows all time in `cmd_capture_pane_exec` → `grid_string_cells` → `grid_string_cells_add_code`.
- **Scope: server-wide.** All tmux operations on the affected server become sluggish — not just portal-managed sessions. TUI redraws, prefix keystrokes, and `tmux ls` itself become multi-second-slow. Load average sustained 5–10 during the observation window.
- **Shared-state corruption.** Multiple daemons writing the same state directory: `sessions.json` (atomic per-commit but two daemons race the read-then-commit window, producing flip-flop content across consecutive ticks), per-pane scrollback `.bin` files (`AtomicWrite` is per-call atomic but the two writers can interleave content versions), and `daemon.pid`/`daemon.version` markers become incoherent — `BootstrapAliveCheck` becomes meaningless once N > 1.

### Trigger frequency

Trigger frequency is **incidental to the bug.** The observed accumulation (~7 orphans over 10 days uptime) is consistent with low-frequency events compounding — stale pidfile recovery, manual session kills, brew upgrades, daemon crashes mid-tick. The version-mismatch path is dormant on a normal user with matching real semver on both `portal version` and `daemon.version`.

The structural defects (covered in Root Cause) make any kill-respawn trigger — high or low frequency — capable of producing an orphan. The fix closes the race regardless of how often the trigger fires.

### Recovery for affected users

Workaround: `tmux kill-session -t _portal-saver` followed by `pkill portal` (or `tmux kill-server` for the heaviest impact). No hotfix needed — likely few users are affected at the observed accumulation rate of roughly one orphan per 34 hours of tmux uptime.

## Root Cause

The bug is the conjunction of **two structural defects** in the saver-bootstrap and daemon-startup pair. Neither alone produces the accumulation; together they make it inevitable on every kill-respawn cycle.

### Defect 1: No singleton enforcement

`state.WritePIDFile` and `BootstrapAliveCheck` cooperate as an **informational pidfile**, not as a singleton lock:

- `cmd/state_daemon.go:226` — `state.WritePIDFile(dir, os.Getpid())` is called by every starting daemon, **before** the tick loop begins. Implementation in `internal/state/daemon_state.go:33-36` is `fileutil.AtomicWrite` (temp + rename) — last writer wins, no locking.
- `internal/state/daemon_state.go:82-88` — `DaemonAlive` reads `daemon.pid` then `IsProcessAlive` (signal-0 probe). After a new daemon overwrites the file, `DaemonAlive` sees only the new PID — the prior daemon's PID is no longer recorded anywhere and is invisible to any future `BootstrapAliveCheck`. The orphan is now unreachable through portal's bootstrap surface.

Consequence: once two daemons coexist for any reason, nothing in the system prevents both from running concurrent capture loops over the same state directory.

### Defect 2: Bootstrap does not synchronise with the killed daemon's exit

`EnsurePortalSaverVersion` issues `KillSession`, then `BootstrapPortalSaver` immediately observes "no session" and creates a fresh one — **no aliveness barrier between kill and respawn**:

- `internal/tmux/portal_saver.go:106-114` — `EnsurePortalSaverVersion` calls tolerant `KillSession(_portal-saver)` then unconditionally calls `BootstrapPortalSaver`.
- `internal/tmux/portal_saver.go:63-83` — `BootstrapPortalSaver` calls `HasSession(PortalSaverName)`, observes the kill just took effect → false, falls through to `createPortalSaverWithRetry`.
- `internal/tmux/portal_saver.go:138-158` — `createPortalSaverWithRetry` calls `NewDetachedSessionNoCwd(PortalSaverName, "portal state daemon")` which forks-execs the new daemon process as soon as tmux returns from `new-session`.

The new daemon starts and writes its PID before the old daemon — currently mid-`tick()` — observes the cancelled context.

### Why the old daemon survives the kill signal

The kill arrives but the old daemon does not exit promptly:

- `cmd/state_daemon.go:54-63` — `defaultDaemonRun` is `for { select { ticker.C / ctx.Done() } }`. `tick()` runs synchronously inside the same select arm; **`ctx.Done()` is only reachable between ticks, never during one.**
- `cmd/state_daemon.go:265-270` — `signal.Notify(sigCh, SIGHUP, SIGTERM)` flips the context via `cancel()`, but the cancellation is observed only after the in-flight `tick()` completes.
- `cmd/state_daemon.go:115-158` — `captureAndCommit` runs sequentially: marker list → `CaptureStructure` → per-pane `CaptureAndHashPane` (always invokes `capture-pane` for the hash) → `WriteScrollbackIfChanged` → `state.Commit`. The dedup only avoids the **file write**, not the expensive `capture-pane` call.

Sweep cost is the cost driver. `internal/tmux/tmux.go:625` uses `capture-pane -e -p -S -` (unbounded scrollback). Measured 24-pane sweep: **3.9 s cold / 1.5 s warm** at the observed scrollback profile (~28 MB rendered text). The Go ticker drops missed fires, so when a sweep overruns the 1 s tick interval the next tick fires immediately on completion — daemons in this regime **never reach `ctx.Done()` between sweeps**, extending the orphan-eligibility window indefinitely after a kill.

### How the defects compose

- (1) alone: a benign condition — at N=1 the informational pidfile is sufficient.
- (2) alone: would produce a brief race window that resolves once the old daemon's sweep ends — except the absence of (1) means the old daemon, having lost its pidfile to the new daemon, becomes a permanent orphan instead of being cleaned up on the next bootstrap.
- (1)+(2) together: every kill-respawn cycle pushes N from 1 to 2+. Orphans accumulate over uptime.

### Why the design promised more than it delivered

The original design comment at `internal/tmux/portal_saver.go:28-33` states: *"tmux owns the daemon's lifecycle: when this session is killed (or the server dies), the kernel delivers SIGHUP to the daemon for graceful shutdown."* This is true in principle, but **graceful ≠ instant** — the daemon shuts down at the end of its current sweep, not on signal receipt. The lifecycle promise is honoured eventually, but the bootstrap path treats it as synchronous.

## Fix Part 1: Daemon-Side Singleton Lock

### Purpose

A structural invariant that guarantees **N ≤ 1 daemons** ever write to the same state directory, regardless of how they were started. This is the floor that holds even if every other guard fails.

### Behaviour

At daemon startup — **before** `WritePIDFile`, **before** the tick loop begins — acquire an advisory file lock on `<stateDir>/daemon.lock`:

- Use `unix.Flock(fd, LOCK_EX|LOCK_NB)` (exclusive, non-blocking).
- **Success path:** lock acquired → daemon proceeds normally (writes pidfile, enters tick loop). Lock is held for the lifetime of the process and released by the kernel on exit (`exit(0)`, `exit(N)`, signal-kill, crash — all release the fd).
- **Contention path:** lock acquisition fails (EWOULDBLOCK) → daemon logs a single WARN line *"another daemon holds the lock; exiting"* (or equivalent — log content not load-bearing, presence of the log line is) and exits with status 0. Exit 0 ensures tmux does not treat this as an abnormal session termination.
- The lock fd must be set `FD_CLOEXEC` so it does not leak into any child process the daemon forks.

### Placement and structure

Lock acquisition belongs in `cmd/state_daemon.go`, before the call to `state.WritePIDFile` at line 226. The lock helper itself may live in `internal/state` (alongside the pidfile helpers) for symmetry — final package placement is a planning decision, but the lock must execute on the daemon-startup path before any state-directory write.

The lock acquire call should be **seamed for testing** via a package-level `var lockAcquire = unix.Flock` (or similar), matching the existing test-seam pattern at `daemonRunFunc` / `daemonShutdownFunc`. Tests inject a fake that simulates acquire success, acquire failure, and inspects FD_CLOEXEC state.

### Why fail-fast (`LOCK_EX|LOCK_NB`) and not blocking (`LOCK_EX`)

A blocking acquire would cause the loser daemon to sit waiting on the lock until the winner exited — but the loser was launched as the initial process of a tmux session, so a long block holds tmux's `new-session` synchronously. Fail-fast keeps the contention path bounded (one log line, immediate exit). The bootstrap retry path covers the rare case where a loser exits and the next `portal` command needs to recover by recreating `_portal-saver` (the tolerant-kill-and-recreate branch in `BootstrapPortalSaver`).

### Why a lock and not `O_EXCL` pidfile create

`O_EXCL` is a one-shot atomic-create primitive, not a held lock. It leaks on crash (the file lingers, blocking startup until manually cleaned) and would need its own cleanup-on-startup dance. `flock` is the right primitive for "held while process is alive."

### Compatibility with the existing pidfile

The pidfile (`daemon.pid`) is retained. It continues to record the current daemon's PID for `BootstrapAliveCheck` and is rewritten by every successful lock-acquirer **after** the lock is held. With the lock in place, the pidfile transitions from "informational and racy" to "informational and authoritative" — it always reflects the single daemon that won the lock.

## Fix Part 2: Synchronous Kill Barrier

### Purpose

Make the **common-case** kill-respawn quiet: before a kill is followed by a respawn, wait for the prior daemon to actually exit. This keeps the daemon-side singleton lock (Part 1) from firing its WARN line on every recycle. Without the barrier, every saver recycle would produce noisy *"lock held; exiting"* lines and (briefly) an empty `_portal-saver` session until the next bootstrap retries. Functionally safe via the lock, but undignified.

### Behaviour

The barrier wraps the kill operation:

1. **Before** issuing `KillSession`, read the prior `daemon.pid` via `state.ReadPIDFile(stateDir)` and capture the PID.
2. Issue `KillSession(_portal-saver)` as today.
3. **After** the kill, poll `state.IsProcessAlive(prior_pid)` until it returns false. Polling cadence is a planning detail (50–100 ms is a reasonable starting point); bound the total wait by a **5-second timeout**.
4. **On clean exit within timeout** → proceed silently to `BootstrapPortalSaver`. This is the expected path.
5. **On timeout** → log a single WARN line and proceed to `BootstrapPortalSaver`. The new daemon's lock acquisition (Part 1) is the safety net — either the prior daemon has already released the lock and the new one succeeds, or the new one will fail-fast and the next bootstrap recovers.
6. **No prior PID file** (file missing, unreadable, or empty) → barrier is skipped, proceed directly to respawn. There is no prior daemon to wait for.
7. **Prior PID file points to a dead process** (signal-0 already returns false) → barrier completes immediately, proceed.

### Timeout rationale

5 seconds is chosen specifically to sit **above the cold-sweep upper bound** of 3.9 s measured on the affected user's scrollback profile (24 panes, ~28 MB rendered text), with margin. A 3 s timeout would fire WARN-and-proceed on cold-cache restarts at this profile. 5 s keeps the WARN path reserved for genuinely stuck daemons (or users with even heavier scrollback than the observed sample) rather than ordinary cold sweeps.

### Both kill sites use the barrier

There are **two kill call sites** in the saver-bootstrap surface and **both** must use the barrier — these are not alternatives:

1. **`EnsurePortalSaverVersion`** version-mismatch branch (`internal/tmux/portal_saver.go:108-112`) — fires when stored ≠ current version.
2. **`BootstrapPortalSaver`** stale-pidfile recovery branch (`internal/tmux/portal_saver.go:66-70`) — fires when the session exists but `BootstrapAliveCheck` reports the daemon dead.

The two sites are factored into a **shared helper** (e.g. `killSaverAndWaitForDaemon(c, stateDir) error`) rather than duplicating the poll loop. The helper takes the tmux client + state directory, reads the prior PID, kills the session, polls, returns nil on clean exit and a non-fatal error on timeout (or returns nil with the call site logging — exact error shape is a planning decision).

### Test seams

The barrier should be testable without spawning real processes. Required seams:

- `ReadPIDFile` — already a function; use as-is or wrap behind a `var` for injection.
- The polling clock — `state.IsProcessAlive` and the time source (`time.Sleep` / `time.After`) need to be injectable so tests can simulate "PID dies after 200 ms" / "PID never dies" / "PID was already dead" without real processes or real waits.
- The existing `BootstrapAliveCheck` seam (`var` in `internal/tmux/portal_saver.go`) is unrelated and unchanged — it operates on the session-presence check, not the kill barrier.

### Critical-path latency budget

The barrier executes on the bootstrap critical path only when a recycle actually happens — i.e. when the kill branch fires. It does **not** add latency to the steady-state path where the saver already exists and the version matches.

- **Median case** when a recycle does fire: prior daemon exits in under 100 ms (most ticks are sub-second; the cold-sweep ceiling is the worst case, not the typical case).
- **Worst case**: bounded 5 s timeout.

This trade is acceptable because (a) the recycle path is rare per `EnsurePortalSaver` invocation and (b) the alternative is silent corruption from N > 1 daemons.

### Why not spin-wait inside `BootstrapPortalSaver` until the lock is acquirable

Rejected during investigation discussion: bootstrap is on the critical path of every `portal` command and must stay fast. A 1.5–4 second hold to wait for lock release would make every command sluggish whether or not a recycle was happening. The barrier scopes the wait to **only the recycle path**.

---

## Working Notes

