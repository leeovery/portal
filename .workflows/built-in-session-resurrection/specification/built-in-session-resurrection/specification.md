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

---

## Working Notes
