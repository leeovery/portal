# Discussion: Restore Host Terminal Windows

## Context

Portal's resurrection layer restores the **tmux/server layer** after reboot (sessions/windows/panes rebuild on attach), but not the **host-local terminal layer** — the actual terminal-emulator windows that fronted those sessions. After a crash with ~32 sessions, ~28 reattached at the server level, but the user still rebuilt every macOS terminal window by hand (~14 Spaces, one project zone per Space) — roughly an hour of manual work.

This feature lets the user reopen N sessions, each springing into its own host terminal surface, via a **multi-select** in the Portal picker. Research closed the feasibility questions and locked a set of foundational decisions; this discussion resolves the **live design and operational decisions** that remain.

### Foundation already settled in research (not re-litigated here unless reopened)

- **MVP shape:** a Sessions-page **multi-select mode** (proposed `M`) → select sessions → `Enter` → each springs open in its own host window, attached. Implemented as a *general selection mode* with spawn as its first action (future bulk ops can reuse it).
- **Windows-only v1:** window-vs-tab fidelity dropped → removes the entire introspection requirement.
- **Spawn command:** the N−1 new windows each run **`portal attach <session>`** (existing chokepoint connector); the **trigger window is reused** as one session via `switch-client`. Net window count = **N, not N+1** (no leftover empty picker window — a hard anti-requirement).
- **Cross-terminal:** Ghostty-first; **dual configurability** (built-in Go adapters + user-config override/escape hatch), shipped in v1. Precedence: **config override → native adapter → unsupported**.
- **Identity (feasibility-validated live):** detect the host terminal via **client-PID → process-tree walk → macOS bundle id**, matched as a **family** (e.g. `dev.warp.Warp-*`), with a **friendly alias** (`ghostty`) as the user-facing key. Client resolved by **highest `client_activity`** (`focused` is unreliable). Remote/mosh clients → NULL bundle id → honest no-op.
- **Unsupported-terminal UX:** info **banner** (not modal) naming the detected identity.
- **Duplicate-surface guard:** none — opening an already-attached session is a fine no-op (tmux synchronises both).
- **Scope yardstick:** MVP is "collapse the attaching into one action per batch" — a **partial win** the user explicitly accepts. Remember-the-grouping + macOS Spaces placement are deferred follow-ons (Spaces already parked in inbox).

### References

- Research: [restore-host-terminal-windows.md](../research/restore-host-terminal-windows.md)
- Deep-dives (cache): terminal-automation-surface (001), identity-detection (002/003)

## Discussion Map

A living index of subtopics tracked during the discussion. Grows as the conversation branches, converges as decisions land.

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — Restore Host Terminal Windows (12 subtopics · all pending)

  ├─ ○ 1. Spawn-execution architecture — where the reopen runs from [F6]
  ├─ ○ 2. Multi-select trigger & keymap coexistence [F7]
  ├─ ○ 3. Burst & partial-failure contract [F1]
  ├─ ○ 4. Trigger-context matrix (in/out tmux × attached × includes-self) [F2]
  ├─ ○ 5. TCC first-run Automation-permission flow [F4]
  ├─ ○ 6. Config schema & command representation [F9]
  ├─ ○ 7. Terminal-identity UX — what we display & accept as config key [rv2-UX]
  ├─ ○ 8. Adapter contract shape & extensibility (capability-based) [fwd-looking]
  ├─ ○ 9. Testing strategy & DI seam [F5]
  ├─ ○ 10. Daemon / state footprint of windows-only v1 [F10]
  ├─ ○ 11. Attach contention vs post-reboot hydration [F12]
  └─ ○ 12. Pre-build validation flags (lsappinfo/ps stability, activity-bump timing) [rv2-F4/F5]

---

*Subtopics are documented below as they reach `decided` or accumulate enough exploration to capture.*

---

## Summary

### Key Insights

*(captured as the discussion progresses)*

### Open Threads

*(captured as the discussion progresses)*

### Current State

- Research foundation settled (see Context); 12 live subtopics seeded, all pending.

## Triage

(none)
