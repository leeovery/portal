TASK: A Pane Whose Waiter Died Lands at a Usable Shell by Either Route (lazy-resume-on-attach-7-4)

ACCEPTANCE CRITERIA:
- A waiting pane whose tty was left raw by a waiter that never ran its restore (ended by SIGTERM, SIGKILL, or a group SIGINT/SIGQUIT the parked chain's trap routes into the tail) is handed the user's shell by the recovery tail on a cooked tty: Lflag ICANON, ECHO, ECHOE, ECHOK, ISIG, IEXTEN, Iflag ICRNL, IXON and Oflag OPOST, ONLCR are all set
- A pane answered by Enter or by a confirmed discard is handed on exactly as today: the tty the waiter found is put back and only signal generation is turned back on
- A parked chain whose tail cannot start (binary gone from the baked path, or present but not executable) writes hydrateResetPreamble, runs `tmux set-option -pu -t <pane id> @portal-resume-pending`, runs `stty sane`, then starts `${SHELL:-/bin/sh}`, in that order; the chain exits with that shell's status
- On that route, a marker clear that fails (tmux non-zero, or absent from PATH) does not stop the route, and nothing the failed clear wrote to stderr reaches the pane
- A parked chain whose tail did start closes on that shell's first exit: no reset bytes, no tmux clear, no stty, no second shell, and the chain exits with that status

STATUS: complete

SPEC CONTEXT: The spec says a waiter that dies hands the pane to the parked shell, and the recovery tail that shell runs next "drops the pane to a usable prompt". The section on a marker wrongly left set now carries a 2026-09-28 corrigendum. It records that the backstop clears the marker itself when the tail cannot start, and that a clear failing on that route leaves no record because no Portal binary is left to emit one. The task's accepted gap matches it.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/tty_signals.go:22-31 — `cookTTY` sets Lflag ICANON|ECHO|ECHOE|ECHOK|ISIG|IEXTEN, Iflag ICRNL|IXON and Oflag OPOST|ONLCR. It goes through the shared updater, renamed `updateTTYModes` (:33-40) now that it writes more than local modes. The stdin form `cookStdin` is at :50-52.
  - cmd/state_resume_recover.go:94 — the tail's `EnableTTYSignals` seam is now `cookStdin`. `runResumeRecover`'s steps and their order (:38-59) are unchanged.
  - cmd/state_resume_wait.go:487 — the waiter's Enter and confirmed-discard answers stay on `setStdinSignals`, applied after `cfg.restore()` (:321-322, :351-352).
  - cmd/state_hydrate.go:290-297 — `parkedChainBackstop(exe, payload)`. Inside the unchanged `126|127` / `[ ! -x exe ]` arm it composes, in order:
    1. `printf '%s' ` + `shellquote.Single(hydrateResetPreamble)`
    2. `shellquote.Join(["tmux","set-option","-pu","-t",string(tmux.PaneIDTarget(payload.Pane)),state.ResumePendingOption]) 2>/dev/null`
    3. `stty sane 2>/dev/null`
    4. `exec "${SHELL:-/bin/sh}"`

    It restates no literal, and `exit $s` passes the status through. `parkedResumeChain` (:299-304) threads the payload into it. `execResumeChainAndExit` (:262-274) still clears signal generation before the exec.
  - All four composers of the backstop were updated: the definition, `parkedResumeChain`, and cmd/state_hydrate_lazy_test.go:119 and :272.
  - CLAUDE.md, "Resume hooks": the parked chain is now shown as `trap : INT QUIT; <draw argv>; <recover argv>; <backstop>`. The backstop's reset → marker clear → `stty sane` → shell sequence is described, with no log record.
- Notes:
  - `string(tmux.PaneIDTarget(...))` after `-t` is the form `internal/tmux/target_composition_guard_test.go`'s `targetIsExact` accepts. It unwraps the `string(...)` conversion and reads a call to the vocabulary.
  - The chain is exec'd with `os.Environ()` (cmd/state_hydrate.go:389), so the backstop's tmux sees the same `TMUX` and `PATH` the helper's own tmux calls used.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/tty_modes_pty_test.go:55-80 `TestCookTTY_RealPTY` (darwin, real pty): a pty left raw by `term.MakeRaw` gets every required flag back from `cookTTY`, and typed `ls\r` echoes as `ls\r\n`, which exercises ECHO, ICRNL and OPOST/ONLCR.
  - cmd/tty_modes_pty_test.go:82-123 `TestResumeHandOffs_TerminalModes_RealPTY`: executes the real `resume-recover` and `resume-wait` cobra bodies, captures the wired seam, and runs it against a raw pty through os.Stdin. The tail seam is asserted to restore all required modes. The waiter seam is asserted to add ISIG alone, with Iflag and Oflag untouched. Reverting either wiring would fail it.
  - cmd/state_resume_signals_test.go:163-209 and :302-341: the waiter turns ISIG on after the raw restore and before the exec, and the tail turns its seam on before the exec. Both still hand the pane on when the seam fails, with the WARN.
  - cmd/state_resume_backstop_test.go:111-191 runs the real composed chain under /bin/sh, with stub `stty`, `tmux` and user shell writing to a shared transcript:
    - Both unstartable cases (gone; present at 0600) produce the exact transcript preamble → `tmux set-option -pu -t %7 @portal-resume-pending` → `stty sane` → `shell`, and the chain exits 0.
    - Both clear failures (tmux exits 1 and writes to stderr; tmux absent from PATH) still reach `stty sane` and the shell, and nothing leaks to stderr.
    - Started tails with statuses 0, 1, 126 and 127 leave an empty transcript and pass their status through.
- Notes: The matrix covers the unstartable cases against the clear failures and the started cases against the edge statuses 126 and 127 without redundancy. The pty tests are darwin-only, like the existing pty helpers (cmd/tty_flush_pty_test.go:1), which matches the project's single-platform development.

CODE QUALITY:
- Project conventions: Followed. The `withFuncSeam` staging is used, there is no `t.Parallel`, the shell tests are unit-lane with no real tmux and no portal binary, and there are no process-artifact references in comments.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The new comments (cookTTY, parkedChainBackstop) hold against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
