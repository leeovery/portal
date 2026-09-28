TASK: The Recovery Tail Parses Under Whatever Binary Answers It (lazy-resume-on-attach-7-2, tick-fd525c)

ACCEPTANCE CRITERIA:
- A recovery tail handed a flag this build does not register — as a chain parked under an earlier or a later build could hand it — parses, takes the pane off the panel's screen, clears its pending marker and hands it the user's shell
- A recovery tail handed no `--pane`, or an empty one, recovers the pane `$TMUX_PANE` names; one handed a non-empty `--pane` addresses that pane
- A parked chain whose tail cannot be started — the binary gone from the path the chain baked (status 127), or no longer executable there (status 126) — leaves the pane at the user's shell rather than closing it
- A pane answered by Enter or by a confirmed discard, and a pane the tail recovered, closes on the first exit of its shell when that shell exits with an ordinary failure status (say 1, after a failed command) — the backstop hands it no second shell
- The draw and the waiter still refuse a flag they do not register: the waiter handed `--drop-input` fails its parse, as today

STATUS: complete

SPEC CONTEXT: The chain a lazy pane parks (`/bin/sh -c 'trap : INT QUIT; <draw>; <recover>; <backstop>'`) ends in a recovery tail that must give a pane whose waiter died back to its user — leave the panel's screen, clear the pending marker, exec the user's shell — because a closed single-pane session is dropped from the saved set with its whole transcript. The spec ranks a live pane carrying an extra shell as the lesser failure against one that closes, but also rules that an answered pane is handed no second shell (first `exit` closes it). Corrigendum 2026-09-28 records the shell backstop for a tail that cannot start at all.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_recover.go:66-98 — `stateResumeRecoverCmd` sets `FParseErrWhitelist{UnknownFlags: true}` and `Args: cobra.ArbitraryArgs` (lines 73-74), with the by-name contract comment at lines 70-72; `--pane` missing/empty falls back to `os.Getenv("TMUX_PANE")` at lines 76-79, and the marker read/clear closures are built from the resolved pane (line 81)
  - cmd/state_resume_recover.go:101 — `--pane` help names the `$TMUX_PANE` default; the old "no flag is required" note is gone
  - cmd/state_hydrate.go:290-304 — `parkedChainBackstop` appended after the tail in `parkedResumeChain`: `; s=$?; case $s in 126|127) if [ ! -x '<exe>' ]; then …; exec "${SHELL:-/bin/sh}"; fi;; esac; exit $s` — keyed to the shell's could-not-run statuses plus an executable check, never an unconditional `|| exec`
  - cmd/state_resume_chain.go:62 — the comment claiming the tail is handed only registered flags because an unknown one fails its parse was removed
  - cmd/state_resume_draw.go:136-187 and cmd/state_resume_wait.go:447-504 — both keep strict parses (no whitelist, `cobra.NoArgs`)
- Notes: Verified against pflag v1.0.9 / cobra v1.10.2 in the module cache: cobra copies `FParseErrWhitelist` onto the flag set's `ParseErrorsAllowlist` (cobra command.go:1880), and pflag's whitelisted path drops an unknown long or short flag and its separate value without consuming a following `--flag`, so `--pane`/`--pane-key` survive any interleaving the tests stage. `ArbitraryArgs` goes past the task's text (flags only) but is a sound extension of the same rule: a later build adding a positional would otherwise close the pane. The backstop was later extended by task 7-4 (reset bytes, marker clear, `stty sane`) and still keys on 126/127 plus `[ ! -x ]`, so this task's criteria hold against the current code.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: cmd/state_resume_recover_test.go:382-443 `TestStateResumeRecoverCommand_ForeignArgv` drives six foreign argvs (the whole draw payload, attached/separate unknown values, trailing switch, stray positional, unknown shorthand) through the real `rootCmd.Execute()` parse, then asserts the full recovery via `assertRecovered` (leave sequence written, clear called, user's shell exec'd, exec marker logged). Removing the whitelist or restoring `NoArgs` fails it.
  - AC2: cmd/state_resume_recover_test.go:445-475 `TestStateResumeRecoverCommand_PaneFromEnvironment` covers no `--pane`, empty `--pane`, non-empty `--pane` overriding `$TMUX_PANE`, and no flags at all; `executeResumeRecover` (lines 353-380) resets the package-level flag values between runs, so no value carries over from an earlier test.
  - AC3/AC4: cmd/state_resume_backstop_test.go:111-191 runs the real `parkedResumeChain` under `/bin/sh` with stub `tmux`/`stty`/user shell: the binary gone (127) and not executable (126) both reach the user's shell with status 0; a started chain whose draw's shell exits 1 (answered by Enter or discard, since the tail then returns 0), or whose recovered shell exits 1, 126 or 127, passes the status through with an empty transcript (no second shell).
  - AC5: cmd/state_resume_drop_input_test.go:381-391 (the waiter refuses `--drop-input`) and cmd/state_resume_recover_test.go:477-487 (the draw refuses an unregistered flag).
  - Composition: cmd/state_hydrate_lazy_test.go:119 and :272 pin the tail and backstop in the exec'd argv.
- Notes: Tests are focused. The shared `assertRecovered` helper keeps the per-case bodies short, and the backstop cases are observed from real shell execution, not by matching the composed string.

CODE QUALITY:
- Project conventions: Followed (function seam staged through `withFuncSeam`, no `t.Parallel`, shell words quoted through `shellquote`, no bare `os.Exit`)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
