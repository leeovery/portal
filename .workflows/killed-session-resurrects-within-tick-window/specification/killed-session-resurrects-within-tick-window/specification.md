# Specification: Killed Session Resurrects Within Tick Window

## Specification

## Problem Statement

When a Portal-managed tmux session is killed (via TUI `K` confirm flow, `portal kill`, the user's `Option-Q` binding, tmux's `M-q` keymap, or external `tmux kill-session`), a subsequent `portal` invocation within ~2–5 seconds:

- Still lists the killed session in the TUI Sessions page.
- Triggers bootstrap step 5 `Restore` to reconstruct the session in tmux as a skeleton pane.

After roughly one daemon tick (~5s on affected scrollback profiles) the session disappears from both surfaces. The symptom is reliably reproducible given the timing window.

## Required Behavior After Fix

For every kill path — TUI `K`, `portal kill`, `Option-Q`, `M-q`, external `tmux kill-session` — the following must hold immediately after the kill (no sleep, no retry):

1. `sessions.json` on disk no longer contains the killed session.
2. A subsequent `portal` invocation does not list the killed session.
3. Bootstrap step 5 `Restore` does not reconstruct the killed session.

The kill must be authoritative and immediate from the user's point of view. "Eventual consistency on the order of one daemon tick" is unacceptable for kill events.

## Severity Classification

High (trust tier). Same user-visible "I killed this, and Portal brought it back" surface as two recently-shipped resurrection-class bugfixes (`daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart`). Symptom is recoverable today by waiting ~5s, so regular release posture — no hotfix.

## Root Cause

`sessions.json` is rewritten **eventually**, not synchronously with kills.

The kill-event plumbing today:

1. Any kill path causes tmux to fire the `session-closed` global hook.
2. The hook runs `portal state notify`, which by deliberate design performs **zero tmux calls and zero `sessions.json` writes** — it creates/truncates `save.requested` (the daemon's dirty flag) and exits.
3. The daemon's 1-second ticker observes the dirty flag, then runs `captureAndCommit`, which walks all panes, captures and hashes scrollback per-pane, and atomically rewrites `sessions.json`. Per-tick wall time scales with rendered scrollback — field-measured ≥3.9–5s on the affected user profile.

The race window is `[0, ticker.period + per-tick wall time]`, bounded above only by worst per-tick capture latency.

Bootstrap step 5 `Restore` reads `sessions.json` on every Portal invocation. During the race window, the consumer-side contract — "if it's in `sessions.json` and not in live tmux, restore it" — faithfully reconstructs the killed session because the file still lists it.

The merge filter shipped by `daemon-merge-reintroduces-dead-sessions` is doing its job: it correctly drops dead sessions from the previous index during the daemon's commit. That fix operates **inside** the daemon's tick. The bug here is that the consumer (`Restore`) runs **before** the daemon ticks at all. This bugfix is therefore a sibling, not a regression, of the two recently-shipped resurrection-class fixes.

### Contributing Factors Worth Preserving

- The daemon's tick is uninterruptible — `ctx.Done()` is unreachable inside an in-flight `tick()`, so dirty-flag immediacy does not translate to commit immediacy.
- The daemon may not be running at all. `_portal-saver` is best-effort (bootstrap step 4 surfaces `SaverDownWarning` on failure). With the daemon down, `save.requested` is touched but no commit happens until the next bootstrap, extending the resurrection window from seconds to "until the next Portal start".
- Same-process kill paths (TUI `K`, `portal kill`) call `tmux.KillSession` and return without a synchronous commit — they rely on the same eventual-consistency contract as external kills.

## Fix Approach

Make the `session-closed` tmux hook **synchronously rewrite `sessions.json`** before the hook subprocess returns.

### Mechanism

A new minimal-cost capture-and-commit path that:

1. Captures the current structural index via the existing `state.CaptureStructure` (no scrollback content, no hash work).
2. Atomically commits it via the existing `state.Commit` primitive (temp file + rename).
3. Short-circuits as a no-op when `@portal-restoring` is set on the tmux server (defers to the daemon's existing restoration discipline).

This path uses **only existing primitives**. `state.CaptureStructure` already takes a `CaptureClient` interface satisfied by `*tmux.Client` — it is not daemon-exclusive. `state.Commit(dir, idx, anyScrollbackChanged, logger)` is already the atomic-write primitive used by the daemon and is structurally available to any caller.

### Where the Synchronous Commit Lives

The new path is invoked from the kill-side hook context as a short-lived `portal state ...` subprocess (matching the existing `portal state notify` pattern). The exact entry-point shape (new sibling subcommand vs new flag on `notify`) is the open design decision documented under "Entry-Point Design Decision" below.

### What Stays Eventually Consistent

The six other save-trigger events keep the existing cheap dirty-flag touch (`portal state notify`):

- `session-created`
- `session-renamed`
- `window-linked`
- `window-unlinked`
- `window-layout-changed`
- `pane-focus-out`

These are creates, renames, and focus changes — none can produce a "consumer sees a session that no longer exists" symptom. Eventual consistency on the daemon's tick is correct and acceptable for them, and raising their cost from a 2-syscall touch to a full structural capture is gratuitous.

### What's Excluded from the Synchronous Path

- **Scrollback `.bin` writes and hash work.** The synchronous path only writes `sessions.json`. The daemon retains exclusive ownership of `.bin` file writes, content hashing, and `gcOrphanScrollback`.
- **Marker management.** `@portal-skeleton-*` server-option markers continue to be managed by the daemon and the existing bootstrap/restore lifecycle.
- **Per-pane environment beyond what `CaptureStructure` already captures.** No new tmux work is added.

### Why This Approach

The user's explicit directive: synchronous at the kill-side path, eliminate the race at its source. The `session-closed` hook is the single tmux-side seam that fires uniformly across **all** kill paths — cmd-internal (TUI `K`, `portal kill`) and external (`Option-Q`, `M-q`, `tmux kill-session`). Making it the synchronous commit point covers every kill path with one change:

- No new IPC.
- No daemon dependency for kill-path correctness — the synchronous commit works even when `_portal-saver` is down.
- No timeout window to size against scrollback profile.
- No ticker rate or retry tuning.

After the fix, every kill path produces a consistent `sessions.json` before the kill-triggered hook subprocess returns, eliminating the race window for the resurrection symptom.

### Cost Profile

Per-kill subprocess cost: one `ListSessionNames` + one `list-panes -a -F …` + per-session `ShowEnvironment` + one atomic file write. Estimated ~50–200ms. This is per-kill, not per-keypress; acceptable cost on a path that previously was free.

## Entry-Point Design Decision

The synchronous capture-and-commit path is exposed as a **new sibling subcommand**: `portal state commit-now`.

### Final Shape

```
portal state commit-now     # captures structural index, atomically rewrites sessions.json, exits
portal state notify         # unchanged: touches save.requested, exits
```

### Why V1 Over V2 (`portal state notify --sync`)

The two commands have **disjoint internal logic** — they don't share a handler:

- `notify` performs zero tmux calls and zero `sessions.json` writes. It opens/truncates `save.requested` and exits. It does not *do* the save; it asks the daemon to.
- `commit-now` calls `state.CaptureStructure` (live tmux queries) and `state.Commit` (atomic write of `sessions.json`), bypassing the daemon entirely.

These are two different operations that happen to converge on the same file eventually, not the same operation in different wait modes. Sharing a verb parameterised by a flag would understate that separation.

Additional supporting points:

- The sync path has an `@portal-restoring` short-circuit; `notify` does not (it touches the flag freely during restoration — the daemon honours the marker). The no-op semantics diverge, so the flag would gate an entire alternate codepath, not a wait mode.
- A named verb composes better with potential future callers (`portal save`, `portal kill --commit`) than a flag on `notify`.

### Trade-Offs Accepted

- One additional top-level verb under `portal state`. This is the explicit cost of semantic clarity.
- Marginally wider cmd-surface change (new file `cmd/state_commit_now.go` alongside `cmd/state_notify.go`).

## Hook Registration Migration

### Today

`internal/tmux/hooks_register.go` defines:

```go
var saveTriggerEvents = []string{
    "session-created", "session-closed", "session-renamed",
    "window-linked", "window-unlinked", "window-layout-changed",
    "pane-focus-out",
}
const notifyCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`
```

`RegisterPortalHooks` (bootstrap step 2) idempotently appends `notifyCommand` to each event in `saveTriggerEvents`. All seven events fire the same dirty-flag touch.

### After Fix

`session-closed` migrates off the shared `notifyCommand` and onto a new command that invokes the synchronous commit:

```go
const commitNowCommand = `run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"`
```

The other six save-trigger events remain on `notifyCommand` unchanged.

### Registration Strategy: Replace (not Alongside)

`session-closed` is registered with **only** `commitNowCommand`, not both `commitNowCommand` and `notifyCommand`. Running both would do redundant work — `commit-now` already writes `sessions.json` synchronously, making the dirty-flag touch obsolete for this event. The daemon's next tick after a kill would just re-confirm a state that's already on disk.

### Idempotency Requirements

`RegisterPortalHooks` must remain idempotent across multiple bootstrap invocations:

- A repeat bootstrap must not append `commitNowCommand` twice to `session-closed`.
- A bootstrap upgrade from a pre-fix Portal install that left `notifyCommand` registered on `session-closed` must:
  1. Remove the stale `notifyCommand` registration from `session-closed`.
  2. Register `commitNowCommand` in its place.

This is the same idempotency discipline `RegisterPortalHooks` already implements for adding hooks; the upgrade path extends it to migrate an event from one command to another.

### Why `session-closed` Is The Right Hook

It is the **single tmux-side seam that fires uniformly across all kill paths**:

| Kill path | Trigger |
|---|---|
| TUI `K` confirm | `tmux.KillSession` → tmux fires `session-closed` |
| `portal kill` | `tmux.KillSession` → tmux fires `session-closed` |
| `Option-Q` user binding | tmux's `kill-session` → fires `session-closed` |
| `M-q` tmux default | tmux's `kill-session` → fires `session-closed` |
| External `tmux kill-session` | tmux fires `session-closed` |

Migrating this one event covers every kill path without per-call-site changes in the cmd layer.

---

## Working Notes
