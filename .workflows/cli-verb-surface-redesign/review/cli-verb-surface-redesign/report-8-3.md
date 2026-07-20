TASK: cli-verb-surface-redesign-8-3 — Refresh stale post-redesign documentation and comments

ACCEPTANCE CRITERIA (from plan row 8-3, Phase 8 Analysis Cycle 2):
- Comment/doc-only change
- process_role.go `roleTUI` mapping and `process_role_test.go` unchanged (comment-only in that file)
- bare `portal` described as prints help/usage, not TUI picker
- CLAUDE.md "Incident of record #2" period-marked or re-anchored so removed `state cleanup` / deleted `TestStateUserFacingSubcommandsExitZero` don't read as current
- underlying lesson preserved
- no new code test, existing `process_role` tests stay green

STATUS: Complete

SPEC CONTEXT:
This is a Phase-8 analysis-cycle-2 remediation chore (source: standards/tasks findings in
.workflows/.../implementation/.../analysis-tasks-c2.md and analysis-standards-c2.md). Two doc
artefacts still asserted removed CLI surfaces as current: (1) process_role.go called bare `portal`
"the TUI picker" — but the redesign made bare `portal` print help/usage (Phase 6 task 6-5: rootCmd
has no Run/RunE, cobra returns ErrHelp before PersistentPreRunE); (2) CLAUDE.md's "Incident of
record #2" cited `portal state cleanup` (subsumed by `portal uninstall` in Phase 4) and
`TestStateUserFacingSubcommandsExitZero` (since removed) as current examples. The explicit
constraint is to fix ONLY the comments/prose — leave the `roleTUI` mapping and its pinned test case
untouched, since reclassifying a closed, forensically-inert process-role taxonomy is out of scope.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/log/process_role.go:44-47 — routing-table comment changed from `open … / x … / bare -> tui`
    to split `open … / x … -> tui` plus a `bare (no subcommand) -> tui` line annotated "retained for
    taxonomy continuity; bare `portal` now PRINTS HELP, it does not launch the picker".
  - internal/log/process_role.go:69-77 — the len(path)==0 arm comment changed from "Bare `portal` … is
    the TUI picker." to "PRINTS HELP/USAGE — it is the control-plane root, not the picker. Post-redesign
    rootCmd has no Run/RunE, so cobra returns ErrHelp before PersistentPreRunE and no TUI is launched.
    roleTUI is retained here purely for continuity…".
  - CLAUDE.md:114 — "Incident of record #2" re-anchored AND period-marked: now reads "(pre-redesign
    history — `portal state cleanup` was since folded into `portal uninstall` and one of the two tests,
    `TestStateUserFacingSubcommandsExitZero`, removed; the surviving
    `TestVersionGuard_NotInvokedForExemptCommands` now Executes the `uninstall` body with its `*Deps`
    injected)".
- Notes: The `roleTUI` return for bare `portal` (process_role.go:76) and the routing code are
  byte-for-byte unchanged — only comments were edited. Commit 8be9d1f0 stat confirms the code diff is
  CLAUDE.md + process_role.go comments only (plus workflow bookkeeping files .tick/tasks.jsonl and
  manifest.json). process_role_test.go was NOT touched at all.
  Every factual claim added to the docs was verified against the tree:
    * TestVersionGuard_NotInvokedForExemptCommands exists (cmd/version_guard_test.go:129) and Executes
      the uninstall body (line 137) with UninstallDeps injected (lines 167-170) — the re-anchor is accurate.
    * TestStateUserFacingSubcommandsExitZero is absent from the code tree (only survives in
      .workflows/.tick metadata and CLAUDE.md's own historical note) — "removed" is accurate.
    * rootCmd (cmd/root.go) has no Run:/RunE:, only PersistentPreRunE — "prints help, no picker" is accurate.

TESTS:
- Status: Adequate (no new test required — comment/doc-only change, per acceptance)
- Coverage: The pinned process_role behaviour remains covered by the existing, unchanged
  process_role_test.go — `{"bare portal", []string{}, "tui"}` (line 32), the DriftTripwire, and the
  ClosedResultSpace test. Because the len(path)==0 arm still returns roleTUI unchanged, these stay green.
- Notes: No test was added or modified, matching the acceptance criterion "no new code test, existing
  `process_role` tests stay green". Tests not executed (read-only review); green status inferred from the
  code path being byte-identical.

CODE QUALITY:
- Project conventions: Followed. Comment-only doc refresh consistent with the codebase's heavy
  self-documenting comment style; the deliberate "retain the closed taxonomy value, fix only the prose"
  posture matches the log-taxonomy-is-amendment-governed convention documented in CLAUDE.md.
- SOLID principles: N/A (no logic change)
- Complexity: N/A (no logic change)
- Modern idioms: N/A
- Readability: Good. The new comments are precise and explain WHY the mapping is retained despite the
  behaviour change (avoids a future reader "fixing" the mapping and churning the taxonomy). The CLAUDE.md
  note is both period-marked and re-anchored, and the still-valid lesson (Execute a real command body =>
  inject every tmux-touching *Deps) is fully preserved verbatim.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. The change is a clean, factually-accurate comment/doc refresh that satisfies every acceptance
  criterion; all pure observations considered proposed no concrete further action and were dropped.
