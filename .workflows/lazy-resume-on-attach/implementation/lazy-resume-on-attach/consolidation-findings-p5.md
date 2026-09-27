# Consolidation Findings: lazy-resume-on-attach (Phase 5)

## Findings

### F1: A two-byte ESC chord makes the waiter swallow the user's next keystrokes on both screens
- **Class**: behaviour
- **Failure**: The escape resolver treats *any* byte that follows an ESC inside the 50 ms follow window as the introducer of a multi-byte sequence. Anything other than `]` then runs under the CSI rule: a blocking read, with no window, until a byte in 0x40–0x7E arrives or 16 bytes have gone by. Several things produce exactly that pair:
  - an Alt/Meta chord, which tmux forwards to the pane as `ESC <key>` in a single write;
  - terminals that map Option+←/→ to `ESC b` / `ESC f`;
  - an Escape pair (M-Escape);
  - an Escape followed within 50 ms by any other key.

  None of those pairs is the start of a longer sequence, so the resolver ends up reading the keys the user presses *next*:
  - **On the waiting panel:** after `ESC b`, 14 consecutive Enter presses are swallowed and only the 15th resumes. `\r` (0x0D) is not a final byte, so it cannot end the "sequence". I confirmed this by running the landed resolver, copied into a scratch harness: `\x1bb` followed by 14×`\r` produces no action, and the 15th acts. A `d` typed after the chord is also eaten, as the "final byte".
  - **On the discard confirmation:** after an Alt chord, the first `y` is eaten as the final byte. After an Escape pair, later Escape presses (0x1B) are eaten one after another, so the confirmation will not back out.
  - **On both screens:** pane resizes are not serviced while the resolver is blocked, so the panel stays drawn at its old size until a letter arrives.

  The user sees a dead keyboard: the key the footer names does nothing, and they have to press it again and again. It fails safe (nothing is ever destroyed), but it is exactly the "pane whose keys silently do nothing" that the chain is built to avoid.

  The task reviewer recorded this in both rounds' NOTES (`fix-tracking-lazy-resume-on-attach-5-3.md`, "Alt-chords and fast second keys eat later keystrokes") and left it as a plan-level matter, because the code follows the plan task's rule verbatim ("every other introducer runs to a byte in the CSI final range"). Nothing has carried it forward since.
- **Evidence**:
  - `cmd/state_resume_wait.go:29-38`: the constant block and its comment state the rule "every other sequence ends at a byte in the CSI final range".
  - `cmd/state_resume_wait.go:160-170`: the loop routes every ESC through the resolver.
  - `cmd/state_resume_wait.go:197-213`: `resolveResumeEscape` hands any byte that arrives inside the window to `consumeResumeSequence`.
  - `cmd/state_resume_wait.go:218-233`: `consumeResumeSequence` applies the CSI rule to every introducer except `]`, with an unbounded blocking `reader.next()`.
  - `cmd/state_resume_wait.go:235-237`: `isCSIFinal`.
  - Test coverage: `cmd/state_resume_screens_test.go:231-243` exercises only `[` and `O` introducers, and `:300-313` pads only `[`, `O` and `]` sequences. No case sends an ESC pair that completes at two bytes.
- **Proposed shape**: Branch on the introducer the way ECMA-48 structures input sequences:
  - `[` and `O` keep the CSI rule under `resumeEscapeSequenceCap`.
  - `]` (and optionally the other string introducers `P`, `_`, `^`, `X`) keeps the terminator rule under `resumeOSCSequenceCap`.
  - Any other byte completes a two-byte sequence: it is consumed with the ESC and nothing more is read.

  The chord itself is still swallowed whole, so Alt-y and Alt-d still never act and every existing screens-test case stays green. Update the constant-block comment to state the three-way rule. Add screens cases that pin the fix:
  - on the panel, `\x1bb\r` is answered by the Enter;
  - on the confirmation, `\x1bxy` confirms;
  - on the confirmation, `\x1b\x1b` followed by a lone ESC backs out.

  This departs from the wording of the phase 5 plan task's resolver rule, so the orchestrator should route it as a deliberate change rather than a refactor.

## Comment Corrections

- `cmd/state_resume_chain.go:62-65`: this phase falsified the comment. The phase added `--drop-input`, which only `resume-draw` registers, and `resumeChainArgv` emits it for whatever subcommand it is given whenever the payload carries it. `TestStateResumeDrawCommand_DropInput`'s "it refuses the drop flag on the waiter" (`cmd/state_resume_drop_input_test.go:382-391`) composes exactly that argv for `resume-wait`. The recovery tail's early return is the only place the function itself enforces "only the flags that subcommand registers".
  OLD: // resumeChainArgv composes one chain command's argv, emitting only the flags
  // that subcommand registers: a flag a subcommand does not know fails its parse,
  // and on the tail's path a failed parse closes the pane the tail exists to keep
  // open.
  NEW: // resumeChainArgv composes one chain command's argv. The recovery tail is
  // handed only the flags it registers: a flag a subcommand does not know fails
  // its parse, and on the tail's path a failed parse closes the pane the tail
  // exists to keep open.
