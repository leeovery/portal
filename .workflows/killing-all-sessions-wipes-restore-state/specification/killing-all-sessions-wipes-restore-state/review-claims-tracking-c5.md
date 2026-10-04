# Review Tracking: killing-all-sessions-wipes-restore-state - Claims Verification

## Findings

### 1. The commands that undo the uninstall cover include `portal completion`

**Source**: Tree measurement — `cmd/root.go:23-34` and `:88-93`, cobra v1.10.2 `completions.go:745-780` and `command.go:955-990`, `portal help`
**Category**: Source defect
**Move**: route
**Affects**: §5.2 Accepted residue — "The first reboot after installing the fix"

**Problem**:
The cover for the first reboot after an upgrade tells the user to run `portal uninstall` and then avoid every Portal command that bootstraps until after the reboot. It names those commands as `portal open` (and `x`), `portal list` and `portal kill`, and presents that as the full list. There is a fourth: `portal completion <shell>`. Cobra adds that command automatically, and it is not in the bootstrap-exempt set, so it runs the same bootstrap as the other three. On an unchanged version it revives the save daemon. After an upgrade it runs a full bootstrap, which also re-registers the `session-closed` hook. A user who regenerates their completion script between `portal uninstall` and the reboot brings back the very savers the cover is meant to keep away. Their resume-hook sessions can then lose their sessions and scrollback at that reboot. Nothing tells them why: they only find the sessions missing after they log in.

**Evidence**:
Claim (§5.2): "Running `portal uninstall` before that reboot, and running no Portal command that bootstraps until after it, covers it. Those commands are `portal open` (and the `x` shell function), `portal list` and `portal kill` (`rg -n 'rootCmd\.AddCommand\(' cmd -g '!*_test.go'` → 11 commands, of which `open`, `list` and `kill` sit outside the 10-entry bootstrap-exempt `skipTmuxCheck` set in `cmd/root.go`). Each one revives the save daemon, and after an upgrade it also re-registers the `session-closed` hook."

Measurements:
- `rg -n 'rootCmd\.AddCommand\(' cmd -g '!*_test.go'` → 11 hits (init, doctor, state, kill, uninstall, open, hook, version, theme, list, alias). The count is right, but this command only sees commands registered explicitly.
- `rg -n 'CompletionOptions|DisableDefaultCmd' --type go .` → no matches. So cobra's `InitDefaultCompletionCmd` (`cobra@v1.10.2/completions.go:745-780`, called from `command.go:1113`) adds a `completion` command with `bash`/`zsh`/`fish`/`powershell` subcommands.
- `portal help` (installed 0.12.0) → lists `completion  Generate the autocompletion script for the specified shell`.
- `cmd/root.go:23-34`: `skipTmuxCheck` = `__complete, alias, doctor, help, hook, init, state, theme, uninstall, version`. It does not contain `completion`.
- `cmd/root.go:88-93`: `for c := cmd; c != nil; c = c.Parent() { if skipTmuxCheck[c.Name()] { return nil } }`. The chain zsh → completion → portal contains no exempt name, so the check falls through to `CheckTmuxAvailable`, then the latch, then `ensureSaverLiveness` (`cmd/abridged_saver.go:13-24`, which revives `_portal-saver`) or the full bootstrap. The full bootstrap runs when the version latch does not match, as it does after an upgrade.
- `cobra@v1.10.2/command.go:955-990`: any runnable subcommand runs the first `PersistentPreRunE` found walking up its parents, which here is root's.
- `cmd/init.go:95,128,161`: `portal init` generates completions in-process through `rootCmd.Gen*Completion`, so the shell-init path does not call `portal completion`. Only a user running it directly is exposed.

Source: the investigation carries the same three-command list in two places: "Current workaround" (line 65: "`x` / `portal open`, `portal list` or `portal kill`, each of which revives the save daemon…") and the accepted-residue paragraph under the fix (line 228: "no Portal command that bootstraps — `x` / `portal open`, `portal list`, `portal kill` — until after the reboot").

**Proposed Text**:

**Resolution**: Routed
**Notes**: Measurement re-run: no CompletionOptions/DisableDefaultCmd in cmd, and `portal help` lists `completion`, which is outside skipTmuxCheck. The uninstall-first cover survives with the command added. Corrected in the investigation (References workaround; Known residue, accepted), then §5.2's first-reboot bullet re-aligned.

---

## Observations

- §2.2's empty-`list-panes` damage path cannot happen against a single exiting server. The `show-environment` reads come after the pane listing, so they are refused ("server exited unexpectedly", exit 1). Capture counts those as anomalous and errors out once every session fails. The path is reachable only when those reads reach a second server, which the same-server rule already covers.
- §6.6 says the listing-failure subtest asserts "an error the production client never delivers". The production `ListSessionNames` does return parse errors (`parseSessionList`, `internal/tmux/tmux.go`). The error it never delivers is a failed tmux invocation.
- The subtests named in §5.1 and §6.6 are registered with an "it " prefix (`internal/state/capture_test.go:759,1614,1672`). The quoted names still match as substrings.
- tmux 3.7c sets `server_exit` in the shared SIGINT/SIGTERM case of `server_signal` (`server.c:437-440`), not in a handler for SIGTERM alone. The "never stops once started" property holds.
