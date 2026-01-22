---
topic: zellij-multi-directory
status: in-progress
date: 2026-01-22
---

# Discussion: Zellij Multi-Directory Sessions

## Context

CX's current design assumes a 1:1 relationship between Zellij sessions and project directories. However, Zellij sessions can have **multiple panes spread across multiple directories/projects**.

This challenges the core model and could dramatically change the tool's architecture.

### References

- [CX Design Discussion](cx-design.md) - Concluded design assuming simpler model
- [CX Tool Plan](../research/cc-tool-plan.md) - Initial research

### Current Model Assumptions (from cx-design.md)

1. **sessions.json** maps each session to exactly one project path
2. **cd before attach** - CX changes to project dir before attaching
3. **Project as anchor** - TUI organizes around projects, sessions as children
4. **Session naming** - `{project-name}-{NN}` implies project ownership

## Questions

- [ ] How do users actually use Zellij multi-pane sessions?
- [ ] Does the session → project mapping need to change?
- [ ] What should "cd before attach" mean for multi-directory sessions?
- [ ] How does this affect the TUI model?
- [ ] Is the naming convention still valid?

---

*Each question gets its own section below. Check off as concluded.*

---

## How do users actually use Zellij multi-pane sessions?

### The Problem

Zellij allows multiple panes per session, each potentially in a different directory. Need to understand how this is actually used in practice to determine if CX's model needs fundamental changes.

### Journey

**Confirmed Zellij behavior:**
- No interactive picker - `zellij attach` without args just lists sessions, requires you to pass a name/prefix
- Sessions restore fully on reattach - not "cd-ing", the shell process never died
- You can pass any unambiguous prefix to attach

**Key realization:**
All three multi-pane scenarios (monorepo, separate projects, mixed) are valid use cases. This means session ≠ project. Session = workspace.

**Existential question raised:** Is CX even needed?

User already has aliases wrapping Zellij (`cc` function). The original "cd to project before attach" value prop disappears since Zellij restores everything.

### Options Considered

**CX as "Claude executor"** (original vision)
- Tightly coupled to Claude Code
- Manages project → session mapping
- Auto-starts Claude in sessions
- *Problem*: Model assumes 1:1 session-project, which doesn't hold

**CX as "workspace manager"** (emerging vision)
- Zellij wrapper with better UX
- Interactive picker (Zellij lacks this)
- Workspace = session, may span multiple directories
- Claude is incidental, not core
- *Problem*: Might be a thin wrapper over `zellij attach`

**Just use aliases** (status quo)
- Already works
- No maintenance burden
- *Problem*: Limited discoverability, no rich UI

### Decision

*Pending - need to explore what value CX adds over aliases...*

---

## What defines a workspace?

### The Problem

If CX is a workspace manager (not project manager), how do workspaces get created and defined? This is the crux of whether CX is worth building.

### Journey

**What user actually wants:**
1. One command (`cx`) → interactive picker
2. Picker shows existing Zellij sessions (queried live)
3. Picker shows remembered projects (for starting new sessions)
4. Select session → attach
5. Select project → cd + start new Zellij session

**Key simplification:** No session → project mapping needed. They're separate concerns:
- Sessions = live data from `zellij ls`
- Projects = CX's memory of "directories I've started sessions in before"

CX doesn't track which project a session belongs to. Select session → attach. Select project → new session.

**New session flow:**
1. Select `[n] new session...`
2. Project picker (remembered dirs + browse option)
3. Name prompt: "Workspace name? [default: dir-name]"
4. cd to dir, `zellij attach -c <name>`

### Decision

*Pending - exploring what info we can show per session...*

---

## Can we query session details from outside Zellij?

### The Problem

Zellij's built-in session manager shows rich info (tabs, panes, commands). Can CX show similar detail from *outside* a running session?

### Research Findings

**Yes - via CLI actions with `--session` flag:**

```bash
# Get tab names for a specific session
zellij --session my-session action query-tab-names

# Dump full layout (KDL format)
zellij --session my-session action dump-layout

# List connected clients
zellij --session my-session action list-clients
```

This works from *outside* Zellij, targeting any running session by name.

**For exited (resurrectable) sessions:**

Layout stored in cache:
```
~/.cache/zellij/<version>/session_info/<session-name>/session-layout.kdl
```

KDL file contains tabs, panes, cwds, commands:
```kdl
layout {
    tab name="my-tab" cwd="/some/path" {
        pane command="claude" cwd="/project/dir"
        pane command="htop"
    }
}
```

**Parsing considerations:**
- `query-tab-names` returns simple text (easy)
- `dump-layout` returns KDL (need parser - Go has `github.com/sblinch/kdl-go`)
- JSON output requested but not yet implemented (Issue #4212)

### Decision

CX can show tab names per session:
```
>  cx-03          ● attached
   └─ 2 tabs: "Claude Code", "Tests"
```

Implementation: call `zellij --session <name> action query-tab-names` for each session.

For more detail (pane cwds, commands), parse `dump-layout` KDL output.

---

## What should the TUI look like?

### The Problem

Need a mobile-friendly, keyboard-driven TUI that works at bare shell (before Zellij is running).

### Journey

**User's actual workflow:**
1. SSH/Mosh to machine → drops into `~` (bare shell)
2. Run `cx`
3. Full-screen picker appears (optimized for small screens)
4. Arrow to selection, Enter
5. Attached to session (or new one started)

**Zellij's built-in session manager** exists but:
- Only works *inside* an existing Zellij session
- Too information-dense for mobile screens
- Can't start from bare shell

### Options Considered

**Minimal info per session:**
```
SESSIONS
  cx-03       ● attached
  api-work
```

**With tab names:**
```
SESSIONS
  cx-03       ● attached
  └─ 2 tabs: "Code", "Tests"
  api-work
  └─ 1 tab: "Main"
```

**With pane details (like Zellij's manager):**
- Too dense for mobile
- Overkill for just picking a session

### Decision

**Minimal with optional expansion.** Default view shows session names + attached status. Could add tab names as one-liner if useful, but keep it scannable.

```
┌─────────────────────────────────────┐
│                                     │
│           SESSIONS                  │
│                                     │
│    >  cx-03          ● attached     │
│       api-work       2 tabs         │
│       client-proj                   │
│                                     │
│           EXITED                    │
│                                     │
│       old-feature    (resurrect)    │
│                                     │
│    ─────────────────────────────    │
│    [n] new in project...            │
│                                     │
└─────────────────────────────────────┘
```

---

## Summary (so far)

### Model Shift

From **project-centric** (original cx-design.md) to **workspace-centric**:

| Original Model | New Model |
|----------------|-----------|
| Session belongs to project | Session = workspace (may span dirs) |
| sessions.json maps session→project | No mapping needed |
| cd before attach | Not needed (Zellij restores) |
| TUI organized by projects | TUI organized by sessions |
| Naming: `{project}-{NN}` | User chooses workspace name |

### CX's Value Proposition

1. **Interactive picker at bare shell** - what Zellij's session-manager does, but *before* you're inside Zellij
2. **Mobile-friendly** - cleaner, less dense than Zellij's built-in
3. **Project memory** - quick start new sessions in remembered directories
4. **One command** - `cx` does everything vs. `zellij ls` + `zellij attach <name>`

### Open Questions

- [ ] Is this enough value over aliases + `zellij ls | fzf`?
- [ ] Session naming convention - still `{name}-{NN}` or free-form?
- [ ] Should CX manage layouts for new sessions?

