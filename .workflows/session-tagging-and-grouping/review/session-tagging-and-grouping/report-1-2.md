TASK: Tag value normalisation helper (trim + lower-case + reject empty) — session-tagging-and-grouping-1-2

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA (edge cases): leading/trailing whitespace trimmed; whitespace-only rejected; mixed/upper case collapses; empty string rejected; internal whitespace preserved.

SPEC CONTEXT: specification.md:55-65 — tag value IS the grouping key, handling load-bearing. Rules: trim edges; empty/whitespace-only rejected as no-op; canonical lower-cased; freeform (no whitelist/max). Line 65 mandates same canonical form everywhere a tag is compared.

IMPLEMENTATION:
- Status: Implemented at internal/project/tags.go:26-32 (NormaliseTag). TrimSpace → guard empty → ("", false) → else ToLower(trimmed), true. Internal whitespace preserved (TrimSpace strips edges only). (string, bool) signature expresses rejection cleanly. In-package callers AddTag (tags.go:64) and RemoveTag (tags.go:98) both route through it. No drift.

TESTS:
- Status: Adequate. tags_test.go:5-67 TestNormaliseTag — table-driven; all five edge cases plus idempotence row; each asserts value AND ok bool. Not under/over-tested.

CODE QUALITY: Conventions followed; pure single-responsibility; low complexity; idiomatic comma-ok. No blocking issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES:
- [idea] tags.go:31 — strings.ToLower is simple per-rune lower-casing, not full Unicode case-folding. Correct for ASCII v1 (spec freeform-by-design). Flagged only for a future conscious Unicode-identity decision. No action required.
