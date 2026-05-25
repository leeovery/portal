TASK: T12-1 — Reconcile Component F design-rationale prose at specification.md:365 with the amended acceptance bullet 3 / Note

ACCEPTANCE CRITERIA:
- Line 365 no longer contains the literal-persistence claim.
- Cross-references acceptance criterion 3 / Note.
- Bullet 3 (line 391) and Note (line 394) unchanged.
- No other spec sections touched; no code changes.

STATUS: Complete

SPEC CONTEXT: T11-3 amended bullet 3 and added a Note demoting the literal-persistence claim in favour of log-noise-absence as the observable contract under tmux 3.6b. Step-3 design prose at line 365 still asserted the demoted claim, creating a top-to-bottom contradiction. T12-1 reconciles the prose.

IMPLEMENTATION:
- Status: Implemented
- Location: `.workflows/slow-open-empty-previews-and-zombie-sessions/specification/slow-open-empty-previews-and-zombie-sessions/specification.md:365`
- The trailing clause "the session persists for the next bootstrap to evaluate" has been replaced with prose asserting "the lock-loser cascade is quiet — every `BootstrapPortalSaver` tmux call targets an extant session and no `no such session` log entries are produced" plus a parenthetical "(Literal session-persistence after daemon exit is a separate concern; see acceptance criterion 3 and the Note below for tmux-version-specific behaviour.)"
- Preceding step-3 sentences (the `-k` flag explanation, placeholder kill/replace, pane survival) preserved verbatim.
- Cross-reference to acceptance criterion 3 and the Note is explicit.
- Grep for "persists for the next bootstrap" across the spec returns zero matches.
- Bullet 3 (line 391) and Note (line 394) verified unchanged.

TESTS:
- Status: N/A (doc-only change).

CODE QUALITY:
- Readability: Lines 361–394 read top-to-bottom without contradiction; step-3 prose aligns with bullet 3 and the Note. Cross-reference placed at end-of-step is the natural location for forward references.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
