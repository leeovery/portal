## Attempt 1

ISSUES:
- `/Users/leeovery/Code/portal/cmd/state_resume_screens_test.go:232-251` and `:720-725`: `pipedKeysHarness` hands out buffered follow timers (`make(chan time.Time, 1)`). The new `chord` helper fires the introducer's follow window and then returns, and the test presses the next key straight away. If the loop has not reached its `select` in `followResumeByte` before that key's read completes, both the fired window and the read result are ready. Go picks between them at random. When it takes the key, the key is consumed as a sequence byte: `y`, `d` and `ESC` sit in the CSI final range or feed the OSC terminator. The loop then blocks on a read no test feeds, and the test fails after 2s with "the wait never ended".
  - Reproduced on the working tree:
    - `go test ./cmd -run TestRunResumeWait_IntroducerChords -count=5000 -cpu 2,4,10`: 2 failures
    - the same with `-cpu 4,10`: 4 failures, in "it backs out on a lone Escape after Alt-] on the confirmation" (line 757) and "it confirms on the y after Alt-[ on the confirmation" (line 771)
  - The Enter and cut-short `d` cases share the same ordering.
  - This is the same class of race (an elapsed window against a queued byte) that defect 3 of this task exists to remove. It also contradicts the harness's own doc comment ("a chord, the key after it and a resize are each ordered against the loop rather than against the clock"). With no CI, a developer running `go test ./...` on a loaded machine sees a spurious 2s timeout.
  FIX: Make the follow-window timers unbuffered and fire them through a bounded send, so the fire lands only while the loop is selecting on that window. This is the pattern `escapeHarness` already uses (lines 106-108, 159-163).
  - In `startPipedKeys`'s `Settle`, create the follow timer with `make(chan time.Time)`. Keep settle timers buffered: no key follows a settle fire.
  - Add `func (h *pipedKeysHarness) elapse(timer chan time.Time)` doing `select { case timer <- time.Now(): case <-time.After(resumeEscapeTestBudget): h.t.Fatal(...) }`.
  - Route every follow fire through it: `chord` (line 724), the lone-Escape fires at lines 513 and 755, and `follow <- time.Now()` at line 785.
  - I trialled exactly this on a scratch copy: 0 failures in 45,000 iterations across `-cpu 2,4,10` (the working tree failed 6 times in 25,000), and the full resume suite stays green.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- /Users/leeovery/Code/portal/cmd/state_resume_wait.go:230 — restates what the name and the one `cfg.Settle(resumeEscapeFollow)` call already say. The pending/outstanding contract that matters is stated on `resolveResumeEscape`.
  OLD: // followResumeByte reads the next byte under the follow window.
  NEW:

NOTES:
- `/Users/leeovery/Code/portal/cmd/state_resume_wait.go:68-70` (not touched by the diff, so not a correction): "Size reads the pane at the moment the window elapses" no longer covers every read, because Size is now also read once at start-up. Worth widening when this file is next edited.
- `/Users/leeovery/Code/portal/cmd/state_resume_signals_test.go:255-269` ("a resize redraws with signal generation still off") sets `Size = fixedSize(80, 20)` over a 100x30 `confirmationPayload`. The start-up check now arms a settle as well as the pre-loaded SIGWINCH, so the redraw goes through either path at random. The assertion (the redraw leaves signal generation off) holds on both, so this is not a failure, but the case no longer isolates the SIGWINCH route.
- The non-positive drawn-size cases (`state_resume_wait_resize_test.go:543-570`) show the wait keeps reading and arms nothing, but no key is pressed there. That "a key still acts" is shown only on the failed-read case. The key path does not depend on the start-up branch, so this is not a failure-mode gap.
- The executor's off-script moves of `\x1bb\r` and `\x1bP\r` onto `queuedKeysConfig`, and the added waiting-panel one-write sequence cases, are within the task's harness remit and pass.
- The other waiter suites are stable under stress: `TestRunResumeWait_StartupSize`, `_Resize`, `_Screens` and `TestResumeWait_Signal*` at `-count=1500 -cpu 2,10`, and 400 `-race` runs of the new tests.
- Nothing rode the dispatch beyond the enumerated inputs.
