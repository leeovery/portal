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

---

## Working Notes
