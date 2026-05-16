TASK: enter-attaches-from-preview-1-8 — Update preview chromeLine to include `enter attach` token between `tab next pane` and `esc back`

ACCEPTANCE CRITERIA:
- chromeLine() returns string containing `· enter attach · esc back` with token between `tab next pane` and `esc back`
- Token in exact spec position
- Chrome byte-identical across viewport content states
- Sessions-page help bar unaffected
- Existing chrome tests updated and pass

STATUS: Complete

SPEC CONTEXT: Spec § Discoverability pins exact format `... · tab next pane · enter attach · esc back`. Wording is unconditional across viewport content states. Sessions-page help bar must not gain the token.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/pagepreview.go lines 165-175 (`chromeLine` method); format string at line 172 matches spec exactly
- Pure function — unconditional wording by construction (no branching on viewport state)

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/pagepreview_chrome_test.go: TestPreviewChromeLine_IncludesBracketAndTabAndEscAsVisibleHints (extended with `enter`), TestPreviewChromeLine_IncludesEnterAttachTokenBetweenTabAndEsc (ordering), TestPreviewChromeLine_FullStringEqualityForCanonicalShape (full byte equality)
  - internal/tui/pagepreview_chrome_enterattach_test.go: TestPreviewChromeLine_EnterAttachTokenByteIdenticalAcrossViewportStates (4 reader shapes), TestSessionsPageView_DoesNotContainPreviewChromeEnterAttachToken (Sessions-page regression)
- Multi-state byte-identity test (4 reader cases) justified because spec mandates byte-identity. Not over-tested.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — minimal format string change.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
