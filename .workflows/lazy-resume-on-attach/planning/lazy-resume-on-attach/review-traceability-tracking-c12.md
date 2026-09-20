# Review Tracking: Lazy Resume On Attach - Traceability

## Findings

### 1. An arrow key closes the discard confirmation

**Type**: Hallucinated content
**Spec Reference**: §6.2 ("While the confirmation is up, `y` and Escape are the only keys that act. Enter, `d` and everything else are swallowed there exactly as they are on the waiting panel (§4.3) — the pane never acts on a key the screen in front of the user does not offer"); §4.3
**Plan Reference**: Phase 5 "Planner's calls"; Phase 5 task table row `lazy-resume-on-attach-5-3`; task `lazy-resume-on-attach-5-3` (Problem, Do, Acceptance Criteria, Tests, Edge Cases, Context)
**Move**: settled
**Change Type**: update-task

**Problem**:
A user reading the discard confirmation — the screen that asks whether to destroy the only copy of a command they wrote — can make it vanish by pressing a key it does not offer. Every arrow key, every function key, Home, End, PgUp and PgDn opens with the same byte Escape sends, and the plan dispatches on that byte the moment it arrives, so `↓` cancels the question exactly as Escape does. The specification says only `y` and Escape act there, and states the property behind it plainly: the pane never acts on a key the screen in front of the user does not offer. The plan decides otherwise and gives its reason as "the waiter is forbidden any timer" — a rule the plan does not in fact hold itself to, since the wait loop already arms the resize settle window. Nothing forced the behaviour.

The same gap has a second mouth on both screens. The plan discards a sequence's bytes one at a time, and a CSI sequence's final byte may itself be `y` (`0x79`) or `d` (`0x64`) — so `\x1b[?1;2y` on the confirmation and `\x1b[5d` on the panel reach an acting key from a key nobody pressed. That is the shape of accident §4.3 exists to foreclose.

**Proposal**:
Hold the confirmation to the rule the specification states: resolve an `\x1b` before dispatching it, and swallow a sequence whole. The waiter requests one further byte against a short follow window; the window elapsing means the byte stood alone (cancel on the confirmation, inert on the panel), and a byte arriving inside it means a sequence, which is consumed to its terminator under a byte cap so no byte of it can reach `y` or `d`. The window arms only after an `\x1b` has been read, so a waiter left alone with no input still arms nothing and task 4.2's criterion that a report stands until a key is pressed passes unchanged — the follow window neither ends the wait nor expires a report, which are the two properties that task pins, and its source guard reads `signal.Notify` calls alone.

What determined the fix is the specification's own rule, not a preference: §6.2 names `y` and Escape as the only keys that act on the confirmation, and §4.3 states the swallow rule as a safety property. The mechanism is open in the specification and is this plan's call, taken on the seam already in place: the follow window reuses `cfg.Settle`, which task 4.6 put on `resumeWaitConfig` for the resize redraw, so no new seam, signal or dependency is added. The 50 ms duration is likewise the plan's call — it delays a genuine Escape imperceptibly while still resolving a sequence split across two writes on a slow link, and it is the order of magnitude the package already lives with in `appearanceDetectTimeout`.

Task 4.2 needs no edit: its criterion is that `\x1b[A` produces no hand-off, which its own byte-by-byte discard satisfies at the point it lands, and which the resolver added here goes on satisfying.

**Current**:

*`planning.md` — Phase 5, "Planner's calls", fifth bullet:*

```
- An escape sequence's first byte backs out of the confirmation: the waiter is forbidden any timer, so there is no disambiguation window, and an arrow key therefore cancels rather than confirms — the harmless direction, needing nothing added.
```

*`planning.md` — Phase 5 task table, the `lazy-resume-on-attach-5-3` row's Edge Cases cell, the fifth clause:*

```
an escape sequence's first byte backs out of the confirmation which is the harmless direction and needs no timer
```

*`phase-5-tasks.md` — task 5.3, `**Problem**`:*

```
**Problem**: The waiter dispatches two keys against one screen. `d` currently hands the pane back to a fresh draw of the waiting panel — task 4.2's placeholder — so the confirmation the chain can now paint is unreachable, and there is no screen on which `y` and Escape mean anything. The keys also have to be *screen-scoped* rather than global: Enter resumes on the panel, so an Enter that acted on the confirmation would confirm an irreversible deletion with the key that means "bring it back" one screen earlier, and Escape must stay inert on the panel — binding the reflex key to an irreversible deletion would make this the one place in Portal where it destroys something.
```

*`phase-5-tasks.md` — task 5.3, `**Do**`, first bullet:*

```
- In `cmd/state_resume_wait.go`, split the read loop's dispatch on `cfg.Screen`. On `resumeScreenPanel`: `\r` and `\n` → `resumeAnswerEnter`, `d` → `resumeOpenDiscardConfirm`, every other byte swallowed (task 4.2's behaviour, unchanged). On `resumeScreenDiscard`: `y` → `resumeAnswerDiscard`, `\x1b` → `resumeCancelDiscardConfirm`, every other byte — `\r`, `\n`, `d`, `Y`, `0x03`, `0x04`, `0x1a`, printable text — swallowed.
```

*`phase-5-tasks.md` — task 5.3, `**Acceptance Criteria**`, seventh bullet:*

```
- [ ] The first byte of an escape sequence (`\x1b` of `\x1b[A`) backs out of the confirmation, and its remaining bytes are then swallowed by the panel's table on the next process image; no timer is armed to disambiguate.
```

*`phase-5-tasks.md` — task 5.3, `**Tests**`, seventh entry:*

```
- `"it backs out on the first byte of an escape sequence"`
```

*`phase-5-tasks.md` — task 5.3, `**Edge Cases**`, fifth bullet:*

```
- An escape sequence's first byte backs out of the confirmation, which is the harmless direction and needs no timer: the waiter is forbidden any timer of its own, so there is no disambiguation window, and an arrow key therefore cancels rather than confirms.
```

*`phase-5-tasks.md` — task 5.3, `**Context**`, final paragraph:*

```
> This phase's calls, applied here: a report's lifetime ends at the next key press, stated once so the two screens do not each invent their own rule; an escape sequence's first byte backs out of the confirmation, because the waiter is forbidden any timer and cancelling is the harmless direction; and task order puts the input-drop guard (task 5.4) before the key that destroys anything (task 5.5), so the confirm key is a redraw until then.
```

**Proposed Text**:

*`planning.md` — Phase 5, "Planner's calls", fifth bullet:*

```
- An escape sequence is swallowed rather than acted on, on both screens. Escape is the confirmation's cancel key and is also the byte every arrow, function and navigation key opens with, so a dispatch taken on that byte alone would let a key the screen does not offer close the question the user is reading — and a CSI sequence's final byte may itself be `y` or `d`, so a sequence discarded byte by byte could reach either acting key from a key nobody pressed. The waiter therefore resolves an `\x1b` before it dispatches it: one further byte is requested against a short follow window, and a byte arriving inside it means a sequence, which is consumed to its terminator under a byte cap. The window reuses the `Settle` seam the resize redraw already put on the wait config and arms only after an `\x1b` has been read, so it neither ends the wait nor expires a report and a waiter left alone with no input still arms nothing.
```

*`planning.md` — Phase 5 task table, the `lazy-resume-on-attach-5-3` row's Edge Cases cell, replacing the fifth clause (the rest of the cell is unchanged):*

```
an escape sequence is swallowed on both screens rather than acted on so no key the screen does not offer closes the confirmation and no CSI final byte reaches `y` or `d`, a bare Escape still backs out of the confirmation and is still inert on the panel
```

*`phase-5-tasks.md` — task 5.3, `**Problem**`:*

```
**Problem**: The waiter dispatches two keys against one screen. `d` currently hands the pane back to a fresh draw of the waiting panel — task 4.2's placeholder — so the confirmation the chain can now paint is unreachable, and there is no screen on which `y` and Escape mean anything. The keys also have to be *screen-scoped* rather than global: Enter resumes on the panel, so an Enter that acted on the confirmation would confirm an irreversible deletion with the key that means "bring it back" one screen earlier, and Escape must stay inert on the panel — binding the reflex key to an irreversible deletion would make this the one place in Portal where it destroys something. And Escape is not a byte the loop can take at face value: every arrow, function and navigation key opens with the same `\x1b`, and a CSI sequence's final byte may itself be `y` (`0x79`) or `d` (`0x64`), so a loop that dispatched on `\x1b` alone — or that discarded a sequence's bytes one at a time — would let a key the screen does not offer close the confirmation, and would let an acting key be reached from a key nobody pressed.
```

*`phase-5-tasks.md` — task 5.3, `**Do**`, first bullet, plus a new second bullet:*

```
- In `cmd/state_resume_wait.go`, split the read loop's dispatch on `cfg.Screen`. On `resumeScreenPanel`: `\r` and `\n` → `resumeAnswerEnter`, `d` → `resumeOpenDiscardConfirm`, `\x1b` resolved through the escape resolver below and acting on nothing either way, every other byte swallowed (task 4.2's behaviour, unchanged). On `resumeScreenDiscard`: `y` → `resumeAnswerDiscard`, `\x1b` resolved through that same resolver and dispatching `resumeCancelDiscardConfirm` only when it stood alone, every other byte — `\r`, `\n`, `d`, `Y`, `0x03`, `0x04`, `0x1a`, printable text — swallowed.
- Resolve an `\x1b` before dispatching it, on both screens. Add `resumeEscapeFollow = 50 * time.Millisecond` and `resumeEscapeSequenceCap = 16` to `cmd/state_resume_wait.go`, and one unexported helper the loop calls when it reads `\x1b`: request one further byte through the loop's own reader and `select` it against `cfg.Settle(resumeEscapeFollow)` — the seam task 4.6 already put on `resumeWaitConfig`, so nothing new is wired. The window elapsing first means the byte stood alone and the helper reports a bare Escape: `resumeCancelDiscardConfirm` on the confirmation, nothing at all on the panel. A byte arriving first means a sequence, and the helper swallows it and goes on swallowing until it has taken a byte in the CSI final range `0x40`–`0x7e` or `resumeEscapeSequenceCap` bytes have gone, whichever comes first, then reports nothing — so no byte of a sequence can reach an acting key, which byte-by-byte discard cannot promise. The window arms only after an `\x1b` has been read, so a waiter left alone with no input arms nothing, and it decides nothing but whether that one byte stood alone: it never ends the wait and never clears the report, which is what keeps task 4.2's criteria and its signal guard green.
```

*`phase-5-tasks.md` — task 5.3, `**Acceptance Criteria**`, replacing the seventh bullet with five:*

```
- [ ] An escape sequence delivered to the confirmation produces no hand-off, no write and no state change, and the loop is still reading afterwards — a `y` pressed after it still confirms (table: `\x1b[A`, `\x1b[3~`, `\x1bOD`).
- [ ] A sequence whose final byte is an acting key produces no hand-off on either screen — `\x1b[?1;2y` on the confirmation and `\x1b[5d` on the waiting panel are each consumed whole, so neither `y` nor `d` can be reached from a key nobody pressed.
- [ ] A bare `\x1b` — no byte following it inside the follow window — backs out of the confirmation, and is still inert on the waiting panel.
- [ ] A sequence longer than `resumeEscapeSequenceCap` is swallowed rather than acted on, and the loop is still reading afterwards.
- [ ] The follow window arms only after an `\x1b` has been read: a waiter left alone with no input arms nothing, and task 4.2's criterion that a waiter launched with a non-empty `--report` goes on holding it and dispatching both keys however long it is left alone passes unchanged.
```

*`phase-5-tasks.md` — task 5.3, `**Tests**`, replacing the seventh entry with four:*

```
- `"it swallows an escape sequence on the confirmation"` (table: `\x1b[A`, `\x1b[3~`, `\x1bOD`)
- `"it swallows a sequence whose final byte is an acting key"` (table: `\x1b[?1;2y` on the confirmation, `\x1b[5d` on the panel)
- `"it backs out on a bare Escape"` (no byte inside the follow window)
- `"it swallows a sequence longer than the byte cap"`
```

*`phase-5-tasks.md` — task 5.3, `**Edge Cases**`, replacing the fifth bullet with two:*

```
- An escape sequence is swallowed rather than acted on, and that is what holds the rule that only `y` and Escape act on the confirmation. Escape is the cancel key and is also the byte every arrow, function and navigation key opens with, so dispatching on that byte alone would let a key the screen does not offer close the question the user is reading. Discarding a sequence's bytes one at a time is not enough either: `y` and `d` are both legal CSI final bytes, so a sequence could reach an acting key from a key nobody pressed. Resolving the `\x1b` first and consuming the sequence whole is what makes "the pane never acts on a key the screen in front of the user does not offer" true for every key a terminal can send.
- The follow window is not the timer the wait path is denied: it arms only after an `\x1b` has been read, it decides nothing but whether that byte stood alone, and it neither ends the wait nor expires a report — the two properties task 4.2 pins. A waiter left alone with no input arms nothing at all. The duration is this task's call: the specification states no figure, and 50 ms delays a genuine Escape imperceptibly while still resolving a sequence split across two writes on a slow link.
```

*`phase-5-tasks.md` — task 5.3, `**Context**`, final paragraph:*

```
> This phase's calls, applied here: a report's lifetime ends at the next key press, stated once so the two screens do not each invent their own rule; an escape sequence is resolved before its first byte is dispatched and then swallowed whole, so no key the screen does not offer acts and no CSI final byte reaches `y` or `d`; and task order puts the input-drop guard (task 5.4) before the key that destroys anything (task 5.5), so the confirm key is a redraw until then.
```

**Resolution**: Pending
**Notes**:
