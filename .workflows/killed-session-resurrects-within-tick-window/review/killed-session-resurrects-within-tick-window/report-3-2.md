TASK: Replace ErrStatusUnhealthy empty-string sentinel with a descriptive message (killed-session-resurrects-within-tick-window-3-2)

ACCEPTANCE CRITERIA:
- Sentinel identity preserved via errors.Is.
- Doc-comment now cites IsSilentExitError.
- main.go silent-exit path unchanged.

STATUS: Complete

SPEC CONTEXT: Cycle-2 analysis identified `ErrStatusUnhealthy` was originally declared as `errors.New("")` — empty-message sentinel depending on Cobra never printing empty strings and main.go's prior `err.Error() == ""` guard. With cycle-1's introduction of `IsSilentExitError`, the empty-string convention became redundant; cleanup makes the sentinel self-describing while preserving silent-exit contract via compile-time-linked helper.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/state_status.go:13-20` — `ErrStatusUnhealthy = errors.New("status unhealthy")` with multi-line doc comment citing `IsSilentExitError` (see `cmd/state_commit_now.go`).
  - `cmd/state_status.go:52` — `return ErrStatusUnhealthy` unchanged at unhealthy branch.
  - `main.go:22-34` — silent-exit path unchanged; uses `cmd.IsSilentExitError(err)` (not empty-string check). Comment explicitly names `cmd.ErrStatusUnhealthy` alongside `errCommitNowFailed`.
  - `cmd/state_commit_now.go:30-35` — `IsSilentExitError` covers both sentinels via `errors.Is`.
- Notes: Sentinel identity preserved — all callers use either `err != ErrStatusUnhealthy` (direct identity) or `errors.Is` semantics implicitly via `IsSilentExitError`. The Go runtime's pointer-equality identity for `errors.New` values is unaffected by message text.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/state_commit_now_test.go:1242-1248` (T27e) — `IsSilentExitError(ErrStatusUnhealthy) == true`.
  - `cmd/state_status_test.go:350-368` — `TestStateStatusUnhealthyDoesNotEmitErrorBanner` confirms stderr remains silent end-to-end via cobra `SilenceErrors=true`.
  - Numerous existing identity checks (`err != ErrStatusUnhealthy`) at multiple test sites continue to function (sentinel identity is by pointer, not message).
- Notes: Not over-tested; single-line assertion alongside cycle-1 `errCommitNowFailed` analogue. Message string is not a public API contract — no test for it.

CODE QUALITY:
- Project conventions: Followed. Sentinel naming (`ErrXxx`), `errors.Is`-based detection, descriptive doc comment.
- SOLID: Good. Single-responsibility sentinel; suppression logic in `IsSilentExitError`.
- Complexity: Low.
- Modern idioms: `errors.New` directly, `errors.Is` for chain-aware detection.
- Readability: Good. Doc comment explains failure semantics and stderr-suppression mechanism.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
