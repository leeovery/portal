TASK: Unify ANSI-stripping in Cascade E2E Test on Package Helper (preview-visual-distinction-2-1)

ACCEPTANCE CRITERIA:
- Delete local `ansiSGRRe` regex + `ReplaceAllString` call in `internal/tui/pagepreview_cascade_e2e_test.go`.
- Use in-package `stripANSI` (defined in `pagepreview_chrome_test.go`).
- Remove now-unused `regexp` import.
- Build/tests still pass.

STATUS: Complete

SPEC CONTEXT: Cascade-tier e2e test (spec Surface 5) drives full Update → View at widths 200/105/95/50/15 covering chrome cascade tiers 1–4 and asserts SGR-reset bytes present. Analysis cycle 1 finding #1 flagged that the cascade test rolled its own SGR-only regex instead of reusing `stripANSI` (delegates to `github.com/charmbracelet/x/ansi`'s `ansi.Strip` — handles full ANSI grammar).

IMPLEMENTATION:
- Status: Implemented.
- Location: `internal/tui/pagepreview_cascade_e2e_test.go:153` — `stripped := stripANSI(raw)`.
- `regexp` import gone; `ansiSGRRe` gone (Grep on `regexp|ansiSGR` in file returns no matches).
- Sole package ANSI helper: `stripANSI` at `pagepreview_chrome_test.go:14-16`.
- All ~19 test sites in `internal/tui` uniformly use `stripANSI`.

TESTS:
- Status: Adequate. Refactor; verification by existing cascade test suite.
- Coverage: `TestPreviewView_CascadeTiersEndToEnd` — 5 widths × per-tier assert closures. SGR-reset assertion (line 161) deliberately checks raw (not stripped) output.
- Notes: Substitution is semantics-preserving — `ansi.Strip` is a strict superset of deleted SGR-only regex on test inputs.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`, no `tmuxtest`.
- SOLID: Good — single source of truth for ANSI stripping.
- Complexity: Low.
- Modern idioms: Yes — delegating to vetted `ansi.Strip` over hand-rolled regex.
- Readability: Good.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
