# Specification: Restore Host Terminal Windows

## Overview

### Problem

Portal restores the **tmux/server layer** after a reboot (sessions/windows/panes rebuild on attach) but not the **host-local terminal layer** — the terminal-emulator windows that fronted those sessions. After a crash with ~32 sessions, the server layer reattaches but the user still rebuilds every macOS terminal window by hand (~14 Spaces, one project zone per Space) — roughly an hour of manual work.

### What this feature does

Adds a **multi-select mode** to the Sessions page of the Portal picker. The user marks N sessions and presses `Enter`; each selected session springs open **attached in its own host terminal window**. Net result is **N windows for N selected sessions** — the trigger (picker) window is reused as one of the N; the other N−1 open as fresh host windows.

### Scope yardstick (what "done" means)

This feature collapses the *attaching* into one action per batch — a deliberate **partial win** the user explicitly accepts. It does **not** remember groupings or place windows on specific macOS Spaces; all N windows open on the current Space. Remember-the-grouping and Spaces placement are separate future features.

### Foundational shape (settled)

- **Multi-select mode** on the Sessions page (trigger `m`) → mark → `Enter` → each springs open attached in its own host window. Built as a *general selection mode* with spawn as its first action (future bulk ops can reuse it).
- **Windows only** — no window-vs-tab fidelity, which removes the entire host-window introspection requirement.
- **Net N windows, never N+1** (hard anti-requirement): the trigger window is reused as one session; only the N−1 others are externally spawned. No leftover empty picker window.
- **Ghostty-first**, cross-terminal via built-in Go adapters + a user-config escape hatch (`terminals.json`), both shipped in this feature.
- **Terminal identity** detected by walking from the triggering process/client to the host terminal's macOS bundle id; remote/mosh clients → NULL → honest no-op.
- **No duplicate-surface guard** — opening an already-attached session is a fine no-op (tmux synchronises both clients).

### Hard dependency (satisfied)

Each spawned window runs `portal attach`, which flows through the full bootstrap unless a warm-server fast-path exists. This feature depended on the **`warm-command-bootstrap-latch`** feature (the version-stamped `@portal-bootstrapped` server-option latch + abridged fast-path, `state.BootstrappedLatchSatisfied`). That feature is **done and merged to `main`** (verified 2026-07-11), so a warm burst is N cheap abridged attaches. Spawn spawns plain `portal attach` with no bootstrap special-casing.

### Out of scope / deferred

- Group-select (marking a whole project/tag group via its header).
- Remembering groupings; macOS Spaces placement; window arrangement/focus control.
- Host-window introspection / window-vs-tab fidelity.
- A truly headless `portal spawn` (no terminal context) and a `--terminal` override.
- A defensive `@portal-spawn-*` marker sweep (drop-in if ever needed).
- Additional adapter capabilities (`introspect` / `place-on-space`) beyond open-window.

### Naming (provisional)

Ships as `portal spawn <sessions…>`, internal package `internal/spawn`, `spawn` log component, `@portal-spawn-*` markers. A logged `cli-verb-surface-redesign` idea may later rename the CLI verb; the picker calls the spawn *package* in-process, so the verb is a secondary, cheap-to-rename surface.

---

## Working Notes

[Optional - capture in-progress discussion if needed]
