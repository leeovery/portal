---
status: in-progress
created: 2026-04-30
cycle: 2
phase: Plan Integrity Review
topic: hidden-sessions-showing-on-startup
---

# Review Tracking: hidden-sessions-showing-on-startup - Integrity

## Summary

Cycle 1 produced three findings (all minor — file/line citation drift and header style). All three were applied verbatim and are now reflected in `planning.md` and `phase-{1,2}-tasks.md`. Verification of the codebase against the cycle 1 corrections:

- `Client.ListSessions` at `internal/tmux/tmux.go:108` — confirmed
- `ListSessionNames` at `internal/tmux/tmux.go:157` — confirmed
- `TestStartServer` at `internal/tmux/tmux_test.go:404` — confirmed
- `TestListSessions` at `internal/tmux/tmux_test.go:44` — confirmed
- Phase 1 task headers now use canonical `### Task N: ...` form — confirmed

Cycle 1 fixes are clean.

The newly-added Task 2-3 (release-notes line for legacy `0` cleanup) was checked carefully for template completeness, scope, and consistency with the rest of the plan. It carries Problem / Solution / Outcome / Do / AC / Tests / Edge Cases / Context / Spec Reference; its scope is correctly bounded to a docs-only deliverable; its sequencing constraint (ships with Phase 2, not Phase 1) is captured in both the task body and Phase 2's acceptance criteria. The absence of an explicit task-level `blocked_by` on Task 2-1 is acceptable because the release-notes wording references the literal `0` (legacy name), not the new `_portal-bootstrap`, so it is content-independent of Task 2-1's code change — the phase-level sequencing (P2 blocked by P1) is sufficient.

A spot-check of remaining line-number citations in `phase-2-tasks.md` Task 2-2 against `cmd/bootstrap/reboot_roundtrip_test.go` was performed:

- `runRebootRoundTrip` line 174 — confirmed
- `o.Run` after-call site line 354 — confirmed
- `verifyLiveStructure` call line 367 — confirmed
- `createSavedTopology` line 435 — confirmed
- `verifyCapturedIndex` body line 533 (the alpha/beta name assertion) — confirmed
- `_seed` creation lines 236, 319 — confirmed
- `bootstrap.NoOpSaver{}` lines 344, 833 — confirmed
- `TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary` line 765 — confirmed
- `//go:build integration` line 1 and `testing.Short()` line 122 — confirmed

All citations are accurate.

## Findings

No new findings. The plan is structurally sound and ready for implementation:

- All five tasks satisfy the canonical task template (Problem / Solution / Outcome / Do / AC / Tests, plus optional Edge Cases / Context / Spec Reference where useful).
- Each task is a single TDD cycle (or appropriate docs-only housekeeping in the case of Task 1-2's verification work and Task 2-3's release-note line) and is independently verifiable.
- Phase ordering (Phase 1 chokepoint filter → Phase 2 rename + end-to-end guard + release note) follows the spec's mandated sequence and the rationale is captured in each phase's "Why this order" block.
- Dependencies are explicit and minimal: T1-2 blocked by T1-1, T2-2 blocked by T2-1, P2 blocked by P1. No circular dependencies. Sequential intra-phase tasks rely on natural ordering, which matches the convention in the format's `reading.md`.
- Acceptance criteria are pass/fail and tied to concrete file paths, line ranges, function names, and assertion shapes.
- File-path / line-number citations are accurate against the current codebase.
- Task self-containment is strong: an implementer can pick up any single task without needing to read another task to understand what to do (cross-task references exist but are informational, not load-bearing for execution).

## Closeout

No findings to action. Recommend the orchestrator mark the integrity review complete after this cycle.

---
