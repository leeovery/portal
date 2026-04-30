# Review Report: built-in-session-resurrection-12-10

**TASK**: Quick-fix — remove stale `migrate-rename` comment in `hooks_register_test.go`

**ACCEPTANCE CRITERIA**:
- Comment-only change; current contract is 9 hooks (migrate-rename deferred to v2 per task 7-2 path b).
- Original review finding: lines 545-547 referenced "the migrate-rename call" no longer present; line 474 said "registers all 10" but assertion uses correct slice-len for 9.
- Required: remove or update stale comments to reflect current state (9 hooks, no migrate-rename).

**STATUS**: Issues Found

**SPEC CONTEXT**:
Spec and CLAUDE.md "Server bootstrap" section document that `RegisterPortalHooks` ships only two hook categories — save-trigger (7 events: session-created, session-closed, session-renamed, window-linked, window-unlinked, window-layout-changed, pane-focus-out) and hydration-trigger (2 events: client-attached, client-session-changed). The migrate-rename category was deferred to v2 (task 7-2 path b). Net contract: 9 hooks total. Test fixtures must reflect this 9-hook truth.

**IMPLEMENTATION**:
- Status: Partial / Drifted
- Location: `/Users/leeovery/Code/portal/internal/tmux/hooks_register_test.go`
  - Lines 43-46 — explicit explanatory comment that migrate-rename was deferred to v2; correct.
  - Lines 84-85 — `allPortalHooksRegisteredOutput` doc-comment correctly notes migrate-rename is intentionally absent.
  - Lines 545-547 — comment now reads "in the current 9-hook registration order"; stale "migrate-rename call" reference removed. Correct.
  - **Line 474** — still reads `// registers all 10. Second bootstrap sees those 10 in show-hooks` (two occurrences of "10"). Stale: assertion on line 503 derives `want := len(expectedSaveTriggerEvents) + len(expectedHydrationTriggerEvents)` (= 9). The "10" wording contradicts the assertion in the same `t.Run` block.
- Notes: One stale numeric reference remains. Original finding named two stale spots (474 and 545-547); only one was fixed.

**TESTS**:
- Status: N/A (comment-only task; the surrounding suite continues to validate the 9-hook contract via slice lengths).
- Coverage: Numeric truth (9) is enforced by computed assertions (lines 503-505), so the stale "10" comment cannot mask a regression — but it actively misleads readers.

**CODE QUALITY**:
- Readability: Concern — line 474's "all 10" / "those 10" is contradicted by the immediately following `want = 7 + 2 = 9` assertion. A reader cross-checking comment against code will be confused.
- Issues: Stale numeric reference at line 474 (two occurrences of "10").

**BLOCKING ISSUES**:
- Line 474 of `internal/tmux/hooks_register_test.go` still contains the stale "registers all 10 / those 10" wording. The original review finding explicitly named both this line and lines 545-547 as needing remediation; only the latter was fixed. Acceptance criterion ("current contract is 9 hooks") is not fully satisfied.

**NON-BLOCKING NOTES**:
- [quickfix] Update line 474 comment from `registers all 10. Second bootstrap sees those 10 in show-hooks` to `registers all 9. Second bootstrap sees those 9 in show-hooks` so the comment stays in sync with the computed `want` on line 503.
