TASK: A Refusal on a Waiting Pane States Its Reason, and Its Record Finds the Pane (tick-4a9e20, lazy-resume-on-attach-16-2)

ACCEPTANCE CRITERIA:
- `y` on the confirmation with hooks.json unreadable for want of permission: confirmation redrawn with `can't read hooks.json: permission denied` (no path, no wrapped text); registration and marker stand; the hooks `discard` WARN (op=discard, via=panel, hook_key) carries the whole chain, path included. Malformed JSON reads `can't read hooks.json: malformed JSON` and the same WARN carries the parser's error.
- Lock held past its bound reads `can't lock hooks.json: another process holds it`; any other lock failure `can't lock hooks.json: <OS words>`; a failed write `can't write hooks.json: <OS words>`. Each logged by the store's existing `discard` WARN alone.
- Unresolvable store location reads `can't locate hooks.json: $HOME is not defined`; one `hydrate` WARN carries the whole error with hook_key and pane_key.
- tmux refuses the pending-marker clear on Enter or on `y` after a removal: waiting panel redrawn with `can't unpause this pane: <tmux stderr>`, no argv, no option name; `can't unpause this pane: tmux could not be run` when tmux cannot be run; pin stays; one `unset resume pending marker failed` WARN with hook_key and pane_key.
- `d` with an undroppable input queue: waiting panel (not the confirmation) with `can't clear pending input: <OS words>`; one `hydrate` WARN with hook_key and pane_key.
- Any other refusal reads `can't carry out that answer`; never empty, never the raw chain.
- Reason and cause first; every example row renders whole in the 52-cell card.
- The parked recovery tail's argv carries `--hook-key <token>` beside `--pane`/`--pane-key`, and `resume-recover` parses it; the tail's clear, unconfirmed leave, failed unpin and failed signal re-enable WARNs carry hook_key.
- The same three WARNs on the Enter and discard paths carry hook_key beside pane_key.
- An older chain with no `--hook-key` still parses and recovers, its WARNs carrying an empty hook_key.
- No new log component or attribute key.

STATUS: complete

SPEC CONTEXT: The report row is the panel's single-line statement of why an answer could not be carried out (a discard the store refuses, reported on the confirmation; a freeze that will not lift, reported on the waiting panel). It stays until the next key, so the user can tell whether a retry can succeed. When a waiter dies, the recovery tail hands the pane over whether or not the clear landed. Its WARN is then the only thing that makes a wrongly-frozen pane findable, and a positional pane key goes stale over a long wait. The project's error-handling rule asks for a user-facing translation, with the technical detail logged separately.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_report.go:17-28 holds the row vocabulary as constants, act first and cause appended.
  - cmd/state_resume_report.go:44-61: `resumeDiscardRefusal` classifies the store's refusals in this order: unlocated, ErrLockHeld, ErrLockFailed, ErrMalformed (before ErrStoreRead, which wraps it), ErrStoreRead, `fileutil.IsWriteFailure`, then the fallback.
  - cmd/state_resume_report.go:66-78: `resumeClearRefusal` puts tmux's trimmed stderr from `*tmux.CommandError`, or "tmux could not be run" for an `*exec.Error`.
  - cmd/state_resume_report.go:88-107: `causeWords` returns the errno message where the chain carries one, else the message at the bottom of the chain, following the last branch of a `%w: %w` join.
  - cmd/state_resume_wait.go:332 uses the discard row. cmd/state_resume_wait.go:354-357 writes the refused clear under the shared `warnResumePendingClearFailed` and redraws the panel with the clear row.
  - cmd/state_resume_wait.go:407-414: an unresolvable store writes one `hydrate` WARN and returns `hookStoreUnlocatedError`.
  - cmd/state_resume_draw.go:56-60: a refused drop writes one WARN, puts the panel up with the drop row and sets InputArriving.
  - internal/hooks/store.go:150-158: `loadForMutation` gives a refused discard load the store's `op=discard` WARN. Lock and save keep their single WARN at :140 and :285.
  - internal/hooks/lock.go:19,69,80 adds the `ErrLockFailed` sentinel that separates a genuine open/flock failure from ErrLockHeld.
  - internal/fileutil/atomic.go:44-51 adds `IsWriteFailure` (no floor).
  - cmd/state_resume_chain.go:68-83: `resumePaneRef.logAttrs` puts hook_key beside pane_key. cmd/state_resume_chain.go:87-92 makes the recover branch emit `--hook-key`.
  - cmd/state_resume_recover.go:92,117 registers and reads the flag; the UnknownFlags whitelist and ArbitraryArgs are retained (:84-85).
  - hook_key now goes on the clear WARN (cmd/state_resume_recover.go:70), the leave/unpin WARNs (cmd/state_resume_altscreen.go:41,45) and the signals WARN (cmd/state_resume_chain.go:209).
- Notes:
  - I checked the error chains each row is built from. Permission denied on the lock (via `%w: open: %w`) and on the read both reach the errno through `errors.AsType[syscall.Errno]`. The locate cause bottoms out at os.UserHomeDir's "$HOME is not defined". The drop sentinels give "input kept arriving".
  - Example-row lengths are 44 to 47 cells, under the 52-cell card.
  - The store-level load WARN in `loadForMutation` also covers `set` and `rm`. That fits the store-as-breadcrumb-chokepoint convention and adds no attribute key.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/state_resume_report_test.go:125-230 drives every store class end to end through the production store resolution: unreadable, malformed, held lock, refused lock, denied write and unlocatable. Each checks the exact row, the confirmation redrawn, the marker untouched, exactly one WARN at or above WARN level, its component/op/via/hook_key, and the whole chain in `error`.
  - cmd/state_resume_report_test.go:251-310 crosses Enter and `y` with tmux stderr, tmux unrunnable and the fallback. It checks the row, the panel screen, the pin left, the single WARN with hook_key/pane_key/whole error, and that the row carries no argv or option name.
  - cmd/state_resume_report_test.go:312-369 renders every example row whole in both cards, and checks that a long drop cause is cut only inside the cause.
  - cmd/state_resume_drop_input_test.go:268-336 covers the drop row and the drop WARN's attributes.
  - The recover argv and its parse: cmd/state_resume_chain_test.go:52-60, cmd/state_resume_recover_test.go:302-335, cmd/state_hydrate_lazy_test.go:265-280.
  - The older-chain tolerance with an empty hook_key: cmd/state_resume_recover_test.go:337-364.
  - hook_key on the leave/unpin WARNs across Enter, discard and the tail: cmd/state_resume_altscreen_test.go:110-155,170-178. On the signals WARNs: cmd/state_resume_signals_test.go:196-213,331-353.
  - The store's discard load WARN: internal/hooks/discard_test.go:406-468.
- Notes: The tests are focused and cover every acceptance criterion. Rows are checked both through the classifier directly and end to end without much overlap. Nothing is over-tested.

CODE QUALITY:
- Project conventions: Followed. It uses the existing `hydrate` catalog and existing attribute keys, no t.Parallel, function seams through `withFuncSeam`, and the single shared clear-failed wording that the count guard enforces.
- SOLID principles: Good. Row composition is isolated in one file, separate from the waiter's control flow.
- Complexity: Low
- Modern idioms: Yes (`errors.AsType`, multi-`%w`)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
