TASK: 1-3 — Re-confirm manifest line numbers against HEAD and produce the corrected edit set

ACCEPTANCE CRITERIA:
- Every manifest edit site has a corrected-edit-set entry with HEAD-confirmed line number.
- Line drift detected/corrected.
- TestCommandPendingBrowseAndNKey -> TestCommandPendingNKey rename recorded as mandatory end-state with both browser subtests marked for deletion.
- Surface-audit "browser"/"ui" keys confirmed + note build/test gate misses a stale key.
- Any new-reconciled item from 1-2 included.

STATUS: Complete

SPEC CONTEXT: Analysis task producing a HEAD-accurate corrected edit set consumed by Phases 2-3. Verified by END STATE.

IMPLEMENTATION:
- Status: Implemented (verified by end state)
- Special case 1 (rename) CONFIRMED: TestCommandPendingNKey exists (model_test.go:6171); no TestCommandPendingBrowseAndNKey; survivor is genuinely n-key-only (both subtests are n-key cases).
- Special case 2 (allow-list) CONFIRMED: preExistingPackages map (pagepreview_surface_audit_test.go:292-322) has no "browser"/"ui" key; alphabetised "tui" sits where "ui" would precede it — confirms the bare "ui" key removed.
- End-state completeness: both packages deleted; core tokens + wider symbol set + import-path tokens absent from Go code; README/CLAUDE clean.
- Scope-boundary survivors intact (cmd/open.go cwd field/WithCWD/cwd: cwd, aliasEditor wiring).

TESTS:
- Status: Adequate for analysis task. Correctness enforced by surviving surface-audit test + renamed TestCommandPendingNKey.

CODE QUALITY: N/A (no code).

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. Markdown hits for file-browser tokens all under `.workflows/` — correctly dispositioned as non-code false positives, out of scope.
