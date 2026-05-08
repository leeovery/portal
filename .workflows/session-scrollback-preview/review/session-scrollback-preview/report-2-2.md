TASK: session-scrollback-preview-2-2 — previewModel constructor with injected seams and initial-open flow

ACCEPTANCE CRITERIA:
- NewPreviewModel constructor-injected — no package-level seam variable.
- On enumeration error, ok=false and no Tail/SetContent calls.
- On empty enumeration (zero groups), ok=false.
- On group with zero panes, ok=false.
- On success, windowIdx == 0 and paneIdx == 0.
- reader.Tail called exactly once with state.SanitizePaneKey(session, groups[0].WindowIndex, groups[0].PaneIndices[0]).
- (nil, err) and (nil, nil) → ok=true.
- (bytes, nil) → exact bytes to viewport.SetContent.
- viewport.AtBottom() true post-construction (explicit GotoBottom).
- Re-invoke triggers fresh enumeration AND fresh Tail (no caching).

STATUS: Complete

SPEC CONTEXT:
Spec § Refresh Semantics > Initial-open ordering (5-step open). Spec § Multi-pane Rendering Shape > Model lifecycle (fresh per Space, no caching). Spec § Cross-cutting Seams > State Package API Reuse (state.SanitizePaneKey byte-identical with daemon writer). Spec § Architecture Summary > Wiring shape (constructor injection).

IMPLEMENTATION:
- Status: Implemented (with forward-integration of Phase 3/4 behaviour already merged)
- Location:
  - internal/tui/pagepreview.go:73-104 — NewPreviewModel
  - internal/tui/pagepreview.go:202-215 — readFocusedPaneIntoViewport dispatcher
  - internal/tui/pagepreview.go:128-131 — currentPaneKey via state.SanitizePaneKey
- Notes:
  - Constructor matches spec ordering: enumerate → err returns (zero, false) → empty groups OR empty first PaneIndices returns (zero, false) → struct literal sets focus to (0,0) → viewport.New → dispatcher reads (0,0) and anchors at bottom.
  - Pane key derivation byte-identical with daemon writer (currentPaneKey → state.SanitizePaneKey(session, raw WindowIndex, raw PaneIndices[paneIdx])).
  - Forward-integration: dispatcher already encodes Phase 4 placeholder ("(no saved content)") and error ("(unable to read scrollback)") wording. The (bytes, nil) verbatim path remains intact.
  - Forward-integration: viewport height reduced by previewChromeHeight=1 — Phase 3 chrome already merged.
  - Constructor-injected, no package-level seam variable.

TESTS:
- Status: Adequate
- Location: internal/tui/pagepreview_test.go (10 tests, lines 40-232)
- Coverage:
  - ReturnsFalseWhenEnumerationErrors (40-52) — ok=false AND Tail count == 0.
  - ReturnsFalseOnEmptyEnumeration (54-66) — ok=false AND Tail count == 0.
  - ReturnsFalseWhenFirstWindowHasZeroPanes (68-84) — ok=false AND Tail count == 0.
  - SetsFocusToZeroZeroOnSuccess (86-106).
  - ReadsTailForZeroZeroPaneSynchronously (108-128) — uses WindowIndex=2, pane=5 so SanitizePaneKey assertion verifies raw indices flow rather than slice ordinals.
  - PassesRawANSIBytesVerbatimToSetContent (130-148).
  - PositionsViewportAtScrollTailOnInitialOpen (150-172) — 50 lines into 24-row viewport; AtBottom() only true if GotoBottom() ran.
  - ReturnsTrueWhenTailReturnsNilNil (174-187).
  - ReturnsTrueWhenTailReturnsNilError (189-202).
  - ConstructsFreshModelPerCallWithNoCarriedState (204-232) — mutates reader.bytes between calls; enum.calls==2 AND reader.calls==2.
- Notes: No tmuxtest import. Stub types minimal. No t.Parallel().

CODE QUALITY:
- Project conventions: Followed. Small interfaces, constructor injection, no package-level mutable state for preview.
- SOLID: Good. Single responsibility. Interface segregation respected.
- Complexity: Low.
- Modern idioms: max() builtin, named-field struct literal, type-switch on tea.Msg.
- Readability: Strong. Doc comments cite spec sections inline.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Forward-integration of Phase 4 placeholder/error wording in readFocusedPaneIntoViewport makes the 2-2 task wording slightly out-of-sync with the code. Consider a code comment cross-referencing 4-2/4-3.
- [idea] TestNewPreviewModel_ReturnsTrueWhenTailReturnsNilNil and ...NilError do not assert the rendered viewport content. Phase 4 tests likely cover it.
- [idea] TestNewPreviewModel_PassesRawANSIBytesVerbatimToSetContent uses strings.Contains on the full View() (chrome included). Byte-equality on viewport content alone would pin the verbatim invariant more strictly.
- [quickfix] previewModel doc notes "methods must not be called on a zero previewModel". A debug-only panic in currentGroup()/currentPaneKey() would make misuse loud.
