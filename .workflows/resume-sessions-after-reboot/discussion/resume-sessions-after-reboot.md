# Discussion: Resume Sessions After Reboot

## Context

Portal manages tmux sessions. After a system reboot, tmux-resurrect restores session layout, CWDs, and pane structure — but processes are dead. The idea is a generic restart command registry so Portal can resume processes (Claude Code, dev servers, etc.) in their original panes.

The core problem: tmux-resurrect's `@resurrect-processes` re-runs original launch commands, which doesn't work for tools like Claude Code where the resume command differs from the launch command (`claude --resume <uuid>` vs `claude`).

### Key Research Findings

- Claude Code's `SessionStart` hook provides `session_id` (the UUID needed for resume) and fires on every session start/resume
- `$TMUX_PANE` is available in every tmux process, so Portal can discover pane association without explicit passing
- Tmux pane IDs (e.g. `%0`, `%1`) persist across resurrect
- A hook script stays tool-specific — it only knows about Claude's session ID; Portal handles pane mapping internally

### References

- [Research: Resume Sessions After Reboot](./../research/resume-sessions-after-reboot.md)

## Questions

- [x] What scoping model should the registry use — per-pane, per-session, or per-window?
- [x] When should restart commands execute — eagerly on bootstrap, lazily on session select, or hybrid?
- [ ] Should Portal detect dead processes or just execute whatever is registered?
- [ ] Should Portal confirm before sending commands to panes, or auto-execute?
- [ ] What should the subcommand be called and what's the CLI surface?
- [ ] What storage format and location for the registry?
- [ ] How should stale registrations be handled (pane closed before reboot, session deleted)?
- [ ] Should Portal warn about or prevent conflicts with `@resurrect-processes`?

---

*Each question above gets its own section below. Check off as completed.*

---

## What scoping model should the registry use?

### Context

A tmux server runs multiple sessions, each with windows (tabs), each with panes (splits). A single project session might have 4 Claude instances across 4 panes. The registry needs to map restart commands to the right target.

### Options Considered

**Per-pane (flat map: pane_id → command)**
- Pane IDs (`%0`, `%1`, etc.) are globally unique across the entire tmux server
- Assigned sequentially by the server, not scoped to session or window
- Persist across tmux-resurrect
- No need to model session/window hierarchy — tmux already knows which panes belong to which session

**Per-session or per-window**
- Can't handle multiple processes per session/window without becoming "list of per-pane commands" anyway
- Adds hierarchy to the registry that tmux already provides

### Decision

**Per-pane, flat registry.** A simple `pane_id → restart_command` map. No session or window hierarchy stored — when Portal needs to act on a session's processes, it queries tmux for that session's panes and cross-references the registry. This is the simplest model that handles all cases.

---

## When should restart commands execute?

### Context

After reboot and resurrect, registered commands need to fire at some point. The question is whether to restart everything immediately or wait until the user actually needs a session.

### Options Considered

**Eager (on server bootstrap)**
- Everything warm by the time you interact
- But could fire up 6+ Claude instances across projects you won't touch today
- Resource-heavy for no benefit on unused sessions

**Lazy (on session select/attach via Portal)**
- Only restarts what you actually use
- Portal already mediates session selection — natural trigger point
- Small delay on attach while processes spin up

**Hybrid**
- Priority system for some eager, some lazy
- Added complexity for unclear benefit

### Journey

Eager was quickly ruled out — spinning up heavy processes across all sessions on boot is wasteful. Lazy fits naturally because Portal already has the moment where the user selects a session (`portal open`). That's the trigger.

Key clarification: this is a Portal-mediated action, not a tmux hook. If someone uses raw `tmux attach`, they've bypassed Portal and don't get Portal's benefits. Portal doesn't try to hook in backwards. Clean boundary — Portal owns the registry, Portal triggers the restarts.

### Decision

**Lazy execution, triggered during Portal's connection flow.** Restart commands fire when the user connects to a session via Portal (e.g. `portal open`). No eager startup, no tmux-level hooks. Bypass Portal, bypass the registry.

---
