TASK: 2-2 — Remove the file browser from the cmd package (open.go + open_test.go)

ACCEPTANCE CRITERIA:
- No cmd references to osDirLister, stubDirLister, dirLister, WithDirLister, internal/browser import (both files).
- No browser.DirEntry / browser.ListDirectories in cmd.
- Surviving tui.WithCWD(cfg.cwd), cwd: cwd production literal, test cwd field all present.
- internal/browser has zero importers.

STATUS: Complete

SPEC CONTEXT: cmd/open.go is the second/last production consumer of file-browser machinery (osDirLister adapter over browser.ListDirectories). Removed together with the field/literals because the cfg-literal entry becomes a compile error once the field is gone.

IMPLEMENTATION:
- Status: Implemented
- Grep across /cmd for osDirLister|stubDirLister|dirLister|WithDirLister|internal/browser|browser.DirEntry|browser.ListDirectories: zero matches (both files).
- Survivors present: tui.WithCWD(cfg.cwd) (open.go:360), cwd: cwd (open.go:504), test cwd field (open_test.go:765), cwd struct field (open.go:347).
- internal/browser zero importers (codebase-wide grep zero matches); directory absent at final HEAD (Phase 3 deletion — not a 2-2 defect).
- Import blocks clean; osDirLister type+method+doc-comment fully removed; tuiConfig has no dirLister field; defaultTestTUIConfig has no dirLister entry; no stubDirLister.

TESTS:
- Status: Adequate. Surviving WithCWD plumbing exercised by TestBuildTUIModel "cwd wired correctly" (open_test.go:862-871). Pure-removal task — no new tests warranted; not over-tested.

CODE QUALITY:
- Idiomatic Go; no orphaned imports; net complexity reduction; no stale browser comments.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
