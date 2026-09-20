# Review Tracking: Lazy Resume On Attach - Integrity

## Findings

### 1. The terminal's own colour reply can open the discard confirmation

**Severity**: Important
**Plan Reference**: Phase 5, task `lazy-resume-on-attach-5-3` (`d` opens the confirmation and Escape backs out)
**Category**: Acceptance Criteria Quality (edge-case coverage)
**Move**: settled
**Change Type**: update-task

**Problem**:
A restored pane nobody has touched can be found sitting on `▲ Discard resume?` — the screen whose whole job is to make an irreversible deletion deliberate — because the terminal's answer to Portal's own background-colour question is read as keystrokes.

Under the shipped adaptive theme the draw writes an OSC 11 query on every draw, including every resize redraw, and abandons its reader at the 50 ms detect timeout. A reply that arrives after that — a redraw over a link slower than 50 ms, or one that crosses the wire to a remote terminal — is left in the pane's input queue, and the waiter that the draw execs a moment later reads it byte by byte. The payload is `rgb:` followed by hex, and hex carries `d`.

The escape resolver does not save it. A rule that ends a sequence at "a byte in the CSI final range `0x40`–`0x7e`" is a rule for CSI and SS3, whose payloads are digits and separators; an OSC payload is ordinary printable text, so the resolver stops at the `r` of `rgb:` and hands `gb:1d1d/...` to the dispatch one byte at a time. The `d` in it opens the confirmation. The 16-byte cap does not catch it either — a colour reply is around 23 bytes, so the cap expires mid-payload and leaves the rest to the same dispatch.

Nothing is destroyed by it (`y` is not a hex digit, so the screen cannot confirm itself), but the user comes back to a destructive confirmation they never asked for, on the one screen the plan promises can only be reached deliberately.

**Proposal**:
Close it in the resolver, which is where the plan already puts this property: "the pane never acts on a key the screen in front of the user does not offer", and task 3.5 already names this exact trap in its own edge cases — a reply "left in the pane's input queue, where the waiter reads it as keystrokes" — for the `os.Stdin` variant of the probe. Binding `/dev/tty` fixed the case where the probe never reads at all; it does not fix the case where the probe reads and gives up before the reply lands.

The fix is that what ends a sequence depends on which sequence it is: an OSC introducer (`]`) runs to its own terminator, `BEL` or `ST`, under a cap that fits a colour reply; every other introducer keeps the CSI final-byte rule under the existing cap. The introducer itself is taken unconditionally either way, which is already what the bullet's "swallows it and goes on swallowing" says and what keeps `[` and `O` from being mistaken for a terminator.

The remaining sliver — a reply split across the follow window, so the `\x1b` resolves as a bare Escape and the payload arrives after it — is named rather than guarded: the only acting key reachable from `rgb:`-plus-hex is `d`, the confirmation it opens cannot self-confirm, and its own input drop empties the queue behind it. A waiter that ignored input for a grace period, or that parsed terminal replies of its own, would cost more than the case it closes.

**Current**:

*(a) the `Do` bullet beginning "Resolve an `\x1b` before dispatching it")*

- Resolve an `\x1b` before dispatching it, on both screens. Add `resumeEscapeFollow = 50 * time.Millisecond` and `resumeEscapeSequenceCap = 16` to `cmd/state_resume_wait.go`, and one unexported helper the loop calls when it reads `\x1b`: request one further byte through the loop's own reader and `select` it against `cfg.Settle(resumeEscapeFollow)` — the seam task 4.6 already put on `resumeWaitConfig`, so nothing new is wired. The window elapsing first means the byte stood alone and the helper reports a bare Escape: `resumeCancelDiscardConfirm` on the confirmation, nothing at all on the panel. A byte arriving first means a sequence, and the helper swallows it and goes on swallowing until it has taken a byte in the CSI final range `0x40`–`0x7e` or `resumeEscapeSequenceCap` bytes have gone, whichever comes first, then reports nothing — so no byte of a sequence can reach an acting key, which byte-by-byte discard cannot promise. The window arms only after an `\x1b` has been read, so a waiter left alone with no input arms nothing, and it decides nothing but whether that one byte stood alone: it never ends the wait and never clears the report, which is what keeps task 4.2's criteria and its signal guard green.

*(b) the acceptance criterion about a sequence longer than the cap)*

- [ ] A sequence longer than `resumeEscapeSequenceCap` is swallowed rather than acted on, and the loop is still reading afterwards.

*(c) the edge case beginning "An escape sequence is swallowed rather than acted on")*

- An escape sequence is swallowed rather than acted on, and that is what holds the rule that only `y` and Escape act on the confirmation. Escape is the cancel key and is also the byte every arrow, function and navigation key opens with, so dispatching on that byte alone would let a key the screen does not offer close the question the user is reading. Discarding a sequence's bytes one at a time is not enough either: `y` and `d` are both legal CSI final bytes, so a sequence could reach an acting key from a key nobody pressed. Resolving the `\x1b` first and consuming the sequence whole is what makes "the pane never acts on a key the screen in front of the user does not offer" true for every key a terminal can send.

**Proposed Text**:

*(a) replaces the `Do` bullet quoted above)*

- Resolve an `\x1b` before dispatching it, on both screens. Add `resumeEscapeFollow = 50 * time.Millisecond`, `resumeEscapeSequenceCap = 16` and `resumeOSCSequenceCap = 64` to `cmd/state_resume_wait.go`, and one unexported helper the loop calls when it reads `\x1b`: request one further byte through the loop's own reader and `select` it against `cfg.Settle(resumeEscapeFollow)` — the seam task 4.6 already put on `resumeWaitConfig`, so nothing new is wired. The window elapsing first means the byte stood alone and the helper reports a bare Escape: `resumeCancelDiscardConfirm` on the confirmation, nothing at all on the panel. A byte arriving first means a sequence, and which sequence it is decides what ends it: the introducer is taken unconditionally — so the `[` and `O` that open one are never mistaken for its end — and then an OSC (`]`) runs to a `BEL` (`0x07`) or an `ST` (`\x1b\`) under `resumeOSCSequenceCap`, while every other introducer runs to a byte in the CSI final range `0x40`–`0x7e` under `resumeEscapeSequenceCap`. Either way the helper then reports nothing, so no byte of a sequence can reach an acting key — which byte-by-byte discard cannot promise, and which one final-byte rule cannot either: an OSC payload is ordinary printable text, so the CSI rule would stop at the `r` of a `rgb:` reply and dispatch the `d` that follows it. The window arms only after an `\x1b` has been read, so a waiter left alone with no input arms nothing, and it decides nothing but whether that one byte stood alone: it never ends the wait and never clears the report, which is what keeps task 4.2's criteria and its signal guard green.

*(b) replaces the acceptance criterion quoted above, and adds one beneath it)*

- [ ] A sequence longer than its own byte cap — `resumeEscapeSequenceCap` for a CSI or SS3 sequence, `resumeOSCSequenceCap` for an OSC one — is swallowed rather than acted on, and the loop is still reading afterwards.
- [ ] A terminal's own OSC reply is consumed whole on both screens: `\x1b]11;rgb:1d1d/1f1f/2121\x07`, the answer the draw's appearance query asks for and whose payload carries `d`, produces no hand-off, no write and no state change, and the loop is still reading afterwards — asserted for the `BEL`-terminated and the `ST`-terminated form alike.

*(c) added to the Tests list, after `"it swallows a sequence whose final byte is an acting key"`)*

- `"it swallows a terminal background-colour reply whole"` (table: `\x1b]11;rgb:1d1d/1f1f/2121\x07`, the same reply terminated with `\x1b\`, per screen)

*(d) added to the Edge Cases list, after the "An escape sequence is swallowed rather than acted on" bullet quoted above)*

- A terminal's own answer to the draw's appearance query is consumed whole rather than dispatched. The draw writes an OSC 11 background-colour query on every draw under an adaptive pair and abandons its reader at the detect timeout, so a reply that arrives after it — a redraw over a link slower than 50 ms — is left in the pane's input queue for the waiter to read as keys. Its payload is `rgb:` plus hex, which carries `d`: under one CSI final-byte rule the resolver stops at the first printable byte and that `d` opens the discard confirmation on a pane nobody touched. Consuming an OSC sequence to its own terminator is what closes it, and it is why the resolver branches on the introducer rather than on a single final-byte range. The cap matters as much as the terminator: a colour reply runs to roughly 23 bytes, so the 16-byte CSI cap would expire mid-payload and hand the rest to the dispatch.
- The one case left open is a reply split across the follow window — the `\x1b` read, then nothing for `resumeEscapeFollow`, then the payload — which resolves as a bare Escape and dispatches the remainder byte by byte. It is bounded and non-destructive, and is accepted rather than guarded: the only acting key reachable from an `rgb:`-plus-hex payload is `d`, so the worst outcome is a confirmation the user did not open — one that cannot confirm itself, because `y`, `\r` and `\n` cannot appear in that payload at all, whose own input drop empties the queue behind it, and which Escape backs out of. A waiter that ignored input for a grace period, or that parsed terminal replies of its own, would cost more than the case it closes.

**Resolution**: Fixed — task 5-3's escape resolver now branches on the introducer (`resumeOSCSequenceCap = 64` added; OSC runs to `BEL`/`ST`, everything else keeps the CSI final-byte rule), its cap criterion is restated per sequence kind, and it gains the OSC-reply criterion, the `"it swallows a terminal background-colour reply whole"` test and both edge cases — the second naming the split-reply residual as accepted. Tick body re-synced and byte-verified.
**Notes**: The premise was measured rather than assumed, because cycle 11's integrity review had recorded the opposite — that tmux "answers instantly or never, never late", which would have made this unreachable. Measured here on disposable `-L ptl-oscf-*` sockets, with a pty-attached client acting as the terminal and the pane reader in **raw mode** (the condition the waiter runs in, and the step whose absence makes this experiment silently capture nothing — my own first two attempts did exactly that, and a `send-keys` sanity check caught it): a client answering in 10ms and a client answering in **400ms** both had their reply delivered into the pane's input queue; no client attached, and an attached client that never answers, both delivered nothing. So tmux does forward a late reply, and cycle 11's finding-free conclusion on that point does not hold — most likely the same canonical-mode trap. Nothing in the plan cited cycle 11's claim, so no other text needed correcting. The resolver is in fact worse than the finding states: `]` is 0x5D, inside the CSI final range, so the swallow would have ended at the introducer itself rather than at the `r`. All test servers were killed; the developer's server on the `default` socket was never targeted.

---
