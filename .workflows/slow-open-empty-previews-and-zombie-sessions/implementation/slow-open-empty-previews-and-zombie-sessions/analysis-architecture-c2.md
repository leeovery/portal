# Architecture Analysis — Cycle 2 (independent re-scan)

STATUS: findings
FINDINGS_COUNT: 5

## Finding 1: Two divergent saver-pane-PID helpers with overlapping responsibility

SEVERITY: medium

FILES:
- `internal/tmux/saver_pane_pid.go:44` (`SaverPanePID(c, name)`)
- `internal/tmux/tmux.go:575` (`Client.FirstPanePIDInSession(name)`)
- `internal/bootstrapadapter/orphan_sweep.go:124`
- `cmd/state_daemon.go:103`

DESCRIPTION: Two near-identical helpers read the first pane PID of a tmux session via `list-panes -t =<name> -F '#{pane_pid}'`:
- free function `tmux.SaverPanePID(c, name)` (Component D probe) — returns typed sentinels `ErrNoSuchSession` / `ErrEmptyPaneList` / `ErrPanePIDParse`.
- method `Client.FirstPanePIDInSession(name)` (Component B adapter) — returns `(0, nil)` for empty-output and unwrapped exec error otherwise.

Differ in (i) `-s` flag presence, (ii) empty-output shape, (iii) "no such session" sentinel wrapping. The adapter at orphan_sweep.go:124 even calls `HasSession` first to recapitulate logic `SaverPanePID` does internally.

RECOMMENDATION: Collapse to one helper. Either make `FirstPanePIDInSession` the lone primitive (Component D's probe maps every failure mode to "absent" anyway), or promote `SaverPanePID` to a `*Client` method and delete `FirstPanePIDInSession`.

(Overlaps with Duplication Finding 3 — same root cause.)

## Finding 2: portal_saver.go seam surface is wide and uses three different setter idioms

SEVERITY: low

FILES:
- `internal/tmux/portal_saver.go:67-278`
- `internal/tmux/export_test.go:1-127`

DESCRIPTION: ~18 package-level mutable seams + ~13 `*Seam()` accessors + two `Set*` setter functions. Setter idiom is inconsistent within one file: (a) bare var with no setter, (b) `Set*` function with nil-guard, (c) `*Seam()` returning `*Func` for direct write.

RECOMMENDATION: Bundle related seams into one or two seam structs (e.g. `killBarrierSeams{...}`, `saverReadinessSeams{...}`) so tests swap whole clusters atomically, and pick ONE setter idiom uniformly.

## Finding 3: killBarrierLogger is reused as the readiness-barrier WARN sink — name lies about scope

SEVERITY: low

FILES:
- `internal/tmux/portal_saver.go:227` (declaration)
- `internal/tmux/portal_saver.go:440` (readiness-barrier emission site)

DESCRIPTION: `waitForSaverDaemonReady` (Component F readiness barrier — a distinct concept from the kill barrier) emits its timeout WARN through `killBarrierLogger.Warn(...)`. A maintainer searching for readiness-barrier WARN emission sites won't grep "killBarrier" and may miss this site.

RECOMMENDATION: Rename to `saverBarrierLogger` (or `portalSaverLogger`) with `SetBarrierLogger` / `BarrierLogger` interface renamed in lockstep.

## Finding 4: identifyPS uses fmt.Sprintf where strconv.Itoa suffices

SEVERITY: low

FILES:
- `internal/state/daemon_identity.go:56`

DESCRIPTION: `fmt.Sprintf("%d", pid)` for a single int pulls fmt's reflection machinery where `strconv.Itoa(pid)` would format in one allocation. The file's docstring elsewhere emphasises low-overhead identity checks. Pure cosmetic.

RECOMMENDATION: Replace `fmt.Sprintf("%d", pid)` with `strconv.Itoa(pid)`.

## Finding 5: NewIsolatedStateEnv mutates the calling test process env behind a constructor-shaped name

SEVERITY: low

FILES:
- `internal/portaltest/isolated_env.go:58-59`

DESCRIPTION: `NewIsolatedStateEnv(t)` calls `t.Setenv("HOME", ...)` and `t.Setenv("XDG_CONFIG_HOME", "")` on the caller's process before returning the subprocess env-slice. Correct for the host-noise mitigation use case (fully documented) but the API conflates "build subprocess env" with "scrub parent env so the fingerprint backstop stays quiet". Acceptable as-is given uniform caller intent.

RECOMMENDATION: Optional rename to `SetupIsolatedStateEnv(t)`, or split into `scrubHostEnv(t)` + `BuildIsolatedEnv(t)`. Low-priority cosmetic.
