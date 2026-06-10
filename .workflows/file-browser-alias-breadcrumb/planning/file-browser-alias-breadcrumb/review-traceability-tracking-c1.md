---
status: complete
created: 2026-06-10
cycle: 1
phase: Traceability Review
topic: file-browser-alias-breadcrumb
---

# Review Tracking: file-browser-alias-breadcrumb - Traceability

## Outcome

**CLEAN.** No findings. The plan is a faithful, complete bidirectional translation of the
specification. Every Removal Manifest edit site is covered by a task; every scope-boundary
"must stay unchanged" item is protected; both blocking manual checks and the zero-references
gate are represented; and nothing in the plan is invented beyond the spec.

## Direction 1 — Specification → Plan (completeness)

Every spec element has plan coverage:

| Spec element | Plan coverage |
|---|---|
| Re-sweep at implementation start — all mandated tokens incl. bare quoted `"ui"`/`"browser"` | Task 1-1 (exact token list + bare-quoted greps reproduced) |
| Reconcile any site not in manifest (esp. new doc/comment refs) | Task 1-2 |
| Scope-boundary survivors recorded as do-not-touch | Task 1-2 (`scope-boundary-keep` list) |
| Re-confirm manifest line numbers against HEAD | Task 1-3 |
| `TestCommandPendingBrowseAndNKey` → `TestCommandPendingNKey` rename (decided end-state) | Task 1-3 (recorded) + Task 2-1 (applied) |
| Surface-audit allow-list `"browser":`/`"ui":` keys (gate-invisible) | Task 1-3 (confirmed) + Task 2-1 (removed) |
| Green build/test baseline as the gate's measuring stick | Task 1-4 |
| Sequencing: consumers before packages | Phase 2 before Phase 3; "Why this order" in both phases |
| All 14 `internal/tui/model.go` edit bullets (incl. the L728-729 `commandPendingHelpKeys` comment, distinct from the L732 binding) | Task 2-1 |
| All 5 `cmd/open.go` edit bullets (KEEP `WithCWD`/`cwd: cwd`) | Task 2-2 |
| All `internal/tui/model_test.go` edits — whole-function deletes, subtest deletes, rename, both reworks | Task 2-1 |
| All `cmd/open_test.go` edits | Task 2-2 |
| All four `pagepreview_*_test.go` edits (entry delete + comment, refetch comment, bracket comment, surface-audit keys) | Task 2-1 |
| Packages deleted entirely (`rm -rf` whole dir incl. any newly-added file) | Task 3-1 |
| Docs: README four→three views; CLAUDE `tui` row + `browser` row + `ui` row | Task 3-2 |
| Iota-safety + dangling-reference verification | Task 1-3 (recorded) + Task 2-1 (verified) |
| Acceptance gate: green build/test, zero-references spot-check, directory-absence | Task 3-3 |
| Blocking manual check 1 — Projects-page `b` is a visible no-op opening no view | Task 3-3 (and embedded as AC in Task 2-1) |
| Blocking manual check 2 — Sessions/Projects/Preview + alias CLI + projects-modal alias editor unchanged | Task 3-3 |
| No new tests (net delta is removal) | Stated in Tasks 3-1, 3-2, 3-3 |

## Direction 2 — Plan → Specification (fidelity / anti-hallucination)

Every plan element traces to the spec:

- **No invented edit sites.** Every site in Tasks 2-1/2-2/3-1/3-2 maps to a Removal Manifest
  bullet (verified by symbol cross-check against the manifest and against HEAD source).
- **No invented behaviour or acceptance criteria.** All acceptance criteria verify spec
  requirements (green gate, zero references, directory absence, the two blocking manual checks).
- **Out-of-scope UX edge correctly excluded.** The spec's "exact-match alias miss silently
  degrades to fuzzy zoxide" note — which the spec explicitly forbids becoming an acceptance
  criterion — does not appear in any task or criterion.
- **Rejected alternatives not present as work.** Alternative A (`SetAndSave` rewiring / in-place
  audit-fix) and Alternative B (finish the feature) appear nowhere as scope; `SetAndSave` is
  referenced only as the surviving projects-modal alias-editor path to protect.
- **Operational additions trace to codebase convention, not invention.** Two non-spec mechanics
  are present but legitimately grounded: (1) the known-flaky `internal/tmux` kill-barrier
  isolated-re-run handling (Tasks 1-4, 2-1, 2-2, 3-3) derives from CLAUDE.md + project memory
  `reference_flaky_killbarrier_test.md` and exists only to make the spec's literal "green
  `go test ./...`" gate trustworthy; (2) the `portaltest.IsolateStateForTest` note (Tasks 2-1,
  2-2) is the CLAUDE.md-mandated requirement for any test run that exercises daemon-spawning
  paths. Both are how-to-run-the-gate context applying codebase conventions, not new product
  scope, and neither adds behaviour or a new test (the spec's "no new tests" rule holds).
- **Wider symbol sweep is spec-sourced.** The extended symbol list Task 1-1 sweeps and Task 3-3
  spot-checks (GitRootResolver, PathChecker, FileBrowserModel, the `Browser*` messages,
  AliasSaver, etc.) is the verbatim non-exhaustive grep-target list from the spec's Acceptance
  gate, not an invented set.

## Findings

None.

**Resolution**: Complete — clean.
