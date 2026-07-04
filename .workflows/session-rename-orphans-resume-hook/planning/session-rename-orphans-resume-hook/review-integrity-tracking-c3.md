---
status: complete
created: 2026-07-01
cycle: 3
phase: Plan Integrity Review
topic: Session Rename Orphans Resume Hook
---

# Review Tracking: Session Rename Orphans Resume Hook - Integrity

## Result: CLEAN (convergence confirmed)

Cycle 3 confirms convergence. The cycle-2 fix is internally consistent and
introduced no new scoping or self-containment defects; the whole plan was
re-checked holistically against the live codebase and no new genuine,
actionable structural problems were found. No findings.

## Cycle-2 fix verification (consistent — no findings)

The cycle-2 fix repointed Task 3-5's in-TUI seam unit test away from a new
internal-`package tui` test and onto the existing `tui_test` `TestRenameSession`
subtest, reframing the assertion as the structural "no hook seam" fact. This was
verified against ground truth and is sound:

- **Existing subtest location and shape match the plan exactly.** `TestRenameSession`'s
  `"enter in rename modal renames session and refreshes"` subtest lives in
  `package tui_test` (`internal/tui/model_test.go`) at lines ~1659-1701 (plan says
  "~1660-1701"), and already: builds a model via the exported `tui.New` +
  `tui.WithRenamer(mockSessionRenamer)` through `newModelWithRenamer` (line 1621-1623),
  presses `r` → `Ctrl+u` → types "new-alpha" → `Enter` (routing through
  `updateRenameModal` → `renameAndRefresh`, model.go:3206), executes the returned
  `tea.Cmd`, and asserts `renamer.renamedOld == "alpha"` / `renamer.renamedNew == "new-alpha"`.
  That is precisely the "exactly one `RenameSession(old, new)` from the genuine
  in-TUI code path" the fix relies on — so the reuse-and-extend instruction is
  concrete and executable with no guesswork, and no duplicate mock/fork is needed.
- **The line references in the fixed text are accurate.** `updateRenameModal` at
  model.go:3194, the `renameAndRefresh` call at model.go:3206, `renameAndRefresh`
  definition at model.go:3220 — all match. `renameAndRefresh` (3220-3228) calls only
  `sessionRenamer.RenameSession` then `sessionLister.ListSessions`, confirming the
  "bare RenameSession + list refresh, zero hook re-keying" claim.
- **The "no hook seam" structural property is substantively true.** The TUI model
  wires no resume-hook collaborator (no hook store, seam, or re-keying dependency),
  so `renameAndRefresh` cannot and does not re-key hooks — the decisive property.
  The unit test therefore actually executes the in-TUI code path (pure-Go, no tmux,
  no `//go:build integration`), and combined with the integration `RenameSession`-
  equivalent leg the "both triggers" claim is genuinely covered.
- **Internal consistency across the three edited spots is intact.** The `Do` bullet
  (line 223), Acceptance Criterion 3 (line 232), and the `Tests` bullet (line 242)
  all now agree on `package tui_test`, the reused subtest, and the "no hook seam"
  framing — no residual reference to the old `package tui` / unexported-method /
  "zero hook mutations observable" framing survives. The integration legs
  (Trigger A external rename + the `RenameSession`-equivalent leg) are untouched and
  remain correct and self-contained.

One immaterial imprecision was noted and deliberately NOT raised as a finding: the
fix's parenthetical grounding aside "`internal/tui/model.go` references nothing
hook-related" is very slightly overstated — `grep -i hook internal/tui/model.go`
matches two lines (a bootstrap doc-comment mentioning `RegisterPortalHooks`, and the
layout term "F10 hook"), neither of which is a resume-hook collaborator. This does
not force the implementer to guess or make any design decision (the actionable
instruction — reuse the existing subtest and assert/comment on the absence of a
resume-hook seam — is correct and executable), and the substantive claim (no
resume-hook seam is wired into the model) is accurate. Under the same severity bar
as prior cycles and the proportionality / no-nitpicking guidance, a parenthetical
overstatement inside a comment-writing instruction is below the Minor threshold.

## Holistic re-check (whole plan — no new findings)

The full plan (planning.md + all three task files, 18 tasks) was re-read end-to-end
and its load-bearing grounding re-verified against the live source:

- **Phase 1 stamping/primitives.** `PortalDirOption = "@portal-dir"` (create.go:14),
  `CreateFromDir` at create.go:80 with `NewSession` (86) then
  `SetSessionOption(..., PortalDirOption, ...)` (96) — Task 1-3's "~line 96" adjacency
  is exact. QuickStart's `ExecArgs` chain (quickstart.go:72-78) is exactly
  `new-session -d ... ; set-option -t <name> @portal-dir <dir> ; attach-session -t <name>`,
  so Task 1-4's "slot the @portal-id step between @portal-dir and attach-session" is
  precise. `ResolveStructuralKey` (tmux.go:323), `StructuralKeyFormat` (tmux.go:779),
  `ListAllPanes` (tmux.go:799) all present as referenced.
- **Phase 2 live-key adoption.** `StructuralKeyResolver` interface (cmd/hooks.go:12-16),
  `HooksDeps.KeyResolver` (cmd/hooks.go:25), `resolveCurrentPaneKey` (cmd/hooks.go:47),
  `AllPaneLister` (cmd/clean.go:17-18), and `runHookStaleCleanup` (cmd/run_hook_stale_cleanup.go:78)
  with the `lister.ListAllPanes()` call at line 89 (Task 2-4's "~line 89" is exact) all
  match. The seam-rename + single-line-switch guidance is accurate and self-contained.
- **Phase 3 persistence + integration.** `captureFormat` has exactly 10 fields with
  `captureFieldCount = 10` (capture.go:35-37); `parsePaneRow` reads `parts[0]`..`parts[9]`
  (capture.go:373-382) — so Task 3-2's append-to-index-10 + bump-to-11 lockstep is
  precise. `createSkeleton` (session.go:128) does `NewSessionWithCommand` (130) then
  `applyEnvironment` (134), matching Task 3-3's placement. `collectArmInfos` (session.go:104)
  bakes `hookKey: tmux.PaneTarget(sess.Name, w.Index, p.Index)` at line 110 with the
  `savedPaneArmInfo.hookKey` doc-comment at 62-66 describing it as "raw saved structural
  identifier" — exactly what Task 3-4 changes and re-documents.

- **Dependencies / ordering.** Phase progression is sound (foundation primitives →
  live-key adoption → persistence + end-to-end proof). Cross-phase consumption edges
  are genuine and correctly directed: Phase 2 consumes `HookKeyFormat` (P1); Phase 3
  Stage-3 baking consumes both `HookKey` (P1) and the persisted `PortalID` (P3-1/3-2).
  Within each phase, sequential intra-phase tasks execute in natural (creation-order)
  order and the convergence points (e.g. 2-5 cross-site test after 2-1..2-4; the 3-5/3-6/3-7
  integration tasks after 3-1..3-4) are stated in the "Why this order" narratives. No
  circular dependency, no missing cross-phase edge, no misordered execution.
- **Template compliance / vertical slicing / acceptance quality.** All 18 tasks carry
  the full canonical template (Problem / Solution / Outcome / Do / Acceptance Criteria /
  Tests / Edge Cases / Context / Spec Reference); each is a single verifiable TDD cycle;
  acceptance criteria are concrete and pass/fail; tests include edge cases (empty id,
  wrong-arity rows, zero-row session, read/list failures, multi-pane routing, legacy
  degradation). The three cycle-1 fixes and the one cycle-2 fix remain applied and
  consistent; no regression re-introduced.

## Findings

None. The plan meets the structural-quality and implementation-readiness bar; this
cycle confirms convergence.
