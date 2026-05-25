TASK: 2-4 — Lock in fail-fatal pre-loop regression coverage for CaptureStructure

STATUS: Complete

SPEC CONTEXT: Spec § Component E (lines 315, 339) — `ListSessionNames`, `ListAllPanesWithFormat`, `parsePaneRows` remain fail-fatal. Pre-loop failures indicate tmux broken; continuing yields destructive empty commits.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/capture_test.go:1514-1624` (`TestCaptureStructurePreLoopFailFatal`)
- Supporting: `failFastCaptureClient` at lines 1494-1512; call-counter fields on `captureMock` at lines 29-31, dispatcher increments at lines 40/43/46
- `failFastCaptureClient` workaround for `*tmux.Client.ListSessions` swallowing exec error; tests `CaptureStructure → CaptureClient` contract rather than implementation accident
- Four sub-tests map 1:1 to acceptance criteria; assert counters (`listPanesCalls`, `showEnvCalls`) where applicable
- All invocations use `state.CaptureStructure(client, nil, nil, nil)` — Task 2-2 signature

TESTS:
- Status: Adequate
- Coverage: All four scenarios, one test each, no duplication of happy-path
- Names match spec's "Tests" bullets verbatim
- Malformed-row test uses 3-field input vs `captureFormat`'s 10 fields — triggers existing `parsePaneRow` error
- Orthogonal to Task 2-3 (nil logger, no envErrs)

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel()`; comments cite spec line numbers
- SOLID: Good; `failFastCaptureClient` single-purpose test double
- Complexity: Low; flat arrange/act/assert
- Modern idioms: errors.New, t.Run subtests
- Readability: Strong; docstrings name spec lines 315/339 and explain protective intent

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `failFastCaptureClient` could be promoted to shared helper if other test files need same pattern; premature extraction is larger risk today
