---
status: complete
created: 2026-06-01
cycle: 2
phase: Traceability Review
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Traceability

Cycle-2 (follow-up) traceability analysis of the plan (6 phases / 52 tasks across `planning.md` + `phase-1..6-tasks.md`, mirrored in tick topic `tick-7ac1a9` — 58 tick nodes = 6 phase parents + 52 leaf tasks) against the validated specification (14 sections). Both directions re-checked: (1) Specification → Plan completeness, (2) Plan → Specification fidelity (anti-hallucination). The cycle-1 fix (task `portal-observability-layer-5-11` homing the `signal` component, plus task 5-8's `saverLogger`-ownership clarification) was re-verified for faithfulness and completeness, and both directions were re-swept for anything cascading or previously missed.

## Result: CLEAN

No findings. The plan is a faithful, complete translation of the specification.

## Cycle-1 fix verification

**Finding c1-1 (the `signal` component gap) is faithfully and completely resolved.**

- **Plan structure updated correctly**: `planning.md` Phase 5 task table now has 11 rows (5-1 … 5-11); the Phase 5 `**Acceptance**` block carries the new `signal`-populated criterion (planning.md:146); `phase-5-tasks.md` front-matter `total: 11` and contains 11 task headers including `## portal-observability-layer-5-11`; the tick mirror has `tick-137c05` for 5-11.
- **Spec ownership fully covered**: the spec's `signal` component owns two code areas — (a) `EagerSignalHydrate` and (b) "the lower-level FIFO signal send/receive plumbing in `internal/state`." Task 5-11 covers BOTH: (a) re-attributes the `EagerSignalHydrate` write-failure WARN from `hydrate` to `signal`, and (b) instruments `internal/state/signal_hydrate.go` (`WriteFIFOSignal`/`SendHydrateSignal`) under `signal` per the call-site/level-discipline pattern.
- **Spec carve-out honored**: the spec's `signal` ownership row explicitly keeps the hydrate helper's own exit-path `signal timeout` line under `hydrate` (Hook-firing catalog). Task 5-11 states it touches only the signaling *mechanism*, never the helper exec-chain; Phase 6 task 6-2 correctly emits `hydrate: signal timeout` under `hydrate`. No conflict, no double-attribution.
- **No hallucination introduced by the fix**: task 5-11 adds no new components, no new attr keys (only closed-vocabulary `path`/`error`/`error_class`), and — correctly — NO cycle-summary INFO (the § Cycle-level summary "Concrete cycle catalog" lists no signal sweep; the EagerSignalHydrate bootstrap-step-7 summary is owned by task 5-2). The `error_class="unexpected"` and the DEBUG retry-ladder breadcrumb both trace to the level-discipline table and the § Call-site logging pattern mechanical rule. The lower-level-plumbing seam choice is correctly carried as an intentional `[needs-info]`.

**Task 5-8 `saverLogger`-ownership clarification verified**: task 5-8 now explicitly states it depends on task 5-7 for the shared `var saverLogger = log.For("saver")` declaration in `internal/tmux/portal_saver.go`, must NOT re-declare it (a second `var saverLogger` would be a duplicate-declaration compile error), and that 5-7 lands first in dependency order. This removes the cycle-1 ambiguity; the two tasks now have a single, unambiguous owner of the `saverLogger` var.

## Direction 1 (Specification → Plan completeness) — re-swept, complete

The root cause of the cycle-1 gap was a Phase-1 "introduced where Phase 5/6 promote them" deferral (phase-1-tasks.md:446) for `capture`/`saver`/`signal` not being honored by a later phase. All three deferred components are now homed:

- `capture` → task 5-1 (`var captureLogger = log.For("capture")`, `capture: tick complete`).
- `saver` → tasks 5-7 / 5-8 (`var saverLogger = log.For("saver")`) + the task 4-4 escalation DEBUG breadcrumb.
- `signal` → task 5-11 (the cycle-1 fix).

The full closed 15-component taxonomy is now wired in the plan: `bootstrap`/`daemon`/`restore`/`hydrate`/`notify`/`hooks`/`preview` (Phase 1 wholesale migration of existing components, task 1-9, incl. `ComponentNotify→notify`), `log-rotate` (Phase 2), `process` (Phase 2 start/exit/exec/panic/log-level-resolved + Phase 6 `hydrate: exec` parallel), `clean` (Phase 2 retention + Phase 5 sweeps), `capture`/`saver`/`signal` (Phase 5), `aliases`/`projects`/`hooks` mutation audit (Phase 3). `grep "<component>:"` now has a producing site for every closed component.

Other spec sections re-confirmed to have full plan coverage:

- **`internal/log` package** (Public API `Init`/`For`/`Close`/`SetTestHandler`, swappable-handler indirection, pre-`Init` stderr-text default, `process_role` resolution, migration sweep / legacy-logger deletion) → Phase 1 tasks 1-1…1-10.
- **Subsystem taxonomy** (15 components, 49 attr keys, mandatory per-record baselines, text/JSON rendering, `component:`-prefix rule, extension policy) → rendering/baselines in task 1-3; component homes across Phases 1/2/3/5; closed-vocabulary discipline enforced per task.
- **Log-level discipline** (4-level contract, INFO production default, mechanical selection table, invalid-value fallback + WARN) → task 1-2 (resolution) + task 2-11 (fallback WARN); applied mechanically throughout.
- **Call-site logging pattern** (independent calls, `.With` sticky context, `LogAttrs`, prohibited patterns) → applied per-task.
- **Log rotation** (date-aware fd reuse + inode identity, `O_CREAT|O_EXCL` first-of-day, pid-scoped symlink swing, migration guard, `chmod 0400` past-day seal, same-day size-cap `.N`, best-effort write) → Phase 2 tasks 2-1…2-7.
- **Retention policy & audit** (single-winner `swept.<today>` gate, per-deletion INFO breadcrumb, invalid-retention WARN, sentinel prune, `portal clean --logs` gate-bypass) → Phase 2 tasks 2-8, 2-9.
- **Defensive invariants** (rotated-file immutability, `O_CREAT|O_EXCL`, per-process `start`/`exit`/`exec`/`panic` markers, lifecycle-marker level-filter bypass, unbuffered-writer constraint, four-way terminal classification, externally-killed footnote) → Phase 2 tasks 2-10…2-14 + the daemon self-eject pairing in task 5-10.
- **Log-level propagation verification** (`log-level resolved` line, `portaltest.AssertLogLevelResolved`) → Phase 2 tasks 2-11, 2-15.
- **State-mutation audit trail** (hooks/aliases/projects store-seam INFO/WARN, closed `op` vocabulary, `error_class` phase space, set-noop DEBUG, batch summaries, sanctioned `migrateConfigFile` emitter) → Phase 3 tasks 3-1…3-6.
- **Diagnostic context preservation** (4 boundary classes; 4 enumerated gap-closure sites: `defaultIdentifyPS`, `escalateKillToSIGKILL` breadcrumb, `ShowGlobalHooks` asymmetry, uncommented defensive branches) → Phase 4 tasks 4-1…4-6.
- **Cycle-level summary** (full Concrete cycle catalog) → daemon tick (5-1), bootstrap orchestration + per-step (5-2), restore phase A/B (5-3/5-4), orphan-daemon + marker sweeps (5-5), orphan-FIFO sweep (5-6); plus the two cross-phase rows — Hooks CleanStale (Phase 3 task 3-3) and Retention sweep (Phase 2 task 2-8) — each with single, non-overlapping coverage (task 5-5 explicitly disclaims them).
- **Saver/daemon lifecycle taxonomy** (placeholder created / destroy-unattached off / respawn-daemon / daemon ready / kill-barrier started / escalated / placeholder died; lock acquired / self-eject / shutdown; dropped `daemon: spawn`) → Phase 5 tasks 5-7…5-10; closed reason value spaces respected.
- **Hook-firing observability limit** (hook-lookup DEBUG, four exit-path INFO lines, terminal `hydrate: exec` parallel to `process: exec`) → Phase 6 tasks 6-1…6-4.

## Direction 2 (Plan → Specification fidelity / anti-hallucination) — re-swept, clean

Every task carries a `**Spec Reference**` tracing it to a named spec section; no task content was found that cannot be traced to the specification. The plan remains notably disciplined about not inventing scope:

- No components outside the closed 15; no attr keys outside the closed 49 (task 1-9 explicitly refuses to invent a `window` contextual key — `windows` exists only as a cycle-summary count); no new reason values (tasks 5-8/5-9 stay within `kill-session-timeout` / `{signal,exit,unknown}` / `{sighup,signal,exit}`); no invented cycle summaries (task 5-11 explicitly omits one, as the catalog mandates none).
- Genuine spec/code mismatches are carried as intentional `[needs-info]` flags rather than resolved by invention — `write-failed-fsync` having no `AtomicWrite` step (3-1), the alias store's `os.WriteFile`-not-`AtomicWrite` shape and split in-memory/persist seam (3-5), the single-batched-`Save` per-entry-WARN unreachability in hooks/projects `CleanStale` (3-3/3-4), the `migrateConfigFile` whole-file-move key attr + error_class (3-6), the daemon `shutdown reason` signal-capture requirement (5-9), the lower-level `signal` plumbing seam (5-11), the `natural_churn` classifier gap (5-1), the JSON-mode selection seam (1-3), the `placeholder died reason` derivation (5-8), and the hydrate `fifo missing` row collapsing under timeout (6-3). Per the cycle brief these are intentional and are NOT traceability defects.

## Notes

This is a follow-up cycle. The sole cycle-1 completeness gap is fixed faithfully and completely, the cycle-1 task-5-8 clarification is in place, and a fresh both-directions sweep — with particular attention to the deferred-component-promise class that produced the cycle-1 finding — surfaces nothing new. No findings to present.
