---
status: complete
created: 2026-07-01
cycle: 3
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

This cycle specifically re-verified that the cycle-2 fix did NOT introduce
spec-coverage gaps or hallucinated content:

**Cycle-2 fix (Task 3-5 in-TUI seam unit test):** the in-TUI seam unit test was
repointed to reuse the existing `tui_test` `TestRenameSession`
"enter in rename modal renames session and refreshes" subtest (instead of a
duplicate `package tui` internal test + forked mock), and its
"zero hook mutations" assertion was reframed as the structural fact that the
`tui` model wires no hook seam.

Verification (both directions):

1. **The reused test exists and matches the plan's description exactly.**
   `internal/tui/model_test.go` (package `tui_test`) `TestRenameSession`'s
   `"enter in rename modal renames session and refreshes"` subtest
   (lines 1659-1700) builds a model via `newModelWithRenamer` (which wraps the
   exported `tui.New` + `WithRenamer(mockSessionRenamer)`), presses `r` → clears
   text → types `new-alpha` → presses `Enter` (routing through
   `updateRenameModal` → `renameAndRefresh` at `model.go:3206`/`3220`), executes
   the returned `tea.Cmd`, asserts it yields `tui.SessionsMsg` (the list
   refresh), and asserts `renamer.renamedOld == "alpha"` /
   `renamer.renamedNew == "new-alpha"` — i.e. exactly one `RenameSession(old,new)`
   from the genuine in-TUI code path. The plan's Task 3-5 description (Do bullet
   at line 223; Acceptance Criterion at line 232; Tests entry at line 242) is a
   faithful account of this real test. The reused driver and the existing
   `mockSessionRenamer` (lines 1608-1619) are real and require no duplicate.

2. **The reframed "no hook seam" assertion is structurally true.** The plan
   claims the `tui` model "wires no hook seam at all" so `renameAndRefresh`
   "cannot and does not re-key hooks". Confirmed against live source: `tui`
   production code (non-test) imports no `internal/hooks`, holds no
   `hooks.Store` / `LookupOnResume` / on-resume seam, and contains no
   re-keying logic. The only "hook"-string occurrences in `tui` production code
   are bootstrap **loading-progress labels** (`loading_progress.go`
   `LabelRegisteredHooks` for the `RegisterPortalHooks` bootstrap step, and the
   "Running resume commands" label) — display strings for the cold-boot flip,
   not hook wiring. `renameAndRefresh` (`model.go:3220`) reduces to
   `m.sessionRenamer.RenameSession(oldName, newName)` + a `ListSessions` refresh
   (lines 3222-3226) with zero hook interaction.

3. **Spec fidelity of the reframing.** The reframed assertion traces to spec §
   Scope & Non-Goals: "Portal's in-TUI rename (`renameAndRefresh`) — **no change
   required**. It continues to do a bare `RenameSession` + list refresh with
   **zero** hook re-keying." Asserting the structural absence of a hook seam is a
   faithful — and stronger, more truthful — rendering of that spec property than
   the prior "zero hook mutations" wording: the fix relies on the in-TUI path
   having nothing to re-key, and the test now proves exactly that. This is a
   coverage-relocation of an existing spec requirement to the package that owns
   the code, not a new requirement, and it invents nothing.

4. **"Both triggers" coverage is intact.** Spec § Testing Requirements → the
   rename gap requires covering **both** raw `tmux rename-session` *and* the
   in-TUI rename path. After the cycle-2 fix, Task 3-5 still covers all three
   legs: (a) the external `tmux rename-session` integration leg (fires the hook
   after reboot); (b) the `RenameSession`-equivalent integration leg (the exact
   `client.RenameSession(old,new)` call the in-TUI path issues, proving
   reboot-survival through the shared call); and (c) the reused `tui_test`
   subtest actually executing the in-TUI code path and proving it reduces to
   that shared `RenameSession` with no hook re-keying. No leg was dropped.

## Findings

None.

## Coverage Matrix (Direction 1: Spec -> Plan)

| Spec section / element | Plan coverage |
|---|---|
| Problem Statement (silent+delayed orphan; both triggers; bites only when inner process does not restart) | Tasks 3-5 (both triggers, pane kept running), 3-7 |
| Root Cause (four stages desync on rename; mutable name anchor) | Whole plan; explicitly Stages 1-4 -> Tasks 2-1/2-2 (S1), 2-3/2-4 (S2), 3-4 (S3), 3-4 ordering-guard (S4) |
| Fix Overview -> The option (`session.PortalIDOption = "@portal-id"`) | Task 1-3 (constant) |
| Fix Overview -> Its value / Generation contract (fresh opaque `crypto/rand` token, fire-and-forget, no uniqueness check, `sc.gen`) | Tasks 1-3, 1-4 |
| Fix Overview -> It is immutable (never rewritten) | Negative property - no task mints a new id post-creation; restore re-stamps the *saved* value only (Task 3-3) |
| Fix Overview -> Where it is stamped -> `CreateFromDir` | Task 1-3 |
| Fix Overview -> Where it is stamped -> `QuickStart.Run` (argv-chain stamp before attach; no error seam) | Task 1-4 |
| Fix Overview -> Hook key = prefer `@portal-id`, else session name | Tasks 1-1, 1-2 |
| Fix Overview -> Coverage / natural migration (no backfill, no `hooks.json` re-key migration) | Reflected in Tasks 1-3, 2-6, 3-1, 3-7 |
| Hook-Key Derivation -> central invariant (all sites derive identical rule; three producers + one consumer) | Tasks 1-1, 1-2, 2-1..2-5, 3-4 |
| Hook-Key Derivation -> Decoupling from `tmux.PaneTarget` (PaneTarget stays as-is) | Tasks 1-1, 1-5 (verified: `collectArmInfos` still uses `PaneTarget` for live targeting at `session.go:222`) |
| Hook-Key Derivation -> Deliverable - retire four stale doc-comments + transfer invariant | Task 1-5 (all four functions named; invariant transfer to HookKey/HookKeyFormat) |
| Hook-Key Derivation -> new primitives `HookKeyFormat` / `HookKey` | Tasks 1-2, 1-1 |
| Stage 1 - Registration (`cmd/hooks.go`) + Failure contract (abort, never synthesize name key) | Tasks 2-1, 2-2 |
| Stage 2 - Stale cleanup live keys (`cmd/run_hook_stale_cleanup.go`, `cmd/clean.go`) | Tasks 2-3, 2-4 |
| Stage 3 - Restore lookup baking (`internal/restore/session.go`, saved indices) | Task 3-4 |
| Stage 4 - Hydrate lookup (No change) + Post-restore consistency | No-change enforced by Task 3-4 (ordering-trap guard); post-restore consistency proven by Task 3-6 |
| Cross-Reboot Persistence -> intro (why persist; @portal-dir divergence) | Task 3-1 |
| Cross-Reboot Persistence -> 1. Schema (`PortalID string`, tolerant decode, no version bump) | Task 3-1 |
| Cross-Reboot Persistence -> 2. Capture (append column, `captureFieldCount` 10->11, zero-row edge) | Task 3-2 |
| Cross-Reboot Persistence -> 3. Restore re-stamp (incl. concerns (a)(b)(c); skip when empty) | Task 3-3 |
| Cross-Reboot Persistence -> Constant (single shared, importable by re-stamp site) | Tasks 1-3, 3-3 |
| Cross-Reboot Persistence -> Firing does not depend on re-stamp (ordering trap; never read live id) | Task 3-4 |
| Scope & Non-Goals -> both triggers fixed at root (external + in-TUI, no interception) | Task 3-5 (external integration leg + `RenameSession`-equivalent leg + reused `tui_test` seam test) |
| Scope & Non-Goals -> in-TUI rename no change required (bare `RenameSession` + refresh, zero hook re-keying) | Task 3-5 (cycle-2 fix: reused `TestRenameSession` subtest + "no hook seam" structural-fact assertion) - verified accurate against `internal/tui/model.go` |
| Scope & Non-Goals -> external start-hook unchanged | Task 2-2 (spec ref AC6) |
| Scope & Non-Goals -> out-of-scope subsystems (`@portal-skeleton-*`, `sessions.json` merge, non-existent `@portal-active-*`) | Correctly NO task (non-goals); `StructuralKeyFormat`/`ListAllPanes` preserved by Tasks 1-5, 2-4 |
| Scope & Non-Goals -> not retrofitting legacy sessions | Reflected in Tasks 2-6, 3-1, 3-7 |
| Testing Requirements -> Derivation primitives (unit) | Tasks 1-1, 1-2, 2-5 |
| Testing Requirements -> Cross-site consistency (reg == cleanup == restore-baker byte-identical) | Live half: Task 2-5; restore-baker leg exercised functionally by Tasks 3-4/3-5/3-6 (survival + firing require byte-identity) |
| Testing Requirements -> Creation & persistence (component) | Tasks 1-3, 1-4, 3-1, 3-2, 3-3 |
| Testing Requirements -> The rename gap (integration, both triggers) | Task 3-5 (all three legs intact after cycle-2 fix) |
| Testing Requirements -> Durable across repeated reboots | Task 3-6 |
| Testing Requirements -> Post-restore cleanup keeps the restored hook | Task 3-6 |
| Testing Requirements -> Legacy / no-regression (integration) | Tasks 2-6, 3-7 |
| Testing Requirements -> Multi-pane (integration) | Task 3-7 |
| Testing Requirements -> Conventions (no `t.Parallel()`, `IsolateStateForTest`, `tmuxtest`/`restoretest`/`portalbintest`) | Applied across all test-bearing tasks (1-1..3-7) |
| Acceptance Criteria 1 (Stamping, non-fatal) | Tasks 1-3, 1-4 |
| Acceptance Criteria 2 (Rename survives reboot, both triggers) | Task 3-5 |
| Acceptance Criteria 3 (Durable across repeated reboots) | Tasks 3-3, 3-6 |
| Acceptance Criteria 4 (Cleanup safety) | Tasks 2-4, 3-6 |
| Acceptance Criteria 5 (No-migration upgrade) | Tasks 2-6, 3-1 |
| Acceptance Criteria 6 (No external/UI change) | Tasks 2-2, 3-5 |
| Acceptance Criteria 7 (Multi-pane) | Task 3-7 |
| Acceptance Criteria 8 (Graceful legacy) | Tasks 1-1, 3-7 |
| Risks -> Missed key-producing site (primary) | Mitigation = single shared primitive (Tasks 1-1/1-2) + cross-site consistency test (Task 2-5) + ordering-trap guard (Task 3-4) |
| Risks -> Best-effort stamping | Tasks 1-3, 1-4 (swallowed failures, non-fatal) |
| Risks -> Token collision | Task 1-3 (DECISION block accepts spec's fire-and-forget residual) |
| Risks -> Release type (regular release, not hotfix) | Delivery note, not buildable scope - correctly no task |

## Direction 2: Plan -> Spec (fidelity / anti-hallucination)

Every task's Problem, Solution, Do, Acceptance Criteria, Tests, and Edge Cases
trace to a specific spec section (each task carries a Spec Reference and its
Context blocks quote the source). This cycle re-audited the cycle-2 fix plus the
previously-cleared judgment-call additions:

- **Task 3-5 - reused `tui_test` `TestRenameSession` subtest + "no hook seam"
  structural-fact assertion** (cycle-2 fix): traces to spec § Testing
  Requirements -> "Cover both triggers" and § Scope & Non-Goals -> in-TUI rename
  "continues to do a bare `RenameSession` + list refresh with **zero** hook
  re-keying". Reusing an existing exported-driver test is a coverage-relocation,
  not a new requirement; the "no hook seam" assertion is verified true against
  live source (no `internal/hooks` wiring in `tui` production code; the only
  "hook" strings are bootstrap loading-progress labels). Nothing invented.
  Traceable.
- **Task 3-5 - `RenameSession`-equivalent integration leg** (from cycle 1, still
  present): traces to spec § Scope & Non-Goals ("both triggers reduce to
  `RenameSession`") and § Testing Requirements ("cover both triggers"). The plan
  states the leg exercises `client.RenameSession(old,new)` - the exact call the
  in-TUI path issues (confirmed: `renameAndRefresh` reduces to
  `sessionRenamer.RenameSession` + refresh). Traceable.
- **Task 3-3 import-cycle rationale** (from cycle 1): grounding-only note
  supporting the spec's single-shared-constant requirement (§ Cross-Reboot
  Persistence -> Constant). No spec-coverage impact. Traceable.
- **Task 2-3 parser-level empty-slice assertion** (from cycle 1): traces to spec
  § Stage 2 discriminating-contract requirement (failure != empty live set).
  Traceable.
- **Task 1-3 token-width DECISION** (reuse `sc.gen` at 6-char, no widening): spec
  explicitly delegates token width as "an implementation detail (the existing
  `NewNanoIDGenerator` scheme, widened if warranted)". Traceable.
- **New methods `tmux.ResolveHookKey` (2-1) and `tmux.ListAllPaneHookKeys` (2-3)**:
  spec names `ResolveHookKey` as the Stage 1 read and requires the Stage 2
  enumeration to switch to `HookKeyFormat` while keeping name-based `ListAllPanes`
  available; a separate method satisfies both. Traceable.
- **`AllPaneLister` interface-method rename (2-4)**: mechanism to enforce the
  Stage 2 switch at the type level; algorithm/guard/swallow policy held unchanged
  per spec. Traceable.
- **"No change" handling**: Stage 4 hydrate and the in-TUI `renameAndRefresh` are
  correctly rendered as constraints/guards (Task 3-4 ordering trap; Task 3-5
  scope guard + relocated unit-test coverage) rather than build tasks, matching
  the spec's "no change required".

No plan content lacks a spec basis. No acceptance criterion tests anything the
spec does not require. No invented edge cases.

## Notes

- Grounding spot-checks against live source (this cycle):
  `renameAndRefresh` (`internal/tui/model.go:3220`) reduces to
  `m.sessionRenamer.RenameSession(oldName, newName)` + a `ListSessions` refresh;
  `SessionRenamer` (`model.go:80-81`) + `WithRenamer` (`model.go:669-672`) are
  real injectable seams; `mockSessionRenamer` (`model_test.go:1608`) and the
  reused `"enter in rename modal renames session and refreshes"` subtest
  (`model_test.go:1659-1700`) exist and match Task 3-5's description; `tui`
  production code imports no `internal/hooks` and wires no hook seam (only
  `loading_progress.go` display labels mention "hooks"). `captureFieldCount = 10`
  and `captureFormat` = 10 fields (confirms Task 3-2's 10->11 bump);
  `state.Session` = Name / Environment / Windows (confirms Task 3-1 adds
  `PortalID`); `collectArmInfos` bakes
  `hookKey: tmux.PaneTarget(sess.Name, w.Index, p.Index)` at `session.go:110`
  (confirms Task 3-4's swap to `tmux.HookKey`, and that `PaneTarget` stays for
  live targeting at `session.go:222`). All accurate (line numbers approximate,
  as the plan states).
- The cycle-2 fix strengthens fidelity: the "no hook seam" structural-fact
  assertion is a truer expression of the spec's "zero hook re-keying" property
  than the prior "zero hook mutations" wording, and reusing the existing
  exported-driver test means the in-TUI code path is genuinely executed rather
  than merely asserted byte-equivalent.
- Cross-site consistency (spec Testing Requirements) is a three-way byte-identity
  requirement (registration read == cleanup enumeration == restore baker). Task
  2-5 explicitly covers the LIVE half and defers the restore-baker leg to Phase
  3, where it is exercised functionally (Task 3-5 firing requires the baked key
  to match the registered id-key; Task 3-6's cleanup-survival requires the
  restore-baked/registered key to be byte-identical to the live
  `ListAllPaneHookKeys` enumeration). Coverage is adequate; recorded as an
  observation, not a finding (carried forward from cycles 1-2).
