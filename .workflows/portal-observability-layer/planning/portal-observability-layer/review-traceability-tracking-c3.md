---
status: complete
created: 2026-06-01
cycle: 3
phase: Traceability Review
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Traceability

Cycle-3 (follow-up) traceability analysis of the plan (6 phases / 52 tasks across `planning.md` + `phase-1..6-tasks.md`, mirrored in tick topic `tick-7ac1a9` — 58 tick nodes = 6 phase parents + 52 leaf tasks, re-confirmed via `tick list`) against the validated specification (14 sections). Both directions re-checked in full: (1) Specification → Plan completeness, (2) Plan → Specification fidelity (anti-hallucination). The cycle-1 fix (task 5-11 homing the `signal` component) and the cycle-2 fix (5-11's logger renamed to `signalLogger` to remove the function-local shadow hazard) were both re-verified for faithfulness, and both directions were re-swept for anything cascading from the rename.

## Result: CLEAN

No findings. The plan is a faithful, complete translation of the specification.

## Cycle-1 + cycle-2 fix verification (no cascade)

**Cycle-1 finding (the `signal` component gap) and the cycle-2 `signalLogger` rename are both faithfully resolved and have not cascaded.**

- **Task 5-11 still covers both spec-assigned `signal` code areas**: (a) re-attributes `EagerSignalHydrate`'s write-failure WARN from `hydrate` to `signal`, and (b) instruments the lower-level `internal/state` FIFO signal send/receive plumbing (`signal_hydrate.go` — `WriteFIFOSignal`/`SendHydrateSignal`) under `signal`. Matches the spec's `signal`-ownership row exactly.
- **Cycle-2 `signalLogger` rename verified and contained**: task 5-11 (phase-5-tasks.md:593, 598–599, 600) now binds `var signalLogger = log.For("signal")` and points the WARN + the success-path DEBUG breadcrumb at `signalLogger` (NOT a bare `logger`), with an explicit in-task note that `EagerSignalHydrate`'s function-local `logger := c.Logger` would otherwise shadow a package-level `logger` and silently leave the WARN on the injected `hydrate`-bound logger. The internal/state arm uses the same `signalLogger` name for the identical shadow reason against the pervasive `logger` function-parameter convention. The rename is a pure naming/correctness change inside 5-11 — it touches no other task, no attr key, no component, and no acceptance criterion elsewhere; nothing cascaded.
- **No double-attribution introduced**: the spec keeps the hydrate helper's own exit-path `signal timeout` line under `hydrate` (Hook-firing catalog). Task 5-11 states it touches only the signaling *mechanism*; Phase 6 task 6-2 correctly emits `hydrate: signal timeout` under `hydrate`. No conflict.
- **Naming consistency confirmed**: `signalLogger` (5-11) now matches the established Phase-5 per-component-logger convention — `captureLogger` (5-1), `cleanLogger` (5-5/5-6), `saverLogger` (5-7/5-8), `daemonLogger` (5-9/5-10). The shared `saverLogger` is owned by 5-7 and referenced (not re-declared) by 5-8 — the cycle-1 ambiguity that cycle-2 noted remains resolved.

## Direction 1 (Specification → Plan completeness) — re-swept, complete

Every spec section, decision, mechanical rule, closed vocabulary, and enumerated site has a producing task. Spot-checked the full closed taxonomies:

- **Closed 15-component taxonomy fully wired** (the spec's "single source of truth for the component count"): `bootstrap`/`daemon`/`restore`/`hydrate`/`notify`/`hooks`/`preview` (Phase 1 wholesale migration, task 1-9, incl. `ComponentNotify→notify`), `log-rotate` (Phase 2), `process` (Phase 2 start/exit/exec/panic/log-level-resolved + Phase 6 `hydrate: exec` parallel), `clean` (Phase 2 retention + Phase 5 sweeps 5-5/5-6), `capture` (5-1), `saver` (5-7/5-8 + the 4-4 escalation DEBUG breadcrumb), `signal` (5-11), `aliases`/`projects`/`hooks` mutation audit (Phase 3). `grep "<component>:"` has a producing site for every closed component.
- **Closed 49-key attr vocabulary**: confirmed by extracting all attr-key tokens across the six task files and cross-referencing every group — Contextual (14), Cycle-summary (14), Lifecycle (7: incl. `flush_completed` in 5-9), Hydrate (3), Process (7), Baseline (4). No attr key outside the closed 49 appears; `reason` is correctly used as the cross-listed Lifecycle/Process key. Task 1-9 explicitly refuses to invent a `window` contextual key (only `windows` cycle-summary exists) — phase-1-tasks.md:451/482.
- **`internal/log` package** (Public API `Init`/`For`/`Close`/`SetTestHandler`, swappable-handler indirection, pre-`Init` stderr-text default, `process_role` 6-value resolution, big-bang migration sweep / legacy-logger deletion) → Phase 1 tasks 1-1…1-10.
- **Log rotation** (date-aware fd reuse + inode identity, `O_CREAT|O_EXCL` first-of-day, pid-scoped symlink swing, migration guard, `chmod 0400` past-day seal, same-day size-cap `.N`, best-effort/unbuffered write) → Phase 2 tasks 2-1…2-7.
- **Retention policy & audit** (single-winner `swept.<today>` gate, per-deletion INFO breadcrumb, invalid-retention WARN, sentinel prune, `portal clean --logs` gate-bypass `cutoff=today`) → Phase 2 tasks 2-8, 2-9.
- **Defensive invariants** (rotated-file immutability, `O_CREAT|O_EXCL`, per-process `start`/`exit`/`exec`/`panic` markers, lifecycle-marker level-filter bypass, unbuffered-writer constraint, four-way terminal classification, externally-killed footnote) → Phase 2 tasks 2-10…2-14 + the daemon self-eject pairing in 5-10.
- **Log-level propagation verification** (`log-level resolved` line, `portaltest.AssertLogLevelResolved`) → Phase 2 tasks 2-11, 2-15.
- **State-mutation audit trail** (hooks/aliases/projects store-seam INFO/WARN, closed `op` vocabulary incl. `set-noop`/`migrate`, `error_class` AtomicWrite-phase space, batch summaries, sanctioned `migrateConfigFile` emitter) → Phase 3 tasks 3-1…3-6.
- **Diagnostic context preservation** (4 boundary classes; all 4 enumerated gap-closure sites — `defaultIdentifyPS` stderr embed in 4-1, `escalateKillToSIGKILL` DEBUG breadcrumb in 4-4, `ShowGlobalHooks` asymmetry WARN in 4-5, uncommented defensive branches in 4-6) → Phase 4 tasks 4-1…4-6.
- **Cycle-level summary** (full Concrete cycle catalog) → daemon tick (5-1), bootstrap orchestration + per-step (5-2), restore phase A/B (5-3/5-4), orphan-daemon + marker sweeps (5-5), orphan-FIFO sweep (5-6); plus the two cross-phase rows — Hooks CleanStale (Phase 3 task 3-3) and Retention sweep (Phase 2 task 2-8), each with single non-overlapping coverage (5-5/5-6 explicitly disclaim them).
- **Saver/daemon lifecycle taxonomy** (placeholder created / destroy-unattached off / respawn-daemon / daemon ready / kill-barrier started / escalated / placeholder died; lock acquired / self-eject / shutdown; dropped `daemon: spawn` with `tmux_pane` re-homed to `lock acquired`) → Phase 5 tasks 5-7…5-10; closed reason value spaces (`kill-session-timeout` / `{signal,exit,unknown}` / `{sighup,signal,exit}`) respected.
- **Hook-firing observability limit** (hook-lookup DEBUG, four exit-path INFO lines — fifo missing / signal timeout / scrollback missing / scrollback replayed, terminal `hydrate: exec` parallel to `process: exec`) → Phase 6 tasks 6-1…6-4.

## Direction 2 (Plan → Specification fidelity / anti-hallucination) — re-swept, clean

Every task carries a `**Spec Reference**` tracing it to a named spec section; no task content was found that cannot be traced to the specification. The plan remains disciplined about not inventing scope:

- No components outside the closed 15; no attr keys outside the closed 49; no new reason values; no invented cycle summaries (5-11 explicitly omits one, as the catalog mandates none for the signal mechanism).
- Genuine spec/code mismatches are carried as intentional `[needs-info]` flags rather than resolved by invention — `write-failed-fsync` having no `AtomicWrite` step (3-1), the alias store's `os.WriteFile`-not-`AtomicWrite` shape + split in-memory/persist seam (3-5), the single-batched-`Save` per-entry-WARN unreachability in hooks/projects `CleanStale` (3-3/3-4), the `Upsert` `LastUsed`-bump-vs-set-noop ambiguity (3-4), the `migrateConfigFile` whole-file-move key attr + error_class (3-6), the `natural_churn` classifier gap (5-1), the JSON-mode selection seam (1-3), the daemon `shutdown reason` signal-capture requirement (5-9), the `placeholder died reason` derivation (5-8), the lower-level `signal` plumbing seam (5-11), and the hydrate `fifo missing` row collapsing under timeout (6-3). Per the cycle brief these are intentional and are NOT traceability defects.

## Notes

Third consecutive both-directions sweep. The cycle-1 completeness gap remains the only finding ever raised; it is fixed faithfully and completely, the cycle-2 `signalLogger` shadow-hazard fix is in place and contained to task 5-11, and a fresh full sweep — with attention to whether the rename cascaded into attr/component/criteria drift — surfaces nothing new. No findings to present.
