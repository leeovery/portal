# Research: Built-in Session Resurrection

Portal should own the full session lifecycle: server start → session restoration → resume hook execution. Currently the middle step depends on tmux-resurrect/continuum, which doesn't work reliably.

## The Problem

Portal's resume hooks feature can restart processes in panes (e.g., `claude --resume <uuid>`), but it depends on the session structure already existing. That structure is supposed to be restored by tmux-resurrect/continuum after a reboot, but those plugins have never worked reliably — sessions simply don't come back. This means the resume feature is effectively broken end-to-end despite the code being correct.

The resume system also has an undocumented dependency on these plugins. Portal doesn't mention that you need resurrect/continuum installed for the resume workflow to function across reboots. This was a deliberate choice to avoid coupling to buggy plugins, but the result is a feature that silently doesn't work in the most important scenario (reboot recovery).

## What We Want

Reboot → open Portal → tmux server starts → all previous sessions/windows/panes/layouts are restored → selecting a session fires any registered resume hooks → seamless continuation.

Portal owns the full chain. No external plugin dependencies.

## Design Principles (Established)

**Portal's hook system is generic.** No awareness of what consumers do with it. Portal stores and fires a command string — it's the caller's responsibility to make that command correct. Claude-specific logic lives in the Claude hook script, not in Portal.

**Portal doesn't maintain a separate session registry.** It reads tmux directly via `list-sessions`. Resurrection follows the same pattern: capture everything tmux has, restore everything captured. Non-Portal sessions get structure restoration for free; Portal sessions additionally get resume hooks.

**Portal-only vs native tmux is a non-issue.** Since `list-panes -a` captures all sessions regardless of origin, and Portal's bootstrap starts the server, non-Portal sessions are captured naturally. No extra work to include them, extra work to exclude them.

## Research Findings

### tmux State Capture & Restore APIs — FEASIBLE

Deep dive confirmed tmux provides everything needed:

**Capture:** `list-panes -a -F` with format variables gets session names, window indices, pane working directories, layout strings, active/zoom state in one call. Key variables verified against tmux source: `session_name`, `window_index`, `window_layout`, `window_zoomed_flag`, `pane_index`, `pane_current_path`, `pane_active`, `pane_current_command`.

Critical detail: `window_layout` returns the pre-zoom layout (correct for restoration). Use this, not `window_visible_layout`.

**Restore sequence (proven by resurrect):**
1. `new-session -d -s name -c dir` — also starts server if needed, no default "0" session problem
2. `new-window -d -t session:index -c dir`
3. `split-window -d -t session:window -c dir`
4. `select-layout -t session:window "layout_string"` — pane count must match exactly
5. `send-keys -t session:window.pane "command" C-m`
6. `select-pane`, `select-window` — restore focus
7. `resize-pane -Z` — restore zoom (must come after layout)

**Limitations:**
- `pane_current_command` returns short name only (no args). tmux maintainer explicitly rejected adding args. Not a problem for Portal — hooks store explicit commands.
- Layout strings are terminal-size dependent. Different terminal size at restore may shift proportions. Neither resurrect nor tmux solves this cleanly.
- Hooks don't fire on crash/SIGKILL — periodic saves needed as safety net.

### tmux Hooks for Save Triggers — AVAILABLE

tmux provides global hooks: `session-created`, `session-closed`, `window-linked`, `window-layout-changed`, etc. Registered via `set-hook -g`. Portal could register these during bootstrap to trigger saves.

`server-exit` hook exists but only in tmux HEAD (post Oct 2025). Workaround: `session-closed` with `if '[ #{server_sessions} -eq 0 ]'`.

### tmux-resurrect/continuum Failure Analysis

Deep dive into resurrect source confirmed why it fails:

**Continuum's auto-save** piggybacks on `status-right` interpolation. If any theme/plugin overwrites `status-right`, saving silently stops. #1 reported failure.

**Continuum's auto-restore** has a hardcoded 1-second sleep and 10-second server-age window. Plugin load timing causes race conditions.

**Save corruption:** 0-byte files written on save failure, `last` symlink points to empty file, no validation.

**Layout restoration:** `resize-pane -U 999` during creation causes transient broken states. ~20-30% failure rate for complex layouts.

**Process detection:** `ps -ao ppid,args` is fundamentally fragile (macOS issues, forked processes invisible, Node.js wrappers lose args).

Portal's architecture addresses all of these: atomic writes (no corruption), direct tmux Client (no bash parsing), event-driven saves (no status bar dependency), deterministic restore order, hooks instead of process detection.

### tmux-assistant-resurrect Analysis

A TPM plugin that piggybacks on resurrect/continuum, adding AI-assistant-specific session ID capture. Depends entirely on resurrect for structure — the exact dependency Portal eliminates.

**Worth noting:**
- Two-guard restore pattern (check shell running + no existing process before sending commands). Portal may not need this since it creates panes fresh during restoration — nothing else could be running. But worth keeping in mind for edge cases.
- CLI args preservation (stripping session args, keeping user flags). Not Portal's concern — the hook system is generic, and it's the caller's responsibility to register the correct resume command with all needed flags.

**Confirms Portal's advantages:** generic hook system, full lifecycle ownership, one-shot + re-registration model avoids stale session IDs.

## Open Research Threads

### Hook Lifecycle Redesign
**Status: needs exploration**

With Portal owning restoration, the hook design might change. Two models:
- **One-shot:** Fire hook, remove it, rely on the resumed process to re-register. Portal's perspective is simple. Works when the process has its own hook that re-registers (e.g., Claude).
- **Persistent:** Hook survives across reboots without re-registration. Useful for processes that don't self-register (e.g., a static `npm start` command).

Could be configurable per-hook.

### Save Triggers
**Status: partially researched**

tmux hooks available for event-driven saves. Still need to determine:
- Which events to hook (session-created, session-closed, window-linked — anything that changes structure)
- Whether periodic saves are also needed (crash safety — hooks don't fire on SIGKILL)
- Where to save (likely `~/.config/portal/` alongside existing stores)
- Save format (JSON, consistent with existing stores)

### Layout Restoration Reliability
**Status: needs investigation**

Resurrect has ~20-30% failure rate for complex layouts. Portal should do better with deterministic ordering (create all panes first, apply layout once). But terminal dimension changes between save and restore are a tmux limitation Portal can't fully solve.

### Restoration Timing
**Status: needs thought**

When exactly does restoration happen in Portal's lifecycle? During bootstrap (`PersistentPreRunE`)? Before the TUI loads? The loading page already exists for the server-start case — restoration could extend that.

---
