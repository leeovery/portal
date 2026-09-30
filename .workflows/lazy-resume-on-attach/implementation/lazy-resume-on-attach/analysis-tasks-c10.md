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
