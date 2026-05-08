TASK: session-scrollback-preview-2-1 — Define TmuxEnumerator and ScrollbackReader seam interfaces in internal/tui

ACCEPTANCE CRITERIA:
- TmuxEnumerator and ScrollbackReader exported in internal/tui.
- TmuxEnumerator.ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error) matches Phase 1 client signature exactly.
- ScrollbackReader.Tail(paneKey string) ([]byte, error) has no stateDir parameter.
- Doc comment on Tail enumerates the three return shapes verbatim.
- Package compiles.
- No package-level mutable seam variable for preview.

STATUS: Complete

SPEC CONTEXT:
Spec § Architecture Summary > Test seams defines the two interfaces and the verbatim three-shape (bytes, err) contract on Tail. Spec § Cross-cutting Seams > State Package API Reuse > stateDir resolution mandates stateDir is captured once at TUI startup and hidden behind ScrollbackReader. Spec § Wiring shape mandates constructor injection over a package-level mutable previewDeps.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/preview_seams.go (lines 1–42)
- Notes:
  - TmuxEnumerator (13–15) — single method ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error); reuses tmux.WindowGroup from Phase 1.
  - ScrollbackReader (39–41) — single method Tail(paneKey string) ([]byte, error), no stateDir.
  - Doc comment (17–38) documents stateDir hiding and references the spec section; lines 29–34 enumerate the three shapes verbatim including the placeholder string "(no saved content)".
  - File contains only declarations + one import — no previewDeps, no adapter wiring (correctly deferred to 2-7).

TESTS:
- Status: Adequate
- Location: internal/tui/preview_seams_test.go (lines 1–83)
- Coverage:
  - TestTmuxEnumeratorIsSatisfiedByTmuxClient — compile-time assertion var _ tui.TmuxEnumerator = (*tmux.Client)(nil).
  - TestScrollbackReaderHidesStateDir — stubScrollbackReader with Tail(paneKey string) only; load-bearing — adding stateDir param breaks compilation.
  - TestScrollbackReaderSupportsThreeReturnShapes — table-driven across (bytes, nil), (nil, nil), (nil, err) shapes.
- Notes: Black-box tui_test package; no tmuxtest; not over-tested; not under-tested.

CODE QUALITY:
- Project conventions: Followed (small DI interfaces per CLAUDE.md; no package-level mutable state in internal/tui).
- SOLID: Good (Interface Segregation, Dependency Inversion both clean).
- Complexity: Low (declaration-only).
- Modern idioms: Yes (reuses Phase 1 tmux.WindowGroup, no local redefinition).
- Readability: Good (doc comments explain the why and reference spec).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Three-shape contract is documented on the seam and presumably also on Phase 1's tail-N helper. Consider a single canonical reference once 2-7 lands to prevent silent drift.
- [idea] Task 2-7 calls for a non-test-file compile-time assertion var _ TmuxEnumerator = (*tmux.Client)(nil); current placement in test file satisfies 2-1 but 2-7 should add the production-side mirror.
