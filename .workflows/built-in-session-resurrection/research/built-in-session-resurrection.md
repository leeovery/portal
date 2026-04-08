# Research: Built-in Session Resurrection

Portal should own the full session lifecycle: server start → session restoration → resume hook execution. Currently the middle step depends on tmux-resurrect/continuum, which doesn't work reliably.

## The Problem

Portal's resume hooks feature can restart processes in panes (e.g., `claude --resume <uuid>`), but it depends on the session structure already existing. That structure is supposed to be restored by tmux-resurrect/continuum after a reboot, but those plugins have never worked reliably — sessions simply don't come back. This means the resume feature is effectively broken end-to-end despite the code being correct.

The resume system also has an undocumented dependency on these plugins. Portal doesn't mention that you need resurrect/continuum installed for the resume workflow to function across reboots. This was a deliberate choice to avoid coupling to buggy plugins, but the result is a feature that silently doesn't work in the most important scenario (reboot recovery).

## What We Want

Reboot → open Portal → tmux server starts → all previous sessions/windows/panes/layouts are restored → selecting a session fires any registered resume hooks → seamless continuation.

Portal owns the full chain. No external plugin dependencies.

## Research Threads

### 1. tmux State Capture & Restore APIs
**Status: needs deep research (internet, not training data)**

Can tmux provide everything Portal needs to snapshot and reconstruct session state? Specifically:
- Session names, window indices, pane layout strings
- Working directories per pane
- Running commands per pane (and how reliable is this?)
- Window/pane focus state, zoom state

The inbox idea listed specific tmux commands and format strings for this, but these are unvalidated claims that need verification against actual tmux documentation.

**Hard constraint:** Cannot touch the running tmux server on this machine. Read-only inspection only. User can test on a separate system if needed.

### 2. Resume Hook Lifecycle Redesign
**Status: needs exploration**

The current resume system was designed around the constraints of not owning restoration. With Portal controlling the full chain, the design might change significantly.

Key mechanic to explore: hooks could be one-shot (Portal fires it, removes it, and the resumed process re-registers a new hook for its new session) or persistent (survives reboots without re-registration). This might be configurable per-hook rather than system-wide.

Example flow for Claude: Portal fires `claude --resume <uuid>` → removes the hook → Claude starts and its own hook re-registers a new resume command with the new conversation UUID. From Portal's perspective it's one-shot, but the system is self-healing because Claude re-registers.

Other processes might want persistent hooks that don't require the process to re-register.

### 3. Portal-Only vs Native tmux
**Status: needs discussion**

Should resurrection work for all tmux sessions or only Portal-managed ones? If you bypass Portal and create sessions via raw `tmux new-session`, should those be captured too?

Current leaning: Portal features require Portal. You start and manage sessions through Portal. But worth exploring whether hooking into tmux natively is feasible or desirable.

### 4. Zellij-Style Confirmation
**Status: noted, not urgent**

Zellij prompts before re-executing commands in restored panes. Worth considering as an option but not a primary research thread. May be relevant to the hook lifecycle design.

### 5. Save Triggers
**Status: needs research**

When should Portal capture state? Options include:
- Event-driven (on session create/destroy — Portal controls this)
- Periodic timer
- Explicit command (`portal save`)
- Some combination

Needs research into what events tmux exposes and what Portal can hook into.

---
