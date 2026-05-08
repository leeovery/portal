TASK: session-scrollback-preview-4-8 — Side-effect-free hermetic invariant test

ACCEPTANCE CRITERIA:
- Test builds against fully-mocked seams; no real tmux, no live daemon, no real filesystem.
- TmuxEnumerator.ListWindowsAndPanesInSession called exactly once across full open + cycle + dismiss flow.
- ScrollbackReader.Tail called exactly once per focus event; no other reads.
- Static audit confirms preview has zero hooks.Store dependency.
- Static audit confirms preview has zero state-package writer references.
- Static audit confirms preview has zero FIFO creation / drain references.
- Test does NOT import tmuxtest or restoretest.

STATUS: Complete

SPEC CONTEXT:
Spec § Overview > Side-effect-free contract: "Opening and dismissing the preview leaves session state byte-identical: no hydration, no resume-hook firing, no tmux marker mutation, no FIFO consumed." Spec § Acceptance Criteria > Side-effect-free contract operationalises the assertions.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview_hermetic_test.go (293 lines)
- Five subtests:
  1. TestPreviewHermetic_FullLifecycleProducesOnlyOpenEnumerationAndPerFocusReads — drives Tab/Tab/]/Tab/Tab/[/Esc against recording mocks on a 2-window x 2-pane fixture; asserts enum.calls == 1 and reader.calls == 1+6; also asserts Esc cmd dispatches previewDismissedMsg.
  2. TestPreviewHermetic_NoHooksDependency — scans all internal/tui production .go files; asserts canonical hooks import path absent.
  3. TestPreviewHermetic_NoStatePackageWriters — scans for qualified state.<Symbol> writer references (SetSkeletonMarker, UnsetSkeletonMarker, UnsetSkeletonMarkerForFIFO, WriteScrollbackIfChanged, CaptureAndHashPane, CaptureStructure, SeedHashMap, Commit, BootstrapPortalSaver, EnsurePortalSaverVersion).
  4. TestPreviewHermetic_NoFIFOReferences — scopes to pagepreview*.go production files only (avoiding false positives from model.go restore plumbing); forbids "FIFO"/"fifo" tokens.
  5. TestPreviewHermetic_TestFilesDoNotImportTmuxtestOrRestoretest — runs over all *_test.go in working dir; forbids imports of internal/tmuxtest and internal/restoretest. Forbidden import strings constructed at runtime to avoid self-tripping.

TESTS:
- Status: Adequate
- Coverage: All six plan-listed test cases present.
- The 1+6 read budget is correct: NewPreviewModel triggers initial-open Tail at (0,0); each of 6 cycle keys lands on new focus and re-reads. Esc does not read.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good. Mocks minimal.
- Complexity: Low.
- Modern idioms: filepath.Glob + os.ReadFile.
- Readability: Good. Every subtest has a leading comment block citing the spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] TestPreviewHermetic_NoStatePackageWriters uses a hand-curated denylist. Future state-package writer added without updating the list would not be caught. An allowlist would be more durable.
- [idea] TestPreviewHermetic_NoFIFOReferences scopes to pagepreview*.go. If future preview code is factored to a different filename, the audit would silently drop it.
- [quickfix] hermeticEnumerator.lastArg is recorded but never asserted. Either drop the field or add an assertion.
