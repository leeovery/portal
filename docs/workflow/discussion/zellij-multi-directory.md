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

