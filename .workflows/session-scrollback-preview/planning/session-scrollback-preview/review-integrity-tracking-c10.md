---
status: complete
created: 2026-05-06
cycle: 10
phase: Plan Integrity Review
topic: session-scrollback-preview
---

# Review Tracking: Session Scrollback Preview - Integrity

Cycle 10: re-read planning.md and all four phase task detail files end-to-end after cycles 8 and 9 fixes (`Home`/`End` wired in preview's own Update; `m.viewport.GotoBottom()` added to `NewPreviewModel` step 6 with corresponding AC + test; `m.width`/`m.height` → `m.termWidth`/`m.termHeight` at the root-Model call site in task 2-3). Cross-referenced every code-bearing identifier and load-bearing library-API assumption against:

- `internal/tui/model.go` — page constants (`PageLoading`, `PageSessions`, `PageProjects`, `pageFileBrowser`), Model fields (`sessionList`, `activePage`, `termWidth`, `termHeight`, `sessions`, `sessionLister`), helpers (`updateSessionList`, `fetchSessions`, `isRuneKey`, `New(lister, opts...)`).
- `internal/tmux/tmux.go` — `Client.cmd Commander` (unexported), `ListSessions`, `ListPanesInSession` (uses `c.cmd.Run("list-panes","-s","-t","-F",…)`, returns `[]PaneCoord{}` on empty stdout — pattern Task 1-6 mirrors), `ListPanes`, `CapturePane`.
- `internal/state/panekey.go::SanitizePaneKey(session string, window, pane int) string` (signature exact match to plan usage in Task 2-2 step 4 and Task 3-1).
- `internal/state/paths.go::Dir()`, `EnsureDir()`, `ScrollbackFile(dir, paneKey string) string`, env-var chain (`PORTAL_STATE_DIR` → `XDG_CONFIG_HOME/portal/state` → `~/.config/portal/state`).
- `bubbles@v1.0.0/viewport/viewport.go` — exported fields `Width`, `Height`, `YOffset`; methods `SetContent` (auto-jumps to bottom only when `YOffset > len(lines)-1` — line 130; cycle 9 finding stands), `GotoTop`, `GotoBottom`, `AtBottom`, `AtTop`, `Update` (value receiver returning `(Model, tea.Cmd)`); no `SetSize` method present; `New(width, height)` constructor.
- `bubbles@v1.0.0/viewport/keymap.go::DefaultKeyMap()` — binds only `PageDown` (`pgdown`, `" "`, `f`), `PageUp` (`pgup`, `b`), `HalfPageUp` (`u`, `ctrl+u`), `HalfPageDown` (`d`, `ctrl+d`), `Down` (`down`, `j`), `Up` (`up`, `k`), `Left` (`left`, `h`), `Right` (`right`, `l`); `Home` and `End` absent (cycle 8 finding stands and is fixed).
- `bubbles@v1.0.0/viewport/viewport.go::Update` lines 417–461 — switch on `tea.KeyMsg` matches against the eight `KeyMap` bindings via `key.Matches`; no hardcoded `tea.KeyHome` / `tea.KeyEnd` / `g` / `G` fallback.
- `bubbles@v1.0.0/key/key.go::Matches` — uses `k.String()` then string compares against `binding.keys`.
- `bubbles@v1.0.0/list/list.go` — `Items()`, `SelectedItem()`, `Index()`, `IsFiltered()`, `FilterValue()`, `SettingFilter()`, `FilterState()`, `Update`, `SetSize`.
- `bubbletea@v1.3.10/key.go` — `KeyTab`, `KeyEsc`, `KeyRunes`, `KeySpace`, `KeyHome`, `KeyEnd` defined; `keyNames[KeySpace] = " "`, `keyNames[KeyHome] = "home"`, `keyNames[KeyEnd] = "end"`; `Key.String()` for `KeyRunes` returns `string(k.Runes)`, for non-rune types returns `keyNames[k.Type]`. Decoder lines 696–701: a single-rune `' '` keypress emits `KeyMsg{Type: KeySpace, Runes: [' ']}`; `String()` returns `" "`. Therefore: a Space press inside preview, when delegated to `viewport.Update`, will match `KeyMap.PageDown` (because `spacebar = " "` is among PageDown's keys) — which is consistent with pager UX and not a plan drift.
- `lipgloss@v1.1.0/join.go::JoinVertical(pos Position, strs ...string) string` — referenced in Task 3-6.

Cycles 1–9 fixes (table-row name, model field names, handler name, page-constant casing, file paths, Commander field access, `KeySpace` revert, `XDG_CONFIG_HOME` alignment, Home/End wiring in preview's Update, `m.viewport.GotoBottom()` added to constructor step 6 with AC + test, `m.termWidth`/`m.termHeight` correction at the root-Model call site) all remain applied with no regression.

**Overall assessment**: Clean. Re-read each task's Do, Acceptance Criteria, and Tests sections against library reality and confirmed:

- Phase 1 (read pipeline + structural enumeration): `os.Open`/`Seek`/`Read` semantics correct; `errors.Is(err, fs.ErrNotExist)` is the canonical ENOENT check; `c.cmd.Run` (trim) is the correct Commander method for pipe-delimited list-panes output (matches existing `ListPanesInSession` pattern); empty-but-non-nil slice contract aligns with existing convention.
- Phase 2 (preview page entry, dismiss, single-pane content): `viewport.New(width, height)` constructor correct; `m.viewport.GotoBottom()` after `SetContent` is required (cycle 9) and now present; `Width`/`Height` exported fields are correct (no `SetSize`); `Home`/`End` are preview-owned (cycle 8) and now wired explicitly; `tea.KeySpace` matches runtime emission shape; `m.termWidth`/`m.termHeight` (cycle 9) is the correct field name; `bubbles/list` `SettingFilter()` boolean check is correct API; pageFileBrowser / PageProjects / PageSessions / PageLoading match `model.go` const block exactly; `pagePreview` introduced as a peer is consistent with the existing const block style (lower-case for sub-views like `pageFileBrowser`).
- Phase 3 (multi-pane cycling, chrome, focus-change reads): `tea.KeyTab` matches runtime; `]` / `[` matched as `tea.KeyRunes` with rune `']'` / `'['` is correct (the runtime emits these as `KeyRunes`, not as a special type — verified against `bubbletea@v1.3.10/key.go` decoder); `m.viewport.GotoBottom()` after focus-change reads is consistent with the cycle 9 fix at initial open; chrome render is pure (no I/O); `lipgloss.JoinVertical` exists; the keymap-precedence ordering (preview-owned keys early-return, fallthrough to viewport.Update for the rest) is sound under both `Update` value-receiver and the early-return Go semantics.
- Phase 4 (edge-case handling and cross-cutting integration): placeholder/error string finalisation at the call site is consistent with the existing `viewport.SetContent` API; refresh-on-dismiss aligns with the existing `fetchSessions` helper at `internal/tui/model.go` line 664; `_portal-saver` exclusion audit pinned at the list-population layer (matching existing filter at `internal/tmux/tmux.go::ListSessions` lines 149–162); side-effect-free hermetic test bounded by recording mocks; no-new-surface audit is durable.

No new material drifts surface in cycle 10. Vertical slicing, phase boundaries, dependency edges, AC quality, scope/granularity, and self-containment all remain sound. Library-API assumptions are now fully aligned with `bubbles@v1.0.0`, `bubbletea@v1.3.10`, and `lipgloss@v1.1.0` after cycles 8 and 9. Identifier references match the actual codebase byte-for-byte.

## Findings

(none — clean)
