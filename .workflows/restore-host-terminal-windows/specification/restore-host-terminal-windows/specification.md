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

## Spawn Architecture

### Model: one service, two callers

Spawn logic lives in a shared internal package (`internal/spawn`): terminal detection, adapter resolution, and window spawning. It is reached two ways:

- **In-process by the picker** — on `Enter`, the picker calls the spawn package directly to open the N−1 external windows, then self-attaches to the Nth. In-process (not a subprocess) so spawn errors surface back into the TUI where the user is looking, and so the picker can collect per-window acknowledgements to decide success/rollback (see *Burst & Partial-Failure Contract*).
- **As `portal spawn <sessions…>`** — a thin CLI over the same package. This is the test seam, backs a `--detect` dry-run, and is the entry point the deferred workspace-restore/Spaces follow-ons reuse. It always runs from a terminal context, never truly headless.

Mental model: one service reached from both a CLI command and the TUI.

### The N vs N−1 split (anti-leftover rule)

The **net-N-windows** anti-requirement forces the picker to own its own window reuse. The picker turns its *own* host window into one of the N selected sessions:

- **Outside tmux** → exec `tmux attach` (existing `AttachConnector`), which replaces the picker process so its window becomes a session.
- **Inside tmux** → `switch-client` (existing `SwitchConnector`).

So the picker **always self-attaches to exactly one** of the N; only the **N−1 others** are externally spawned. Each spawned window runs the **existing `portal attach <session>`** command — `portal spawn` is *not* what runs inside the spawned windows.

### Order is load-bearing

1. Detect the host terminal.
2. Spawn the N−1 windows (one adapter call per window — for failure isolation), collecting each window's ack.
3. **Only after all N−1 confirm**, exec self into the Nth session.

Step 3 is a point of no return (exec replaces the picker), so the N−1 spawns must complete first. This ordering is what makes cancellation and all-or-nothing rollback clean (see *Burst & Partial-Failure Contract*).

### Command composition — spawn via the picker's own executable

The N−1 windows spawn running **`<os.Executable()> attach <session>`** — the picker's own absolute binary path, **not** a bare `portal` PATH lookup. Rationale: the warm-command latch is **version-gated** (satisfied only when stored version == running version, per `state.BootstrappedLatchSatisfied`). A PATH-resolved spawn of a *different* portal version would read the latch unsatisfied and full-bootstrap per window, resurrecting the burst storm. Using the picker's own binary guarantees version parity → latch satisfied → each attach takes the abridged fast-path.

Side effect: `portal` no longer needs to be *on* `PATH` (only `tmux` does, since portal shells out to it).

### Spawned-window environment (PATH injection)

The host terminal launches the spawned command in a **bare environment**. (Validated on Ghostty: its `command` execs an **argv, not a shell**, in a bare `PATH` — `/usr/bin:/bin:/usr/sbin:/sbin` plus Ghostty's bin — with no Homebrew/login `PATH`, so `tmux` and any subprocess `portal` shells to would not be found.)

Fix: the picker resolves what the spawn needs and **injects its own full `PATH` (and required env) into the spawned window's environment** so `tmux` resolves. Combined with the absolute-`portal` path above, both `portal` and `tmux` resolve. The command handed to the terminal is a real **argv** (`<abs>/portal attach <session>` plus the ack token), never shell syntax. For the native Ghostty adapter this is Ghostty's `environment variables` property; each adapter owns its own equivalent (see *Adapter Contract*). The config-driven path gets the same guarantee uniformly (see *Config Schema*).

---

## Multi-Select Mode (TUI Interaction)

### Trigger & marking

- **`m` enters an explicit multi-select mode** from the normal Sessions list. It is a real mode you can sit in with **zero selected** — not an implicit mark-on-entry. `M` (uppercase) stays retired (per §12.2's dropped uppercase bindings).
- **`m` again toggles the cursor (highlighted) row** in/out of the selection. The same key both enters the mode and toggles marks — no second key.
- **`Enter` = open the marked set** (runs the pre-flight → all-or-nothing spawn flow). Enter stays "commit" in both modes: normal mode attaches the cursor row, multi-select mode opens the marked set.
- **`Esc` = exit mode and clear selection.**
- Grouping `HeaderItem` rows are non-selectable and skipped by marking/navigation (existing `skipHeaderRow` invariant).

### N=0 / N=1 boundary

- **N=1** (one marked, Enter): zero windows to spawn — the picker self-attaches to that one session, i.e. it **degenerates to a plain single attach** in the current window. No special-casing.
- **N=0** (nothing marked, Enter): a **no-op that exits multi-select mode**, dropping back to the standard picker (Portal stays open) — same effect as `Esc`. Nothing opens.

### Key coexistence within the mode

- **Live in mode:** `Space` (preview — a firm requirement, still useful while selecting), `/` (filter), `s` (regroup). `/` and `s` stay live so you can filter/regroup to find things to mark.
- **Suppressed in mode:** `k` (kill), `x` (page-toggle), `r` (rename), and other row actions.
- While the `/` filter is focused, `s` and `m` are literal filter characters (the filter input owns typing).

### Sticky selection

Selection is **sticky** across filtering, paging, regrouping, **and the `Space`-preview round-trip**. On return from preview, `rebuildSessionList` re-renders **in-mode with the selection intact**, pruning only a selection whose session was **externally killed** during the preview (consistent with the pre-flight rule — a gone session can't be opened). A row filtered out stays selected and reappears when the filter clears.

### Filter as an inner sub-state

Filter is an **inner sub-state** of multi-select — the existing filter/browse layering, nested:

- **The focused filter input owns `Enter`/`Esc`.** While the filter is focused it keeps its normal meaning (`⏎`/`↓` commit-to-browse, `Esc` clear-filter); multi-select's `⏎` (open-marked) and `Esc` (exit-mode) apply **only when the filter is not focused**.
- **The single notice-band header slot time-shares by focus:** filter-focused → orange filter line + filter footer; otherwise → the multi-select banner + multi-select footer. One claimant at a time (single-slot arbiter).
- **Selections persist underneath** while filtering.

### Mode affordance (visual)

Multi-select must be **as unmistakably a distinct mode as filtering is**, modelled on filter mode:

- Its own **mode colour** + a **banner** in the existing notice-band slot (single-slot arbiter — the multi-select banner owns the slot while in mode), reading e.g. `N selected · m toggle · space preview · ⏎ open · esc cancel`.
- **Selected rows carry a glyph marker + the mode colour, never colour-only** (MV's NO_COLOR / colourless-render rule).
- Exact colour token, glyph, and banner/footer copy are fixed by the delivered Paper design (see *Design References*): **violet** reused as the selection accent, `●` marker on selected rows, footer `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel`. No new colour tokens.

### Granularity: per-session only

Group-select (marking a whole project/tag group via its header) is **deferred as separate future work** — it would require letting the cursor land on the currently non-selectable `HeaderItem` rows. This feature ships **per-session marking only**.

---

## Working Notes

[Optional - capture in-progress discussion if needed]
