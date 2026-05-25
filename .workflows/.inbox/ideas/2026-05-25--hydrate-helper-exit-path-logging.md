# Hydrate helper exit-path logging

During investigation of a reboot where some Claude `--resume` hooks fired and others didn't, `portal.log` couldn't tell us which path each helper actually took. Only the warning paths emit log lines today — the success and lookup-decision paths are silent — so for any pane whose hook didn't visibly resume, we had to guess between several mutually-exclusive failure modes purely from circumstantial evidence (pane's current command, scrollback timestamps, hooks.json deltas).

The concrete gap is in `cmd/state_hydrate.go`. `runHydrate` has three terminal points that fall through to `execShellOrHookAndExit`:

- the silent ENOENT exit at line ~120 (helper opened FIFO and got "no such file or directory" — never reaches the hook)
- the timeout path at line ~115 (helper waited 3 s, gave up, fires hook)
- the file-missing path at line ~147 (scrollback couldn't be read, fires hook)
- the success path at line ~188 (signal arrived, scrollback dumped, fires hook)

Each of those should emit a single INFO line including `hook-key` and the resolved hook command (or "<none>" if no hook registered). `execShellOrHookAndExit` itself should also log lookup decisions — hit vs miss vs error — so we can distinguish "hooks.json drifted from the saved hook-key" from "helper never reached the lookup."

After this, `grep 'hydrate:' portal.log` would give a complete per-pane audit trail of the resurrection. The recent uTLWWz-vs-a2vfgB analysis would have been a five-second log-grep instead of a multi-step reconstruction across `tmux list-windows`, scrollback mtimes, and `hooks.json` diffs.

There's an architectural limit worth flagging in the idea: Portal exec's the hook via `syscall.Exec`, replacing the helper process. So Portal will never see the hook command's own exit status (e.g. whether `claude --resume <UUID>` actually launched Claude or exited immediately with an invalid-session error). Capturing that would require wrapping the exec'd command in a shell envelope that records exit status before chaining to `$SHELL` — a separate, more invasive change with its own correctness considerations. The cheap exit-path logs above are the high-signal-per-LOC win regardless of whether the wrapper idea is later pursued.

The investigation context that surfaced this is preserved in `MEMORY.md` (`project_reboot_hooks_followup`); this idea is the actionable improvement that fell out of it.
