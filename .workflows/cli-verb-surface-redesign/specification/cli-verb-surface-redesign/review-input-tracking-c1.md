---
status: in-progress
created: 2026-07-17
cycle: 1
phase: Input Review
topic: cli-verb-surface-redesign
---

# Review Tracking: cli-verb-surface-redesign - Input Review

## Findings

### 1. Per-window ack-poll timeout (~8s) absent from burst mechanics

**Source**: Discussion — "Attach Disposition › Decision" (the `--ack` bullet): "the spawned Portal process, as its last act before exec'ing into tmux, writes `@portal-spawn-<batch>-<token>` as a tmux server option — a delivery receipt the parent polls for (**~8s/window**); no receipt ⇒ window classified failed."
**Category**: Enhancement to existing topic
**Affects**: `portal open` — Multi-Target Burst Mechanics (Atomic pre-flight & partial failure) and the Hidden `--ack` flag section

**Details**:
The spec describes the ack mechanism ("a delivery receipt the parent burst polls for") and the partial-failure classification ("failed/un-acked surfaces don't retry automatically"), but never states the concrete per-window poll timeout. The discussion pins it at ~8s/window, and (per CLAUDE.md's existing `spawnAckTimeout` note) the timer starts at *each window's own spawn* so cumulative sequential delay never eats a later window's budget. This is the mechanism by which a surface is deemed "un-acked / failed" — the spec's partial-failure contract is incomplete without the timeout that defines "failed." It is preserved existing behavior, not a new redesign decision, but the spec already documents burst mechanics to this depth, so the timeout belongs alongside the poll it governs.

**Current**:
"Its behavior: the spawned Portal process, as its last act before exec'ing into tmux, writes `@portal-spawn-<batch>-<token>` as a tmux server option — a delivery receipt the parent burst polls for. Full burst mechanics are in the multi-target topic."

**Proposed Addition**:
Added a bullet to "Atomic pre-flight & partial failure": "**Per-window ack timeout (~8s).** The parent polls for each window's `@portal-spawn-<batch>-<token>` receipt with a per-window timeout of ~8s, the timer starting at *that window's own spawn* so cumulative sequential delay never eats a later window's budget. A window whose receipt has not appeared by its timeout is the 'un-acked / failed' case above."

**Resolution**: Approved
**Notes**: Auto-approved. Sourced from discussion "Attach Disposition" (~8s/window) + preserved per-window-timer-start behavior. Logged to spec.

---

### 2. `doctor` check catalog: discussion positioned it as a spec-level detail; spec defers it to planning

**Source**: Discussion — "Maintenance & Diagnostics Reorg › Decision": "**`portal doctor`** … read-only health report … **Subsumes `state status`.** The exact check catalog is **a spec-level detail**." Spec — `doctor` section: "The exact check catalog is a spec-level detail **for planning to enumerate**."
**Category**: Gap/Ambiguity
**Affects**: `doctor` — Diagnostics & Repair section

**Details**:
The discussion flagged the exact check catalog as a *spec-level* detail — i.e., a thing the specification should pin down (as distinct from a discussion-level decision). The spec re-scopes it to "planning to enumerate." The check catalog is *what the command inspects* (spec content — the command's contract), not delivery phasing/task breakdown (planning's remit), so pushing the full catalog to planning is a mild hand-off drift. The spec does provide a representative list (daemon alive, hooks registered without duplicates, saver up, state dir sane, `sessions.json` valid, stale entries — matching the discussion's examples), so the gap is narrow: either promote that representative list to the authoritative catalog here, or explicitly acknowledge it is illustrative-not-exhaustive. Worth a decision so planning inherits an unambiguous scope.

**Proposed Addition**:
Reclaimed the check catalog as a spec-level authoritative list (daemon alive; hooks registered without duplicates; `_portal-saver` up; state dir sane; `sessions.json` valid; no stale entries; host terminal detected + supported), replacing "The exact check catalog is a spec-level detail for planning to enumerate." Planning implements the concrete probe per check.

**Resolution**: Approved
**Notes**: Auto-approved. Faithful digest of the discussion's own enumerated checks + the `--detect` fold; no new checks invented. Logged to spec.

---

### 3. Resolver-decision log line has no home in the closed log-component taxonomy

**Source**: Discussion — "Wrong-Guess Feedback › tmux is the receipt": "One cheap addition locked: **the resolver logs its decision** (`resolve: 'blog' → zoxide → ~/Code/blog`) **under the existing log taxonomy**." Mirrored in spec — `portal open` Grammar & Target Resolution, Wrong-guess feedback: "the resolver logs its decision under the existing log taxonomy, e.g. `resolve: 'blog' → zoxide → ~/Code/blog`". Verified against code: `log.For(...)` call sites use aliases, bootstrap, capture, clean, daemon, hooks, hydrate, notify, preview, process, projects, restore, saver, signal, spawn — there is **no `resolve`/`resolver` component**.
**Category**: Gap/Ambiguity
**Affects**: `portal open` — Grammar & Target Resolution (Wrong-guess feedback subsection)

**Details**:
Both the discussion and the spec assert the resolver-decision line rides "the existing log taxonomy," and the illustrative line uses a `resolve:` component prefix (Portal's log format is `<component>: <msg>`). But `resolve`/`resolver` is not in the current closed 16-name component set, and `open` itself owns no log component (it logs exec markers under `process` and the spawn burst under `spawn`). CLAUDE.md is explicit: "New components/attrs require amending the spec — never invent at call-site." So this locked observability addition cannot land as stated without either (a) a governed amendment adding a new `resolve` (or `resolver`) component to the closed vocabulary, or (b) a decision to route the line under an existing component. The source material glossed over this — it assumed "existing taxonomy" fits when it does not. This is a blind spot worth surfacing so planning doesn't invent a component at the call site (the exact thing the log spec prohibits).

**Proposed Addition**:
_(leave blank until discussed)_

**Resolution**: Pending
**Notes**:

---
