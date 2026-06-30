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

  Discussion Map — Restore Host Terminal Windows (12 subtopics — 1 decided · 11 pending)

  ┌─ ✓ 1. Spawn-execution architecture — where the reopen runs from [F6] [decided]
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

## 1. Spawn-Execution Architecture

### Context

Research (review-F6) framed this as "where spawn executes architecturally": picker action shelling out from the TUI process vs a new `portal` subcommand vs both — flagged as coupled to identity detection because it determines which process's env feeds detect-self, how the attach line is assembled, and whether a headless/scriptable reopen is possible. It's the keystone: settling it shapes the config schema, test seam, and daemon footprint.

### The constraint that narrows the space

The decision is tighter than F6 implies. The **no-leftover-window** anti-requirement (net N windows, never N+1) forces the picker to **own its own window reuse**: it turns its own host window into one session via `switch-client` (inside tmux) or exec-`tmux attach` (outside tmux), which *replaces the picker process* so the window becomes a session rather than falling back to an empty shell. Therefore the picker always self-attaches to one of the N; only the **N−1 others** are externally spawned, and each just runs the **existing `portal attach <session>`**. So "where spawn runs" reduces to: *where does the detect-terminal + spawn-the-N−1 logic live?*

### Options Considered

**Option A — inline in the TUI.** The Bubble Tea process, on `Enter`, detects the host terminal and fires the spawns itself, then self-attaches.
- Cons: spawn logic buried in the update loop is hard to unit-test; capability locked inside the TUI (no headless/scriptable reuse); no clean DI seam.

**Option B — shared internal package + `portal reopen` subcommand (chosen).** Detection + adapter resolution + spawn live in an internal package; `portal reopen <sessions…>` is a thin CLI over it; the picker calls the **same package in-process** for the N−1, then self-attaches.
- Pros: argv→effects boundary is unit-testable with a faked `Adapter` (command construction, detect-self resolution, precedence); `portal reopen` becomes a first-class headless command the deferred "remember-and-restore workspace" + Spaces follow-ons can reuse; matches the project's DI pattern.
- Cons: slightly more surface than A (a new subcommand + package).

### Journey

Started from F6's three-way framing (picker vs subcommand vs both). Realised the "both" tension mostly dissolves once you see the picker *must* keep ownership of its own window reuse (the anti-leftover rule), so the subcommand can never own the whole flow — it owns the N−1 spawns, the picker owns its self-attach. That reframes A-vs-B as purely "where does the reusable spawn logic live," which testability + the explicitly-deferred workspace-restore feature settle decisively for B.

Considered detection placement as a complication (does the subcommand vs TUI change what env detect-self sees?) and concluded it doesn't fight the choice: detection's backbone is the process-tree walk (`list-clients` → client PID by highest `client_activity` → walk to terminal bundle id), a library call both callers can make; env vars are only an optional fast-path. Detection anchors on the **triggering picker process** — outside tmux it walks its *own* tree to the terminal; inside tmux it hops via `list-clients` to the host client and walks that (one extra hop, same destination). Full identity resolution is subtopic #7.

Walked the concrete 3-session flow to confirm the model: (1) detect terminal → (2) one `osascript` call per N−1 window, each carrying `portal attach <session>` as its startup command → (3) exec self into the last session. **Order is load-bearing**: step 3 is a point of no return (exec replaces the picker), so the N−1 spawns must complete first. One spawn call per window (not one combined script) for failure isolation.

In-process vs subprocess for the picker→reopen call: chose **in-process** so spawn errors surface back into the TUI where the user is looking; the `portal reopen` subprocess remains the headless/test front door. Both the "in-process vs subprocess" detail and "does the picker wait to confirm the N−1 spawned before it execs into the Nth?" are **coupled to #3** (partial-failure contract) — left open there.

### Decision

**Option B.** Build a shared internal reopen package (detection + adapter resolution + spawn), exposed two ways: called **in-process by the picker** for the N−1 spawns, and as a **`portal reopen <sessions…>` subcommand** for headless/scriptable/test use. Each spawned window runs the existing `portal attach <session>`; `portal reopen` is *not* what runs in the new windows. The picker self-attaches to the remaining session via its existing connector, reusing its own window (anti-leftover). Confidence: high.

- **Mental model:** one service, two callers — like a Laravel Service class reached from both an Artisan command and an HTTP controller.
- **Coupled-out:** in-process-vs-subprocess + wait-for-spawn-confirmation → #3; full terminal-identity detection → #7.
- **Impl flag (review-002 F3, for spec):** spawned windows run `portal attach` as their startup command, so `portal`/`tmux` must be on `PATH` in Ghostty's launch context (not guaranteed a login shell).

---

## Summary

### Key Insights

*(captured as the discussion progresses)*

### Open Threads

*(captured as the discussion progresses)*

### Current State

- Research foundation settled (see Context); 12 live subtopics seeded.
- **#1 Spawn-Execution Architecture — decided** (Option B: shared reopen package + `portal reopen` subcommand, picker calls in-process; N−1 spawned, picker self-reuses for the Nth).
- Open coupling threads: #3 (partial-failure / in-process-vs-subprocess / wait-for-spawn), #7 (terminal-identity detection).

## Triage

(none)
