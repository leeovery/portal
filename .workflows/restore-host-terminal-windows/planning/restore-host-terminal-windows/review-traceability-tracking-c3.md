---
status: complete
created: 2026-07-12
cycle: 3
phase: Traceability Review
topic: Restore Host Terminal Windows
result: clean
finding_count: 0
---

# Review Tracking: Restore Host Terminal Windows - Traceability

## Result

**CLEAN** — no findings. Cycle-3 is a fresh full two-directional pass over the
complete specification and all 45 authored tasks (planning.md + phase-1…phase-6
task files). Every specification element is represented in the plan with
implementer-ready depth, and every task traces back to a cited specification
section. The cycle-1 and cycle-2 fixes are coherent and have not dropped or
contradicted any specification requirement.

## Scope of this pass

- Specification: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` (read in full).
- Plan: `planning.md` + `phase-1-tasks.md` … `phase-6-tasks.md` (45 tasks, all authored detail read in full).
- Both directions traced at the level of authored task detail (Problem / Solution / Do / Acceptance Criteria / Tests / Edge Cases / Context / Spec Reference).

## Direction 1 — Specification → Plan (completeness)

Every spec section has task coverage with preserved essence and instruction:

| Spec section | Covering tasks |
|---|---|
| Overview / Foundational shape / Naming | 1-1, 1-6 (naming); overall structure across all phases |
| Spawn Architecture — one service/two callers | 1-6 (CLI seam), 2-6 (pipeline), 6-3 (in-process picker) |
| Spawn Architecture — N vs N−1 split / Order load-bearing | 2-6, 3-5, 6-3, 6-4 |
| Spawn Architecture — command composition (`os.Executable()`) | 2-3, 3-5 |
| Spawn Architecture — env self-sufficiency (PATH inject, TMUX/TMUX_PANE strip) | 2-3 |
| CLI behaviour + Reporting & exit codes (0/1/2 matrix) | 1-6, 2-6, 2-7, 3-4, 3-6, 3-7 |
| Multi-Select Mode — trigger/marking, N=0/N=1, key coexistence, sticky, filter sub-state, affordance, per-session identity | 5-1, 5-3, 5-4, 5-5, 5-6, 5-7, 5-2 |
| Burst Contract — pre-flight + all-or-nothing / leave-what-opened | 3-4, 3-6, 6-6, 6-7 |
| Burst Contract — token ack (ids, channel, `--spawn-ack`, timeout, cleanup) | 3-1, 3-2, 3-3, 3-5 |
| Burst Contract — in-picker execution (async, input-lock, in-burst feedback, cancellation) | 6-3, 6-5, 6-8 |
| Burst Contract — sequential spawn | 2-6, 3-5 |
| Trigger-Context Matrix / Open order (list order) / Enter-opens-marked-set | 2-6, 6-3, 6-4, 5-7, 6-7 |
| Terminal Identity — standalone op, inside/outside model, bundle-id family, host-local principle | 1-1, 1-2, 1-3, 1-4, 1-5 |
| Terminal Identity — user-facing display (both), config keys layered | 1-6, 6-2, 4-3 |
| Detection lifecycle — detect-once cached, off first-paint, error vs NULL, in-flight at Enter | 6-1, 1-5, 6-3, 6-9 |
| Unsupported-terminal behaviour (banner + Enter N=1/N≥2) | 2-7, 6-2, 6-9 |
| Adapter Contract — detection separate, resolution precedence, `OpenWindow(command)`, two impls, capability extensibility | 2-1, 2-2, 4-6 |
| Config Schema — location/format, structure, recipe (argv/script), `{command}`, execution contract, validation, precedence | 4-1, 4-2, 4-3, 4-4, 4-5, 4-6 |
| Permissions & Error Quarantine — typed result, TCC self-exempt, defensive net (-1743/-1712), burst-stop | 2-1, 2-5, 3-7 |
| Observability — spawn component, closed attr set, count semantics, INFO summary + DEBUG per-window | 1-5, 2-6, 3-5, 6-10 |
| Concurrency & Post-Reboot Safety | context in 6-1, 1-6 (reasons already built-in; no new work) |
| Testing Strategy & DI Seams — Adapter fake, detection seams, driver split, mode state machine, manual residue | 2-1, 1-2/1-3/1-4, 2-4/2-5/3-7, 5-x, 2-5/5-8/6-11 |
| Design References — three frames, tokens, toss-up settled, visual-gate process | 5-8, 6-11, 5-2 |
| Dependencies / Deferred scope / Build-time residuals | deferred items correctly out-of-scope; residuals carried in task Context (2-4, 2-5, 1-3) |

Deferred/out-of-scope items (group-select, Spaces placement, window introspection,
headless `portal spawn`/`--terminal`, defensive `@portal-spawn-*` sweep, parallel
spawn, detect-and-wait cap, daemon-readable ack follow-on) are all correctly left
unbuilt and flagged in task Context, matching the spec's deferral list.

## Direction 2 — Plan → Specification (fidelity / anti-hallucination)

All 45 tasks trace to a specific specification section (cited in each task's
**Context** + **Spec Reference**). No task introduces scope, behaviour, or
acceptance criteria absent from the spec.

Implementation details the spec does not pin are each **honestly flagged** as
implementation choices (not presented as spec requirements) and stay within the
scope of an already-specified capability:

- Friendly-name derivation algorithm (1-1) — flagged "spec does not pin beyond non-empty, human-readable"; the walk supplies the true `.app` name when available.
- `__CFBundleIdentifier` plausibility rule + `GHOSTTY_*` key set (1-3) — flagged "not enumerated by the spec beyond 'accurate outside tmux'"; empty/whitespace is the clear fallback trigger.
- Per-client transient-walk-error policy (1-4) — flagged as the reasonable reading of "transient error distinct from clean NULL" applied to the multi-client loop.
- `env -u TMUX -u TMUX_PANE` strip mechanism (2-3) — a faithful implementation of the spec's load-bearing "explicitly strips TMUX/TMUX_PANE" invariant; reconciles the spec's literal argv example with its prose invariant.
- Pre-flight ordering before detect/unsupported gate (3-4) — flagged "not pinned by the spec; both paths exit 1, so cosmetic."
- Section-header-row placement of banners/flashes vs the spec's abstract "single-slot" language (5-3, 6-2, 6-7) — flagged "the golden Paper frame governs placement," which the spec explicitly authorises ("Exact colour token, glyph, and banner/footer copy are fixed by the delivered Paper design").

None of these constitute hallucinated scope; all are necessary concreteness that
the spec delegates to implementation, and each is transparently marked.

## Cycle-1 / Cycle-2 fix coherence (verified this pass)

- **Config-resolver divergence (cycle-1 6-3) + config-aware Resolve seam (cycle-2):** injected as a single site in 6-1 (`spawn.NewResolver(terminals.json).Resolve`), reused by 6-3's burst and 6-9's gate; the CLI builds the identical resolver in 4-6. Picker and CLI resolve `terminals.json` identically. No divergence remains.
- **`DetectUnsupported()` resolution-based classification (cycle-2):** consistently the unsupported test in 6-1/6-2/6-3/6-9 (covers NULL remote/mosh AND non-NULL recognised-but-undriven identities); user-facing copy branched on `IsNull()` in 6-2/6-9, matching CLI 2-7; the 6-11 capture fixture seeds `detectResolution` so the Apple Terminal (non-NULL) banner renders. Coherent end-to-end.
- **Opening-counter inconsistency (cycle-1 6-5):** `burstTotal` fixed at N at dispatch (6-3), `burstDone` advances 0…N−1, band never reaches N/N; reconciled with the `opened N/N` log summary (6-10) and the `Opening 2/3` fixture (6-11). Coherent.
- **`Burster.Run` call-site (cycle-1 6-3):** the additive `Run(ctx, external, progress)` signature change explicitly updates the approved Phase-3 CLI call sites to `Run(context.Background(), external, nil)`, preserving Phase-2/3 behaviour. No dangling call site.
- **Undeclared `AckChannelFull` (cycle-1 3-2):** declared in 3-2 as `AckCollector`+`AckCleaner`; referenced by 3-5 (`SpawnDeps.Ack`) and 6-3 (`tui.Deps.AckChannel`). Type resolves.

## Findings

None.

**Resolution**: Complete — clean pass, no tracking entries.
