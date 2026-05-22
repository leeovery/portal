---
status: complete
created: 2026-05-22
cycle: 3
phase: Traceability Review
topic: Slow Open Empty Previews And Zombie Sessions
---

# Review Tracking: Slow Open Empty Previews And Zombie Sessions - Traceability

Cycle 3 follow-up after applying the single critical cycle-2 integrity finding (Task 6-6 redesigned around the `tmux break-pane` / `move-pane` mechanism so the Do, Acceptance Criteria, Tests, and Edge Cases all consistently exercise Component D's `os.Exit(0)` self-eject path rather than a SIGKILL-induced death).

Cycle 1 traceability was clean. Cycle 2 traceability was clean. The cycle-2 integrity fix modified mechanism-level prose inside a single task (Phase 6 Task 6-6) — it did not add, remove, or relocate phases, did not change which spec components are covered by which phase, and did not introduce technical approaches or acceptance criteria that lack a spec basis.

## Direction 1: Specification → Plan (completeness)

Re-walked each spec section against the plan post-fix. All spec elements remain represented:

- **Shared Primitive — Daemon Identity Check** → Phase 1 Task 1-1 (`state.IdentifyDaemon`); consumed by Phase 3 Task 3-3 (readiness barrier), Phase 4 Tasks 4-1 / 4-3 / 4-6 (A, B, C identity gates), Phase 5 Task 5-2 indirectly via probe.
- **Component A — Kill-Barrier Escalation** → Phase 4 Tasks 4-1 (SIGKILL escalation with identity-check) and 4-2 (no-final-flush snapshot test).
- **Component B — Bootstrap-Time Orphan Sweep** → Phase 4 Tasks 4-3 (core), 4-4 (orchestrator wiring as step 4 + CLAUDE.md update), 4-5 (integration test).
- **Component C — `daemon.lock` inode-replacement stabilisation** → Phase 4 Tasks 4-6 (pre-acquire daemon.pid check), 4-7 (post-flock inode cross-check + retry), 4-8 (AST-walking adjacency test), 4-9 (upgrade-path two-binary integration).
- **Component D — Daemon self-supervision** → Phase 5 Tasks 5-1 through 5-9; the composite live-context assertion is Task 6-6, whose break-pane mechanism is a planning-phase refinement of the spec's "(or equivalent)" allowance under § Component D's Test staging note.
- **Component E — `CaptureStructure` per-session log-and-continue** → Phase 2 Tasks 2-1 through 2-5 (sentinel, logger plumbing, log-and-continue, fail-fatal regression cover, daemon call-site wiring).
- **Component F — Saver creation sets `destroy-unattached=off` before daemon starts** → Phase 3 Tasks 3-1 through 3-6 (constant split, reorder, readiness barrier, unhealthy-saver recreate, end-state integration test, env-inheritance parity test).
- **Component G — Test isolation contract** → Phase 1 Tasks 1-2 (helper), 1-3 (cleanup backstop), 1-4 (audit + migration), 1-5 (CLAUDE.md documentation).
- **Composite End-to-End Verification** → Phase 6 Tasks 6-1 through 6-8 (harness, pre-fix reproduction, convergence, scrollback stability, lock-refusal, D-eject live, F observables, portaltest backstop).
- **End-State Verification observables** → captured across Phase 4 acceptance bullet "composition: …converges to pgrep -fxc == 1 within ~6 s" and Phase 6 acceptance bullets (sub-second open implicit via removal of the 5 s ceiling; scrollback stability; permanent kill via singleton enforcement).
- **Risk Summary — Component D hysteresis measurement** → Phase 5 Task 5-1 explicitly performs the empirical measurement and locks `selfSupervisionHysteresisTicks`.
- **Transitional Recovery snippet** → spec marks it explicitly as out-of-band manual recovery and not part of the shipped fix; correctly absent from the plan.

No spec element is missing from the plan.

## Direction 2: Plan → Specification (fidelity)

Every plan element still traces to the specification:

- The break-pane / move-pane mechanism in Task 6-6 traces to spec § Component D Test staging note ("via `tmux respawn-pane -k -t _portal-saver 'sh -c \"exec tail -f /dev/null\"'` (or equivalent)") in combination with spec § Component D's "No final flush on self-eject" acceptance criterion (the two snapshots must be identical) and spec § Composite End-to-End Verification step 8 (self-ejects within (N+1) tick intervals). The mechanism choice is the implementer-facing refinement of "or equivalent" required to satisfy the snapshot invariant in live composite state — it is not a new requirement, technical approach not discussed in the spec, or invented acceptance criterion.
- All Phase 6 tasks continue to map 1:1 to the spec § Composite End-to-End Verification scenario steps 1–9.
- No new tasks, phases, acceptance criteria, or edge cases lacking a spec foundation were introduced by the cycle-2 fix.

## Findings

None. The plan remains a faithful, complete translation of the specification after the cycle-2 integrity fix.

---
