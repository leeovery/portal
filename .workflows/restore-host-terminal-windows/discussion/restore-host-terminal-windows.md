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

  Discussion Map — Restore Host Terminal Windows (13 subtopics — 4 decided · 1 exploring · 8 pending)

  ┌─ ✓ 1. Spawn-execution architecture — where the reopen runs from [F6] [decided]
  ├─ ✓ 2. Multi-select trigger & keymap coexistence [F7] [decided]
  ├─ ✓ 3. Burst & partial-failure contract [F1] [decided]
  ├─ ✓ 4. Trigger-context matrix (in/out tmux × attached × includes-self) [F2] [decided]
  ├─ ○ 5. TCC first-run Automation-permission flow [F4]
  ├─ ○ 6. Config schema & command representation [F9]
  ├─ ◐ 7. Terminal-identity UX — what we display & accept as config key [rv2-UX] [exploring]
  ├─ ○ 8. Adapter contract shape & extensibility (capability-based) [fwd-looking]
  ├─ ○ 9. Testing strategy & DI seam [F5]
  ├─ ○ 10. Daemon / state footprint of windows-only v1 [F10]
  ├─ ○ 11. Attach contention vs post-reboot hydration [F12]
  ├─ ○ 12. Pre-build validation flags (lsappinfo/ps stability, activity-bump timing) [rv2-F4/F5]
  └─ ○ 13. Design in Paper — page + interactions (deliverable, this discussion) [pending]

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
- **Bootstrap cost → external dependency (review-001 F1).** `attach` is not in `skipTmuxCheck`, so each spawned `portal attach` re-runs the full 11-step bootstrap orchestrator — a 14-window burst would fire 13 near-simultaneous full bootstraps against one server (a distinct concern from #11's tmux-attach race). We rejected the two workarounds (a hidden `--skip-bootstrap` flag; an internal bootstrap-exempt `portal state attach`-style command) — the latch belongs in bootstrap, not in a parallel attach path, and the awkward command name was the tell. Resolved by a **separate `warm-command-bootstrap-latch` feature** (logged to inbox `2026-06-30--warm-command-bootstrap-latch`): a once-per-server-lifetime tmux server-option latch (`@portal-bootstrapped`) set at end of bootstrap, so warm commands fast-skip the 11 steps. **This feature depends on that landing first**; reopen then spawns *plain* `portal attach` with no special-casing. This largely **subsumes #11** (attach contention).
- **Spawn via the picker's own executable path (review-001 F3).** The N−1 windows spawn running `<os.Executable()> attach <session>`, **not** a bare `portal` PATH lookup. The warm-command latch is *version-gated* — satisfied only if stored version == running version (verified against the skip-bootstrap code: `state.BootstrappedLatchSatisfied`) — so a PATH-resolved spawn of a *different* portal version would read the latch unsatisfied and full-bootstrap per window, resurrecting the burst storm. The picker's own binary guarantees version parity → latch satisfied → burst stays abridged. Side-effect: `portal` no longer needs to be *on* PATH (only `tmux` does, since portal shells to it), dissolving half of review-002 F3.

---

## 2. Multi-Select Trigger & Keymap Coexistence

### Context

Research pencilled `M` as the trigger, but §12.2 deliberately dropped all uppercase bindings, and the Sessions keymap is tight (`Enter`=attach, `Space`=preview, `/`=filter, `k`=kill, `x`=Sessions⟷Projects, `s`=grouping, `r`=rename, `?`=help). Multi-select must coexist without clobbering these.

### Decision — interaction model

- **Trigger: lowercase `m`** (convention-consistent; `M` stays retired). `m` from the normal Sessions list **enters an explicit multi-select mode** — you can sit in it with **zero selected**; it is a real mode, not an implicit mark-on-entry.
- **Mark: `m` again.** In mode, `m` toggles the cursor (highlighted) row in/out of the selection. The *same* key enters the mode and toggles marks — no second key, and no reuse of an already-meaningful one. (Rejected: `x` — already means Sessions⟷Projects; `Tab`; and repurposing `Space` — Space must stay preview.)
- **`Space` = preview, unchanged in mode** (firm user requirement — still useful while selecting).
- **`Enter` = open the selected set** (the #1/#3 flow). Enter stays "commit" in both modes — normal mode attaches the cursor row, multi-mode attaches the marked set. N=1 → plain single attach; N=0 Enter → exits mode (per #3 / F6).
- **`Esc` = exit mode, clear selection.**
- **Visually unmistakable mode**, modelled on filter mode (orange + a typable filter area): multi-select gets its **own mode colour + a banner** in the existing notice-band slot (single-slot arbiter — the multi-select banner owns the slot while in mode), e.g. `N selected · m toggle · space preview · ⏎ open · esc cancel`. Selected rows carry a **glyph marker + the mode colour**, never colour-only (MV's NO_COLOR / colourless-render rule). Exact colour token + banner copy are a **design-phase** call (MV token layer + fixture/visual-gate process); the *requirement* is "as obviously a distinct mode as filtering is."
- **Live vs suppressed in mode (from the agreed #2.2 set):** `/` filter and `s` regroup stay **live** (so you can filter/regroup to find things to select); `k` kill, `x` page-toggle, `r` rename, and other row actions are **suppressed**. Selection is **sticky** across filtering, paging, and regrouping. Grouping `HeaderItem` rows are skipped (non-selectable).

### Decision — granularity: per-session only (v1)

Group-select (mark a whole project/tag group via its header) is **deferred to a v2 fast-follow**. Tempting — it maps onto the per-Space/per-project rebuild — but it requires letting the cursor land on the currently non-selectable `HeaderItem` rows (the `skipHeaderRow` invariant, itself a pagination-bug fix), and the research already accepts "manual re-selection per zone" as the v1 partial-win yardstick. v1 ships **per-session marking only**.

*(decided — interaction model + per-session granularity resolved)*

---

## 3. Burst & Partial-Failure Contract

### Context

The motivating scenario is a *large* burst (rebuild ~14 windows post-crash), not the clean 3-window path. This subtopic owns the contract for when a burst does **not** fully complete: a spawn/attach fails, or the user aborts mid-burst.

**Reframe first (review-001 F1):** the "burst = N concurrent full bootstraps" angle is dissolved by the separate warm-command bootstrap-latch dependency (see #1's dependency note), leaving the burst as N cheap attaches. So #3 is about genuine *spawn/attach* partial failure, not bootstrap contention.

### Options Considered — partial-failure stance

**All-or-nothing** — rejected. You can't un-spawn windows that already opened, and in a 14-window rebuild you want every one you can get.

**Best-effort-with-report** — chosen.

### Decision — best-effort, picker-orchestrated, self-attach-last

The contract falls out of one structural fact: once the picker self-execs into the Nth session it is *gone* (can't report, summarise, or retry), and a *failed* spawn has no window of its own to show an error. So **the picker is the only sane reporting surface**, which fixes the shape:

- Spawn the N−1 first, **collect confirmations**, and **self-attach LAST**, gated on having handled failures.
- **Full success (common case): self-attach silently** — no "14/14 ✓" nag.
- **Partial failure: the picker stays alive** as the report surface (*"opened 11 of 14 — S5/S9/S12 didn't come up, retry?"*), then attaches.
- **Resolves the parked #1 coupling: in-process call wins** — the picker needs the spawn results to report them; a detached subprocess throws them away.
- **Cancellation (review-001 F4):** self-exec being the *last* step makes cancellation clean — `Ctrl-C`/`Esc` *before* it stops the picker mid-loop and reports what opened; *after* it there's nothing to cancel (already attached). The point of no return being last is what buys this.

### Decision — confirmation mechanism: token ack (not tmux-watching)

`osascript` returning success is **shallow** — it only confirms "Ghostty accepted the request," not that the window rendered, `portal` ran, the session existed, or attach happened. We need a real "it came up" signal.

- **Rejected — tmux client-watching** (snapshot `list-clients`, diff new clients per session). Handles a *static* pre-existing client via set-diff, but **fragile in this user's environment**: lingering/reconnecting mosh clients churn the client list during the exact burst window (research-documented), risking false confirms or masked failures. It infers from a shared, noisy registry.
- **Chosen — explicit token ack.** The picker issues a batch id + per-window token, threads it into each spawned command (arg/env); the spawned `portal attach` **writes its token right before exec**; the picker watches for the token set with a **timeout**; missing tokens after the timeout = the report. A **direct signal from our own spawned process**, immune to how many other clients are attached (local/remote, stable/flapping) — this is what makes **reopening a session already attached elsewhere (e.g. the iPhone) confirm correctly**, satisfying the explicit "allow multi-client / cross-device reopen" requirement.
- **Honest boundary:** the ack fires at the last instant before exec (once `portal` execs into tmux it's replaced, so it can't ack *after* attaching). It confirms "window opened, `portal` ran, session found, attach handoff starting" — covering every real failure mode; the final tmux handoff is essentially guaranteed once there.
- **Channel: namespaced `@portal-reopen-<batch>-<session>` tmux server option, behind a small ack seam** (write-token / collect-tokens interface — Portal DI shape). Code-verified safe: the only all-server-options enumerator, `ListSkeletonMarkers`, skips any name not prefixed `@portal-skeleton-` (`internal/state/markers.go:86`), so a distinct `@portal-reopen-` prefix is invisible to it; namespacing isolates sweeps in both directions; server options die with the server.
- **Cleanup:** the picker self-cleans its batch markers before self-exec. Bounded, harmless leaks (a late-laggard ack, or a crashed picker) self-expire with the server and never collide (unique batch ids). A defensive `@portal-reopen-*` sweep mirroring `CleanStaleMarkers` (bootstrap step 9) is a drop-in if ever needed — **deferred**.
- **Pivot to the daemon (channel c) is additive, not a rewrite:** the daemon already ticks every second reading tmux state, so the future remember-and-restore-workspace feature just teaches it to *read the same markers* and record outcomes — no change to how the picker collects.

### Decision — sequential spawn

Spawn the N−1 **sequentially** (one `osascript` completes before the next fires) for v1. The token ack already makes spawn *order* irrelevant to reporting, so the choice rests on: sidesteps the unverified Ghostty rapid-fire AppleScript throughput risk (#12), gives clean per-window cancellation points, and turns the per-window focus-steal into an orderly cascade rather than unpredictable thrash. Reversible — flip to parallel only if #12's validation shows it's both safe *and* meaningfully faster.

### Dependency verified — skip-bootstrap latch suffices

Checked the near-complete `warm-command-bootstrap-latch` against reopen's need. `portal attach` (not in `skipTmuxCheck`) flows through the version-stamped `@portal-bootstrapped` latch → on a warm server it takes the **abridged path** (skips the orchestrator, still injects the tmux client `attach` needs; Restore/hydrate/hooks already ran at the picker's bootstrap). Per-command hook-stale-cleanup was also moved off bootstrap onto the daemon's throttled tick. So a warm burst = N cheap abridged attaches. **Confirmed sufficient.** The latch's version-gate is what forces the #1 `os.Executable()` decision above.

### Deferred — "detect-and-wait" hardening for the multi-bootstrap edge

With F3 fixed, the picker always bootstraps *first* (its own `PersistentPreRunE`) and stamps the latch to its own version, then spawns that *same* binary — so at burst time the latch is **always** satisfied and no spawned window full-bootstraps. The only residual is a mid-picker-session in-place binary swap (negligible). User accepts it (rare; full bootstrap is a safe no-op). A conditional "if the first spawn triggers a full bootstrap, wait for its ack before firing the rest" was floated — sequential + the token ack (which fires *after* the latch re-stamp) would cap it at exactly **one** bootstrap for free. **Deferred as YAGNI** given F3 renders the case negligible; recorded as optional future hardening, and the ack is the natural wait-signal if it's ever wanted.

### Decision — N=0 / N=1 boundary (review-001 F6)

The "self-attach to the Nth of N" rule is total:

- **N=1** (one selected, Enter): zero windows to spawn — the picker self-attaches to that one session, i.e. it **degenerates to a plain single attach** to the current window. No special-casing; selecting one in multi-mode and pressing Enter is just a normal attach.
- **N=0** (nothing selected, Enter): a **no-op that exits multi-select mode**, dropping back to the standard picker (Portal stays open) — the same effect as pressing `Esc`. Nothing opens.

*(decided — full partial-failure contract, token-ack confirmation, spawn-via-own-exe, sequential spawn, and N=0/N=1 boundary all resolved)*

---

## 4. Trigger-Context Matrix

### Context

Behaviour across: in/out of tmux at trigger × selected session detached / attached-elsewhere × selection includes the current context.

### Decision — matrix (mostly consolidated from #1/#3)

- **In vs out of tmux at trigger.** *Out* (bare-shell picker): trigger window reuses via `AttachConnector` (exec `tmux attach`); detection walks the picker's own process tree. *In* tmux: trigger window reuses via `SwitchConnector` (`switch-client`); detection takes the `list-clients` → client-PID hop. The **spawned N−1 are always fresh host windows running `portal attach` out of tmux**, independent of the picker's context; only the trigger-window reuse differs.
- **Selected session already attached elsewhere** (this host or a remote/iPhone client): allowed — no dup guard (research); the token-ack confirms *our* new window regardless of other clients.
- **Includes-self:** the trigger window becomes one attached session, the rest spawn; the marked origin session ends up attached either way.
- **Selected session vanished** between picker-load and Enter: its spawn fails → best-effort report (#3).
- **Enter opens the marked set only.** The cursor/highlight at Enter time is irrelevant — a highlighted-but-unmarked row is **not** opened (marking is `m`, not Enter). Enter always commits the `m`-marked set.
- **Which marked session the trigger window becomes: unspecified / impl-convenience.** Cosmetic in v1 (no Spaces placement — all N windows open on the current Space regardless), so not pinned.

### Decision — open order: list order (selection is a set)

Open in **list order** (top-to-bottom as shown), not pick order. The selection is a plain **set**, not an ordered list. Pick-order's only payoff is window arrangement/focus, which is OS/Ghostty-controlled and can't be reliably honoured; list order is predictable and matches the visual; and the future Spaces/workspace feature will record *explicit* placement rather than infer from tick-order, so capturing pick-order banks nothing. Trigger-window session left to implementation; focus left to the OS.

*(decided — matrix + open order resolved)*

---

## 13. Design in Paper (Page & Interactions)

### Context

A **deliverable**, tracked here at the user's request: this feature's UI must be designed in **Paper** — where the rest of Portal's Modern Vivid design system lives — as part of this discussion, feeding implementation. Not a decision subtopic; a required design artefact.

### Scope of what to design

- The **multi-select mode** on the Sessions page: the distinct mode affordance (own colour + notice-band banner, per #2 — the filter-mode analogue), row **selection marker** (glyph + mode colour, never colour-only), and the mode's states — empty, N-selected, and the **partial-failure report surface** (#3, "opened 11 of 14 …").
- The **terminal-identity surfaces** (#7): the unsupported/unconfigured **banner** and whatever detect/identity display we land on.

### Process

Follows the project's reference-first visual workflow — export the Paper frame(s) to `reference/` before implementing, verify against `cmd/capturetool` fixtures. Exact colour tokens/copy are settled in the Paper design, consistent with the MV token layer.

*(pending — design artefact to produce as part of this work)*

---

## Summary

### Key Insights

*(captured as the discussion progresses)*

### Open Threads

*(captured as the discussion progresses)*

### Current State

- Research foundation settled (see Context); 12 live subtopics seeded.
- **#1 Spawn-Execution Architecture — decided** (Option B: shared reopen package + `portal reopen` subcommand, picker calls in-process; N−1 spawned, picker self-reuses for the Nth).
- **#3 Burst & Partial-Failure — decided.** Best-effort; picker-orchestrated, self-attach-last; in-process; token-ack confirmation via `@portal-reopen-*` server option; spawn via `os.Executable()` (F3); sequential; N=1 degenerates to plain attach, N=0 exits multi-mode (F6). Skip-bootstrap latch verified sufficient.
- **#2 Multi-Select Trigger & Keymap — decided.** `m` enters explicit (empty-able) multi-select mode; `m` toggles cursor row; `Space` stays preview; `Enter` opens marked set; `Esc` exits. Distinct mode colour + notice-band banner (design-phase visual). Sticky selection; filter/regroup live, kill/rename/page-toggle suppressed. Per-session only; group-select deferred to v2.
- **#4 Trigger-Context Matrix — decided.** In/out-tmux reuse (switch-client / exec-attach), already-attached allowed, includes-self handled, vanished→best-effort. Enter opens the marked set only (cursor irrelevant). Selection is a **set**, opened in **list order**; trigger-window session + focus left to impl/OS.
- Open coupling thread: #7 (terminal-identity detection) — also home to outstanding review findings F2 (headless reopen has no terminal) and F7 (detect-self package shape). F5 (reopen observability) still to place.

### Open Threads

- **External dependency:** reopen depends on the `warm-command-bootstrap-latch` feature (inbox `2026-06-30--warm-command-bootstrap-latch`) landing first. The user will **not spec reopen until warm-command-bootstrap is done**; discussion proceeds assuming warm attaches are cheap by implementation time.
- Outstanding review-001 findings to surface at their subtopics: F2 (headless `portal reopen` has no terminal to detect → #1/#7), F3 (spawned attach binds to PATH `portal`, version skew → #1/#7), F5 (reopen observability/log component → likely new subtopic), F6 (N=0/N=1 boundary of self-attach-last → #3), F7 (detect-self as standalone query / package shape → #7/#8).

## Triage

(none)
