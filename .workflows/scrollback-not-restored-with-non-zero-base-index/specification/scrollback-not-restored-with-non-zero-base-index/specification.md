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

When tmux fires `client-attached` / `client-session-changed` for a session whose name begins with `-` (e.g. `-dotfiles-HM9Zhw`), the resolved shell command becomes `portal state signal-hydrate -dotfiles-HM9Zhw`. cobra/pflag parses the leading-dash token as a short-flag cluster, fails with `unknown shorthand flag: 'd'`, and exits non-zero before `runSignalHydrate` executes. No FIFO byte is written; the hydrate helper times out at 3s (`openFIFOWithTimeout` at `cmd/state_hydrate.go:100`) and exec's a bare `$SHELL` (`handleHydrateTimeout` at `cmd/state_hydrate.go:248`) with no scrollback replay.

The parse-error text is written to stderr, which `tmux run-shell` captures into its own output stream rather than `portal.log`. As a result, the failure produces no Portal log line — the only observable artefact is the downstream `hydrate timeout` WARN.

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

**Potentially affected:**
- Any other Portal subcommand invoked from a tmux hook with `#{session_name}` as a positional arg. `signalHydrateCommand` is currently the only such site (per `internal/tmux/hooks_register.go`); `notifyCommand` is argument-free and unaffected.
- User-issued `portal <subcommand> -<dashed-name>` from a shell prompt — same parse-failure class. **Not addressed** by the chosen fix: the `--` separator is added only to the hook command, so a user invoking the CLI manually with a leading-dash positional argument would still hit the parse error. This case is intentionally out of scope (see Out of Scope below).

## Fix Scope

The fix has two parts. Both are required: Part 1 stops the hydration failure; Part 2 removes the misleading diagnostic that the bug report mistook for the cause.

### Part 1 — Add `--` End-Of-Flags Separator to `signal-hydrate` Hook

**Change:** `internal/tmux/hooks_register.go:39` — update `signalHydrateCommand` so the resolved hook command is:

```
run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate -- #{session_name}"
```

The `--` token tells cobra/pflag to stop flag parsing and treat `#{session_name}` as a positional argument unconditionally, regardless of its first character.

**One-shot bootstrap migration:** Existing installs already have a hook entry written without `--`. Hook registration is idempotent (skips if an entry matching the current command substring is found), so without migration the new fixed entry would either be added alongside the old broken entry (both fire — broken one still errors) or rejected as "already present" (depending on dedupe substring). Both outcomes leave users broken.

The existing `RegisterPortalHooks` step (bootstrap step 2) must evict any hook entry whose command contains `portal state signal-hydrate` but does **not** contain the `--` separator before installing the fixed entry. The dedupe substring used to detect whether a hook is already present must be tightened to `portal state signal-hydrate --` so the migration distinguishes the fixed entry from the broken one.

The migration runs at every bootstrap (idempotent) — once a user's install has only the fixed entry, subsequent bootstraps perform no eviction work.

### Part 2 — Delete `PredictLiveIndices` and Its Consumers

Delete the dead diagnostic prediction path entirely:

- `internal/restore/session.go::PredictLiveIndices` — function and its helpers `readIndexOption` (if unused after removal).
- `internal/restore/session.go::flattenSavedPanePositions` — only consumer was `warnOnPaneKeyDrift`.
- `internal/restore/restore.go::Orchestrator.warnOnPaneKeyDrift` — the diagnostic itself.
- Any call site in the orchestrator's restore loop that invokes `warnOnPaneKeyDrift`.
- Tests covering the deleted functions.

**Rationale for deletion over repair:** The function exists only to power a diagnostic WARN with no functional consumer. The spec's "Index Semantics" section mandates "re-query live indices, never predict" — a repaired predictor would buy a marginal post-restore drift signal at the cost of new tmux-client surface area (session-scope and window-scope option getters) and continued conceptual tension with the spec mandate.

If post-restore drift visibility ever becomes valuable, a saved-vs-live comparison is the better shape than predicted-vs-live and can be added later without resurrecting prediction. Pane-count mismatch is already logged at `armPanes:202`, providing a coarser but consistent signal.

### Out of Scope

The following were considered and explicitly excluded from this fix:

- **Renaming `SanitiseProjectName`'s `.` → `-` substitution to `_` or another safe char.** Fixes one symptom (no more leading-dash names from dotfiles projects) but leaves the broader class — any user-issued or scripted invocation passing `-anything` to a hook-invoked Portal subcommand would still break. Also a backwards-incompatible change for existing users whose projects/sessions use the current scheme. Worth re-evaluating in a separate, larger discussion later — not as a fix for this bug.
- **Pass session via env var or `set-environment` instead of positional argv.** Most robust to weird names (quotes, semicolons, etc.) but requires invasive run-shell setup. Overkill for the constrained name alphabet Portal generates; `--` solves the actual observed class.
- **`cobra.Command.DisableFlagParsing = true` on `stateSignalHydrateCmd`.** Works but loses the ability to add real flags later and is less intent-preserving than `--`.
- **Repairing `PredictLiveIndices` to read `base-index` from session scope and `pane-base-index` from window scope.** Considered and rejected for the reasons above (no functional consumer, conflicts with spec mandate).

## Acceptance Criteria

The fix is complete when all of the following hold:

1. **Hydration succeeds for leading-dash session names.** After `tmux kill-server` and reattach, a Portal-managed session whose name begins with `-` (e.g. `-dotfiles-HM9Zhw`) has its saved scrollback replayed into each pane. No `hydrate timeout` WARN appears in `~/.config/portal/state/portal.log`. This holds regardless of `base-index` / `pane-base-index` values in the user's tmux config.

2. **`signal-hydrate` accepts leading-dash session names from a tmux hook.** Manual verification: `portal state signal-hydrate -dotfiles-HM9Zhw` invoked via `run-shell` (the hook's invocation context) exits 0 and writes the FIFO byte. The previous parse error (`unknown shorthand flag: 'd'`) no longer occurs.

3. **Existing installs are migrated on first bootstrap.** After upgrading, the next Portal command that runs the bootstrap orchestrator removes any pre-existing hook entry that lacks the `--` separator and installs the fixed entry. Subsequent bootstraps are no-ops.

4. **No misleading `predicted=...__0.0 live=...__X.Y` WARN appears in `portal.log`** under any tmux config. The diagnostic is gone, not silenced.

5. **No regression in the existing live-index path.** Sessions whose names do not start with `-` continue to restore and hydrate as before. Pane-count mismatch logging at `armPanes:202` is preserved.

## Testing Requirements

The following test coverage must be in place before the fix is considered complete:

1. **Cobra-level argv parse test for `signal-hydrate`.** A unit test exercising `runSignalHydrate` end-to-end via the cobra `Execute()` path (not direct `signalHydrateConfig` construction) with a session name starting with `-`. Today's tests in `cmd/state_signal_hydrate_test.go` bypass argv parsing by calling the run function directly; they would not have caught this bug. The new test must drive the cobra command tree the same way production does.

2. **Reboot round-trip integration coverage with leading-dash session name.** Extend `cmd/bootstrap/reboot_roundtrip_test.go` (or add a sibling integration test) using a session name that begins with `-` to exercise the full hook-firing path: client-attached fires → `signal-hydrate` runs via `run-shell` → FIFO byte written → helper unblocks → scrollback replays. The existing test's session names ("alpha", "beta") would not have surfaced this failure.

3. **Hook content unit test.** A test asserting that `signalHydrateCommand` includes the `--` separator before `#{session_name}`, so future edits to the constant cannot silently regress the fix.

4. **Migration test for `RegisterPortalHooks`.** A unit test verifying that bootstrap evicts a pre-existing hook entry containing `portal state signal-hydrate` without `--` and installs the fixed entry. A second invocation of the same bootstrap step must be a no-op (idempotent).

### Testing Constraint — Do Not Restart The Active Tmux Server

The tmux server hosting the developer's working session must **not** be killed (`tmux kill-server`) as part of executing tests or manual verification. Doing so terminates the running session and halts work in progress.

Reboot round-trip tests and any manual reproduction must use a **separate, isolated tmux server** — typically by pointing tmux at a dedicated socket via `tmux -L <test-socket>` (or equivalent fixture, e.g. `internal/tmuxtest`'s real-tmux socket helper). The `kill-server` step exercised by these tests targets only the test socket; the developer's primary tmux server is unaffected.

This constraint applies to all automated tests, manual repro steps documented in PRs, and any debugging session.

---

## Working Notes
