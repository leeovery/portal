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
- [x] Should Portal detect dead processes or just execute whatever is registered?
- [x] Should Portal confirm before sending commands to panes, or auto-execute?
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

## Should Portal detect dead processes or just execute whatever is registered?

### Context

After reboot, panes have dead processes (empty shells). But in normal operation, a pane might still have its process running. Portal needs to know when to fire a restart command and when to skip.

### Journey

Initially considered two simple approaches: registry-driven (just execute if entry exists) or detection-first (check if process is dead). Both had problems — registry-driven would re-execute on normal reattach; detection couldn't distinguish "user quit Claude" from "reboot killed Claude."

The real problem is distinguishing "server restarted" from "same server, process just stopped." An "executed" flag was proposed but had a flaw: if the user manually exits Claude without the exit hook firing, the flag would be unset, causing a false restart on next attach.

Server PID/timestamp tracking was explored — record which server lifetime the entry was registered under, compare on attach. Felt brittle and overcomplicated.

The breakthrough: **use the tmux server itself as volatile storage.** Set a tmux server-level option (`set-option -s @portal-active-{pane_id}`) when registering. This marker lives only in server memory — it dies when the server dies and resurrect does NOT restore it.

Verified via research:
- `set-option -s @custom-var` works — tmux supports `@`-prefixed user options at server level
- tmux-resurrect does not save or restore any tmux options (server, session, window, or pane level)
- `set-environment -g` also dies with the server and isn't restored

### Decision

**Two-condition check: persistent entry exists AND volatile marker absent.**

- **Persistent store** (file on disk): `pane_id → restart_command` — survives reboot
- **Volatile marker** (tmux server option): `@portal-active-{pane_id}` — dies with server
- **On register**: write persistent entry + set volatile marker
- **On deregister**: remove persistent entry + remove volatile marker
- **Execute when**: entry exists AND marker absent

No process detection needed. No executed flags. No server PID tracking.

| Scenario | Entry? | Marker? | Result |
|----------|:---:|:---:|--------|
| Reboot, tool was running | Yes | No (server died) | Execute |
| Normal reattach, tool running | Yes | Yes | Skip |
| Reattach after clean exit | No (deregistered) | No (removed) | Skip |
| Reattach after crash/kill -9 | Yes | Yes (same server) | Skip |
| Reboot after clean exit | No | No | Skip |
| Reboot after crash (no deregister) | Yes | No (server died) | Execute |

Row 6 (crash then reboot) is arguably correct — tool was running, didn't signal intentional shutdown, server restarted. User can close it again if unwanted.

---

## Should Portal confirm before sending commands to panes, or auto-execute?

### Decision

**Auto-execute.** The entire point is restoring state to what it was before reboot. Confirmation would defeat the purpose — the user already registered these commands as "restart me." The two-condition check (entry + no marker) provides sufficient safety. If something restarts that shouldn't have, the user can close it.

---
