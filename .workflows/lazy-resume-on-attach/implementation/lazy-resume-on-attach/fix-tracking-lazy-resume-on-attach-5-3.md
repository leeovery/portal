## Attempt 1

ISSUES:
- `cmd/state_resume_screens_test.go:246-262` ("it swallows a terminal background-colour reply whole") and `:272-292` ("it swallows a sequence longer than the byte cap") would pass if an OSC sequence were consumed under the CSI cap. I applied `limit, ended = resumeEscapeSequenceCap, oscTerminator()` at `cmd/state_resume_wait.go:217` through a `go test -overlay` (no repo edits), and `go test ./cmd -run 'TestRunResumeWait|TestStateResumeWait'` stays green.
  - Why the reply test misses it: the reply it uses, `rgb:1d1d/1f1f/2121`, has its only `d`s at bytes 11 and 13, inside the first 16. The bytes left over after the CSI cap (`1f/2121\x07`) contain no key that acts, so "consumed whole" is never actually checked.
  - Why the cap test misses it: it places the `y` past both caps, so any cap at or below the sequence length passes.
  - What it costs: the task itself says the cap matters as much as the terminator, and the spec names `#2d2d2d` as a real background. Its reply, `\x1b]11;rgb:2d2d/2d2d/2d2d\x07`, has a `d` at byte 18. Under the mutation, my probe running that reply on the panel failed with a hand-off (`runResumeWait() error = <nil>`). The same probe passes on the current code. So a regression that opens the discard confirmation on a pane nobody touched would ship with the suite green.
  FIX: add the spec's `#2d2d2d` reply, BEL-terminated and `\x1b\`-terminated, to the background-colour table at `cmd/state_resume_screens_test.go:246`. Run it on the panel at minimum (running it per screen matches the table). Assert it through the existing `swallowed` helper. Its `d` at byte 18 is only unreachable if the OSC is consumed to its own terminator under the 64-byte cap.
  ALTERNATIVE: additionally pin each cap from both sides in the cap test at `:272`. An acting key at the last byte inside the cap must be swallowed:
  - CSI: `"\x1b[" + strings.Repeat("1", resumeEscapeSequenceCap-3) + "y"` on the confirmation, 16 bytes.
  - OSC: `"\x1b]" + strings.Repeat("a", resumeOSCSequenceCap-3) + "d"` on the panel, 64 bytes.
  The existing "`y` after the sequence acts" cases pin the other side once the padding is `cap-2`. This pins the exact constants, but it is more tests than the real failure (the terminal's own reply) needs. I recommend the reply case above, with this pair as an optional extra.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- `cmd/state_resume_wait.go:332` — "yet" refers to the plan's task order ("removes nothing yet"), which the comment discipline does not allow.
  OLD: // resumeAnswerDiscard is the confirmation's y. It removes nothing yet: it hands
  NEW: // resumeAnswerDiscard is the confirmation's y. It removes nothing: it hands

NOTES:
- Behaviour the executor flagged, which follows the task's resolver rule. Any introducer other than `]` runs to the next byte in 0x40–0x7e, and it does so with a blocking read and no window. So after Alt+key, a double Escape, or Alt+digit, the next keys are absorbed up to the 16-byte cap. That includes Enter on the panel and Escape on the confirmation, because neither 0x0d nor 0x1b is a final byte. It fails safe (a key is swallowed, never acted on), but a user can press Enter or Escape several times with no response. If that matters, the fix is in the task's rule, not in this code.
- The shared swallow table runs a lone `"\x1b"` on the confirmation as a "non-acting" key. It passes only because EOF reaches the resolver before the real 50 ms `time.After` follow window. The confirmation's sequence tests also depend on sequence bytes beating that real window. I could not make it flake: 300 runs of `TestRunResumeWait` under `-race` passed with load average around 220 on 10 cores. It stays a note.
- `resolveResumeEscape` returns `alone` and `pending`, which are always equal: `(true, true)` or `(false, false)`.
- The waiter's `oscTerminator` accepts BEL and `ESC \`. The draw's own parser, `backgroundPayload` at `internal/tui/pane_appearance.go:141`, also accepts the 8-bit ST (0x9c). The gap can't be hit as far as I can tell: tmux answers an OSC 11 query itself, using BEL or `ESC \`.
- Two doc comments the diff touched have broken line wraps after re-flowing: `runResumeWait` at `cmd/state_resume_wait.go:93` and `resumeRedraw` at `:347-349`. The content is accurate; only the wrapping is off.
- The reworked idle test ("it holds its report and goes on waiting with no input at all") is justified. The `d` hand-off now clears the report by design, so the report being held is shown through a settled-resize redraw. The test also asserts that no window of either kind starts while idle.

## Attempt 2

ISSUES:
- `/Users/leeovery/Code/portal/cmd/state_resume_screens_test.go:246-269` ("it swallows a terminal background-colour reply whole"): the test passes even if neither OSC terminator is recognised.
  - Each case runs `swallowed(reply+term.end)` and nothing more, so the input ends at EOF right after the terminator.
  - Traced mutation: make `oscTerminator` never match. `consumeResumeSequence` keeps reading, the next read hits EOF inside the sequence, and the loop returns the wrapped EOF with no hand-off. The read count is still `len(input)+1`, so all eight subtests stay green.
  - The criterion's clause "the loop is still reading afterwards" is therefore never observed. The sibling CSI test does verify it (lines 218-228 answer `seq+"y"` after the sequence); the OSC test has no equivalent.
  - What goes wrong: a regression in BEL or ST detection would ship silently. Every pane whose terminal answers the draw's appearance query late would then swallow up to about 40 real keystrokes (Enter, `d`, `y`, Escape) after the reply, which looks like the dead keyboard the spec warns against.
  FIX: in each (screen, reply, terminator) subtest, keep the `swallowed` call and add `answered(t, payload, reply+term.end+key)`. Use `key = "d"` on the panel and `key = "y"` on the confirmation, and in both cases assert `assertHandOff(t, probe, onScreen(payload, resumeScreenDiscard))`. This mirrors the `seq+"y"` follow-up at lines 224-225. Each reply is about 25 bytes, well under `resumeOSCSequenceCap`, so the trailing key can act only if BEL or ST ended the sequence. A terminator that never matched would absorb the key and fail on EOF.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- `/Users/leeovery/Code/portal/cmd/state_resume_wait.go:189-192` — the claim that the whole sequence is consumed and no byte of it can reach a key is false past the cap: bytes beyond `resumeEscapeSequenceCap` or `resumeOSCSequenceCap` are dispatched, which the over-cap test itself pins.
  OLD: // resolveResumeEscape decides whether an ESC just read stood alone, and if it
// opened a sequence consumes that sequence whole, so none of its bytes can
// reach a key the screen acts on. A window that elapses leaves its read
// outstanding, which pending reports so the loop does not request a second.
  NEW: // resolveResumeEscape decides whether an ESC just read stood alone, and if it
// opened a sequence consumes it to its end or its cap, so no byte inside that
// bound can reach a key the screen acts on. A window that elapses leaves its
// read outstanding, which pending reports so the loop does not request a second.

NOTES:
- **Alt-chords and fast second keys eat later keystrokes.** The task's rule "every other introducer runs to a byte in the CSI final range" treats an ESC followed by a byte that opens no multi-byte sequence as a CSI. tmux delivers an Alt/Option chord as `\x1b<key>` in one write, and a second key arriving within the 50 ms window looks the same.
  - Effect on the panel: after such a pair, later keystrokes are eaten until a byte in 0x40–0x7e or the 16-byte cap. For example, up to 14 Enter presses can be swallowed before one acts.
  - Effect on the confirmation: Esc-Esc within 50 ms does not back out, and further Escapes are eaten until a letter arrives.
  - The damage only ever runs in the swallow direction; nothing is destroyed. The implementation follows the task text exactly, so any change belongs at plan level. Under ECMA-48, only `[` (CSI), `O` (SS3, one more byte) and the string introducers `]`, `P`, `X`, `^`, `_` open multi-byte sequences; any other byte completes a two-byte one.
- **Escape tests depend on a real 50 ms window.** `newResumeWaitConfig` defaults `Settle` to real `time.After`. So every escape-sequence test on the confirmation that is driven by `keystrokes` needs the reader goroutine to deliver the byte after ESC within a real 50 ms. I could not reproduce a flake: 300 runs under `-race -cpu 1` at a load average of about 115 on 10 cores all passed. I have not raised it as an issue. A never-firing default for the follow window would take these tests off the clock, matching the rendezvous style of the resize and escape harnesses.
- **The `\x1b` entry in the shared swallow table is not a non-acting key on the confirmation.** It passes because EOF arrives before the follow window elapses, so what it actually checks is "an ESC followed by a read error ends the wait without a hand-off". Bare Escape does act on the confirmation, and the harness-driven tests cover that correctly.
- Sizes: the size table covers only the acting keys. Swallowed keys cannot depend on size, because the dispatch never reads width or height.
- Nothing arrived with the dispatch beyond the enumerated inputs.
