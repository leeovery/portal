# Specification: Scrollback Not Restored With Non-Zero Base Index

## Specification

## Problem & Root Cause

### Observed Symptom

After `tmux kill-server` and reattach, Portal restores sessions/windows/panes, cwd, and layout, but saved scrollback never appears in the pane. `~/.config/portal/state/portal.log` shows two correlated lines per pane:

```
WARN | restore | session "<name>": pane 0 predicted=<name>__0.0 live=<name>__1.1
WARN | hydrate | timeout waiting for signal on --hook-key=<name>:1.1 --fifo=...
```

The bug report attributed this to non-zero `base-index` / `pane-base-index`. That framing is incorrect: base-index is a confound that surfaces a misleading diagnostic WARN, not the cause of hydration failure.

### Primary Root Cause — Leading-Dash Session Name Breaks `signal-hydrate` Argv Parsing

`internal/tmux/hooks_register.go:39` defines the global hook command as:

```
run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"
```

When tmux fires `client-attached` / `client-session-changed` for a session whose name begins with `-` (e.g. `-dotfiles-HM9Zhw`), the resolved shell command becomes `portal state signal-hydrate -dotfiles-HM9Zhw`. cobra/pflag parses the leading-dash token as a short-flag cluster, fails with `unknown shorthand flag: 'd'`, and exits non-zero before `runSignalHydrate` executes. No FIFO byte is written; the hydrate helper times out at 3s and exec's a bare `$SHELL` with no scrollback replay.

Leading-dash session names arise because `internal/session/naming.go::SanitiseProjectName` (line 24) replaces `.` and `:` with `-`. Project basenames like `.dotfiles` or `.config` become `-dotfiles` / `-config`, then `GenerateSessionName` appends a 6-char nanoid yielding `-dotfiles-HM9Zhw`.

**Empirical verification:**

```
$ portal state signal-hydrate -dotfiles-HM9Zhw      → exit 1 (parse error)
$ portal state signal-hydrate myrepo-AbCdEf         → exit 0
$ portal state signal-hydrate -- -dotfiles-HM9Zhw   → exit 0
```

`stateSignalHydrateCmd` defines no flags of its own (cmd/state_signal_hydrate.go:132), but cobra inherits parent persistent flags and pflag still attempts to parse leading-`-` tokens as short-flag clusters.

### Secondary Root Cause — `PredictLiveIndices` Reads Wrong Tmux Option Scope (Diagnostic-Only)

`Orchestrator.warnOnPaneKeyDrift` (`internal/restore/restore.go:153`) calls `SessionRestorer.PredictLiveIndices()` (session.go:424), which reads `base-index` and `pane-base-index` via `client.GetServerOption(...)`.

`GetServerOption` queries tmux **server-scope** options (via `show-options -sv`). However:
- `base-index` is a **session option** (`set -g` writes the global session value).
- `pane-base-index` is a **window option** (`setw -g` writes the global window value).

Neither is a server option. `GetServerOption` always returns `ErrOptionNotFound`, so `readIndexOption` falls back to `0` for both. `PredictLiveIndices` therefore returns `(0, 0)` regardless of user config.

Whenever the user has non-zero `base-index`/`pane-base-index`, the live key differs from the always-zero predicted key, and `warnOnPaneKeyDrift` fires. The WARN is **non-causal** — it does not affect any FIFO path, marker, or hook handshake — but it consistently misdirects diagnostic attention toward "prediction vs live drift" when the actual failure is the argv parse.

`PredictLiveIndices` has no functional consumer beyond this diagnostic WARN.

### Why the End-to-End Path Otherwise Works

The implementation already follows the spec's "Index Semantics" section (`internal/restore/session.go`):
- `armPanes` (session.go:195) calls `ListPanesInSession` to get live `[]tmux.PaneCoord` after `new-session` / `split-window` / `new-window`.
- FIFO path (session.go:215) is built from `state.SanitizePaneKey(sess.Name, live.Window, live.Pane)` — the **live** key.
- Helper is dispatched via `respawn-pane -k` against the live pane target.
- `ApplySkeletonMarkers` (session.go:354) iterates live panes and writes `@portal-skeleton-<liveKey>` for each.

The helper waits at `hydrate-<sess>__<live>.fifo`, the marker is set at `@portal-skeleton-<sess>__<live>`, and `signal-hydrate` enumerates live panes via `list-panes -s` to compute the same live key. The live-index path is end-to-end consistent and would succeed under base-index drift in isolation. Hydration only fails because `signal-hydrate` exits before doing any work for leading-dash session names.

### Blast Radius

**Directly affected:** Any session whose name starts with `-`. Includes Portal-generated names from projects whose basename begins with `.` or `:` (after `SanitiseProjectName`'s substitution — `.dotfiles`, `.config`, etc.).

**Potentially affected:** Any other Portal subcommand invoked from a tmux hook with `#{session_name}` as a positional arg. `signalHydrateCommand` is currently the only such site (per `internal/tmux/hooks_register.go`); `notifyCommand` is argument-free and unaffected.

---

## Working Notes
