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

## Trigger-Context Matrix & Open Order

### Behaviour across trigger contexts

- **In vs out of tmux at trigger.** *Out* (bare-shell picker): the trigger window reuses via `AttachConnector` (exec `tmux attach`); detection walks the picker's own process tree. *In* tmux: the trigger window reuses via `SwitchConnector` (`switch-client`); detection takes the `list-clients` → client-PID hop. The **spawned N−1 are always fresh host windows running `portal attach` out of tmux**, independent of the picker's context; only the trigger-window reuse differs. One mental model, inside or out.
- **Selected session already attached elsewhere** (this host or a remote/iPhone client): allowed — no dup guard; the token ack confirms *our* new window regardless of other clients.
- **Includes-self** (selection includes the current context's session): the trigger window becomes one attached session, the rest spawn; the marked origin session ends up attached either way.
- **Selected session vanished** between picker-load and Enter: caught by the pre-flight check → atomic abort, nothing opens.

### Enter opens the marked set only

The cursor/highlight at Enter time is irrelevant — a highlighted-but-unmarked row is **not** opened (marking is `m`, not Enter). Enter always commits the `m`-marked set.

### Open order: list order (selection is a set)

Open in **list order** (top-to-bottom as shown), not pick order. The selection is a plain **set**, not an ordered list. Pick-order's only payoff would be window arrangement/focus, which is OS/terminal-controlled and can't be reliably honoured; list order is predictable and matches the visual. The future Spaces/workspace feature will record *explicit* placement rather than infer from tick-order, so capturing pick-order banks nothing.

- **Which marked session the trigger window becomes: unspecified (implementation-convenience).** Cosmetic — no Spaces placement, so all N windows open on the current Space regardless. Not pinned.
- **Window focus** is left to the OS.

---

## Terminal Identity & Detection

### Detection is a standalone operation

Detection (detect-self) is a **separately-callable operation**, not buried in the spawn path — because the unsupported banner must show identity *without* spawning anything. It backs: the unsupported/unconfigured banner, the `portal spawn --detect` dry-run, and the deferred workspace-introspection. It is **not** an adapter method (a resolver maps identity → adapter — see *Adapter Contract*).

### Detection model (refined by live validation)

Live-probed against the real server (~33 clients, many lingering mosh/Blink). The identity walk cleanly separates local Ghostty from remote mosh (→ NULL); `focused` and raw highest-`client_activity` proved **unreliable across clients**. The model anchors on the triggering process's own context, not a shared registry:

1. **Outside tmux (primary flow — fresh terminal → picker):** the picker **self-walks its own process tree** to the terminal (`picker → zsh → ghostty`), or uses the env fast-path (`GHOSTTY_*` / `__CFBundleIdentifier`, accurate outside tmux). Direct — no client list, no tiebreak.
2. **Inside tmux:** take the current session's clients, **NULL-filter to local host clients** (drop mosh/remote/other-machine). The local client's app = the terminal.
3. **`client_activity` demoted to a local-only tiebreak** — used *only* to choose among 2+ local clients on the same session. The trigger keypress makes your window freshest, and mosh noise is already filtered, so it is robust in that narrow role. Never the primary cross-client signal.
4. **Host-local principle (multi-machine).** Portal opens windows only on the machine it runs on, for local clients; other-machine access is a remote client → NULL → filtered. A purely-remote trigger (no local client) → the honest "no host-local terminal" no-op — run Portal on that machine to spawn there.

### Identity resolution: macOS bundle id, matched as a family

The system-blessed identity is the terminal's macOS **bundle id**. The walk resolves `client_pid → process-tree → .app bundle` via an Info.plist read (`defaults read` of the bundle's Info.plist — a clean `lsappinfo`-free route). Matching is by **bundle-id family** (e.g. `dev.warp.Warp-*`), channel-aware. Remote/mosh clients resolve to a **NULL bundle id** → unsupported → honest no-op.

Validated (this Mac): `ps -o comm=` returns full paths; the walk cleanly separates local Ghostty (`→ login → /Applications/Ghostty.app/…/ghostty` → bundle id) from remote mosh (`→ mosh-server` at ppid 1 → NULL). Read-only, no `osascript`/Apple-event needed. *(Build-time residual: confirm the walk on ≥1 other macOS version.)*

### User-facing display: both

The unsupported/unconfigured banner and the `--detect` command show **both**:

- the friendly `.app` name (for reading), and
- the exact **bundle id** (the copy-paste config key).

This solves the chicken-and-egg the research flagged: a custom-config user cannot guess the key a priori, so Portal *shows* it — copy-paste, never guess. (Design copy example, from the delivered banner frame: `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal` with a `see docs` link.)

### Config keys accepted: layered

Custom config accepts, layered (see *Config Schema*):

- **Friendly alias** (`ghostty`, `warp`) — Portal-shipped, for *known* terminals; maps to the bundle-id family.
- **`.app` name** / **raw bundle id** / **`*`-glob** — the escape hatch for custom/unknown terminals.

Whatever Portal displayed, the user can paste it and it resolves. Internal **matching** stays on bundle-id families; user-facing keys are the friendlier forms.

### No headless story

`portal spawn` exists to open terminal windows, so it only ever runs from a terminal context (the picker is in one; a script is run in one; the future workspace feature triggers from a terminal). There is no sensible headless caller — chicken-and-egg. So: **no special headless handling and no `--terminal` override**. If detection ever returns empty, it folds into the **same NULL-bundle path** already decided for remote/mosh → unsupported → clean error/banner.

---

## Adapter Contract & Extensibility

### Detection is separate from the adapter

Detect-self resolves *identity*; a **resolver** maps identity → adapter via the precedence chain; the adapter is per-terminal and only opens windows. Detection is **not** an adapter method.

### Resolution precedence

**config override → native adapter → unsupported.** Config can override a built-in too (e.g. Ghostty + a resize). A NULL/unmatched identity → unsupported.

### Generic contract: open a window running a command

The adapter's single job is **open a new host window running a given command** — `OpenWindow(command)` — **not** "attach to a session." The **spawn layer composes the command** (`<os.Executable()> attach <session>` + the ack token) and hands it to the adapter.

- Rejected: a session-aware `OpenAttached(session)` — it would bake `portal attach` into every adapter and scatter the attach+ack composition.
- Keeps adapters dumb and portable (one thing: open a window running a command); keeps the `portal attach` + ack composition in one place; and future-proofs the adapter — the same open-window primitive can be handed *different* commands later (the workspace feature, other actions) without touching adapters.
- Knock-on for config: the custom-terminal placeholder is **`{command}`** (the thing to run), not `{session}`.

### Two implementations, same contract

- **Built-in Go adapters** (Ghostty ships in this feature) — compiled in, not config.
- **User-config entries** (`terminals.json`) — the escape hatch (see *Config Schema*).

Each adapter owns its own terminal-specific concerns, including injecting the picker's `PATH`/env into the spawned window (see *Spawn Architecture*) and quarantining all OS/terminal specifics behind a typed result (see *Permissions & Error Quarantine*).

### Capability-based extensibility

Adapters implement exactly one capability in scope: **open-window-with-command**. Future `introspect` / `place-on-space` slot in as *additive* optional capabilities (Go interface segregation, checked by type assertion) without touching existing adapters. Only the open capability is in scope for this feature; the capability mechanics are an implementation concern.

---

## Config Schema (`terminals.json`)

### Location & format

`~/.config/portal/terminals.json` — Portal's JSON-store convention (like `projects.json` / `hooks.json`), XDG-resolved via the existing `configFilePath`. Read-only at spawn time; user-authored.

### Structure

Each **entry = identity-matcher → capability map**:

- **Key** = whatever identity form the user pastes: a friendly alias (to override a built-in), a `.app` name, a raw bundle id, or a `*`-glob.
- **Value** → `commands` → `open` (the only capability key in scope) → a **recipe**.
- Future `introspect` / `place` are **additive sub-keys**, not a breaking schema change — config and adapter extend in lockstep.

### Recipe: explicit fields, not magic

The recipe is **`argv`** (inline argv-array template) **or** **`script`** (path to a file Portal runs) — chosen over auto-detecting "is it a path on disk?" (clearer, no disk-probing surprise):

- **`argv`** is an **argv array** (not a string), to sidestep shell-quoting hell. The inline field is named `argv` (not `command`) to avoid colliding with the placeholder.
- **`script`** is a path to a file Portal executes.

### Placeholder `{command}`

Portal substitutes `{command}` with `<os.Executable()> attach <session>` + the ack token. Config expresses only "how my terminal opens a window running `{command}`"; Portal fills in the attach.

```json
// ~/.config/portal/terminals.json
{
  "dev.warp.Warp-*": {
    "commands": { "open": { "argv": ["osascript", "-e", "tell app \"Warp\" to create window with command \"{command}\""] } }
  },
  "com.example.MyTerm": {
    "commands": { "open": { "script": "~/.config/portal/terminals/myterm.sh" } }
  }
}
```

### Recipe execution contract

The config-driven path gets the *same* execution guarantees the native Ghostty adapter got, so a custom recipe never re-hits the PATH/exec fixes the native path already absorbed:

- **Portal makes `{command}` self-sufficient, uniformly.** This *refines* the per-adapter "inject the picker's PATH" rule: the picker (which has your full PATH) resolves what the spawn needs and threads it into `{command}` itself — so recipe authors only ever describe "open a window running this command," never PATH/env plumbing.
- **`{command}` substitutes as a single, already-resolved command string**, dropped literally into the recipe. Escaping it for an *embedding* context (e.g. inside an AppleScript string) is the recipe author's responsibility — they wrote that AppleScript. The `argv`-array form exists precisely so simple CLI terminals (kitty/wezterm) avoid quoting entirely.
- **`script` recipes receive `{command}` as `$1`** (first positional arg) — the standard, obvious contract.

### Precedence

config override → native adapter → unsupported. Config can override a built-in (e.g. Ghostty + a resize).

---

## Permissions & Error Quarantine (TCC)

### Architectural boundary (the core decision)

All terminal/OS-specific concerns — the AppleScript, `osascript`, the `-1712`/`-1743` AppleEvent codes, TCC, any macOS deep-link — live **inside the terminal driver and nowhere else**. The driver translates them into a **generic typed result** — a small taxonomy:

- `permission-required` (with guidance text)
- `unsupported`
- `spawn-failed`

Portal's general spawn/report/UI code switches on the category and **never sees an AppleScript string or AppleEvent number**. Every future terminal driver gets the same clean contract.

### TCC is self-exempt — no first-run gate (validated live)

Live-tested: reset `com.mitchellh.ghostty` AppleEvents grant → fresh spawn → **succeeded with no dialog and no TCC row recreated**. So **Ghostty-scripting-Ghostty via `osascript` is self-exempt**. Because detection always resolves to the terminal you are in and we spawn *that same* terminal, the AppleScript path is **always self→self → always exempt** — there is no design path to a genuine cross-app event.

**Therefore there is no TCC first-run prompt in the normal flow.** Portal is never the TCC subject; the responsible process is the host terminal.

### Defensive net (not a load-bearing gate)

The `-1743` (denied) / `-1712` (timeout) handling stays as a **defensive net**, not a first-run gate:

- The driver recognises its own `-1743`/`-1712`, returns `permission-required`.
- General code surfaces actionable guidance — names the target terminal and offers to open the Automation settings pane (the deep-link composed *in the driver*, handed up as opaque guidance).
- Grant persists; re-triggering works — the standard macOS permission model. Per-`(source, target)` pair: switching terminals re-prompts, handled identically.

### Residual (build-time check)

iTerm2 / Terminal.app self-scripting is **assumed** same-exempt but **unverified** — a per-adapter check at build time.

---

## Observability & State Footprint

### State / daemon footprint (near-zero persistent state)

New *code* lands (the `internal/spawn` package, terminal drivers, a `terminals.json` config store), but near-zero persistent *state*:

- **Reads** `terminals.json` (user-authored, read-only at spawn time).
- **Writes** only transient `@portal-spawn-*` tmux server options (self-cleaned per the ack contract — not files, not captured; the daemon's capture is structural sessions/panes, not a server-option dump).
- **Does not touch** `sessions.json`, the daemon capture loop, `prefs.json`, or the restore machinery.

### Observability (`spawn` log component)

The spawn flow gets its **own `spawn` log component** — a deliberate amendment to Portal's closed logging taxonomy (a new spec-governed component, not a call-site invention). Closed event catalog, emitted from the spawn chokepoint:

- detection outcome (identity / unsupported / NULL-bundle)
- adapter resolution (config-override vs native)
- per-window spawn + ack outcome (confirmed / timeout / failed)
- `permission-required`
- batch summary

Emission shape matches bootstrap/restore/daemon instrumentation: **one INFO cycle-summary** (e.g. `spawn: opened 11/14`) + **DEBUG per-window**. The driver's OS-specific detail rides up as an opaque `detail` attr so the closed vocabulary stays intact (honours the driver-quarantine rule).

---

## Concurrency & Post-Reboot Safety

N near-simultaneous `tmux attach` against a server that may still be hydrating post-reboot is **not** a contention risk, for reasons already built in:

1. **The skip-bootstrap latch removed the big race.** Burst attaches take the abridged path — no full bootstrap per window — so N concurrent sweeps/restores/cleans against one server is gone (see *Spawn Architecture* / dependency).
2. **The picker gates the burst to *after* hydration.** The cold+TUI path reaches the Sessions page (where multi-select lives) only on `BootstrapCompleteMsg` — after Restore + EagerSignalHydrate + `@portal-restoring` cleared. So the burst cannot be triggered until hydration is done. (A direct `portal spawn` CLI as the first post-reboot command runs its own bootstrap synchronously first — same guarantee.)
3. **Abridged attaches don't perturb capture.** A new client attaching adds a *client*, not session/window/pane structure — all the daemon captures — so the 1s capture tick and daemon self-supervision are untouched.

---

## Testing Strategy & DI Seams

Portal drives a real GUI terminal (`osascript` → Ghostty), hard to automate. The strategy is seam-based coverage with a live terminal only for the last inch.

### Primary seam: the `Adapter` interface

A **fake adapter** records "would open a window running command X" without touching a real terminal → the entire spawn pipeline is unit-testable:

- adapter resolution + precedence (config → native → unsupported)
- `{command}` substitution
- token-ack collection
- pre-flight + abort/rollback logic

### Detection behind small seams

Detection reads behind small (1–3-method) interfaces (process-tree walk / `ps` / `lsappinfo`-or-Info.plist / `tmux list-clients`), Portal's existing DI pattern → detect-self *resolution* (local-client NULL-filter, walk-to-bundle-id, family match, NULL→unsupported, the local-only activity tiebreak) is unit-testable with fabricated data. The real walk is integration (real-tmux `tmuxtest` fixture) / manual.

### Driver split for testability

Each terminal driver splits into:

- **Pure command-construction** (building the `osascript`/argv) — unit-tested (assert the built command).
- **Error-mapping** (`-1712`/`-1743` → typed `permission-required`) — unit-tested (fabricated `osascript` outcome; assert the mapped typed result).
- **Thin exec boundary** (real `osascript` + TCC modal) — manual/integration-gated only.

### Mode/keymap state machine

The multi-select mode is unit-tested as a Bubble Tea model (existing `internal/tui` pattern): enter / toggle / exit, sticky selection, suppressed keys, filter-inside-multi-select, N=0/N=1 boundary.

### Irreducible manual/integration residue

The real window actually opening + the TCC modal need a live Mac — covered by manual verification + the Paper visual gates (see *Design References*), not automated CI.

---

## Working Notes

[Optional - capture in-progress discussion if needed]
