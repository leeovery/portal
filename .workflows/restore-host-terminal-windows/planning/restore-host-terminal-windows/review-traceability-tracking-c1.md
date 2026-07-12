---
status: complete
created: 2026-07-12
cycle: 1
phase: Traceability Review
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Traceability

## Result

**STATUS: clean** — no findings.

The 6-phase, 45-task plan is a faithful, complete translation of the specification in
both directions. Every specification element maps to a task with matching acceptance
criteria and implementer-grade depth; every task's content traces back to an identifiable
specification section (each task carries an explicit Spec Reference plus a Context block of
verbatim spec quotes). The handful of implementation-only mechanics (process-walk hop bound,
ack poll interval, `__CFBundleIdentifier` plausibility check, friendly-name derivation
algorithm, pre-flight-vs-detect ordering) are consistently flagged in-task as choices the
spec does not pin — they realize spec'd behaviour rather than invent requirements, so they
do not rise to hallucination findings.

This file is retained as review history per the tracking-file convention.

## Findings

None.

---

## Direction 1 — Specification → Plan (completeness)

Every spec section verified as covered by tasks with adequate depth:

| Spec section | Plan coverage |
|---|---|
| Overview / Foundational shape (multi-select `m`, net-N never N+1, Ghostty-first + config, detection walk, no dup guard) | 1-1, 2-6, 4-x, 5-1, 6-3/6-4 |
| Spawn Architecture — one service two callers | 1-1 (doc), 2-6 (CLI), 6-3 (picker) |
| `portal spawn` CLI behaviour + `--detect` + usage error | 1-6, 2-6 |
| Reporting & exit codes (success self-exec / pre-flight abort / partial fail / unsupported N≥2 / permission / usage) | 2-6, 2-7, 3-3, 3-4, 3-6, 3-7 |
| N vs N−1 split / Order is load-bearing | 2-6, 3-5, 6-3 |
| Command composition (`os.Executable()`, not PATH lookup) | 2-3, 6-3 |
| Spawned-window env (PATH-only inject, TMUX/TMUX_PANE strip) | 2-3 |
| Multi-Select — trigger & marking, N=0/N=1, key coexistence, sticky, filter sub-state, affordance, per-session granularity | 5-1..5-8 |
| Burst & Partial-Failure — pre-flight all-or-nothing | 3-4, 6-7 |
| Token-ack confirmation (option-safe ids, marker channel, `--spawn-ack`, per-window timeout, cleanup) | 3-1, 3-2, 3-3, 3-5 |
| Leave-what-opened partial failure | 3-6, 6-6 |
| permission-required burst-stop | 3-7, 6-6 |
| In-picker execution model (async tea.Cmd, in-burst feedback, input-lock, cancellation) | 6-3, 6-5, 6-8 |
| Sequential spawn | 2-6, 3-5 |
| Trigger-Context Matrix (in/out tmux, attached-elsewhere, includes-self, vanished) | 6-3, 6-4, 3-4/6-7 |
| Open order = list order | 6-3 |
| Terminal Identity & Detection (standalone, outside/inside model, bundle-id family, display both, layered keys, no headless, lifecycle, unsupported behaviour) | 1-1..1-6, 6-1, 6-2, 6-9, 2-7 |
| Adapter Contract (detection separate, precedence, `OpenWindow(command)`, two impls, capabilities) | 2-1, 2-2, 4-6 |
| Config Schema (location/format, structure, argv/script recipe, `{command}`, execution contract, validation, precedence) | 4-1..4-6 |
| Permissions & Error Quarantine (typed result boundary, TCC self-exempt, defensive net) | 2-1, 2-5, 3-7, 6-6 |
| Observability (spawn component, closed attr keys, count semantics) | 1-5, 2-6, 3-5, 6-10 |
| State footprint (reads terminals.json, transient markers only, no sessions.json/daemon/prefs/restore) | 3-2, 4-6 |
| Concurrency & Post-Reboot Safety (burst gated post-hydration; CLI runs own bootstrap) | 6-1 (context), 1-6 (context) |
| Testing Strategy (Adapter fake seam, detection seams, driver split, mode state machine, manual residue) | 2-1, 1-2/1-4, 2-4/2-5, Phase 5, 2-5/5-8/6-11 |
| Design References (3 frames, tokens violet/amber/red no-new, clean selected-only, visual-gate) | 5-2/5-3/5-4/5-8, 6-2/6-7/6-11 |
| Deferred scope + build-time residuals (iTerm2/Terminal.app, ps walk cross-version, Ghostty preview API) | correctly out-of-scope; residuals noted in 2-4/2-5/1-2/1-3 context |

## Direction 2 — Plan → Specification (fidelity / anti-hallucination)

No task content was found that cannot be traced to the specification. Spot-checks of the
most likely hallucination candidates all resolved to spec-grounded or explicitly-flagged
implementation mechanics:

- `spawnAckTimeout = 8s` (3-5) → spec "default ~8s per window".
- `total = N incl. trigger`, `opened = confirmed + trigger-on-success` (2-6, 6-10) → spec Count semantics verbatim.
- `friendlyAliases = {ghostty, warp}` (4-3) → spec's two named examples.
- within-config precedence tiers bundle-id > .app/alias > glob, bare `*` lowest (4-3) → spec Precedence verbatim.
- `-1743`/`-1712` → permission-required + driver-composed guidance/deep-link (3-7) → spec Defensive net / Architectural boundary.
- section-header-row placement of the multi-select / unsupported / abort / Opening banners → the plan flags this as design-anchored (delivered Paper frames govern placement over the spec's abstract "notice-band single slot" language) in 5-3, 6-2, 6-5, 6-7 context — an explicitly-reasoned realization, not silent invention.
- `● selected-only, no dim ○` (5-2) → spec "Open toss-up (settled): frames built clean".
- hop-bound / poll interval / env plausibility / name-derivation → each flagged in-task as "not pinned by the spec", scoped as safe implementation mechanics.

No task introduces a new colour token (spec: no new tokens), a `--terminal` override, group-select,
Spaces placement, window introspection, or any other deferred item — all deferrals are honoured.
The `spawn` log component and closed attr set are introduced exactly as the spec governs, with
Phase-1 attr-key scoping guards (1-5) preventing later-phase attrs from leaking early.

**Resolution**: N/A (no findings)
**Notes**: Clean first-cycle pass. Spec is highly settled (5 prior gap-analysis cycles); plan
authored with per-task Spec References + verbatim Context quotes, which made bidirectional
tracing unambiguous.
