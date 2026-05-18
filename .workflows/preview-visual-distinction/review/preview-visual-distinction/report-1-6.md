TASK: Add injectSGRResets Helper (preview-visual-distinction-1-6)

ACCEPTANCE CRITERIA:
- `injectSGRResets(s string) string` exists in `internal/tui/pagepreview.go`.
- Every non-empty line in the output ends with `\x1b[0m`.
- Empty lines (including trailing-newline-induced empty trailing elements) unchanged.
- Idempotency: already-reset line gets a second reset; no panic.
- All six spec edge cases covered by tests.

STATUS: Complete

SPEC CONTEXT: Spec § SGR reset injection. Scrollback lines may end with unterminated SGR; without a reset, lipgloss `BorderForeground` does not reliably restore background state and the right border picks up scrollback colour. Algorithm: split on `\n`, append `\x1b[0m` to lines with `len > 0`, join. Trailing-newline empty trailer ignored; whitespace-only lines count as non-empty; idempotency harmless because terminals collapse `\x1b[0m\x1b[0m`.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tui/pagepreview.go:111-127` (helper); applied at line 573 in `View()` via `bodyBorderStyle.Render(injectSGRResets(m.viewport.View()))`.
- Notes: Byte-for-byte matches plan's reference snippet. Doc comment cites spec section, documents purity and idempotency rationale.

TESTS:
- Status: Adequate
- Coverage: `internal/tui/pagepreview_sgr_test.go` — table-driven with 7 cases: unterminated SGR, already-reset (idempotency), middle empty line, whitespace+embedded SGR, trailing newline empty trailer, fully empty input, multi-line happy path. Each case asserts full-string byte equality.
- Notes: No `t.Parallel()`. Stdlib-only imports. No `tmuxtest`. Not over-tested; not under-tested.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Single responsibility. Pure function.
- Complexity: Low.
- Modern idioms: Idiomatic stdlib usage.
- Readability: Doc comment explains why (right-border colour leak), contract, idempotency.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Per-line `line + "\x1b[0m"` allocates per non-empty line; a single pre-sized `strings.Builder` would reduce allocations. Not worth changing at current cadence.
