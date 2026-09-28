## Attempt 1

ISSUES:
- cmd/state_hydrate.go:297 — the backstop hands a second shell to a recovered pane whose user's shell exits 127 (or 126). This is the two-exits outcome AC4 and the spec's corrigendum rule out. When the tail recovers a pane, it execs the user's shell, so the tail's status is that shell's status. After a command-not-found, both `exit` and Ctrl-D exit 127. Measured here: `bash --norc -i` and `zsh -f -i` both return 127 for "typo, then exit" and for "typo, then EOF". I reproduced the whole chain with a stub tail whose shell ends that way: the parked shell ran `stty sane` and exec'd `SECOND-SHELL`. The typical trigger is a user landing in a recovered pane (after `pkill portal`, a crash, or a group signal), mistyping a command, then pressing Ctrl-D: the pane needs a second exit. The task's own reason for rejecting the unconditional `||` ("any `exit` after a failed command") covers this case. The executor saw it and left it, reading the Do as forbidding a fix; the fix below stays inside the Do's "only when the tail's status is 126 or 127".
  FIX: Inside the 126|127 branch, run the backstop only when the baked binary is no longer executable. Make the backstop a function of `exe` so it can quote the path:
  `; s=$?; case $s in 126|127) if [ ! -x <shellquote.Single(exe)> ]; then stty sane 2>/dev/null; exec "${SHELL:-/bin/sh}"; fi;; esac; exit $s`
  Use `if …; then …; fi`, not `&&`: `TestHydrateLazy_SeparatesTheDrawAndTheTailWithASemicolon` (cmd/state_hydrate_lazy_test.go:245-258) fails on any `&&` in the chain. In a scratch run this form gave a shell for a binary that is gone and for one at mode 0600, and passed 127 through with no second shell when the stub's shell exited 127. Then:
  - Update `lazyChainArgs` (cmd/state_hydrate_lazy_test.go:119) and the `want` in `TestHydrateLazy_ComposesTheTailWithThePaneFlagsAlone` (:271) for the new signature.
  - Add rows to the `started` table in cmd/state_resume_backstop_test.go for `RECOVER_STATUS=127` and `126` with the stub present and executable. Each should expect no output, no stty call, and the status passed through.
  - Reword the constant's first sentence if it no longer matches.
  ALTERNATIVE: Keep status-only keying and explicitly narrow AC4 to statuses other than 126/127, recording the residual. This keeps the one case the fix gives up: a binary at the path that is executable but the kernel refuses to run (wrong architecture, reported as 126), which the fix would close instead of leaving at a shell. I recommend the fix. That trigger is a broken install that fails every Portal command, while typo-then-exit is ordinary use. A check ahead of the tail (`if [ -x exe ]; then tail; else shell; fi`) also avoids the gap, but it adds a check-then-exec race that the post-status form mostly closes.
  CONFIDENCE: medium

COMMENT_CORRECTIONS:
- cmd/state_resume_recover.go:73-74 — "accepts any argv" is false: a trailing `--pane` with no value still fails the parse ("flag needs an argument"), and `--help` still prints help.
  OLD: 	// Chains parked under another build call this subcommand by name, so it
	// accepts any argv: a parse it refuses closes the pane it exists to keep open.
  NEW: 	// Chains parked under another build call this subcommand by name, so it
	// tolerates flags and arguments it does not register: a parse it refuses
	// closes the pane it exists to keep open.
- cmd/state_hydrate.go:292-296 — "a tail that ran exec'd the user's shell" is false for an answered pane: that tail exits 0 without exec'ing anything. Only a tail that recovered the pane hands its shell's status to the backstop.
  OLD: // parkedChainBackstop leaves the pane at a shell when the tail could not start
// at all. It keys on the shell's could-not-run statuses, not on any failure: a
// tail that ran exec'd the user's shell, whose exit status arrives here, and a
// second shell after it would make the pane take two exits to close. stty
// restores the terminal the chain left with signal generation off.
  NEW: // parkedChainBackstop leaves the pane at a shell when the tail could not start
// at all. It keys on the shell's could-not-run statuses, not on any failure: a
// tail that recovered the pane exec'd the user's shell, whose exit status
// arrives here, and a second shell after it would make the pane take two exits
// to close. stty restores the terminal the chain left with signal generation off.
- cmd/state_resume_chain.go:62 — after the stale claim was removed, what remains only restates the function name.
  OLD: // resumeChainArgv composes one chain command's argv.
  NEW: 

NOTES:
- `TestStateResumeRecoverCommand_PaneFromEnvironment` checks `cfg.Pane`. It cannot see which target the production `ReadMarker`/`ClearMarker` closures address, because the probe replaces them. This is correct today because `target` is derived after the fallback (cmd/state_resume_recover.go:78-83). A future reorder that derived `target` first would not be caught.
- CLAUDE.md's "Resume hooks" section still describes the parked chain as `/bin/sh -c '<draw argv>; <recover argv>'`. It now ends with the backstop too; that is doc drift, not a code comment.
- An answered pane whose binary was later removed from the baked path (e.g. `brew uninstall`) gets a second shell on exit, because the tail returns 127 and the backstop runs. This follows the spec's ranking: when the tail cannot run, it cannot tell the pane was answered, and an extra shell is the lesser failure.
- The "Addition from the orchestrator" arrived marked with its origin as task content, and I reviewed against it. `stty sane` exists in macOS BSD stty and runs before the exec, and a test covers it. Nothing else rode the dispatch beyond the enumerated inputs.
