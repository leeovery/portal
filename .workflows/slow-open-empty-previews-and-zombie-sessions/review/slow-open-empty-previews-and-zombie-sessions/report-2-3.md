TASK: 2-3 — Replace abort-on-error with per-session log-and-continue plus natural-churn discriminator

STATUS: Complete

SPEC CONTEXT: Component E (spec 281-342). Per-pane defensive pattern mirrored from `cmd/state_daemon.go:185-192`. Discriminator errs on the side of preserving scrollback when any failure was anomalous.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/capture.go:109-142`
- `tmuxerr.ErrNoSuchSession` sentinel; classification via `errors.Is`
- Natural-churn: increments `naturalChurnCount`, logs WARN, continues
- Anomalous: appends to `anomalousErrs`, logs WARN, continues
- Discriminator gates strictly on `len(keep) > 0 && len(sessions) == 0 && len(anomalousErrs) > 0` — mixed-with-success returns partial idx with nil err
- All-natural-churn total failure falls through to normal idx-return → nil err
- Error wrap uses `errors.Join(anomalousErrs...)` inside `fmt.Errorf` — sentinels reachable via `errors.Is`

TESTS:
- Status: Adequate
- Coverage: `capture_test.go:533-760` `TestCaptureStructurePerSessionLogAndContinue` — 8 sub-tests covering every acceptance criterion 1:1 including real-logger WARN delivery
- Helper `noSuchSessionErr` constructs `*tmux.CommandError` exercising production-shape sentinel propagation
- Not over-tested

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel()`
- SOLID: Good; per-session loop single responsibility; discriminator separate guard clause
- Complexity: Low; single linear loop + post-loop guard
- Modern idioms: errors.Is + errors.Join (Go 1.20+)
- Readability: Good; comments and godoc cover corner cases

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Log format strings could be harmonised to a `"capture: <natural-churn|anomalous> session %q: %v"` shape for filter-by-classification
- [idea] All-natural-churn branch lacks tick-level summary; future operability tidy
