# Specification: Built-In Session Resurrection

## Specification

## Overview

Portal will own the full tmux session lifecycle end-to-end: server start → session restoration → resume hook execution. The current middle step depends on tmux-resurrect/continuum, which fails to reliably restore sessions after reboot, breaking the resume hook feature despite the hook code being correct.

This replaces reliance on external plugins with a built-in save/restore mechanism.

## Product Goal

**"Zellij in tmux."** When a user reboots, their tmux sessions come back as they were — structure *and* content. Restoration is automatic, on by default, with no opt-in required.

## Organizing Principles

- **Portal owns the full lifecycle.** Save, restore, and hook execution are all internal. No external plugin dependencies for resurrection.
- **Portal's hook system is generic.** Portal stores and fires command strings. It has no awareness of what consumers do with them. Caller wrapper scripts own process-specific policy (e.g., re-registering dynamic hooks).
- **Portal does not maintain a separate session registry.** Live tmux state is read directly via `list-panes -a`, `list-sessions`, etc. Portal's saved state mirrors tmux state; it never diverges from or substitutes for it.
- **Portal captures all sessions — both Portal-created and native tmux.** Consistent with existing behavior. Sessions prefixed with `_` are reserved for Portal internals and excluded from capture, restore, and the TUI picker.
- **Bootstrap is the natural point for restoration.** Portal is always the entry point; `PersistentPreRunE` already runs before every user-facing command.
- **Degrade locally, log, continue.** No single failure may crash Portal or leave the user stuck. All failures log to a file and degrade the affected pane/session to a workable state.
- **Single-writer architecture.** All state-file writes funnel through one long-running process. Other triggers signal via a dirty flag. Eliminates write races by construction.
- **YAGNI rigorously.** Speculative features (ephemeral opt-out, background prefetch, compression, multi-host sync) are deferred until concrete user demand surfaces.

## Scope & Constraints

### Minimum Versions

- **tmux ≥ 3.0** (Feb 2020). Array-indexed hooks (`set-hook -a` semantics) require 3.0; the hook-lifecycle coexistence model depends on them.
- **Go, macOS, Linux**: inherits Portal's existing requirements. No change.

### In Scope — Captured and Restored

**Structural state:**
- Session names
- Window indices, names, layout strings, active flag, zoom flag
- Pane indices, current working directories, active flag

**Content:**
- Full pane main-screen scrollback with ANSI escape sequences — colors, attributes, formatting preserved via `tmux capture-pane -e -p -S - -t <pane>`
- tmux per-session environment via `show-environment -t <session>` — the tmux-level env used for initializing new panes (not live shell env). Restored in full without filtering; tmux's own `update-environment` refreshes stale values (`SSH_AUTH_SOCK`, `DISPLAY`) on client attach.

**Already stored elsewhere (unchanged):**
- Resume hooks (`hooks.json`)

### Out of Scope — Ephemeral Interaction State

Excluded from capture even though tmux exposes them, because they are transient:
- Copy mode state
- Active selections
- Paste buffers
- Cursor position within panes
- Scroll position within scrollback
- Per-client state (which client has which pane focused, client-specific dimensions)

### Out of Scope — Uncapturable by tmux

Not Portal's problem to solve. Users can compensate via resume hooks where meaningful:
- Live shell environment variables — tmux cannot observe shell-side `export`. Callers compensate via resume hooks if they care.
- Running process state (REPL state, interactive sessions like vim/htop/less) — alt-screen buffer contents are explicitly not scrollback. The resume hook system exists specifically for process relaunch.
- Open file descriptors, pipes, sockets, ptrace state.

### Explicit Non-Goals

- **Live shell environment variables.** Scope-boundary non-goal; see above.
- **Running process state.** Same.
- **tmux server-level options and user's `.tmux.conf` customizations.** Portal reads live tmux state via standard APIs; does not introspect or capture config files. Restoration uses the user's current `.tmux.conf` as baseline.
- **Third-party tmux plugin state.** Portal coexists with other TPM plugins via hook-append semantics but does not capture, understand, or interact with their state.
- **Multi-server, multi-host, remote sync.** Portal works with a single local tmux server per machine. No cross-machine sync, no remote tmux, no session groups distributed across hosts.
- **Non-tmux multiplexers.** Portal is tmux-specific. Zellij, GNU screen, wezterm, abduco — not supported.
- **Shell-specific behavior.** Portal is shell-agnostic. Helper `exec`s `$SHELL`. No special-casing bash/zsh/fish.
- **Mouse state / clipboard state.** Ephemeral interaction state.
- **Generic tmux option capture** (session/window/pane options). Nearly all tmux options are set globally via `~/.tmux.conf` and apply on restore automatically. Per-session overrides are niche. Capturing them requires diffing `show-options` against global defaults — complexity not justified. If a specific flag becomes important, add it as an explicit field later.
- **Marks / position bookmarks.** tmux's `<prefix>m` marks panes (not scrollback positions), and copy-mode position marks have no tmux API to capture or restore. No functional gain.
- **Last-pane tracking.** No confirmed tmux format variable exposes this.

### Deferred (Not Non-Goals, Just Not Now)

- **Ephemeral session opt-out** (per-session/per-pane exclusion from capture). Speculative without concrete user demand. `history-limit 0` on a window is the tmux-native workaround documented in the README.
- **Scrollback compression.** Deferred until disk usage becomes a complaint.
- **Parallel capture** for many-pane configurations. Deferred until performance complaints surface.
- **Schema migration (v1 → v2).** Standard practice when the time comes; not a v1 design decision.
- **Background prefetch / full-eager hydration.** Rejected in favor of pure-lazy hydration; can be reconsidered if attach-time latency becomes a complaint.

## Hook System Lifecycle Behavior

### Behavior

Portal's resume hook system has **a single persistent behavior**. Hooks registered via `portal hooks set --on-resume "cmd"` live in `hooks.json` across reboots and survive tmux server restarts. They are removed only by explicit `portal hooks rm --on-resume` invocation.

There is no `mode` field, no `once` vs `always` flag, no per-hook lifetime configuration. Portal is a mechanism; hook lifecycle policy (one-shot, bounded-lifetime, dynamic re-registration) is the caller's responsibility.

### Caller Pattern for Dynamic or One-Shot Semantics

Long-running processes that need dynamic hook management (e.g., `claude --resume <uuid>` where the UUID changes each session) are invoked via a wrapper script that owns the hook lifecycle:

- Wrapper calls `portal hooks set --on-resume "<cmd>"` when the process starts, inferring the current pane from `TMUX_PANE`.
- Wrapper re-registers on each resume if the command includes dynamic state (e.g., a new resume UUID).
- Wrapper calls `portal hooks rm --on-resume` on explicit process exit, via an exit trap or teardown routine.

Portal exposes `set` and `rm` as primitives; callers wire them into their own lifecycle hooks.

### Rejected Alternative: `&&` Shell-Chaining for Self-Removal

A pattern like `portal hooks set --on-resume "my-cmd && portal hooks rm --on-resume"` does **not** work for the flagship class of hook commands. Long-running processes (`claude --resume`, `npm start`, `tail -f`) never exit, so the `&&` clause never fires, and the hook never self-removes. This pattern is architecturally broken for the exact use case hooks exist to serve and is not supported as a recommendation.

### CLI Surface (Unchanged User-Facing Shape)

- `portal hooks set --on-resume "<cmd>"` — register or replace the resume hook for the current pane (pane inferred from `TMUX_PANE`, keyed by structural key `session:window.pane`).
- `portal hooks list` — list registered hooks.
- `portal hooks rm --on-resume` — remove the resume hook for the current pane.

The storage file (`hooks.json`) and structural-key scheme are unchanged. Internal implementation changes are covered in "Resume Hook Firing" and "CleanStale Behavior" sections below.

### Rationale

A mode field can be added later if a concrete use case surfaces where wrapper-script management is genuinely impractical. None was identified during discussion — the flagship Claude use case is satisfied by caller-side wrapping, and static commands (`npm start`, file watchers, `tail -f`) only make sense as persistent.

## Save-Side Architecture: Execution Model

### Host Process: Detached tmux Session

Portal hosts its long-running save process inside a detached tmux session named `_portal-saver`. The session's command is `portal state daemon` — a long-running Go process. tmux owns the process lifecycle: when tmux terminates, the PTY is closed, the kernel delivers SIGHUP to the daemon, and the daemon flushes its final state before exiting.

**Why a detached tmux session (not a platform-specific service and not subprocess-per-event):**

- tmux already owns process supervision. No launchd/systemd per-platform code, no install/uninstall service lifecycle, no double-fork fallback with silent-failure mode.
- The daemon has nothing useful to do when tmux is dead, so tying its lifetime to tmux's is correct.
- Portal creates `_portal-saver` idempotently at bootstrap. Crash recovery is automatic — a dead session is recreated on the next `portal open`.
- Pattern has real-world precedent (tmux-slay). The concerns are concrete and addressed.

**Why not subprocess-per-event:** structural-event-driven saves miss scrollback drift. A user sitting in one pane with a program outputting for hours produces no structural events; crash loses everything since the last event.

**Why not a full external daemon (Zellij path):** ~500 LOC of platform-specific lifecycle code (install, supervise, PID files, IPC, upgrade). Zellij's daemon is intrinsic to the tool; Portal bolting one on for one feature is a different engineering calculus.

### Session Visibility and Filtering

`_portal-saver` shows up in `tmux ls`. There is no tmux mechanism to hide it. Portal filters sessions whose names begin with `_` (underscore prefix is reserved for Portal internals) from:

- The TUI session picker.
- `sessions.json` capture (the save process skips `_*` sessions when enumerating live state).
- Any future internal-only sessions.

This keeps the internal machinery invisible in Portal's own UX while remaining inspectable via `tmux ls` for debugging.

### Defensive Session Setup

On every `EnsureServer()` call, Portal runs `set-option -t _portal-saver destroy-unattached off` unconditionally (idempotent). This defends against users with `destroy-unattached on` set globally in `.tmux.conf` — without this override, their global setting would kill `_portal-saver` immediately on creation (the session is `-d` and has zero attached clients).

### Signal Handling

The daemon traps two signals:

- **SIGHUP** — delivered by the kernel when tmux closes the PTY master fd. This is the dominant shutdown path (tmux `kill-server`, server crash, reboot). Discussion verified the kernel sends SIGHUP, not SIGTERM, in this case — Portal must trap SIGHUP explicitly.
- **SIGTERM** — delivered by direct `kill <pid>` from outside tmux. Less common but handled for completeness.

Handler behavior:

1. If the `@portal-restoring` marker is set, skip the final flush (an in-progress restore is underway; capturing now would capture mid-transition state).
2. Otherwise, flush the current state atomically via `AtomicWrite` and exit.

No configurable grace period. Atomic rename guarantees either the old or new state file is always valid on disk, so there is no mid-write corruption risk from signal-timing.

### Lifecycle Summary

- **Creation:** `EnsureServer()` calls `has-session -t _portal-saver`. If absent, `new-session -d -s _portal-saver "portal state daemon"`.
- **Auto-destroy:** when the daemon exits (normal shutdown, crash, or version-mismatch restart), tmux's default session-auto-destroy behavior removes the session. No `remain-on-exit` tuning required.
- **Recreation:** the next `portal open` after destruction runs bootstrap, finds `_portal-saver` absent, and recreates it.
- **Version-based restart:** see the "tmux Hook Registration Lifecycle" section for the version-marker restart protocol.

## Save-Side Architecture: Triggers & Serialization

### Trigger Layers

Two complementary trigger mechanisms ensure both responsiveness and bounded data loss:

**1. Event-driven (immediate response).** tmux global hooks (`set-hook -ga`) fire on structural events. Each event runs `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`, which signals that state is dirty.

The events:
- `session-created`
- `session-closed`
- `session-renamed`
- `window-linked`
- `window-unlinked`
- `window-layout-changed`
- `pane-focus-out`

These catch structural changes (session/window/pane topology, renames, layout changes, focus transitions) as they happen.

**2. Periodic (bounded worst-case).** The hosted daemon runs a 1-second ticker internally. Each tick: if the dirty flag is set OR it has been ≥30 seconds since the last save, capture and persist state; otherwise no-op.

- **Dirty-flag check** produces ≤1 second event-to-save latency.
- **30-second max-gap** bounds worst-case scrollback loss on unexpected tmux/system termination at 30 seconds, even during periods with zero structural events.

### No Opportunistic Trigger

Earlier framing proposed additional saves fired from `portal open` / `portal attach` based on last-save staleness. This is **not** included. If the hosted daemon is running, it is already saving via events + ticker. If it is not running, `EnsureServer()` will recreate `_portal-saver` and its first tick fires within ~1 second — coverage is already complete. An opportunistic trigger would only add a code path racing with the hosted process.

### Single-Writer Serialization via Dirty Flag

All state-file writes flow through the hosted daemon. Other trigger paths only *signal*. This eliminates write races by construction.

**Mechanism:**

1. tmux fires a structural event.
2. The hook command runs `portal state notify`.
3. `portal state notify` is a small binary: touch (create or bump mtime of) `~/.config/portal/state/save.requested`, exit.
4. The hosted daemon's 1-second ticker checks on each tick:
   - Is `save.requested` present? → capture, then clear the flag.
   - Has it been ≥30 seconds since the last save? → capture, then clear the flag.
   - Neither? → no-op.
5. Capture inspects `@portal-restoring` first; if set, skip the tick entirely (restoration in progress).

**Properties:**

- **Single writer by construction.** Only the hosted daemon writes scrollback files and `sessions.json`. No filesystem coordination beyond the dirty flag.
- **Natural coalescing.** Five events firing in 100ms all just set the flag; the next tick does exactly one save.
- **Max-gap guarantee.** 30 seconds is the ceiling on save staleness, even during idle periods.
- **Event latency.** ≤1 second from tmux event to save completion (bounded by the ticker interval).
- **Restoration guard.** The daemon's tick checks `@portal-restoring` at the top of the cycle. While set (during the skeleton-restore window), no capture runs, regardless of dirty-flag state.

### Daemon Tick Loop (Pseudocode)

```
for {
    select {
    case <-ticker.C:  // 1 second
        if isRestoringFlagSet() {
            continue  // skip entire tick during restore
        }
        if isDirty() || timeSinceLastSave() >= 30*time.Second {
            captureAndWrite()
            clearDirty()
        }
    case <-ctx.Done():  // SIGHUP or SIGTERM
        if !isRestoringFlagSet() {
            captureAndWrite()  // flush final state on shutdown
        }
        return
    }
}
```

### Defensive Dirty-Flag Clear on Daemon Startup

On daemon startup, the first action is to clear `save.requested` if present. This prevents a stale dirty flag from a prior (crashed or version-mismatch-restarted) daemon from triggering an immediate save of a mid-restore state.

Correctness does not depend on this — even without it, the eventual capture converges to the correct state once restoration completes and `@portal-restoring` is cleared. The clear is a belt-and-braces cleanup that avoids a redundant capture during the restore window.

### Tick Cadence Rationale

The 1-second ticker is a **poll cadence**, not a save cadence. Per-tick idle cost is a filesystem stat of `save.requested` and a `time.Since` comparison — measured in microseconds. Heavy work (capture-pane, hashing, writes) only fires when the dirty flag is set or the 30-second max-gap elapses.

This polling approach is not load-bearing. It could be swapped for `fsnotify`-style filesystem watching for sub-10ms responsiveness, at the cost of cross-platform watcher complexity. Current polling is simpler and good enough for the responsiveness target.

### Crash Safety

- **Mid-capture crash:** in-memory state discarded. On-disk `sessions.json` still points to the previous save's scrollback files. Previous save remains fully valid.
- **Mid-write crash:** `AtomicWrite` (temp file + rename) guarantees either the old or new file is intact. No partial-file states exist on disk.
- **Orphan scrollback files** from an interrupted save are cleaned up by the GC step on the next successful save (see Save Format & Schema section).

---

## Working Notes
