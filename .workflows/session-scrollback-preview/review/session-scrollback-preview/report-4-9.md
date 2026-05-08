TASK: session-scrollback-preview-4-9 — No-new-surface audit and regression guard

ACCEPTANCE CRITERIA:
- Audit test exists and passes.
- internal/tmux/tmux.go has exactly one new method beyond pre-feature baseline; CapturePane signature unchanged.
- internal/state/ adds only the tail-N helper.
- internal/restore/ unchanged.
- cmd/bootstrap/ unchanged.
- internal/hooks/ unchanged.
- Save-format constants and .bin file shape unchanged.
- No new package added in service of preview.

STATUS: Issues Found (non-blocking gap)

SPEC CONTEXT:
Spec § Cross-cutting Seams > State Package API Reuse and § Architecture Summary > "No changes to" lock the feature's footprint. 4-9 is the durable regression guard.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview_surface_audit_test.go (7 test funcs, 351 lines)
- Notes: Audit-only task; the test file IS the deliverable. Symbol-shape matching avoids comment false-positives. _test.go files excluded from scans.

TESTS:
- Status: Under-tested by one bullet.
- Covered (7 subtests):
  - TestSurfaceAudit_TmuxNoNewCaptureWrapper — scans for CapturePaneTail / CapturePaneN / CaptureTail / CapturePaneLastN / CapturePaneRange / CapturePaneBounded.
  - TestSurfaceAudit_TmuxCapturePaneSignatureUnchanged — pins literal CapturePane signature.
  - TestSurfaceAudit_StateExposesExistingWriters — asserts SetSkeletonMarker / UnsetSkeletonMarker / WriteScrollbackIfChanged / Commit declarations remain, plus TailScrollback present.
  - TestSurfaceAudit_RestoreNoPreviewTokens, TestSurfaceAudit_BootstrapNoPreviewTokens, TestSurfaceAudit_HooksNoPreviewTokens — share auditNoPreviewTokens scanning for pagePreview, previewModel, TmuxEnumerator, ScrollbackReader.
  - TestSurfaceAudit_NoNewPackageForPreview — internal/ package allow-list with explicit forbidden names (preview, scrollback, snapshot).
- Missing: Plan's Tests list explicitly enumerates "audit: save-format constants unchanged" — pin scrollbackSubdir = "scrollback", paneKey+".bin" in ScrollbackFile, "hydrate-" / ".fifo" in FIFOPath. No dedicated subtest pins these literals. Corresponding acceptance criterion "Save-format constants and .bin file shape are unchanged" has no direct assertion.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good — each test has extensive doc comment quoting the spec section.
- Issues: TestSurfaceAudit_TmuxNoNewCaptureWrapper uses a hand-curated forbidden list — a novel name like CapturePaneSlice could slip through. Plan allows this.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Add an "audit: save-format constants unchanged" subtest pinning literals from internal/state/paths.go: scrollbackSubdir = "scrollback", paneKey+".bin" in ScrollbackFile, "hydrate-" / ".fifo" in FIFOPath. This is the one Tests-list bullet from the plan that has no corresponding subtest.
- [idea] Forbidden-symbols list in TestSurfaceAudit_TmuxNoNewCaptureWrapper is hand-curated. A regex scan for `func \(c \*Client\) CapturePane[A-Z]\w*\(` would catch any future capture-wrapper variant by shape.
- [idea] TestSurfaceAudit_NoNewPackageForPreview's preExistingPackages allow-list duplicates knowledge from CLAUDE.md's package table. A one-line comment pointing future maintainers at the canonical source when updating either side would prevent drift.
