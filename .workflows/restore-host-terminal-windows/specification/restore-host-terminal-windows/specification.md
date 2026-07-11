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

## Burst & Partial-Failure Contract

### Framing

The motivating scenario is a *large* burst (rebuild ~14 windows post-crash), not the clean 3-window path. The "burst = N concurrent full bootstraps" concern is dissolved by the warm-command latch dependency (each attach takes the abridged fast-path). So this contract is about genuine **spawn/attach partial failure**, not bootstrap contention.

### Stance: pre-flight + all-or-nothing

Either the whole batch opens, or nothing does.

**Pre-flight validate on Enter.** Before opening a single window, verify every selected session still exists (quick `has-session` checks). The dominant failure cause is a session killed between picker-load and Enter; pre-flight catches exactly that. If any selected session is gone:

- **Abort atomically** — nothing spawns, no window opens, no self-attach.
- Show a clean one-line error in the picker naming the gone session (design copy: `⚠ '<session>' is gone — nothing opened`), and stay put in multi-select mode with the remaining selections intact.
- Zero windows opened → no rollback, no flash.

**Spawn, then self-attach LAST — gated on ALL N−1 confirming.** After pre-flight passes, sequentially spawn the N−1 and collect their acks:

- **All confirm** → the trigger window self-attaches silently (no "14/14 ✓" nag).
- **Any fails** (a transient `osascript`/terminal hiccup *after* pre-flight passed — genuinely rare) → **roll back**: close the windows that opened (safe — it detaches the client; the tmux sessions persist), skip the self-attach, show the same clean error; back in the picker to redo.

This deletes the report / `r retry` / deferred-attach tangle entirely. Trade-off accepted: on a rare mid-rebuild failure you get nothing and re-select, rather than keeping the partial.

### Confirmation mechanism: explicit token ack

`osascript` returning success is shallow — it only confirms "the terminal accepted the request," not that the window rendered, `portal` ran, the session existed, or attach happened.

- **Rejected — tmux client-watching** (snapshot `list-clients`, diff new clients). Fragile here: lingering/reconnecting mosh clients churn the client list during the exact burst window, risking false confirms or masked failures.
- **Chosen — explicit token ack.** The picker issues a **batch id + per-window token**, threads it into each spawned command (arg/env); the spawned `portal attach` **writes its token right before exec**; the picker watches for the token set with a **timeout**. A missing token at timeout = a failed spawn → abort + roll back. A direct signal from our own spawned process, immune to how many other clients are attached — this is what makes spawning a session **already attached elsewhere** (e.g. the iPhone) confirm correctly.

**Ack channel.** A namespaced **`@portal-spawn-<batch>-<session>` tmux server option**, behind a small ack seam (write-token / collect-tokens interface). Code-verified safe: the only all-server-options enumerator, `ListSkeletonMarkers`, skips any name not prefixed `@portal-skeleton-` (`internal/state/markers.go`), so a distinct `@portal-spawn-` prefix is invisible to it; namespacing isolates sweeps in both directions; server options die with the server.

**Timeout is per-window, not global.** Under sequential spawn the Nth window's `osascript` fires seconds after Enter and then runs its own abridged attach before writing its token; a single global clock from Enter would over-report late windows as failed. Each window's ack timer starts when *its* spawn fires — the cumulative sequential delay never eats the budget.

**Honest boundary.** The ack fires at the last instant before exec (once `portal` execs into tmux it's replaced, so it can't ack *after* attaching). It confirms "window opened, `portal` ran, session found, attach handoff starting" — covering every real failure mode; the final tmux handoff is essentially guaranteed once there.

**Cleanup.** The picker self-cleans its batch markers before self-exec (and on abort/rollback). Bounded, harmless leaks (a late-laggard ack, a crashed picker) self-expire with the server and never collide (unique batch ids). A defensive `@portal-spawn-*` sweep mirroring bootstrap's `CleanStaleMarkers` is a drop-in if ever needed — deferred.

### Sequential spawn

Spawn the N−1 **sequentially** (one adapter/`osascript` call completes before the next fires). The token ack already makes spawn *order* irrelevant to reporting, so the choice rests on: sidesteps the unverified rapid-fire AppleScript throughput risk, gives clean per-window cancellation points, and turns per-window focus-steal into an orderly cascade rather than thrash. (Validated: 4 sequential `osascript` opens ~1.05s / ~260ms each → a 14-window burst is ~3–4s, no pacing needed.) Reversible — flip to parallel only if a future validation shows it both safe and meaningfully faster.

### Cancellation

Self-exec being the *last* step keeps cancellation clean: `Ctrl-C`/`Esc` before it aborts (roll back what opened); after it there is nothing to cancel (already attached).

### Deferred hardening (recorded, not built)

Because the picker always bootstraps first (its own `PersistentPreRunE`) and stamps the latch to its own version, then spawns that same binary, the latch is always satisfied at burst time and no spawned window full-bootstraps. The only residual is a mid-picker-session in-place binary swap (negligible; a full bootstrap is a safe no-op). A conditional "if the first spawn triggers a full bootstrap, wait for its ack before firing the rest" — which would cap it at exactly one bootstrap — is **deferred as YAGNI**; the ack is the natural wait-signal if ever wanted.

---

## Working Notes

[Optional - capture in-progress discussion if needed]
