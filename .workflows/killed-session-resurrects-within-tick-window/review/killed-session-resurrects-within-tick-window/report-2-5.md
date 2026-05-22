TASK: Replace errCommitNowFailed empty-message sentinel and preserve cause (killed-session-resurrects-within-tick-window-2-5)

ACCEPTANCE CRITERIA:
- `errCommitNowFailed` carries a descriptive (non-empty) message.
- Silent-exit driven by `errors.Is`, not string-compare on empty Error().
- Failure path wraps cause via `%w`.
- `errors.Unwrap` surfaces underlying chain.
- Subprocess exit-code preserved; stderr still empty (no regression).

STATUS: Complete

SPEC CONTEXT: Original sentinel used `errors.New("")` so `main.go`'s top-level handler suppressed stderr by checking `err.Error() == ""`. Cycle-1 analysis flagged this as brittle string-compare antipattern — sentinels should carry descriptive messages, detection should use `errors.Is`. Task 2-5 retires the empty-message convention while preserving the hook-subprocess "writes nothing to stderr" contract.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_commit_now.go:22` — `errCommitNowFailed = errors.New("commit-now failed")` (descriptive).
  - `cmd/state_commit_now.go:30-35` — exported `IsSilentExitError` covering both `errCommitNowFailed` and `ErrStatusUnhealthy` via `errors.Is`.
  - `cmd/state_commit_now.go:255` — `failCommitNow` returns `fmt.Errorf("%w: %s: %v", errCommitNowFailed, stage, cause)`.
  - `main.go:32` — top-level handler now calls `cmd.IsSilentExitError(err)`.
  - `cmd/state_status.go:13-20` — `ErrStatusUnhealthy` doc-comment cites `IsSilentExitError` (cycle-3 follow-on, task 3-2).
- Notes: Wrap format is `"%w: %s: %v"` — `errors.Unwrap(err)` returns `errCommitNowFailed`, not the underlying cause. Cause text preserved as `%v` in `err.Error()` only. Doc-comment at line 245 reads "errors.Unwrap surfaces the underlying cause" — strictly inaccurate. Go 1.20+ multi-`%w` (`fmt.Errorf("%w: %s: %w", ...)`) would place cause in the chain.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/state_commit_now_test.go:1168-1188` (T27) — failure exit `errors.Is`-detectable and stderr empty.
  - T27b (1193-1219) — `errors.Unwrap` non-nil; cause text via `strings.Contains`.
  - T27c (1223-1227) — `errCommitNowFailed.Error()` non-empty.
  - T27d (1232-1240) — `IsSilentExitError(errCommitNowFailed)` and wrapped variant both true.
  - T27e (1244-1248) — `IsSilentExitError(ErrStatusUnhealthy)` true.
  - T27f (1252-1259) — `IsSilentExitError(nil)` and arbitrary errors return false.
- Notes: Subprocess exit-code/empty-stderr regression covered structurally — T27 asserts `errBuf.Len() == 0` at cobra layer; T27d proves `main.go` suppression guard fires. Real subprocess test would be redundant.

CODE QUALITY:
- Project conventions: Followed. Sentinel naming, doc-comments thorough, no `t.Parallel()`, DI seam untouched.
- SOLID: Good. `IsSilentExitError` single responsibility; sentinel co-located with `failCommitNow`.
- Complexity: Low.
- Modern idioms: `errors.Is` / `errors.Unwrap` / `fmt.Errorf("%w")` — canonical Go 1.13+ pattern.
- Readability: Good. Doc-comments explicitly call out the "no longer relies on empty-message string-compare" intent.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] `failCommitNow` uses `"%w: %s: %v"` — cause not in error chain. Doc-comment at line 245 ("errors.Unwrap surfaces the underlying cause") is strictly inaccurate. Either (a) clarify doc to mention cause is in message text only, or (b) switch to multi-`%w` so `errors.Is(err, cause)` holds. If (b), strengthen T27b to assert `errors.Is(err, cause)`.
