TASK: killed-sessions-resurrect-on-restart-2-2 — Replace line-262 "marker stays set so the next attach re-signals" comment with one-line recovery-contract note

ACCEPTANCE CRITERIA:
- Substring "marker stays set so the next attach re-signals" no longer appears in cmd/state_hydrate.go.
- Replacement comment documents recovery contract matching handleHydrateFileMissing and 100ms settle-sleep preservation in runHydrate.
- FIFO-unlink comment preserved verbatim.
- Warn-log comment preserved verbatim.
- Comment-only change; no behavioural test edits required.

STATUS: Complete

SPEC CONTEXT: Spec § "Fix 2 → Specific Changes → 3" mandates removing the misleading comment. § Specific Changes → 4 preserves the 100ms sleep before exec. Comment-only follow-up to task 2-1.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/state_hydrate.go:248-277
- Notes:
  - Inline recovery-contract note at line 275: "Recovery path matches handleHydrateFileMissing: marker unset above; runHydrate's exec fall-through still pays the 100ms settle sleep before exec (preserved per spec — same posture as the success path)."
  - Function doc comment (lines 248-259) rewritten; explains marker-unset behaviour plus spec supersession of built-in-session-resurrection line 838.
  - FIFO-unlink rationale (lines 264-267) preserved verbatim.
  - Warn-log purpose (lines 270-271) preserved verbatim.
  - "Deliberately NO 100ms sleep" comment at line 303 (in handleHydrateFileMissing) correctly untouched.

TESTS:
- Status: Adequate (regression-only by design)
- Coverage: Comment-only task; existing TestHydrate_Timeout* tests serve as regression signal. Grep confirms no "marker stays set" leakage in test files.

CODE QUALITY:
- Project conventions: Followed.
- Readability: Good. New comment makes the FIFO-unlink → warn-log → marker-unset → fall-through sequence self-documenting.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Line 275 is a single physical line (~220 chars); surrounding comments wrap at ~80 chars. Wrapping would improve diff-readability.
- [idea] The doc-comment paragraph at lines 256-259 duplicates rationale already recorded in spec § Spec Supersession. Inline placement defensible.
