# Analysis Tasks: Lazy Resume On Attach (Cycle 10)

## Task 1: An Answered Pane Closes on Its First Exit Even After Its Binary Has Gone
severity: medium
sources: architecture

**Problem**: The recovery tail is written twice: once in Go (`runResumeRecover`, `cmd/state_resume_recover.go:35-62`) and once in shell (`parkedChainBackstop`, `cmd/state_hydrate.go:291-298`). The Go tail opens with the gate that tells an answered pane from an abandoned one. It reads `@portal-resume-pending`, and when the read succeeds and the marker is clear, it does nothing. The shell copy starts at the step after that gate. Its only guard is that the tail could not start: status 126 or 127, and `[ ! -x <exe> ]`.

That guard separates the two kinds of pane only if the binary is still present after every answer, and nothing ensures that. An answered pane can stay open for days before its shell exits and the tail runs. That is long enough for a package-manager upgrade to remove the versioned path `os.Executable` resolved, which happens on every Linuxbrew upgrade and anywhere else the baked path is versioned or temporary.

The sequence runs like this. A pane is answered by Enter or by a confirmed discard. Later its hook shell or plain shell exits, and the parked shell runs `<exe> state resume-recover`, which exits 127. The backstop finds the executable gone. It writes the reset bytes, clears a marker that is already clear, releases the pin, runs `stty sane` and execs `${SHELL:-/bin/sh}`. The user types `exit` and gets a fresh prompt instead of a closed pane. That is the two-exits regression the specification rules out: "a pane that has been answered is handed no second shell". Nothing is logged, because no Portal binary is left to log it.

The backstop cases in `cmd/state_resume_backstop_test.go:135` stage an unstartable binary only for a pane whose draw never ran, so this case has no test.

**Solution**: Open the backstop's could-not-run arm with the tail's own gate. This follows from the specification's rule that the chain's recovery step does nothing for an answered pane, and from the rule `runResumeRecover` already applies: a clear marker means answered, and a failed read counts as still pending.
- **The gate.** The backstop reads the pane's `#{@portal-resume-pending}` through tmux, with stderr discarded. It composes the read from `state.ResumePendingOption` and `tmux.PaneIDTarget(payload.Pane)`, as its other tmux calls are composed.
  - A read that succeeds and comes back empty ends the chain with `exit $s`, before any reset, clear, unpin, `stty` or shell.
  - A read that fails, because tmux refuses or is missing from `PATH`, counts as still pending. The existing steps then run in their existing order.
- **A plain format read is enough.** It needs no show-options existence probe before it. The pane being read is the one the parked shell runs in. The one misreading a plain format read allows is a gone pane reading empty, and a gone pane is exactly one the chain should end without a shell.
- **Existing directions stand.** This builds on the cycle-1 and phase-7 directions without changing them. The backstop stays keyed to status 126 or 127 plus the executable check, `exit $s` passes the status through, the trap is unchanged, and review cycle 2 fixed the unanswered path's order (reset, marker clear, confirmed-leave pin release, `stty sane`, shell). That order is unchanged.
- **Documentation.** `parkedChainBackstop`'s doc comment names the gate. So does the backstop sentence in CLAUDE.md's "Resume hooks" section.
- **Tests.** The backstop suite gains the missing case: the draw has run, the marker is clear, and the binary is gone when the tail runs.

**Outcome**: A pane answered on its panel closes on its shell's first exit whether or not the baked binary is still there. A pane whose waiter died unanswered still reaches its shell by the backstop, exactly as it does today.

**Acceptance Criteria**:
- [ ] A pane is answered on its panel, by Enter or a confirmed discard, so its pending marker reads clear. By the time its shell exits, the baked binary has gone from the baked path. The parked chain reads the marker and exits with the shell's could-not-run status, 127. It writes no reset bytes, issues no marker clear, makes no `#{alternate_on}` read and no pin unset, runs no `stty sane` and starts no shell, so the pane closes on that first exit.
- [ ] The same answered pane, with its baked binary still at the path but no longer executable, ends the same way with status 126.
- [ ] A pane's waiter ended unanswered, so its marker still carries a value, and any non-empty value counts as set. Its tail cannot start. After the marker read, the backstop takes its existing steps in their existing order: the reset bytes, the marker clear, the pin lifted once `#{alternate_on}` reads `0` within the bound (and kept if it never does), `stty sane`, then the user's shell. The chain exits with that shell's status.
- [ ] tmux refuses the marker read. The pane counts as still pending: the backstop takes the same steps in the same order and reaches the user's shell, and the refused read writes nothing to the pane's stderr.
- [ ] No tmux is on `PATH`. The pane counts as still pending and reaches the user's shell, and the backstop writes nothing to the pane's stderr.
- [ ] A tail that started gets nothing from the backstop, not even the marker read, and the chain exits with the tail's status. That covers an answered pane whose tail ran and did nothing, and a recovered pane whose shell exited with any status, 126 and 127 included.

**Do**:
- The gate goes in `parkedChainBackstop` (`cmd/state_hydrate.go:291-299`) as the first step of its could-not-run arm. It comes after the `126|127` status match and the `[ ! -x <exe> ]` check, and before the reset bytes.
- The gate is a plain format read of `#{@portal-resume-pending}` for the pane. It uses the same `display-message -p … -F` form `backstopReleasePin` already uses for `#{alternate_on}`. It is composed from `state.ResumePendingOption` and `tmux.PaneIDTarget(payload.Pane)` the way the backstop's other tmux calls are, and its stderr is discarded. No `show-options` probe comes before it.
- If the read succeeds and comes back empty, the chain ends with `exit $s`. If the read fails, the existing steps run.
- These stay as they are: the keying on status 126 or 127 plus the executable check, `exit $s`, `parkedChainTrap`, and the unanswered path's order (reset, marker clear, pin release after a confirmed leave, `stty sane`, shell).
- `parkedChainBackstop`'s doc comment names the gate. So does the backstop sentence in the "Resume hooks" section of `CLAUDE.md`.
- `TestParkedResumeChain_Backstop` (`cmd/state_resume_backstop_test.go:135`) gains the answered case: the draw runs, and the binary is removed before the tail runs. In its existing unstartable-tail cases, the transcripts gain the marker read ahead of the reset bytes.

## Task 2: A Refusal on a Waiting Pane States Its Reason, and Its Record Finds the Pane
severity: low
sources: standards, architecture

**Problem**: The specification gives two records the job of making a failed act on a waiting pane actionable. Each drops the part that makes it useful.

**The panel's report row loses the reason.** The row is built from `err.Error()` verbatim: `resumeReport` at `cmd/state_resume_wait.go:393-399`, and `dropErr.Error()` in the draw at `cmd/state_resume_draw.go:55-58`. `resumeReportRow` (`internal/tui/resume_panel_parts.go:49-57`) then truncates it to the card's 52-cell content width, or to the pane's width in the plain stack. Wrapped Go errors put the context first and the cause last, so the truncation cuts off the cause:
- For a refused marker clear, the row reads `failed to unset pane option @portal-resume-pending o…` (`internal/tmux/tmux.go:329-335`). Tmux's own answer comes after the argv, and the row never reaches it.
- For an unreadable store on discard, the row reads `failed to load hooks: open /Users/<name>/.config/p…` (`internal/hooks/store.go:227`), and `permission denied` is gone.

The specification says the reason "is stated on a single line" for exactly these cases. The pane swallows every key but the act keys, so the user can only press the key again without knowing whether a retry can succeed. The project's error-handling rule also says a technical error is translated for the user and its detail is logged separately. Here the detail is logged nowhere. The only copy of the full chain is the `--report` argv inside the `exec` marker's `args` attr: the refused clear on Enter or discard, the store load failure, the unresolvable store and the refused input drop all write no WARN.

**The chain's records name the pane by a stale address.** Every WARN on the wait and recovery paths identifies the pane by `pane_key` alone: `unset resume pending marker failed` (`cmd/state_resume_recover.go:54`), `alternate screen leave unconfirmed` and `unset alternate-screen pin failed` (`cmd/state_resume_altscreen.go:41,45`), and `enable terminal signals failed` (`cmd/state_resume_chain.go:179`). `pane_key` is the positional address baked from the FIFO path at restore time. The feature itself relies on that address going stale over a wait: a session renamed from the picker, a sibling window closed under `renumber-windows`, a pane broken out or moved.

The specification calls the recovery tail's clear-failed WARN "the whole of what makes that pane findable". An operator who searches for it is sent to another pane, or to none, while the frozen pane's transcript stays frozen until the next reboot. `resumeChainArgv` (`cmd/state_resume_chain.go:64-65`) gives the tail only `--pane` and `--pane-key`, dropping the hook key that the draw and the wait both carry, so the tail has no durable name to log.

**Solution**: Each record carries what the specification says it is for. For the row, that is derived from the single-line-reason rule and the project's error-handling rule. For the log, it is derived from the rule that the WARN is what makes a frozen pane findable.
- **The report row.** It states a short Portal-worded reason naming the act that failed and its cause, in the cause's own short words, and it no longer carries the raw error chain. The reason and cause come before anything the truncation can cut. The reachable failures form a closed set:
  - the store could not be located;
  - the store could not be read, with the OS or parse cause, such as `permission denied`;
  - the hooks lock is held, or could not be taken;
  - the store write failed;
  - the pending-marker clear was refused, with tmux's own stderr, or tmux could not be run;
  - the confirmation's queued input could not be dropped.

  The exact strings are the task author's to fix within that shape. An unrecognised error still gets a reason, never an empty row.
- **The technical detail goes to the log once.** Each refusal the row reports leaves its whole error chain in `portal.log` at WARN, since the row no longer carries it. Where an existing record already covers the refusal, that record carries it: the store's own `op=discard` WARN, which also covers the lock and save failures and gains the load failure the store currently returns unlogged, and `unset resume pending marker failed` for a clear refused on Enter or discard. Otherwise one WARN on the existing `hydrate` catalog carries it. No new component and no new attribute key.
- **A durable pane identity.** The recovery tail's argv carries the hook key: `resumeChainArgv`'s recover branch passes `--hook-key`, and `resume-recover` registers the flag. A chain parked by an older build still parses, because the tail tolerates unregistered flags, and a missing flag reads as empty. Every WARN on the wait and recovery paths carries `hook_key` beside `pane_key`: the four listed above, and any the refusal records add. `hook_key` is already part of the `hydrate` vocabulary, and `portal hook list` resolves a token to the pane's current `<session>:w.p` location, so the record leads to the live pane however it has moved.

**Outcome**: When an answer cannot be carried out, the panel's report row says what failed and why within its one line, and the full error sits in `portal.log`. Every record the wait and recovery paths write names the pane by its durable hook key as well as by its restore-time address.

**Acceptance Criteria**:
- [ ] The user presses `y` on the discard confirmation while `hooks.json` cannot be read for want of permission. The confirmation is drawn again with the report row `can't read hooks.json: permission denied`, which carries no path and no wrapped error text. The registration and the marker both stand. `portal.log` holds the `hooks` component's `discard` WARN (`op=discard`, `via=panel`, `hook_key`), and its `error` carries the whole chain, path included. When `hooks.json` holds malformed JSON instead, the row reads `can't read hooks.json: malformed JSON` and the same WARN carries the parser's error.
- [ ] The user presses `y` while another process holds the hooks lock past its bound. The row reads `can't lock hooks.json: another process holds it`. A lock that cannot be taken for any other reason reads `can't lock hooks.json: ` followed by the OS's words. A failed write reads `can't write hooks.json: ` followed by the OS's words, such as `no space left on device` or `permission denied`. Each of these is logged by the store's existing `discard` WARN alone, and no second WARN is written for it.
- [ ] The user presses `y` when the hooks file's location cannot be resolved: no `PORTAL_HOOKS_FILE`, no `XDG_CONFIG_HOME` and no home directory. The row reads `can't locate hooks.json: $HOME is not defined`. One `hydrate` WARN carries the whole error with `hook_key` and `pane_key`.
- [ ] tmux refuses the pending-marker clear on Enter, or on `y` after a successful removal. The waiting panel is drawn again with `can't unpause this pane: ` followed by tmux's own stderr, such as `can't find pane: %7`, and the row carries no tmux argv and no option name. When tmux cannot be run at all, the row reads `can't unpause this pane: tmux could not be run`. The pin stays in both cases. One `unset resume pending marker failed` WARN (`hydrate`) carries the whole error with `hook_key` and `pane_key`.
- [ ] The user presses `d` and the queued input cannot be dropped. The waiting panel comes up, not the confirmation, with `can't clear pending input: ` followed by the OS's words. One `hydrate` WARN carries the whole error with `hook_key` and `pane_key`.
- [ ] A refusal outside those classes reads `can't carry out that answer`. The row is never empty and never the raw error chain.
- [ ] Each row puts its reason and cause first, so the one-line truncation cuts only the end of the cause. That truncation is at the card's 52-cell width, or at a narrower pane's width in the plain stack. Every example row named above renders whole in the card.
- [ ] A lazy pane parks a recovery tail whose argv carries `--hook-key <token>` beside `--pane` and `--pane-key`, and `portal state resume-recover` parses it. The tail's clear can fail, its pin leave can go unconfirmed, its unpin can fail, or its terminal signals can fail to come back. Each of those WARNs carries `hook_key`, set to the pane's token, beside `pane_key`.
- [ ] The same WARNs written on the Enter and discard paths carry `hook_key` beside `pane_key`: the unconfirmed pin leave, the failed unpin and the terminal signals that did not come back.
- [ ] An older build parked a recovery tail whose argv carries no `--hook-key`. It still parses and recovers the pane, and its WARNs carry an empty `hook_key`.
- [ ] No record this task adds or extends brings in a log component or an attribute key outside the existing vocabulary.

**Do**:
- **The row.** The text is built where the report is composed. `resumeReport` (`cmd/state_resume_wait.go:393-399`) composes the discard confirmation's store refusals and the panel's refused clear. The draw composes the drop report (`cmd/state_resume_draw.go:55-58`). `resumeReportRow` (`internal/tui/resume_panel_parts.go:49-57`) keeps its one-line truncation. The rows:

  | Failure | Row |
  |---|---|
  | the store's location cannot be resolved (`loadHookStore`) | `can't locate hooks.json: <cause>` |
  | `hooks.json` cannot be read | `can't read hooks.json: <OS words>` |
  | `hooks.json` holds malformed JSON | `can't read hooks.json: malformed JSON` |
  | the hooks lock is held past its bound | `can't lock hooks.json: another process holds it` |
  | the hooks lock cannot be taken | `can't lock hooks.json: <OS words>` |
  | the write of `hooks.json` fails | `can't write hooks.json: <OS words>` |
  | tmux refuses the pending-marker clear | `can't unpause this pane: <tmux's stderr>` |
  | tmux cannot be run for the clear | `can't unpause this pane: tmux could not be run` |
  | the confirmation's queued input cannot be dropped | `can't clear pending input: <OS words>` |
  | anything else | `can't carry out that answer` |

  - `<OS words>` is the operating system's own message, such as `permission denied` or `no space left on device`, with no path and no wrapping.
  - `<tmux's stderr>` is tmux's own trimmed message, with no argv.
  - `<cause>` is the resolution's own message, such as `$HOME is not defined`.

  Three of these classes already have names in the tree: `hooks.ErrLockHeld` (`internal/hooks/lock.go:15`), `hooks.ErrMalformed` (`internal/hooks/store.go:334`), and `*tmux.CommandError` with its `Stderr` (`internal/tmux/command_error.go:13`).
- **The log.**
  - A discard's load failure is returned unlogged today (`internal/hooks/store.go:225-228`). It gains the store's `discard` WARN, which the lock and save failures already write (`:220`, `:245`). Those two keep their one WARN.
  - `resumeUnfreeze` (`cmd/state_resume_wait.go:380-388`) reports a clear refused on Enter or discard with no record. That refusal is now written as `unset resume pending marker failed`, the recovery tail's record. `TestRunResumeRecover_FailedClearRecord` (`cmd/state_resume_recover_test.go:274-278`) counts that message's WARN call sites and requires exactly one, so no other event shares the wording.
  - An unresolvable store in `discardResumeRegistration` (`cmd/state_resume_wait.go:433-439`) writes one `hydrate` WARN. So does a refused input drop in `runResumeDraw`.
  - Add no new component and no new attribute key.
- **The hook key.**
  - `resumeChainArgv`'s recover branch (`cmd/state_resume_chain.go:64-65`) passes `--hook-key`. `stateResumeRecoverCmd` registers the flag (`cmd/state_resume_recover.go:103-106`) and hands it to the tail. The command's tolerance of unknown flags stays, so an older chain still parses, and a missing flag reads as empty.
  - `hook_key` goes beside `pane_key` on these WARNs:
    - `unset resume pending marker failed` (`cmd/state_resume_recover.go:54`)
    - `alternate screen leave unconfirmed` and `unset alternate-screen pin failed` (`cmd/state_resume_altscreen.go:41,45`)
    - `enable terminal signals failed` (`cmd/state_resume_chain.go:179`)
    - the `hydrate` WARNs this task adds
- **Tests.**
  - These cases pin the raw error as the row and move to the rows above:
    - `reportedOn` (`cmd/state_resume_discard_test.go:38-43`)
    - `TestResumeAnswerEnter_FailedClear` (`cmd/state_resume_enter_test.go:121-186`)
    - the drop-failure draw cases (`cmd/state_resume_drop_input_test.go:263-288`)
  - These expectations of the recover argv gain `--hook-key`:
    - `cmd/state_resume_chain_test.go:52-60`
    - `cmd/state_resume_draw_screen_test.go:258-264`
    - `cmd/state_hydrate_lazy_test.go:274`
