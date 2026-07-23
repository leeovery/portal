# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.10.3] - 2026-07-23

🐛 Fixed

- Fixed multi-window spawn opening new terminal windows on the wrong machine when a remote client shared a tmux session with a local one — Portal now identifies the client that actually triggered the burst and drives from that machine, not just any local client attached to the session.

## [0.10.2] - 2026-07-22

✨ Added
- New `sessions-unsupported-null` capture fixture — pins the "remote client with no local terminal" screen state for visual regression checks.

🔧 Changed
- Unsupported-terminal messaging now speaks plainly — "can't open new windows over a remote connection" / "can't open new windows in `<name>` · `<bundleID>`" — instead of the old "no host-local terminal" / "unsupported terminal" phrasing.
- The unsupported-terminal banner now shows only for a named terminal (e.g. Terminal.app); a remote/mosh connection with no local terminal keeps the normal `Sessions ··· N` header instead of a banner with nothing actionable in it.
- Pressing `m` for multi-select on an unsupported terminal now shows a clear "isn't available" message and refuses to enter the mode, instead of letting you mark sessions for a burst that can never open — the `m` entry also drops out of the `?` help in that state.

## [0.10.1] - 2026-07-21

🐛 Fixed

- Ghostty windows spawned from a multi-window open no longer dead-end on "Process exited. Press any key to close the terminal." — they now drop into your interactive login shell when the session command finishes.

## [0.10.0] - 2026-07-20

✨ Added
- `x work api db` opens several targets at once, each in its own host-terminal window, in one command.
- Domain-pinning flags on `x`/`portal open`: `-s/--session`, `-p/--path`, `-a/--alias`, `-z/--zoxide` force resolution into one specific domain, and `-f/--filter` skips resolution to open the picker pre-filtered.
- `portal doctor` gives a scriptable health report (daemon, saver, hooks, state dir, sessions.json, stale entries, host terminal); `portal doctor --fix` applies the low-stakes repairs and re-checks.
- Tab completion for session names (on `open`/`kill`) and alias keys (on `open --alias`).
- Exact session names and glob patterns (e.g. `x 'api-*'`) now attach directly through `x`/`portal open` instead of always minting a new session.

🔧 Changed
- `x`/`portal open` now hard-fails with a clear message when a target resolves to nothing, instead of silently falling back to the picker.
- `xctl hooks` is renamed to `xctl hook` (singular); `hooks` is kept as a permanent, silent alias so existing scripts keep working.
- `portal uninstall` replaces `portal state cleanup`: it only stops the save daemon and removes global tmux hooks, and never touches any files — saved sessions and config always survive.
- Stale project entries are now pruned automatically in the background by the save daemon, in addition to the manual `doctor --fix` sweep.
- The `portal state` command tree is now hidden from `--help` and shell completion (it's internal plumbing, still fully usable).

🗑️ Removed
- `portal attach` is retired — the picker's spawned burst windows and manual attaches now go through `portal open --session <name>`.
- `portal spawn` is retired — multi-window opening is now built into `x`/`portal open` (multi-target) and the picker's multi-select.
- `portal clean` is retired — stale-project/hook pruning and log sweeping now live under `portal doctor --fix`.
- `portal state status` is retired — replaced by `portal doctor`.
- `portal state cleanup --purge` is retired along with the rest of `state cleanup`; `portal uninstall` no longer offers a data-wiping option.

## [0.9.2] - 2026-07-17

🐛 Fixed

- Spawning a session in Ghostty no longer opens zero windows — the AppleScript now uses the terminal's actual scripting API.
- Session names or paths containing spaces are no longer shredded into separate words when spawning a new terminal window.
- The "failed to open" message after a spawn failure now says "nothing opened" when no windows opened at all, instead of incorrectly claiming others were left open.
- Failed spawn windows are now logged as warnings instead of being silently buried at debug level.

## [0.9.1] - 2026-07-15

✨ Added
- Multi-window spawn — mark several sessions with `m` in the picker and press `Enter` to open each in its own host-terminal window, rebuilding a full window layout in one action.
- `xctl spawn` command — the CLI form of multi-select, with a `--detect` flag to print the host terminal Portal detects (useful for writing `terminals.json` recipes).
- `terminals.json` config — teach Portal how to spawn windows on terminals other than Ghostty via a user-authored recipe file.

🔧 Changed
- Entering multi-select mode with `m` now marks the currently-highlighted session immediately, so opening just that one session takes a single press instead of two.

## [0.9.0] - 2026-07-14

✨ Added
- `portal spawn` opens multiple restored sessions in separate host-terminal windows on supported terminals, with `--detect` to check what's supported.
- Multi-select mode on the sessions list (`m` to mark rows, `Enter` to open) lets you launch several sessions at once, spawning extras in new host windows and attaching the last one in place.
- Ghostty is natively supported for host-window spawning, with a `terminals.json` escape hatch to configure custom recipes for other terminals.
- A proactive banner warns when the host terminal doesn't support spawning extra windows, before you try to use it.

🔧 Changed
- `portal attach` gained an internal `--spawn-ack` flag used by spawned windows to confirm they came up.

🐛 Fixed
- A spawn burst that partially fails now leaves already-opened windows alone and keeps the failed sessions marked so you can retry, instead of losing track of them.
- Canceling a spawn in progress (Ctrl-C/Esc) now reliably returns you to the picker instead of occasionally locking up input.

## [0.8.5] - 2026-07-12

🐛 Fixed
- Fixed a rare startup deadlock where the daemon could hang the first time the log rolled over to a new day.

## [0.8.4] - 2026-07-05

🔧 Changed
- Once a server has fully bootstrapped, later commands (open, attach, etc.) now take a fast path that just checks the background saver is alive instead of re-running full startup — noticeably quicker warm-command startup.
- Stale resume-hook entries are now pruned by the background daemon every ~10 seconds instead of during every command's startup sequence.

## [0.8.3] - 2026-07-01

✨ Added
- Renaming a session — from the picker's `r` modal or an external `tmux rename-session` — no longer orphans its resume hooks, so hooks keep firing on reboot recovery.

🐛 Fixed
- Sessions freshly created during restore no longer lose their stable identity after a single capture cycle, which previously risked misfiring or dropping resume hooks.

## [0.8.2] - 2026-06-26

🐛 Fixed

- Cold-boot restore now lands on the Projects page correctly — the session list was being evaluated against a stale pre-restore snapshot, causing the picker to open on Projects even when restored sessions existed.

## [0.8.1] - 2026-06-25

🔧 Changed

- Go module updated to Go 1.26, enabling use of modern standard library idioms throughout the codebase.
- Multiple string-splitting hot paths migrated to `strings.SplitSeq` and `strings.FieldsSeq` — avoids allocating intermediate string slices when iterating lines.
- Slice containment checks, map copies, and backward iteration replaced with `slices.Contains`, `maps.Copy`, and `slices.Backward` from the standard library.
- Clamped width and count calculations replaced with `min`/`max` builtins, removing repetitive if-guards across rendering code.

✨ Added

- `golangci-lint` configuration added (`.golangci.yml`), enabling the `modernize` linter alongside the standard set — run locally or from the release step to catch idiom drift.
- README hero image updated to show the animated cold-boot sequence, and a new feature-tour animation added to the Screenshots section.

## [0.8.0] - 2026-06-25

✨ Added

- Automatic light and dark canvas. Portal detects the terminal background via OSC 11 on first paint and matches it. A new `appearance` preference (`auto`, `light`, `dark`) in `prefs.json` overrides detection, and `NO_COLOR` disables colour entirely.
- An honest loading screen on cold start. When the tmux server is not already running, bootstrap now runs concurrently with the UI behind a real loading view (block `PORTAL` wordmark, progress bar, and a live step list) instead of a blank pause.

🔧 Changed

- The session and project picker has been fully reskinned to the new Modern Vivid theme: a Tokyo Night inspired palette over an owned, mode matched canvas (dark `#0b0c14`, light `#e1e2e7`) that fills the terminal gutter.
- Keymap revision: navigation is now arrow keys only (vim and page-jump aliases removed), `x` toggles between Sessions and Projects in both directions, `s` cycles session grouping (Flat, By Project, By Tag), `k` kills a session, `d` deletes a project, and `?` opens a per-page help modal.
- The terminal background is now restored when Portal exits, with a guard that prevents terminals which echo the canvas colour back (such as Ghostty) from being left tinted.
- The README has been restructured with screenshots of the new interface.
## [0.7.7] - 2026-06-17

🔧 Changed

- Bumped CI release workflow to use latest major versions of `actions/checkout`, `actions/setup-go`, and `goreleaser/goreleaser-action`.

## [0.7.6] - 2026-06-17

🔧 Changed

- The `release` script now delegates to the `mint` CLI — install `mint` via `brew install leeovery/tools/mint` to continue using it.
- GoReleaser is set to `keep-existing` release mode so manually authored release notes and bodies are preserved when binaries are uploaded.

🗑️ Removed

- Deleted the in-repo bug report for resume hooks lost on server restart — no longer needed as a tracking file.

## [0.7.5] - 2026-06-10

🗑️ Removed

- The file browser view has been removed from the TUI — the picker now has three views (session list, project picker, scrollback preview) instead of four.

## [0.7.4] - 2026-06-09

🐛 Fixed

- `KillSession` and `RenameSession` now use tmux's exact-match target syntax (`=name`) — prevents prefix collisions from silently operating on the wrong session when a session name is a prefix of another live session's name.

## [0.7.3] - 2026-06-09

🐛 Fixed

- `portal state status` recent-warnings display was broken — the log parser used a legacy pipe-delimited format that no longer matched the actual log output, so warnings were silently missed and the last-warning line showed raw log text instead of a clean summary.
- The "Recent warnings" last-entry display now shows `<LEVEL> <component>: <message>` instead of the full raw log line with timestamp and internal attributes.

## [0.7.2] - 2026-06-08

🔧 Changed

- Tags are now case-sensitive and stored exactly as typed — "Work" and "work" are distinct tags rather than being folded to lowercase.
- Grouped session rows are visually indented under their group headings, making the hierarchy clearer in By Project and By Tag views.

## [0.7.1] - 2026-06-08

🐛 Fixed

- Group headings in the sessions list are now real list rows, eliminating the bug where the title bar and cursor scrolled off the top of the terminal when many grouped sessions were displayed.
- Switching between Flat / By Project / By Tag views now resets to the first page and lands the cursor on the first session row rather than leaving it on a stale offset or a non-selectable header.
- Cursor navigation (up, down, g, G) no longer stops on group header rows — the selection always lands on a session.

🔧 Changed

- Tags added or removed in the project edit modal are now written to disk immediately on each keystroke (`Enter` to add, `x` to remove), so closing the modal with `Esc` never silently discards a tag change.
- New sessions opened with `portal open <path>` are now stamped with their origin directory at creation time, so the session stays correctly grouped even after the pane `cd`s elsewhere.
- The lazy directory fallback for ungrouped legacy sessions now caches the derived path in memory rather than writing it back to tmux, preventing permanent mis-grouping when a pane's working directory has drifted.

## [0.7.0] - 2026-06-08

✨ Added

- Session list groups — press `s` on the sessions page to cycle between Flat, By Project, and By Tag views; the last-used mode is remembered across launches in `prefs.json`.
- Directory tags — open the projects editor, focus the new Tags field with `Tab`, and type a tag then `Enter` to add it; `x` removes a highlighted tag. Every session opened in that directory inherits its tags.
- `prefs.json` config file — stores UI preferences (currently the session-list grouping mode); supports `PORTAL_PREFS_FILE` override and migrates from the legacy macOS path like other config files.
- `Session.Dir` field — `ListSessions` now reads `#{@portal-dir}` so each session carries its stamped directory, enabling O(1) project lookups without pane reads in grouped modes.
- `ActivePaneCurrentPath` tmux method — reads a session's active pane cwd via `display-message` for the lazy directory-resolution fallback used in grouped renders.

🔧 Changed

- New sessions are stamped with `@portal-dir` (the resolved git root) at creation time, so grouped renders take a fast map-lookup path rather than reading active pane cwd on every render.
- The projects-to-sessions transition (`s`/`x` on the projects page) now dispatches a session-list refresh so tag edits are reflected immediately on return.
- The edit-project modal Tab cycle now visits three fields — Name → Aliases → Tags → Name — instead of two.
- `ListSessions` format string extended to a four-field pipe-delimited format; the directory field is last so embedded pipes in paths are preserved via an unbounded `SplitN` slot.

🐛 Fixed

- Sessions opened with the `QuickStart` (exec-handoff) path were missing `@portal-dir` stamps, causing them to always land in the Unknown bucket in grouped modes — they now self-correct on the first grouped render via the lazy stamp-on-render fallback.
- In a persisted grouped mode, sessions arriving before projects were loaded always fell into the Unknown/Untagged catch-all; `ProjectsLoadedMsg` now triggers a re-group so sessions are placed under the correct heading once project data arrives.
- The `WithInsideTmux` construction path now routes through the mode-aware rebuild chokepoint, so an already-populated grouped list is grouped correctly and the current-session decoration composes with the mode title rather than overwriting it.

## [0.6.5] - 2026-06-04

🐛 Fixed

- Integration tests that spawned subprocesses (e.g. `portal list`) could silently wipe a developer's real `~/.config/portal/hooks.json` and `projects.json` — the symptom fixture now isolates all per-file config env vars alongside the state directory.

## [0.6.4] - 2026-06-04

🐛 Fixed

- Test isolation for `portal state` subcommands — each subtest now uses its own temp state dir, preventing `cleanup` and `cleanup --purge` cases from mutating the developer's real config directory.

## [0.6.3] - 2026-06-04

🔧 Changed

- The kill-barrier timeout ceiling is now a single exported constant (`KillBarrierTimeoutCeiling`) shared between production code and integration tests — eliminating the risk of the two silently drifting apart if the value changes.
- Integration tests that run on hardware where tmux `capture-pane` is too fast to exercise the cancellation path now skip cleanly instead of logging a non-fatal warning and continuing.

## [0.6.2] - 2026-06-03

🔧 Changed

- The `p` and `s` navigation hints in the TUI footer now show `p/x` and `s/x` to reflect that `x` is an additional keybinding for switching between the sessions and projects pages.

## [0.6.1] - 2026-06-03

🔧 Changed

- Hook registration now reads each tmux event individually via `show-hooks -g <event>` instead of a single global enumeration — fixing silent hook accumulation on `pane-focus-out` and `window-layout-changed`, which tmux 3.6b omits from the no-arg global read.
- Bootstrap step 2 converges each managed event to exactly one Portal entry per run, collapsing any pre-existing duplicate stack in place rather than appending blindly — installations with hundreds of stacked hooks self-heal on the next `portal open`.
- `portal hooks reset` (unregister) now also reads per-event, so it correctly removes Portal hooks from the previously-blind events and continues tearing down remaining events when a single event's read fails.
- The `session-closed` hook now evicts stale `portal state notify` entries using substring matching rather than exact-string matching, converging them to `commit-now` as an ordinary side effect of bootstrap.

🐛 Fixed

- Duplicate `portal state notify` hooks stacking unbounded on `pane-focus-out` and `window-layout-changed` across repeated bootstraps — each tmux event that fires a managed hook now spawns exactly one `portal state notify` process instead of N.
- `portal hooks reset` left converged `session-closed commit-now` entries installed because the teardown fingerprint set omitted `portal state commit-now`; the fingerprint set is now derived from the registration table so no registered category can be un-reapable.

## [0.5.13] - 2026-05-28

🐛 Fixed

- Added a negative-control integration test for the eager-signal hydrate step — a regression that silently wired the no-op signaler would now be caught before reaching users.

## [0.5.12] - 2026-05-27

🐛 Fixed

- Stale hook cleanup no longer wipes `hooks.json` when `list-panes -a` returns an empty result — a mass-deletion hazard guard now defers the cleanup rather than treating an empty live-pane set as authoritative.
- `ListAllPanes` now propagates errors from tmux instead of silently returning an empty slice, letting callers distinguish a genuine tmux failure from a server with no panes.

🔧 Changed

- Stale hook cleanup logic is unified into a single shared helper (`runHookStaleCleanup`) consumed by both bootstrap step 11 and `portal clean`, so log format strings and failure-mode behaviour are identical at both call sites.
- `portal clean` hook cleanup now emits an auditable log breadcrumb to `portal.log` on every invocation, including a debug line when there are no hooks to clean.
- `bootstrap.NoopLogger` is now exported so adapter packages can reference it directly instead of declaring local no-op stand-ins.
- The canonical tmux structural-key format string (`#{session_name}:#{window_index}.#{pane_index}`) is now a single exported constant (`tmux.StructuralKeyFormat`) shared across all pane-enumeration and key-resolution call sites.

## [0.5.11] - 2026-05-26

🔧 Changed

- `portal hooks` commands (`list`, `set`, `rm`) no longer trigger the full tmux bootstrap orchestrator — they run as lightweight config-file operations, eliminating spurious ENOENT errors when hooks fire at session start.

## [0.5.10] - 2026-05-26

🐛 Fixed

- Session names containing whitespace, shell metacharacters (`$`, backtick, `;`), or other special bytes no longer break pane restore — the hydrate command arguments are now single-quoted so they survive word-splitting in the shell.
- Session name sanitization now replaces all non-alphanumeric, non-`._-` bytes (not just `/`, `\`, and NUL) with `_`, preventing shell-unsafe characters from leaking into pane keys, FIFO paths, and scrollback filenames.

## [0.5.9] - 2026-05-26

🔧 Changed

- `hooks rm --on-resume` accepts a `--pane-key <key>` flag to remove a hook for any pane by structural key — works outside tmux and allows pruning entries for panes that no longer exist.

## [0.5.7] - 2026-05-22

🔧 Changed

- The sessions and projects keymap footers are now rendered as a fixed three-column layout beneath each page — disabling the built-in bubbles/list help renderer in favour of a manually composed footer that stays visible and correctly sized at all terminal dimensions.

## [0.5.6] - 2026-05-22

✨ Added

- New `portal state commit-now` command writes `sessions.json` synchronously from live tmux state — invoked by the `session-closed` hook so killed sessions are removed before the next bootstrap can resurrect them.

🔧 Changed

- The `session-closed` tmux hook now fires `portal state commit-now` instead of `portal state notify` — closing the window where a killed session could be resurrected by the next portal bootstrap within the daemon's 1-second tick interval.
- `TouchSaveRequested` extracted as a shared function in `internal/state` — the dirty-flag touch sequence is now a single canonical implementation used by both `state notify` and the new `commit-now` failure/short-circuit paths.
- Silent-exit error detection replaced brittle empty-string comparison with `cmd.IsSilentExitError` — compile-time-linked across `cmd` and `main` so neither side can drift; `ErrStatusUnhealthy` now carries a descriptive message instead of an empty one.
- When `@portal-restoring` is set during a `session-closed` hook, `commit-now` skips the structural commit and touches `save.requested` so the daemon's first post-restoration tick commits promptly without waiting for the gap rule.

## [0.5.5] - 2026-05-21

🐛 Fixed

- Killing a session while a filter is active no longer clears the filtered list — the sessions view stays filtered with the killed entry removed.
- Pressing Space on the scrollback preview now dismisses it, matching the behaviour of Esc.

## [0.5.4] - 2026-05-20

✨ Added

- New `internal/portalbintest` package consolidates binary-build and PATH-staging helpers for integration tests, usable by daemon, saver, and restore tests without pulling in restore-specific scaffolding.
- `PollUntil` helper in `internal/tmuxtest` provides a reusable, deadline-bounded polling primitive for integration tests.
- `WriteVersionFile` now emits a DEBUG breadcrumb (`daemon.version write:`) to `portal.log` on every call, including the bootstrap-side defensive write, giving a single grep anchor for version-file lifecycle investigations.

🔧 Changed

- Bootstrap no longer kills the `_portal-saver` session when the daemon is alive but `daemon.version` is missing — it repairs the file defensively instead, eliminating unnecessary kill-respawn cycles on healthy systems.
- The daemon's `captureAndCommit` loop checks for context cancellation at three points (entry, post-enumeration, and between per-pane iterations) so a SIGHUP mid-tick exits within one pane's capture time rather than grinding through the rest of the loop.
- `EnsurePortalSaverVersion` consults daemon aliveness before the version-mismatch predicate — a dead daemon no longer triggers the kill barrier regardless of version state.

🐛 Fixed

- Daemon shutdown flush after SIGHUP now uses a non-cancellable context so the final state save completes rather than being aborted by the already-cancelled signal context.

## [0.5.3] - 2026-05-19

🔧 Changed

- `tmux attach-session` no longer passes `-A` — attaching to an existing session uses exact-match target resolution only, removing the implicit create-or-attach fallback.

🐛 Fixed

- Release note generation strips null bytes from the git diff before passing it to Claude, preventing silent truncation or prompt corruption on binary-adjacent diffs.

## [0.5.2] - 2026-05-18

✨ Added

- Press Enter in the scrollback preview to attach to the previewed session, with optional window and pane pre-selection via `]` / `[` / `Tab` before committing.
- Preview frame now renders with a styled rounded border, adaptive border colour, and a cascading chrome line that shows window/pane counters, window name, and keymap hints — compressing gracefully at narrow terminal widths.
- When a previewed session is killed externally and Enter is pressed, Portal returns to the sessions list and shows a brief inline flash (`session "<name>" no longer exists`) that auto-clears after ~3 seconds or on the next keypress.
- New `SelectWindow` tmux client method for pre-selecting a window before attaching.
- New `HasSessionProbe` tmux client method that discriminates a genuine missing-session exit from an OS-layer fault, enabling the externally-killed bail path.

🔧 Changed

- All tmux target resolution (`has-session`, `switch-client`, `select-pane`, `resize-pane -Z`, `attach-session`) now uses the `=` exact-match prefix, preventing silent prefix-collision matches (e.g. a killed `foo` matching live `foo-2`).
- Attach-session now passes `-A` so tmux creates the session atomically if absent rather than failing — the residual fallback for the TOCTOU window between existence check and handoff.
- The connector handoff runs after the TUI exits rather than inside the live event loop, preventing an orphan portal process that would keep running after `switch-client` moved the terminal away.

## [0.5.1] - 2026-05-14

✨ Added

- Space bar opens the scrollback preview from the sessions list — the key hint now appears in the help bar.

🔧 Changed

- Preview chrome bar labels are expanded to spell out each navigation key (`next win`, `prev win`, `tab next pane`, `esc back`) and prefixes the window name with `win:` for clarity.

## [0.5.0] - 2026-05-14

✨ Added

- Eager hydration signaling at bootstrap — all restored sessions now receive their scrollback replay signal immediately at startup, eliminating the bug where only the attached session recovered while N-1 sessions' helpers timed out and leaked their `@portal-skeleton-*` markers.
- Stale `@portal-skeleton-*` marker cleanup as a new bootstrap step — markers whose pane no longer exists are unset each boot, unblocking the daemon's scrollback-save loop for any pane previously stuck behind a leaked marker.
- On-resume hooks now fire on every restored pane including non-attached sessions — the timeout recovery path was unified with the success and file-missing paths so hooks always execute.
- Daemon singleton lock (`daemon.lock`) — a `flock`-based advisory lock prevents more than one `portal state daemon` from running against the same state directory, eliminating duplicate-daemon races during rapid restarts.
- Kill barrier before saver-session respawn — portal now waits for the prior daemon to fully exit before spawning a replacement, making the recycle path silent on the happy path.
- `tmux.CommandError` type — tmux command failures now carry the child's stderr for precise absence-vs-transport-fault discrimination, fixing cases where a lost-server error was silently treated as "option not found."

🔧 Changed

- Restored panes are now launched with a bare `portal state hydrate` invocation instead of a `sh -c '...; exec $SHELL'` wrapper, so typing `exit` once in a restored pane closes it immediately with no orphan shell parent.
- The hydrate helper's timeout path now unsets the `@portal-skeleton-*` marker and fires on-resume hooks instead of leaving the marker set and skipping hooks.
- FIFO-write retry logic moved from `cmd/state_signal_hydrate` to `internal/state` as `WriteFIFOSignal` / `SendHydrateSignal` / `DefaultFIFOSignaler`, giving both the bootstrap eager-signal step and the `client-attached` hook a shared, tested implementation.
- The daemon's merge filter now rejects stale skeleton markers pointing at killed sessions, windows, or panes — preventing killed sessions from being resurrected into `sessions.json` on the next capture tick.
- `WaitForSkeletonMarkersCleared` now requires an explicit poll-tick argument so call sites are unambiguous about their cadence.
- `golang.org/x/sys` promoted from an indirect to a direct dependency.

🐛 Fixed

- Sessions other than the one the user attached to no longer have their scrollback silently skipped forever after a reboot — the eager-signal step delivers the FIFO byte to every restored pane without requiring a client attach.
- Typing `exit` in a restored pane now closes the pane on the first invocation instead of requiring two exits due to the former outer `sh -c` wrapper.
- Transport errors from tmux (lost server, socket connect failure) are no longer misclassified as "option not found," allowing the daemon's restoring-marker check to correctly skip captures during transient tmux failures.
- Multiple daemons could accumulate after rapid saver-session recycles; the flock singleton and kill barrier together enforce at most one daemon per state directory.

## [0.4.0] - 2026-05-08

✨ Added

- Press `Space` on any session in the picker to open a read-only scrollback preview — browse saved terminal output across windows and panes without attaching to the session.
- Cycle windows with `]`/`[` and panes with `Tab` inside the preview; scroll the loaded buffer with arrow keys, `j`/`k`, `PgUp`/`PgDn`, `Home`, and `End`; dismiss with `Esc`.
- The sessions list automatically refreshes when preview is dismissed, so a session killed externally while you were previewing disappears from the list immediately.
- Panes with no saved content yet (brand-new session or daemon hasn't ticked) render `(no saved content)` rather than an empty screen.

## [0.3.1] - 2026-05-01

🐛 Fixed

- Session names beginning with `-` (e.g. from projects whose directory starts with `.`) no longer cause the hydration hook to exit silently — a `--` end-of-flags separator in the registered tmux hook command prevents cobra/pflag from misreading the session name as a flag cluster.
- Stale hook entries from older Portal installs that lack the `--` separator are automatically evicted at bootstrap startup, leaving exactly one correct entry per hydration event.
- The tmux server's bootstrap session is now created with an explicit reserved name (`_portal-bootstrap`) and is filtered out of all user-facing session listings alongside `_portal-saver`, so it never appears in the TUI picker or `portal list`.
- A spurious per-pane warning comparing predicted vs. live pane keys (which fired incorrectly when tmux was configured with non-zero `base-index` or `pane-base-index`) has been removed.

## [0.2.5] - 2026-04-07

🔧 Changed

- Server startup now creates a detached bootstrap session instead of calling `start-server` directly — prevents tmux's `exit-empty` from terminating the server before plugins like tmux-continuum can restore saved sessions.

## [0.2.4] - 2026-04-03

🐛 Fixed

- On-resume hooks are no longer silently deleted when the tmux server is killed and restarted — hooks are now keyed by stable structural position (`session:window.pane`) instead of ephemeral pane IDs (`%N`) that reset on every server restart.
- Stale-hook cleanup is now skipped when no live panes are found, preventing all stored hooks from being wiped on the first open after a server restart.

## [0.2.3] - 2026-03-30

Maintenance release — no notable source changes
## [0.2.2] - 2026-03-30

🐛 Fixed

- Config migration no longer overwrites files when a permission error prevents checking the destination path — previously a non-"not found" stat error could trigger an unsafe migration.

## [0.2.1] - 2026-03-28

🔧 Changed

- Config files now resolve using XDG conventions — `$XDG_CONFIG_HOME/portal/` when set, falling back to `~/.config/portal/`, so Portal behaves correctly on Linux and with custom XDG setups.
- On first run after upgrading, config files are automatically moved from the old macOS location (`~/Library/Application Support/portal/`) to the XDG path — no manual migration needed.

## [0.2.0] - 2026-03-28

✨ Added

- `portal hooks` command — register per-pane on-resume commands that re-execute automatically when a session is attached after a reboot.
- Resume hooks fire via `tmux send-keys` on `open` and `attach`; a volatile tmux server-option marker prevents duplicate execution within the same boot cycle.
- `portal clean` now prunes hook entries for panes that no longer exist, in addition to stale projects.
- hooks are stored in `~/.config/portal/hooks.json` (overridable via `PORTAL_HOOKS_FILE`).

🔧 Changed

- Atomic file writes extracted into a shared `fileutil.AtomicWrite` helper — previously inlined only in the project store, now used by both the project and hooks stores.

## [0.1.0] - 2026-03-21

✨ Added

- Portal automatically starts the tmux server on any command that needs it, eliminating the need for LaunchAgents or other workarounds after a reboot.
- The TUI shows a "Starting tmux server..." loading screen while waiting for sessions to restore, exiting as soon as sessions appear or after a 6-second maximum.
- CLI commands (`list`, `attach`, `kill`) print "Starting tmux server..." to stderr and wait briefly for sessions before proceeding.

## [0.0.3] - 2026-03-01

✨ Added

- The help bar is now always fully expanded on the sessions and projects pages — all key bindings are visible at a glance without pressing `?`.
- List navigation now wraps around — pressing down past the last item jumps to the first, and pressing up past the first jumps to the last.

🔧 Changed

- Secondary text (session details, project paths, help bar labels) uses lighter, more readable colours.

## [0.0.2] - 2026-03-01

✨ Added

- Sessions and projects now appear together in one unified full-screen list — switch between them with `s`/`p`/`x` without leaving the picker.
- Kill (`k`), rename (`r`), and delete project (`d`) actions now open centered modal overlays with border styling instead of inline text prompts.
- Project edit modal supports renaming and managing aliases in-place — `Tab` switches fields, `x` removes an alias, `Enter` saves immediately.
- Built-in `bubbles/list` filtering replaces the custom filter mode — press `/` to filter and `Esc` to clear, consistent across both pages.
- Help bar updates dynamically per page, showing context-appropriate keybindings (attach, rename, kill, projects, new in cwd, filter, quit).
- New `n` key creates a session in the current working directory from any page.
- Command-pending mode (`portal open --command`) locks the picker to the Projects page and shows a `Select project to run: <cmd>` status line; `s`, `x`, `e`, and `d` keys are suppressed in this mode.
- The picker auto-selects the default page on load: Sessions when sessions exist, Projects otherwise.

🔧 Changed

- Kill shortcut changed from uppercase `K` to lowercase `k`; rename shortcut changed from `R` to `r`.
- Session list title now shows `Sessions (current: <name>)` when inside tmux instead of a separate header line above the list.
- Command-pending banner changed from `Command: <cmd>` to `Select project to run: <cmd>`, inserted after the list title line.
- `Esc` is now progressive: dismisses an open modal first, then clears an applied filter, then quits — rather than quitting immediately from any state.
- The standalone `internal/ui/projectpicker.go` component is removed; project-picking logic is now part of the main TUI model.
- `ProjectStore`, `ProjectEditor`, and `AliasEditor` interfaces moved from `internal/ui` to `internal/tui`; callers updated accordingly.

## [0.0.1] - 2026-02-26

Initial release.
