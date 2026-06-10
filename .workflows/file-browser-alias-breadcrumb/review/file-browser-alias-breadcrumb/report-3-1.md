TASK: 3-1 — Delete the internal/ui and internal/browser packages (+ remove stale surface-audit allow-list keys)

ACCEPTANCE CRITERIA:
- internal/ui/ no longer exists on disk.
- internal/browser/ no longer exists on disk.
- No compiled Go file imports github.com/leeovery/portal/internal/ui or .../internal/browser.
- Stale surface-audit allow-list keys "browser"/"ui" removed from pagepreview_surface_audit_test.go; preExistingPackages has neither; TestSurfaceAudit_NoNewPackageForPreview stays coherent.

STATUS: Complete

SPEC CONTEXT: Removal Manifest lists both dirs for whole-directory rm -rf, deleted LAST per sequencing (consumers first in Phase 2). The bare "ui"/"browser" allow-list keys escape path-prefixed greps and the build/test gate (audit only errors on an on-disk dir absent from the list, not on a stale extra key).

IMPLEMENTATION:
- Status: Implemented
- Glob internal/ui/** and internal/browser/** both return nothing (dirs absent).
- pagepreview_surface_audit_test.go:292-322 preExistingPackages map has neither "ui" nor "browser".
- Grep for github.com/leeovery/portal/internal/(ui|browser) across tree returns only `.tick`/`.workflows` artifact hits — zero in any `.go`. Qualified-symbol grep `\b(ui|browser)\.[A-Z]` across `*.go`: no matches. Bare "ui":/"browser": map-key grep across `*.go`: no matches.

TESTS:
- Status: Adequate (removal task — deleted packages took their own tests). TestSurfaceAudit_NoNewPackageForPreview (pagepreview_surface_audit_test.go:281-360) iterates on-disk internal/ entries and errors only when a present dir is ABSENT from the allow-list; a key with no on-disk dir is never consulted, so removing the deleted dirs' keys keeps it green and removes the dangling references. No over/under-testing.

CODE QUALITY:
- N/A (deletion + map-key removal). Allow-list now contains only live packages; no dangling keys.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
