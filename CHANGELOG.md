# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.7] - 2026-06-17

🔧 Changed

- CI release workflow actions updated to their latest major versions (checkout@v6, setup-go@v6, goreleaser-action@v7).

## [0.7.6] - 2026-06-17

🔧 Changed

- Release workflow is now delegated to the `mint` CLI — run `brew install leeovery/tools/mint` if the `release` script reports mint is missing.
- GoReleaser is set to `keep-existing` release mode so binary uploads no longer overwrite a release body written by other tooling.

🗑️ Removed

- Deleted the inline bash release script (~350 lines) — replaced by the `mint release` delegation above.
- Removed the `BUG-resume-hooks-lost-on-server-restart.md` tracking document from the repository.

## [0.7.2] - 2026-06-08

- Preserve tag case in `NormaliseTag` (trim only, no lower-casing); tag matching is now case-sensitive, so "Work"/"work" are distinct tags and By-Tag headings
- Indent grouped session list: headers align with the title box, grouped rows nest under their header (Flat rows unchanged)


## [0.7.1] - 2026-06-08

- Stamp @portal-dir at QuickStart creation via chained `new-session -d ; set-option ; attach-session` so sessions stay anchored to their origin dir after the pane cd's away
- Remove the lazy stamp-on-render fallback (`dirstamp.go`); render-layer dir resolution now derives from the active pane → git-root and caches in-memory only, never re-stamping tmux (fixes mis-grouping of drifted panes, e.g. .dotfiles-under-portal)
- Cache derived dirs back into `m.sessions` so subsequent rebuilds skip the pane read (fixes the 2-3s "switch view" stall)
- Fix grouped-list viewport overflow: render group headings as real non-selectable `HeaderItem` rows (delegate Height 1) instead of uncounted in-delegate lines, so pagination is exact and the title/cursor no longer scroll off the top
- Add cursor-skip (`ensureSessionRowSelected`/`skipHeaderRow`) keeping selection off header rows; reset paginator to page 1 on view switch
- Persist project tags live (Enter adds, x removes immediately) so Esc no longer discards them; remove batched tag reconciliation in confirm


## [0.6.5] - 2026-06-04

- Add package-level TestMain to 15 packages poisoning PORTAL_*_FILE / PORTAL_STATE_DIR to /nonexistent paths, making test state-isolation opt-out fail loudly instead of mutating the developer's real ~/.config/portal/
- Isolate symptom-fixture's per-file config env vars (hooks/projects/aliases) so the bootstrap subprocess's step-11 CleanStale can't wipe real config
- Hoist per-subtest PORTAL_STATE_DIR isolation in TestStateInternalSubcommandsAcceptValidArgv to cover all subcommands, not just daemon


## [0.6.3] - 2026-06-04

- Export `tmux.KillBarrierTimeoutCeiling` as single source of truth; daemon integration test imports it instead of mirroring the 5s literal
- Convert fast-host no-op-pass to `t.Skip` in mid-tick SIGHUP test so a too-fast capture-pane host skips rather than passes trivially


## [0.6.2] - 2026-06-03

- Restructure agentic-workflows skill suite (v0.4.12 → v0.4.17): collapse separate `start-*` and `continue-*` skills into a unified `workflow-discovery` first phase plus `workflow-continue-*` skills; route all new work through discovery with work-type detection
- Add inbox working-set and archived-item management to `workflow-start` (multi-select, archive/restore/delete, seed-into-discovery)
- Add `seeds[]` manifest tracking and `seeds/` storage for inbox-promoted work units; wire seed context into research/discussion/investigation/scoping and knowledge indexing (new `seeds.json` chunking config)
- Add `[do-now]` review category for zero-risk fixes applied in-place, with file:line recommendations and clustering in review output
- Widen TUI page-jump footer hints to surface the `x` toggle (`p/x`, `s/x`) in `internal/tui/model.go` (display-only)


## [0.4.0] - 2026-05-08

- Add scrollback preview page (`pagePreview`) opened via Space on Sessions list, with `]`/`[`/Tab cycling, chrome line, and Esc dismiss
- Add `state.TailScrollback` reverse-chunked tail-N reader with three-shape return contract and single-fd invariant
- Add `tmux.Client.ListWindowsAndPanesInSession` for window-grouped pane enumeration
- Wire constructor-injected `TmuxEnumerator`/`ScrollbackReader` seams with production adapters at TUI startup
- Refresh sessions list on preview dismiss to drop externally-killed sessions, re-anchoring cursor by name
- Extract `applySessions` helper to deduplicate session-list refresh between `SessionsMsg` and preview-refresh handlers
- Remove unmaintained CHANGELOG.md


## [0.3.1] - 2026-05-01

- Fix scrollback hydration failure for sessions with leading-dash names by adding `--` end-of-flags separator to signal-hydrate hook command, with one-shot bootstrap migration to evict legacy un-separated entries
- Hide internal sessions on startup: filter `_*` prefixed sessions in `Client.ListSessions` and rename bootstrap session to `_portal-bootstrap` via new `PortalBootstrapName` constant
- Delete dead `PredictLiveIndices` diagnostic path and misleading `predicted=...__0.0 live=...__X.Y` WARN that fired under non-zero base-index configs
- Bump agentic-workflows to v0.3.2 with new knowledge base subsystem (semantic search over completed artifacts via OpenAI embeddings), discussion gap analysis, polarity-pair perspective agents with stable finding IDs, and `completed_at` tracking for work units
- Add real-tmux integration test coverage for leading-dash sessions and post-bootstrap session set assertions


## [0.2.5] - 2026-04-07

- Fix server bootstrap: replace `tmux start-server` with `tmux new-session -d` to prevent exit-empty killing server before continuum restores sessions
- Add bootstrap-to-query regression test validating EnsureServer → ListSessions → ServerRunning flow
- Bump agentic-workflows to v0.2.10
- Add research deep-dive and review background agents
- Add convergence analysis for review/fix cycle escalation diagnostics
- Add epic dependency map view
- Add manifest CLI `pull` command for array element removal
- Add final gap review step to discussion and research processes
- Categorize review recommendations as quickfix/idea/bug with inbox surfacing
- Surface pending discussion topics from research analysis in epic menus


## [0.2.4] - 2026-04-03

- Fix resume hooks lost on tmux server restart (empty-pane guard + structural key migration)
- Replace ephemeral pane IDs with structural keys (session:window.pane) in hook storage
- Add tmux.Client.ResolveStructuralKey for pane ID to structural key resolution
- Migrate hooks set/rm/list and clean command to structural key model
- Update agentic-workflows skills to v0.2.1 with step signposting


## [0.2.3] - 2026-03-30

- Workflow housekeeping: close config-homedir-error-test idea as not worth doing, register in manifest


## [0.2.2] - 2026-03-30

- Add quick-fix work type with scoping → implementation → review pipeline
- Add scoping phase: single-pass spec + plan generation for mechanical changes
- Add verification workflow (baseline → change → verify) as TDD alternative for quick-fixes
- Rework discussion process: replace linear Questions list with Discussion Map subtopic lifecycle (pending/exploring/converging/decided)
- Add step banners and contextual descriptions across all workflow skills
- Fix migrateConfigFile to use os.IsNotExist explicitly on newPath stat check
- Add PORTAL_HOOKS_FILE isolation to 7 project-only clean tests


## [0.2.1] - 2026-03-28

- Fix config path resolution on macOS — replace `os.UserConfigDir()` with XDG-compliant logic (`XDG_CONFIG_HOME` → `~/.config`)
- Add one-shot migration of config files from `~/Library/Application Support/portal/` to XDG path
- Update agentic-workflows to v0.1.4 (discussion agents, investigation synthesis, compliance checks)


## [0.2.0] - 2026-03-28

- Add resume hooks system (`hooks set/rm/list`) for restarting processes after reboot via per-pane registry with volatile tmux server-option markers
- Wire hook execution into all connection paths (attach, TUI picker, direct path) before session connect
- Extend `clean` command with stale hook pruning alongside stale project removal
- Extract `internal/fileutil.AtomicWrite` shared by hooks and project stores
- Add tmux client methods: SetServerOption, GetServerOption, DeleteServerOption, ListPanes, ListAllPanes, SendKeys
- Upgrade agentic-workflows to v0.1.0 (.js → .cjs, project-level manifest defaults, new migrations)


## [0.0.3] - 2026-03-01

- Show full help bar permanently and swallow `?` toggle
- Enable infinite list scrolling (cursor wraps around)
- Brighten help bar and detail text styles (#777777)
- Cache terminal dimensions and re-apply after data loads for correct pagination
- Mark TUI session picker phases 3–6 complete


