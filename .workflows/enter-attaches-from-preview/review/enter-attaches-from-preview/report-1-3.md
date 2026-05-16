TASK: enter-attaches-from-preview-1-3 — Add ExitError discriminator for has-session probe

ACCEPTANCE CRITERIA:
- [x] `HasSessionProbe` returns `(true, nil)` on zero exit.
- [x] `HasSessionProbe` returns `(false, non-nil-err)` when underlying error unwraps to `*exec.ExitError`; preserves `*CommandError` shape.
- [x] `HasSessionProbe` returns `(true, non-nil-err)` on non-ExitError underlying cause.
- [x] Existing `HasSession(name) bool` signature, behaviour, and callers unchanged.

STATUS: Complete

SPEC CONTEXT: Spec § Pre-select + attach sequence > step 1 mandates the discriminator separating `*exec.ExitError` (non-zero tmux exit — bail) from OS-layer errors (missing binary, exec lookup — proceed). The build chose a sibling method preserving the boolean `HasSession` for non-discriminating callers.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go:131-166
- Notes:
  - Signature `HasSessionProbe(name string) (bool, error)` matches spec.
  - Uses `=`-prefixed target consistent with task 1-2.
  - Three return shapes correct: `(true, nil)` on nil err; `(false, err)` when `errors.As(err, &exitErr)` succeeds; `(true, err)` otherwise.
  - `errors.As` walks the wrap chain via `*CommandError.Unwrap()` (verified at internal/tmux/command_error.go:47).
  - Godoc documents the three shapes, the `=` prefix rationale, and pins the spec section.
  - `HasSession(name) bool` at lines 126-129 left intact; no caller churn.

TESTS:
- Status: Adequate
- Coverage: internal/tmux/tmux_test.go:457-535 — three sub-tests for the three observable shapes:
  - `"returns (true, nil) when tmux exits zero"` — also pins `=`-prefixed argv.
  - `"returns (false, err) when tmux exits non-zero"` — uses `syntheticExitError(t)` (real `*exec.ExitError` from `sh -c 'exit 1'`) wrapped in `*CommandError`. Asserts both `errors.As(err, &cmdErr)` and `errors.As(err, &asExit)` succeed.
  - `"returns (true, err) on OS-layer failure"` — `errors.New("exec: ...")` underlying cause; asserts `errors.As(err, &asExit)` fails.
- `syntheticExitError` helper at line 540-549 is a rigorous way to generate a real `*exec.ExitError` rather than synthetic stand-in.
- `TestHasSession` at lines 324-366 still passes — boolean form unaffected.
- Not over-tested; three branches, three tests.

CODE QUALITY:
- Project conventions: Followed. Mirrors wrapper-style godoc and single-purpose method shape used throughout `internal/tmux`.
- SOLID principles: Good. Single responsibility (discriminator). Sibling method preserves simpler `HasSession` for non-discriminating callers.
- Complexity: Low. Three branches, linear flow.
- Modern idioms: Correct use of `errors.As` (walks unwrap chain); works against the test commander returning a bare wrapped error too.
- Readability: Good. Godoc comprehensive and pins spec section directly.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
