---
status: complete
created: 2026-07-01
cycle: 2
phase: Traceability Review
topic: Session Rename Orphans Resume Hook
---

# Review Tracking: Session Rename Orphans Resume Hook - Traceability

## Result: CLEAN

Both directions traced against the full specification (re-read in full for this
cycle). Every specification element has depth-adequate plan coverage, and every
task, acceptance criterion, and edge case traces back to a specific
specification requirement. No missing content, no hallucinated content, no
incomplete coverage.

This cycle specifically re-verified that the three cycle-1 integrity fixes did
NOT introduce spec-coverage gaps or hallucinated content:

1. **Task 3-5 split** (RenameSession-equivalent integration leg + a `tui`-package
   `renameAndRefresh` unit test). Both legs trace to spec § Testing Requirements
   → "Cover **both** triggers: raw `tmux rename-session` *and* the in-TUI rename
   path (`renameAndRefresh`)". The new `tui` unit test asserting "one
   `RenameSession(old,new)` with zero hook re-keying" traces to spec § Scope &
   Non-Goals → "It continues to do a bare `RenameSession` + list refresh with
   **zero** hook re-keying". The split relocates genuine coverage of an existing
   spec requirement to the package that owns the code — it adds no new
   requirement and invents nothing. Grounding verified live: `renameAndRefresh`
   (`internal/tui/model.go:3220`) reduces to
   `m.sessionRenamer.RenameSession(oldName, newName)` + a `ListSessions` refresh;
   `SessionRenamer` is a real injectable seam (`WithRenamer`) with an existing
   `mockSessionRenamer` double — so the unit test is feasible and truthful.

2. **Task 3-3 import-cycle rationale correction** — changes only a grounding
   note (the dependency-closure explanation for why `restore → session` closes no
   cycle). The requirement it supports — a single shared `PortalIDOption`
   constant referenced by the restore re-stamp site — traces to spec §
   Cross-Reboot Persistence → Constant. No spec-coverage impact; no invented
   content.

3. **Task 2-3 empty-slice test pinned to a parser-level assertion**
   (`parsePaneOutput("")` returns non-nil `[]string{}`). Traces to spec § Stage 2
   — the enumeration must inherit `ListAllPanes`' discriminating contract
   (`(nil, err)` on tmux failure vs. a non-nil empty slice on empty output, so a
   failure is never mistaken for "no live panes" — the mass-orphan hazard). The
   change narrows an ambiguous test to one concrete assertion without dropping
   any spec-required behaviour. No hallucination.

## Findings

None.

## Coverage Matrix (Direction 1: Spec → Plan)

| Spec section / element | Plan coverage |
|---|---|
| Problem Statement (silent+delayed orphan; both triggers; bites only when inner process does not restart) | Tasks 3-5 (both triggers, pane kept running), 3-7 |
| Root Cause (four stages desync on rename; mutable name anchor) | Whole plan; explicitly Stages 1-4 → Tasks 2-1/2-2 (S1), 2-3/2-4 (S2), 3-4 (S3), 3-4 ordering-guard (S4) |
| Fix Overview → The option (`session.PortalIDOption = "@portal-id"`) | Task 1-3 (constant) |
| Fix Overview → Its value / Generation contract (fresh opaque `crypto/rand` token, fire-and-forget, no uniqueness check, `sc.gen`) | Tasks 1-3, 1-4 |
| Fix Overview → It is immutable (never rewritten) | Negative property — no task mints a new id post-creation; restore re-stamps the *saved* value only (Task 3-3) |
| Fix Overview → Where it is stamped → `CreateFromDir` | Task 1-3 |
| Fix Overview → Where it is stamped → `QuickStart.Run` (argv-chain stamp before attach; no error seam) | Task 1-4 |
| Fix Overview → Hook key = prefer `@portal-id`, else session name | Tasks 1-1, 1-2 |
| Fix Overview → Coverage / natural migration (no backfill, no `hooks.json` re-key migration) | Reflected in Tasks 1-3, 2-6, 3-1, 3-7 |
| Hook-Key Derivation → central invariant (all sites derive identical rule; three producers + one consumer) | Tasks 1-1, 1-2, 2-1..2-5, 3-4 |
| Hook-Key Derivation → Decoupling from `tmux.PaneTarget` (PaneTarget stays as-is) | Tasks 1-1, 1-5 |
| Hook-Key Derivation → Deliverable — retire four stale doc-comments + transfer invariant | Task 1-5 (all four functions named; invariant transfer to HookKey/HookKeyFormat) |
| Hook-Key Derivation → new primitives `HookKeyFormat` / `HookKey` | Tasks 1-2, 1-1 |
| Stage 1 — Registration (`cmd/hooks.go`) + Failure contract (abort, never synthesize name key) | Tasks 2-1, 2-2 |
| Stage 2 — Stale cleanup live keys (`cmd/run_hook_stale_cleanup.go`, `cmd/clean.go`) | Tasks 2-3, 2-4 |
| Stage 3 — Restore lookup baking (`internal/restore/session.go`, saved indices) | Task 3-4 |
| Stage 4 — Hydrate lookup (No change) + Post-restore consistency | No-change enforced by Task 3-4 (ordering-trap guard); post-restore consistency proven by Task 3-6 |
| Cross-Reboot Persistence → intro (why persist; @portal-dir divergence) | Task 3-1 |
| Cross-Reboot Persistence → 1. Schema (`PortalID string`, tolerant decode, no version bump) | Task 3-1 |
| Cross-Reboot Persistence → 2. Capture (append column, `captureFieldCount` 10→11, zero-row edge) | Task 3-2 |
| Cross-Reboot Persistence → 3. Restore re-stamp (incl. concerns (a)(b)(c); skip when empty) | Task 3-3 |
| Cross-Reboot Persistence → Constant (single shared, importable by re-stamp site) | Tasks 1-3, 3-3 |
| Cross-Reboot Persistence → Firing does not depend on re-stamp (ordering trap; never read live id) | Task 3-4 |
| Scope & Non-Goals → both triggers fixed at root (external + in-TUI, no interception) | Task 3-5 (integration leg + `tui` unit test) |
| Scope & Non-Goals → external start-hook unchanged | Task 2-2 (spec ref AC6) |
| Scope & Non-Goals → out-of-scope subsystems (`@portal-skeleton-*`, `sessions.json` merge, non-existent `@portal-active-*`) | Correctly NO task (non-goals); `StructuralKeyFormat`/`ListAllPanes` preserved by Tasks 1-5, 2-4 |
| Scope & Non-Goals → not retrofitting legacy sessions | Reflected in Tasks 2-6, 3-1, 3-7 |
| Testing Requirements → Derivation primitives (unit) | Tasks 1-1, 1-2, 2-5 |
| Testing Requirements → Cross-site consistency (reg == cleanup == restore-baker byte-identical) | Live half: Task 2-5; restore-baker leg exercised functionally by Tasks 3-4/3-5/3-6 (survival + firing require byte-identity) |
| Testing Requirements → Creation & persistence (component) | Tasks 1-3, 1-4, 3-1, 3-2, 3-3 |
| Testing Requirements → The rename gap (integration, both triggers) | Task 3-5 |
| Testing Requirements → Durable across repeated reboots | Task 3-6 |
| Testing Requirements → Post-restore cleanup keeps the restored hook | Task 3-6 |
| Testing Requirements → Legacy / no-regression (integration) | Tasks 2-6, 3-7 |
| Testing Requirements → Multi-pane (integration) | Task 3-7 |
| Testing Requirements → Conventions (no `t.Parallel()`, `IsolateStateForTest`, `tmuxtest`/`restoretest`/`portalbintest`) | Applied across all test-bearing tasks (1-1..3-7) |
| Acceptance Criteria 1 (Stamping, non-fatal) | Tasks 1-3, 1-4 |
| Acceptance Criteria 2 (Rename survives reboot, both triggers) | Task 3-5 |
| Acceptance Criteria 3 (Durable across repeated reboots) | Tasks 3-3, 3-6 |
| Acceptance Criteria 4 (Cleanup safety) | Tasks 2-4, 3-6 |
| Acceptance Criteria 5 (No-migration upgrade) | Tasks 2-6, 3-1 |
| Acceptance Criteria 6 (No external/UI change) | Tasks 2-2, 3-5 |
| Acceptance Criteria 7 (Multi-pane) | Task 3-7 |
| Acceptance Criteria 8 (Graceful legacy) | Tasks 1-1, 3-7 |
| Risks → Missed key-producing site (primary) | Mitigation = single shared primitive (Tasks 1-1/1-2) + cross-site consistency test (Task 2-5) + ordering-trap guard (Task 3-4) |
| Risks → Best-effort stamping | Tasks 1-3, 1-4 (swallowed failures, non-fatal) |
| Risks → Token collision | Task 1-3 (DECISION block accepts spec's fire-and-forget residual) |
| Risks → Release type (regular release, not hotfix) | Delivery note, not buildable scope — correctly no task |

## Direction 2: Plan → Spec (fidelity / anti-hallucination)

Every task's Problem, Solution, Do, Acceptance Criteria, Tests, and Edge Cases
trace to a specific spec section (each task carries a Spec Reference and its
Context blocks quote the source). This cycle re-audited the cycle-1 fixes plus
the previously-cleared judgment-call additions:

- **Task 3-5 — RenameSession-equivalent leg + `tui` seam unit test** (cycle-1
  fix): traces to spec § Testing Requirements → "Cover both triggers" and § Scope
  & Non-Goals → "bare `RenameSession` + list refresh with zero hook re-keying".
  The `tui` unit test asserts exactly the spec-stated property (one
  `RenameSession`, no re-keying) — no invented behaviour. Verified the seam and
  reduction against live source. Traceable.
- **Task 3-3 — corrected import-cycle rationale** (cycle-1 fix): grounding-only
  note supporting the spec's single-shared-constant requirement (§ Cross-Reboot
  Persistence → Constant). No new scope. Traceable.
- **Task 2-3 — parser-level empty-slice assertion** (cycle-1 fix): traces to the
  spec's Stage 2 discriminating-contract requirement (failure ≠ empty live set).
  Narrows a test, adds nothing. Traceable.
- **Task 1-3 token-width DECISION** (reuse `sc.gen` at 6-char, no widening) — spec
  explicitly delegates token width as "an implementation detail (the existing
  `NewNanoIDGenerator` scheme, widened if warranted)". Traceable.
- **New methods `tmux.ResolveHookKey` (2-1) and `tmux.ListAllPaneHookKeys` (2-3)**
  — spec names `ResolveHookKey` as the Stage 1 read and requires the Stage 2
  enumeration to switch to `HookKeyFormat` while keeping name-based `ListAllPanes`
  available; a separate method is the mechanism that satisfies both. Traceable.
- **`AllPaneLister` interface-method rename (2-4)** — mechanism to enforce the
  Stage 2 switch at the type level; algorithm/guard/swallow policy held unchanged
  per spec. Traceable.
- **"No change" handling** — Stage 4 hydrate and the in-TUI `renameAndRefresh`
  are correctly rendered as constraints/guards (Task 3-4 ordering trap; Task 3-5
  scope guard + relocated unit-test coverage) rather than build tasks, matching
  the spec's "no change required".

No plan content lacks a spec basis. No acceptance criterion tests anything the
spec does not require. No invented edge cases.

## Notes

- Grounding spot-checks against live source (this cycle): `renameAndRefresh`
  (`model.go:3220`) → `sessionRenamer.RenameSession` + `ListSessions` refresh;
  `SessionRenamer` seam + `mockSessionRenamer` double present (confirms the Task
  3-5 `tui` unit test is feasible); `captureFieldCount = 10` and `captureFormat`
  = 10 fields (confirms Task 3-2's 10→11 bump); `state.Session` = Name /
  Environment / Windows (confirms Task 3-1 adds `PortalID`); `collectArmInfos`
  bakes `hookKey: tmux.PaneTarget(sess.Name, w.Index, p.Index)` at `session.go:110`
  (confirms Task 3-4's swap to `tmux.HookKey`). All accurate (line numbers
  approximate, as the plan states).
- Cross-site consistency (spec Testing Requirements) is a three-way byte-identity
  requirement (registration read == cleanup enumeration == restore baker). Task
  2-5 explicitly covers the LIVE half and defers the restore-baker leg to Phase 3.
  The restore-baker leg is exercised functionally (Task 3-5 firing requires the
  baked key to match the registered id-key; Task 3-6's cleanup-survival requires
  the restore-baked/registered key to be byte-identical to the live
  `ListAllPaneHookKeys` enumeration). Coverage is adequate; recorded as an
  observation, not a finding (carried forward from cycle 1).
