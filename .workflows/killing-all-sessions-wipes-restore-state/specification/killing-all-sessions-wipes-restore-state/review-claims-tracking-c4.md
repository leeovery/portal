# Review Tracking: killing-all-sessions-wipes-restore-state - Claims Verification

## Findings

### 1. The pre-reboot workaround leaves out other commands that bring Portal's save machinery back

**Source**: Tree measurement — `cmd/root.go:23-34` (bootstrap-exempt set), `cmd/root.go:109-157` (pre-run), `cmd/abridged_saver.go:13-24`, `cmd/uninstall.go:52-63`
**Category**: Source defect
**Move**: route
**Affects**: §5.2 Accepted residue — "The first reboot after installing the fix"

**Problem**:
The specification tells the user how to get through the first reboot after upgrading safely: run `portal uninstall` first, and don't run `portal open` until after the reboot. That procedure only works while nothing brings the save daemon or the `session-closed` commit hook back before the reboot. `portal open` is not the only command that does. `portal list` and `portal kill` run the same pre-run step:
- When the version that bootstrapped the server no longer matches, which is the case straight after `brew upgrade`, they run a full bootstrap. That re-registers the hooks and starts a new daemon.
- When the version still matches, they bring the killed save daemon back, because `portal uninstall` leaves the bootstrapped marker in place.

So a user can follow the advice to the letter, run `portal list` to check their sessions before rebooting, and reboot with commits running again. The resume-hook panes have not been rebuilt yet, so they can still lose their sessions and scrollback. The user only finds out after the reboot, when those sessions or their scrollback are missing. The safe procedure has to rule out every command that bootstraps, not just `portal open`.

**Evidence**:
Claim (§5.2): "Running `portal uninstall` before that reboot, and not running `portal open` again until after it, covers it."

Measurements:
- `cmd/root.go:23-34`: the bootstrap-exempt set is `__complete`, `alias`, `doctor`, `help`, `hook`, `init`, `state`, `theme`, `uninstall`, `version`. `list` and `kill` are not in it. Both are registered user commands: `cmd/list.go:43` `Use: "list"`, `cmd/kill.go:25` `Use: "kill [name]"`.
- `cmd/root.go:109-111`: `if latchSatisfied { … ensureSaverLiveness(client, stateDir)`. In `cmd/abridged_saver.go:16-20`, an absent saver leads to `tmux.BootstrapPortalSaver(client, stateDir)`, which revives the daemon.
- `cmd/root.go:135` `started, warnings, err := runBootstrap(cmd.Context(), runner)` and `cmd/root.go:157` `if err := registerHooks(client); err != nil {` run on the path where the version does not match: full bootstrap plus hook registration, including the `session-closed` → `portal state commit-now` body (`internal/tmux/hooks_register.go:84`).
- `cmd/uninstall.go:56-62`: uninstall runs only `killSaver(client, logger)` and `unregister(client)`. Nothing clears `@portal-bootstrapped` (`internal/state/markers.go:21`), so a later same-version `portal list` takes the warm path and revives the saver.

Source: the investigation, Fix Direction → Chosen Approach (line 228): "the verified safe-reboot procedure (`portal uninstall` first, no `x` / `portal open` until after the reboot) covers it".

**Proposed Text**:

**Resolution**: Routed
**Notes**: Measurement re-run: of the 11 top-level commands, `open`, `list` and `kill` sit outside the bootstrap-exempt set; the uninstall-first conclusion survives with the full command set. Corrected in the investigation (Symptoms → References workaround; Known residue, accepted), then §5.2's first-reboot bullet re-aligned.

---

## Observations

- §2.1 says "every reader shares one session listing that swallows the failure". The search form's count already reads through `ListSessionsProbe` (`cmd/open_search.go:178`). The decision to leave the shared method alone is unaffected.
- §2.1's recorded result for `rg -n 'ListSessionNames\(\)' internal/state/capture.go internal/restore/restore.go` leaves out a third hit, `capture.go:20` (the `CaptureClient` interface declaration). The two call sites it names are correct.
- §3.1's `rg -n '"sh", "-c"' … | wc -l` → 2 only sees `sh -c` written as separate argv elements. The `_portal-saver` placeholder `sh -c 'exec tail -f /dev/null'` (`internal/tmux/portal_saver.go:30`) is a single-string `sh -c` pane. It execs straight into `tail` and the daemon replaces it, so the claim still holds.
- The subtest names in §5.1 and §6.6 are missing the leading "it " that the tree has (e.g. "it returns an empty index with nil error when keep is empty after filtering"). A substring search still finds each one.
- §3.3 says "tmux 3.7c sends no signal to a pane's process itself". That holds on the kill path. Elsewhere tmux does SIGCONT a stopped pane process (`server.c:522`, `server_child_stopped`).
