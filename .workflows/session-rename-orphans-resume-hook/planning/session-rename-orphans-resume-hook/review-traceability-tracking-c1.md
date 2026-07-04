---
status: complete
created: 2026-07-01
cycle: 1
phase: Traceability Review
topic: session-rename-orphans-resume-hook
---

# Review Tracking: session-rename-orphans-resume-hook - Traceability

## Result: CLEAN

Both directions traced. Every specification element has depth-adequate plan
coverage, and every task/acceptance-criterion traces back to a specific
specification requirement. No missing content, no hallucinated content, no
incomplete coverage.

## Findings

None.

## Coverage Matrix (Direction 1: Spec → Plan)

| Spec section / element | Plan coverage |
|---|---|
| Fix Overview → The option (`session.PortalIDOption = "@portal-id"`) | Task 1-3 (constant) |
| Fix Overview → Its value / Generation contract (fresh opaque `crypto/rand` token, fire-and-forget, no uniqueness check, `sc.gen`) | Tasks 1-3, 1-4 |
| Fix Overview → It is immutable (never rewritten) | Negative property — no rewrite path exists; restore re-stamps the *saved* value (Task 3-3), no task mints a new id post-creation |
| Fix Overview → Where it is stamped → `CreateFromDir` | Task 1-3 |
| Fix Overview → Where it is stamped → `QuickStart.Run` | Task 1-4 |
| Fix Overview → Hook key = prefer `@portal-id`, else session name | Tasks 1-1, 1-2 |
| Fix Overview → Coverage / natural migration (no backfill, no `hooks.json` re-key migration) | Reflected in Tasks 1-3, 2-6, 3-1, 3-7 |
| Hook-Key Derivation → central invariant (all sites derive identical rule) | Tasks 1-1, 1-2, 2-1..2-5, 3-4 |
| Hook-Key Derivation → Decoupling from `tmux.PaneTarget` | Tasks 1-1, 1-5 |
| Hook-Key Derivation → Deliverable — retire four stale doc-comments | Task 1-5 |
| Hook-Key Derivation → new primitives `HookKeyFormat` / `HookKey` | Tasks 1-2, 1-1 |
| Stage 1 — Registration (`cmd/hooks.go`) + Failure contract | Tasks 2-1, 2-2 |
| Stage 2 — Stale cleanup live keys (`cmd/run_hook_stale_cleanup.go`, `cmd/clean.go`) | Tasks 2-3, 2-4 |
| Stage 3 — Restore lookup baking (`internal/restore/session.go`) | Task 3-4 |
| Stage 4 — Hydrate lookup (No change) + Post-restore consistency | No-change constraint enforced by Task 3-4 (ordering-trap guard); post-restore consistency proven by Task 3-6 |
| Cross-Reboot Persistence → intro (why persist) | Task 3-1 |
| Cross-Reboot Persistence → 1. Schema (`PortalID string` field) | Task 3-1 |
| Cross-Reboot Persistence → 2. Capture (append column, `captureFieldCount` 10→11) | Task 3-2 |
| Cross-Reboot Persistence → 3. Restore re-stamp (incl. concerns (a)(b)(c)) | Task 3-3 |
| Cross-Reboot Persistence → Constant (single shared) | Tasks 1-3, 3-3 |
| Cross-Reboot Persistence → Firing does not depend on re-stamp (ordering trap) | Task 3-4 |
| Scope & Non-Goals → both triggers fixed at root (external + in-TUI) | Task 3-5 |
| Scope & Non-Goals → external start-hook unchanged | Task 2-2 (spec ref AC6) |
| Scope & Non-Goals → out-of-scope subsystems (`@portal-skeleton-*`, `sessions.json` merge, `@portal-active-*`) | Correctly NO task (non-goals); `StructuralKeyFormat`/`ListAllPanes` preserved by Tasks 1-5, 2-4 |
| Scope & Non-Goals → not retrofitting legacy sessions | Reflected in Tasks 2-6, 3-1, 3-7 |
| Testing Requirements → Derivation primitives (unit) | Tasks 1-1, 1-2, 2-5 |
| Testing Requirements → Cross-site consistency (reg == cleanup == restore-baker) | Live half: Task 2-5; restore-baker leg exercised functionally by Tasks 3-4/3-5/3-6 (survival + firing require byte-identity) |
| Testing Requirements → Creation & persistence (component) | Tasks 1-3, 1-4, 3-1, 3-2, 3-3 |
| Testing Requirements → The rename gap (integration, both triggers) | Task 3-5 |
| Testing Requirements → Durable across repeated reboots | Task 3-6 |
| Testing Requirements → Post-restore cleanup keeps the restored hook | Task 3-6 |
| Testing Requirements → Legacy / no-regression (integration) | Tasks 2-6, 3-7 |
| Testing Requirements → Multi-pane (integration) | Task 3-7 |
| Testing Requirements → Conventions (no `t.Parallel()`, `IsolateStateForTest`, `tmuxtest`/`restoretest`/`portalbintest`) | Applied across all test-bearing tasks (1-1..3-7) |
| Acceptance Criteria 1 (Stamping) | Tasks 1-3, 1-4 |
| Acceptance Criteria 2 (Rename survives reboot) | Task 3-5 |
| Acceptance Criteria 3 (Durable across repeated reboots) | Tasks 3-3, 3-6 |
| Acceptance Criteria 4 (Cleanup safety) | Tasks 2-4, 3-6 |
| Acceptance Criteria 5 (No-migration upgrade) | Tasks 2-6, 3-1 |
| Acceptance Criteria 6 (No external/UI change) | Tasks 2-2, 3-5 |
| Acceptance Criteria 7 (Multi-pane) | Task 3-7 |
| Acceptance Criteria 8 (Graceful legacy) | Tasks 1-1, 3-7 |
| Risks → Missed key-producing site (primary) | Mitigation = single shared primitive (Tasks 1-1/1-2) + cross-site consistency test (Task 2-5) |
| Risks → Best-effort stamping | Tasks 1-3, 1-4 (swallowed failures, non-fatal) |
| Risks → Token collision | Task 1-3 (DECISION block accepts spec's fire-and-forget residual) |
| Risks → Release type (regular release, not hotfix) | Delivery note, not buildable scope — correctly no task |

## Direction 2: Plan → Spec (fidelity / anti-hallucination)

Every task's Problem, Solution, Do, Acceptance Criteria, Tests, and Edge Cases
trace to a specific spec section (each task carries a Spec Reference and the
Context blocks quote the source). Judgment-call additions were checked and are
all spec-delegated mechanisms, not invention:

- **Task 1-3 token-width DECISION** (reuse `sc.gen` at 6-char, no widening) — the
  spec explicitly delegates token width as "an implementation detail (the
  existing `NewNanoIDGenerator` scheme, widened if warranted)". Verified
  `internal/session/naming.go` uses `crypto/rand` (matches spec's "`crypto/rand`
  nanoid"). Traceable.
- **New methods `tmux.ResolveHookKey` (2-1) and `tmux.ListAllPaneHookKeys` (2-3)** —
  the spec names `ResolveHookKey` as the Stage 1 read and requires the Stage 2
  enumeration to switch to `HookKeyFormat` while keeping name-based
  `ListAllPanes` available; a separate method is the mechanism that satisfies
  both. Traceable.
- **`AllPaneLister` interface-method rename (2-4)** — mechanism to enforce the
  Stage 2 switch at the type level; the algorithm/guard/swallow policy are held
  unchanged per spec. Traceable.
- **"No change" handling** — Stage 4 hydrate and the in-TUI `renameAndRefresh`
  are correctly rendered as constraints/guards (Task 3-4 ordering trap; Task 3-5
  scope guard) rather than build tasks, matching the spec's "no change required".

No plan content lacks a spec basis. No acceptance criterion tests anything the
spec does not require. No invented edge cases.

## Notes

- Cross-site consistency (spec Testing Requirements) is a three-way byte-identity
  requirement (registration read == cleanup enumeration == restore baker). Task
  2-5 explicitly covers the LIVE half and defers the restore-baker leg to Phase
  3. The restore-baker leg is not given a dedicated "byte-identity" assertion,
  but it is exercised functionally: Task 3-5 firing requires the baked key to
  match the registered id-key, and Task 3-6's cleanup-survival assertion requires
  the restore-baked/registered key to be byte-identical to the live
  `ListAllPaneHookKeys` enumeration (a mismatch would delete the hook). Coverage
  is adequate; recorded here as an observation, not a finding.
