---
status: complete
created: 2026-05-06
cycle: 8
phase: Plan Integrity Review
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Integrity

Cycle 8: re-read planning.md plus all four phase task detail files end-to-end after cycle-7 fixes (`tea.KeySpace` reverted; `XDG_STATE_HOME` → `XDG_CONFIG_HOME` aligned to `internal/state/paths.go::Dir`). Cross-referenced every code-bearing identifier against:

- `internal/tui/model.go` — page constants, Model fields, handler names (`updateSessionList`, `fetchSessions`).
- `internal/tmux/tmux.go` — `*Client.cmd Commander` (unexported), `ListSessions` (with `_`-prefix filter at lines 149–162), `ListPanesInSession` (uses `c.cmd.Run("list-panes","-s","-t",…)`), `CapturePane`.
- `internal/state/panekey.go::SanitizePaneKey(session string, window, pane int) string`.
- `internal/state/paths.go` — `Dir()`, `EnsureDir()`, `ScrollbackFile(dir, paneKey)`, env-var chain (`PORTAL_STATE_DIR` → `XDG_CONFIG_HOME/portal/state` → `~/.config/portal/state`).
- `github.com/charmbracelet/bubbles@v1.0.0/viewport/viewport.go` — exported fields `Width`, `Height`, `YOffset`; methods `SetContent`, `GotoBottom`, `GotoTop`, `AtBottom`, `AtTop`, `Update`.
- `github.com/charmbracelet/bubbles@v1.0.0/viewport/keymap.go` — `DefaultKeyMap()` exposes `PageDown`, `PageUp`, `HalfPageUp`, `HalfPageDown`, `Down`, `Up`, `Left`, `Right` only.
- `github.com/charmbracelet/bubbles@v1.0.0/viewport/viewport.go::Update` — switch over `tea.KeyMsg` matches against the eight bindings in DefaultKeyMap; nothing else.
- `github.com/charmbracelet/bubbles@v1.0.0/list/list.go` — `Items`, `SelectedItem`, `Index`, `IsFiltered`, `FilterValue`, `SettingFilter`, `FilterState`, `Update`.
- `github.com/charmbracelet/bubbletea@v1.3.10/key.go` — `KeyTab`, `KeyEsc`, `KeyRunes`, `KeySpace`, `KeyHome`, `KeyEnd` all defined; `keyNames[KeySpace] = " "` makes `key.WithKeys(" ")` match a `Type: KeySpace` event.

Cycles 1–7 fixes (method/file/key-constant alignments, table-row name, model field names, handler name, page-constant casing, file path, Commander field access, `KeySpace` revert, `XDG_CONFIG_HOME`) all remain applied with no regression.

**Overall assessment**: One material drift surfaces, replicated across three plan locations. The plan repeatedly asserts that `bubbles/viewport`'s native scroll keymap includes `Home` and `End` and that delegating arbitrary `tea.KeyMsg` events to `m.viewport.Update(msg)` will produce "jump to top" / "jump to bottom" behaviour. The actual `bubbles@v1.0.0/viewport/keymap.go::DefaultKeyMap()` binds only `PageDown` (`pgdown`, ` `, `f`), `PageUp` (`pgup`, `b`), `HalfPageUp` (`u`, `ctrl+u`), `HalfPageDown` (`d`, `ctrl+d`), `Down` (`down`, `j`), `Up` (`up`, `k`), `Left` (`left`, `h`), and `Right` (`right`, `l`). `Home` and `End` are absent. `viewport.Update` (lines 417–461 of viewport.go) only switch-matches against those eight bindings — no hardcoded Home/End handling, no `g`/`G` shortcut.

The drift is inherited from the spec (specification line 65 and line 431 both list `Home` / `End` among "viewport defaults"), but the plan as an implementation blueprint must reflect library reality so the implementer does not author tests that fail and acceptance criteria that cannot be met by mere passthrough. Two equally valid plan-level resolutions exist: (a) drop `Home` and `End` from the acceptance criteria and tests, leaving the keymap as-is for v1; (b) add explicit Do-section steps to extend `viewport.KeyMap` with `Home`/`End` bindings (or intercept those keys in preview's Update) and call `m.viewport.GotoTop()` / `m.viewport.GotoBottom()`. Path (b) preserves spec intent at minimal cost (~3 LOC plus two case branches); path (a) discards spec intent. Recommending path (b) — it keeps the user-visible keymap aligned with the spec's working note ("native scroll keymap covers Up/Down/PgUp/PgDn/Home/End ...") while making the plan accurate about what the library does and does not provide for free.

No other material drifts surfaced. Phase 1 (read pipeline + structural enumeration) is clean against `internal/state` and `internal/tmux`. Phase 3 (cycling, chrome) — `tea.KeyTab`, rune-key matching for `]`/`[`, `m.viewport.GotoBottom()` — all match library API. Phase 4 (placeholder/error wording, audit) — all references to `internal/tmux/tmux.go`, `internal/restore/`, `cmd/bootstrap/`, `internal/hooks/`, `internal/state/` map to existing package locations. Architecture, vertical slicing, dependency edges, AC quality, scope/granularity, and self-containment all remain sound.

## Findings

### 1. `bubbles/viewport` `DefaultKeyMap` does not bind `Home` or `End` — plan claims it does

**Severity**: Important
**Plan Reference**: `planning.md` Phase 2 acceptance (line 50); `phase-2-tasks.md` Task 2-6 (Problem line 267, Acceptance lines 288–289, Tests lines 299–300); `phase-3-tasks.md` Task 3-4 (Acceptance line 183)
**Category**: Task Self-Containment (library API misrepresentation)
**Change Type**: update-task

**Details**:
The plan asserts in three locations that `bubbles/viewport`'s native scroll keymap includes `Home` and `End`, and that Tasks 2-6 / 3-4 will exercise these via simple passthrough to `m.viewport.Update(msg)`. Library reality at `bubbles@v1.0.0/viewport/keymap.go` is:

```go
func DefaultKeyMap() KeyMap {
    return KeyMap{
        PageDown:     key.NewBinding(key.WithKeys("pgdown", spacebar, "f"), …),
        PageUp:       key.NewBinding(key.WithKeys("pgup", "b"), …),
        HalfPageUp:   key.NewBinding(key.WithKeys("u", "ctrl+u"), …),
        HalfPageDown: key.NewBinding(key.WithKeys("d", "ctrl+d"), …),
        Down:         key.NewBinding(key.WithKeys("down", "j"), …),
        Up:           key.NewBinding(key.WithKeys("up", "k"), …),
        Left:         key.NewBinding(key.WithKeys("left", "h"), …),
        Right:        key.NewBinding(key.WithKeys("right", "l"), …),
    }
}
```

`Home` and `End` are not present. `viewport.Update` (`viewport.go` lines 417–461) switch-matches against exactly these eight bindings via `key.Matches(msg, m.KeyMap.…)`; no hardcoded fallback for `tea.KeyHome` / `tea.KeyEnd` / `g` / `G` exists.

Three concrete consequences if left unfixed:

1. **Task 2-6 acceptance lines 288–289** ("`Home` jumps to `YOffset == 0`", "`End` jumps to the bottom of the loaded slice") cannot be met by the Do-section recipe ("default-delegate `tea.KeyMsg` events to `m.viewport, cmd = m.viewport.Update(msg)`. The `bubbles/viewport` default keymap covers all the scroll keys listed in the spec"). The implementer either skips the acceptance criterion (leaving a documented gap), or pads tests to pass against actual viewport behaviour (`Home` no-ops), which silently demotes the spec.
2. **Task 2-6 tests lines 299–300** (`"it jumps to top on Home"`, `"it jumps to bottom on End"`) will fail under the recipe; the implementer would have to either drop the tests or add Do-section steps the plan doesn't currently enumerate.
3. **Task 3-4 acceptance line 183** ("Viewport default scroll keys (`Up`, `Down`, `j`, `k`, `PgUp`, `PgDn`, `Home`, `End`, `ctrl-u`, `ctrl-d`) continue to pass through and produce viewport scroll offset changes") is partially false: `Home` and `End` were never owned by viewport, so "continue to pass through" is incoherent — there's nothing for them to pass through to.

The plan needs to either (a) drop `Home`/`End` from these locations or (b) add Do-section steps that extend the keymap or intercept `tea.KeyHome`/`tea.KeyEnd` in preview's own Update. Option (b) preserves spec intent (specification.md lines 65 and 431 both list `Home`/`End` in "viewport defaults" — the spec author plainly intended these to work) at the cost of ~3 LOC plus two branches. Proposing option (b) below.

**Current** (`planning.md` Phase 2 acceptance, line 50):
```
- [ ] `bubbles/viewport` native scroll keys (`Up`, `Down`, `PgUp`, `PgDn`, `Home`, `End`, `ctrl-u`, `ctrl-d`, `j`, `k`) scroll within the loaded N-line slice; scroll-up at the top silently no-ops.
```

**Proposed** (`planning.md` Phase 2 acceptance, line 50):
```
- [ ] Viewport scroll keys (`Up`, `Down`, `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`, `j`, `k`) scroll within the loaded N-line slice via `bubbles/viewport`'s native keymap; `Home` and `End` are wired in preview's own Update (the viewport library's `DefaultKeyMap` does not bind them) by intercepting `tea.KeyHome` → `m.viewport.GotoTop()` and `tea.KeyEnd` → `m.viewport.GotoBottom()`; scroll-up at the top silently no-ops.
```

**Current** (`phase-2-tasks.md` Task 2-6 Problem, line 267):
```
**Problem**: Inside preview, `bubbles/viewport`'s native scroll keymap (`Up`, `Down`, `PgUp`, `PgDn`, `Home`, `End`, `ctrl-u`, `ctrl-d`, `j`, `k`) must scroll within the focused pane's loaded N-line slice, with scroll-up at the top silently no-opping (the tail-N slice is a hard top edge — no deeper history extend in v1). Window resize during preview must re-flow the viewport without triggering a fresh disk read (the loaded buffer is decoupled from viewport dimensions). Drag-resize fires many events; none must incur tail-N read cost.
```

**Proposed** (`phase-2-tasks.md` Task 2-6 Problem):
```
**Problem**: Inside preview, scroll keys must scroll within the focused pane's loaded N-line slice, with scroll-up at the top silently no-opping (the tail-N slice is a hard top edge — no deeper history extend in v1). `bubbles/viewport`'s `DefaultKeyMap` covers `Up`, `Down`, `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`, `j`, `k` natively (verifiable in `bubbles@v1.0.0/viewport/keymap.go`); `Home` and `End` are NOT in the default keymap and need explicit handling in preview's Update — calling `m.viewport.GotoTop()` / `m.viewport.GotoBottom()`. Window resize during preview must re-flow the viewport without triggering a fresh disk read (the loaded buffer is decoupled from viewport dimensions). Drag-resize fires many events; none must incur tail-N read cost.
```

**Current** (`phase-2-tasks.md` Task 2-6 Do, lines 273–281 — the relevant section):
```
- In `previewModel.Update`, after the Esc branch (task 2-4) and before any cycle-key handlers (Phase 3), default-delegate `tea.KeyMsg` events to `m.viewport, cmd = m.viewport.Update(msg)`. The `bubbles/viewport` default keymap covers all the scroll keys listed in the spec.
- Add a `tea.WindowSizeMsg` branch:
  - Update `m.width = msg.Width`, `m.height = msg.Height`.
  - Call `m.viewport.Width = msg.Width` and `m.viewport.Height = msg.Height` (or use whatever the `bubbles/viewport` API exposes — some versions use `SetSize`).
  - Do NOT call `m.reader.Tail`.
  - Return updated model with no command (or any non-`Tail` command needed by viewport).
- Confirm by code review that `m.reader.Tail` is invoked only from `NewPreviewModel` (task 2-2). Phase 3 will add focus-change reads; this phase has only the initial-open read.
- Note: chrome takes vertical space (Phase 3); for Phase 2 the viewport occupies the full terminal. Resize uses the full terminal dimensions.
```

**Proposed** (`phase-2-tasks.md` Task 2-6 Do):
```
- In `previewModel.Update`, after the Esc branch (task 2-4) and before any cycle-key handlers (Phase 3), intercept `tea.KeyHome` and `tea.KeyEnd` explicitly — `bubbles@v1.0.0/viewport/keymap.go::DefaultKeyMap` does NOT bind these. Concretely:
  ```go
  case msg.Type == tea.KeyHome:
      m.viewport.GotoTop()
      return m, nil
  case msg.Type == tea.KeyEnd:
      m.viewport.GotoBottom()
      return m, nil
  ```
- For the remaining scroll keys (`Up`, `Down`, `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`, `j`, `k`), default-delegate `tea.KeyMsg` events to `m.viewport, cmd = m.viewport.Update(msg)` — the viewport's default keymap covers exactly these eight via `PageDown`, `PageUp`, `HalfPageUp`, `HalfPageDown`, `Down`, `Up`, `Left`, `Right` bindings (note: `Left`/`Right` are also passed through as harmless no-ops since the loaded N-line slice has no horizontal scroll dimension).
- Add a `tea.WindowSizeMsg` branch:
  - Update `m.width = msg.Width`, `m.height = msg.Height`.
  - Set `m.viewport.Width = msg.Width` and `m.viewport.Height = msg.Height` directly — `bubbles@v1.0.0/viewport.Model` exposes `Width` and `Height` as exported fields; there is no `SetSize` method in this version.
  - Do NOT call `m.reader.Tail`.
  - Return updated model with no command (or any non-`Tail` command needed by viewport).
- Confirm by code review that `m.reader.Tail` is invoked only from `NewPreviewModel` (task 2-2). Phase 3 will add focus-change reads; this phase has only the initial-open read.
- Note: chrome takes vertical space (Phase 3); for Phase 2 the viewport occupies the full terminal. Resize uses the full terminal dimensions.
```

**Current** (`phase-2-tasks.md` Task 2-6 Acceptance Criteria, lines 283–293):
```
**Acceptance Criteria**:
- [ ] `Down` scrolls the viewport down by one line; observable via `m.viewport.YOffset` increasing.
- [ ] `Up` scrolls the viewport up by one line.
- [ ] `Up` at the top of the loaded slice (`YOffset == 0`) is a silent no-op (no error, `YOffset` stays at 0).
- [ ] `Down` at the bottom of the loaded slice is a silent no-op (`YOffset` stays at max).
- [ ] `Home` jumps to `YOffset == 0`.
- [ ] `End` jumps to the bottom of the loaded slice.
- [ ] `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`, `j`, `k` all behave per `bubbles/viewport` defaults (no preview-specific overrides).
- [ ] `tea.WindowSizeMsg` resizes the viewport without calling `m.reader.Tail`.
- [ ] 100 successive `tea.WindowSizeMsg` events trigger zero additional `Tail` calls beyond the initial-open call (drag-resize simulation).
- [ ] Scroll offset is preserved across a `tea.WindowSizeMsg` (verified by setting `YOffset = 5`, then resizing, then asserting `YOffset == 5`).
```

**Proposed** (`phase-2-tasks.md` Task 2-6 Acceptance Criteria):
```
**Acceptance Criteria**:
- [ ] `Down` scrolls the viewport down by one line; observable via `m.viewport.YOffset` increasing.
- [ ] `Up` scrolls the viewport up by one line.
- [ ] `Up` at the top of the loaded slice (`YOffset == 0`) is a silent no-op (no error, `YOffset` stays at 0).
- [ ] `Down` at the bottom of the loaded slice is a silent no-op (`YOffset` stays at max).
- [ ] `Home` jumps to `YOffset == 0` via preview-owned `m.viewport.GotoTop()` (the viewport library does not bind `Home` natively).
- [ ] `End` jumps to the bottom of the loaded slice via preview-owned `m.viewport.GotoBottom()` (the viewport library does not bind `End` natively).
- [ ] `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`, `j`, `k` all behave per `bubbles/viewport` defaults (no preview-specific overrides).
- [ ] `tea.WindowSizeMsg` resizes the viewport without calling `m.reader.Tail`.
- [ ] 100 successive `tea.WindowSizeMsg` events trigger zero additional `Tail` calls beyond the initial-open call (drag-resize simulation).
- [ ] Scroll offset is preserved across a `tea.WindowSizeMsg` (verified by setting `YOffset = 5`, then resizing, then asserting `YOffset == 5`).
```

**Current** (`phase-2-tasks.md` Task 2-6 Tests, lines 295–303):
```
**Tests**:
- `"it scrolls down on Down key"` — open preview with multi-line content, drive Down; assert `YOffset` increased.
- `"it silently no-ops scroll-up at the top"` — open preview, drive Up while `YOffset == 0`; assert `YOffset == 0` and no error.
- `"it silently no-ops scroll-down at the bottom"` — drive End, then Down; assert `YOffset` unchanged.
- `"it jumps to top on Home"` — scroll down then drive Home; assert `YOffset == 0`.
- `"it jumps to bottom on End"` — drive End; assert `YOffset` is at max for the loaded slice.
- `"it does NOT call Tail on WindowSizeMsg"` — open preview (records 1 Tail call), send `tea.WindowSizeMsg{Width: 120, Height: 40}`; assert `Tail` call count remains 1.
- `"it does NOT call Tail across 100 successive WindowSizeMsg events"` — open preview, send 100 resize messages with varying dimensions; assert `Tail` count remains 1.
- `"it preserves scroll offset across resize"` — scroll to a known offset, resize, assert offset preserved.
```

**Proposed** (`phase-2-tasks.md` Task 2-6 Tests):
```
**Tests**:
- `"it scrolls down on Down key"` — open preview with multi-line content, drive Down; assert `YOffset` increased.
- `"it silently no-ops scroll-up at the top"` — open preview, drive Up while `YOffset == 0`; assert `YOffset == 0` and no error.
- `"it silently no-ops scroll-down at the bottom"` — synthesise `tea.KeyMsg{Type: tea.KeyEnd}` to jump to bottom, then drive Down; assert `YOffset` unchanged.
- `"it jumps to top on Home via preview-owned binding"` — scroll down with PgDn, then synthesise `tea.KeyMsg{Type: tea.KeyHome}`; assert `YOffset == 0`. Recipe note: the test exercises preview's own Home interception (`m.viewport.GotoTop()`), not viewport's default keymap, which has no Home binding in `bubbles@v1.0.0`.
- `"it jumps to bottom on End via preview-owned binding"` — synthesise `tea.KeyMsg{Type: tea.KeyEnd}`; assert `YOffset` equals viewport's max offset for the loaded slice. Recipe note: same as Home — preview owns this via `m.viewport.GotoBottom()`.
- `"it does NOT call Tail on WindowSizeMsg"` — open preview (records 1 Tail call), send `tea.WindowSizeMsg{Width: 120, Height: 40}`; assert `Tail` call count remains 1.
- `"it does NOT call Tail across 100 successive WindowSizeMsg events"` — open preview, send 100 resize messages with varying dimensions; assert `Tail` count remains 1.
- `"it preserves scroll offset across resize"` — scroll to a known offset, resize, assert offset preserved.
```

**Current** (`phase-3-tasks.md` Task 3-4 Acceptance Criteria, line 183):
```
- [ ] Viewport default scroll keys (`Up`, `Down`, `j`, `k`, `PgUp`, `PgDn`, `Home`, `End`, `ctrl-u`, `ctrl-d`) continue to pass through and produce viewport scroll offset changes.
```

**Proposed** (`phase-3-tasks.md` Task 3-4 Acceptance Criteria, line 183):
```
- [ ] Viewport default scroll keys (`Up`, `Down`, `j`, `k`, `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`) continue to pass through to `m.viewport.Update(msg)` and produce viewport scroll offset changes; `Home` and `End` (preview-owned via task 2-6 bindings) continue to invoke `m.viewport.GotoTop()` / `m.viewport.GotoBottom()` after this task's keymap-precedence reordering.
```

**Resolution**: Fixed
**Notes**: The drift is inherited from specification.md lines 65 and 431, which describe `Home` and `End` as "viewport defaults". The plan is the implementation blueprint; correcting it does not contradict spec intent (the spec wants `Home`/`End` to work) — it makes the wiring explicit so the implementer doesn't burn cycles on tests that fail because the library doesn't provide what the spec assumed it provided. The fix adds ~6 LOC to preview's Update (two `case` branches), aligns acceptance criteria with library reality, and pins test recipes against the actual API. No phase boundary, no architectural change, no scope expansion. Keeps the user-visible keymap matching the spec's working note.

---
