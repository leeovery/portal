---
status: complete
created: 2026-05-06
cycle: 7
phase: Plan Integrity Review
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Integrity

Re-read planning.md plus all four phase task detail files end-to-end with fresh eyes after cycle 6's two-fix landing (`internal/tmux/client.go` → `internal/tmux/tmux.go`, `c.Commander.Run` → `c.cmd.Run`). Cross-referenced every code-bearing identifier in the plan against:

- `internal/tui/model.go` — page constants (`PageLoading`, `PageSessions`, `PageProjects`, `pageFileBrowser`), Model fields (`sessionList`, `activePage`), handlers (`updateSessionList`), and `SessionsMsg`.
- `internal/tmux/tmux.go` — `*Client` shape (unexported `cmd Commander` field), `ListSessions` (with prefix-`_` filter at lines 149–162), `ListPanesInSession`, `PaneCoord`, `Session`, `PortalSaverName = "_portal-saver"`.
- `internal/state/panekey.go` — `SanitizePaneKey(session string, window, pane int) string` (line 27).
- `internal/state/paths.go` — `Dir()`, `EnsureDir()`, `ScrollbackFile(dir, paneKey)`, env-var resolution: `PORTAL_STATE_DIR` → `XDG_CONFIG_HOME/portal/state` → `~/.config/portal/state`.
- `internal/state/scrollback.go` — writers `SeedHashMap`, `CaptureAndHashPane`, `WriteScrollbackIfChanged`; `internal/state/markers.go::SetSkeletonMarker` — all referenced correctly in the audit task.
- `github.com/charmbracelet/bubbles@v1.0.0/viewport/viewport.go` — `Width`/`Height`/`YOffset` exported fields, `SetContent`, `GotoBottom`, `AtBottom`.
- `github.com/charmbracelet/bubbles@v1.0.0/list/list.go` — `Items`, `SelectedItem`, `Index`, `IsFiltered`, `FilterValue`, `SettingFilter`, `FilterState`.
- `github.com/charmbracelet/bubbletea@v1.3.10/key.go` — KeyType constants including `KeyTab`, `KeyEsc`, `KeyRunes`, **and `KeySpace`** (line 219). The runtime parser converts a standalone space rune to `Type: KeySpace`, not `Type: KeyRunes` (lines 696–702: `if len(runes) == 1 && runes[0] == ' ' { k.Type = KeySpace }`). Confirmed by existing in-tree usage at `internal/ui/browser.go:176` (`case tea.KeySpace:`) and `internal/ui/browser_test.go:477` (`tea.KeyMsg{Type: tea.KeySpace}`).
- `github.com/charmbracelet/bubbles@v1.0.0/key/key.go::Matches` — string-compares `Key.String()` against `Binding.keys`; `key.WithKeys(" ")` matches both `Type: KeySpace` (via the `keyNames` map entry `KeySpace: " "`) and `Type: KeyRunes, Runes: ' '` (via the rune render path), so the **production** binding shape is correct in the plan; the **test** synthesis shape introduced in cycle 1 is not.

Cycles 1–6's prior fixes (method/file/key-constant alignments, table-row name, model field names, handler name, page-constant casing, file path, Commander field access) all remain applied with no regression.

**Overall assessment**: One material drift surfaces — and notably, this drift was **introduced** by the cycle 1 review itself, rather than being a pre-existing error. The original task 2-3 test wording said `tea.KeyMsg{Type: tea.KeySpace}` (correct, matches what the bubbletea runtime emits and what the existing Portal handler `internal/ui/browser.go:176` consumes); cycle 1's reviewer misread the bubbletea API, asserted "Bubble Tea has no `tea.KeySpace` constant", and replaced the correct shape with `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}` plus a parenthetical that is factually wrong. That parenthetical now stands as the single sentence in the plan an implementer would read and trust over the standard library, contradicting both the language and the existing in-tree pattern. Reverting to the original form (with no misleading parenthetical) is mechanical and isolates the regression.

A second, smaller misalignment surfaces in task 2-7's Edge Cases section, where `XDG_STATE_HOME` is named as a possible env source. The actual `internal/state/paths.go::Dir` consults `PORTAL_STATE_DIR` and falls back through `XDG_CONFIG_HOME` (not `XDG_STATE_HOME`). The surrounding direction ("use the existing `internal/state` paths helper") steers the implementer to the right call site, so this is not load-bearing — Minor — but worth pinning while we're here.

No other findings. Phases 1, 3, and 4 are clean against current code. Architecture, vertical slicing, dependency edges, AC quality, scope/granularity, and self-containment all remain sound.

## Findings

### 1. Cycle-1 regression: task 2-3 test wording asserts a false claim about `tea.KeySpace`, contradicting bubbletea API and existing Portal handlers

**Severity**: Important
**Plan Reference**: `phase-2-tasks.md` — Task 2-3 (Tests, first entry — line 147)
**Category**: Task Self-Containment / Tests
**Change Type**: update-task

**Details**:
The plan's first Tests entry for task 2-3 currently reads:

> `"it transitions to pagePreview on Space when a session is highlighted"` — synthesise a `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}` (Bubble Tea has no `tea.KeySpace` constant — space is a runes key), drive `Update`, assert `m.activePage == pagePreview`.

The parenthetical claim is false. `tea.KeySpace` is a defined `KeyType` constant in `bubbletea@v1.3.10/key.go:219`, and the runtime parser at `key.go:696–702` actively re-types a standalone space rune as `Type: KeySpace`:

```go
// If we found at least one rune, we report the bunch of them as
// a single KeyRunes or KeySpace event.
if len(runes) > 0 {
    k := Key{Type: KeyRunes, Runes: runes, Alt: alt}
    if len(runes) == 1 && runes[0] == ' ' {
        k.Type = KeySpace
    }
    return i, KeyMsg(k)
}
```

The codebase already follows this pattern: `internal/ui/browser.go:176` matches with `case tea.KeySpace:` and `internal/ui/browser_test.go:477` synthesises `tea.KeyMsg{Type: tea.KeySpace}`. If the preview handler follows the in-tree convention (a `case tea.KeySpace:` branch — the natural choice given `internal/ui/browser.go`'s precedent), a test that hand-builds `Type: tea.KeyRunes, Runes: []rune{' '}}` will not exercise that branch — it will fall through unmatched and the test will silently pass even when the production handler is broken (because the test never reached it). Conversely, if the implementer reads the parenthetical and writes the production handler with `case tea.KeyRunes:` checking `Runes` content (against the `case tea.KeySpace:` convention used elsewhere), the test would pass but the production code would not match real keystrokes from bubbletea — every actual `Space` press in production goes through `Type: KeySpace`.

The bubbles/key binding form (`key.NewBinding(key.WithKeys(" "))`) suggested in the plan's Do section happens to match both shapes via `Key.String()` rendering through different code paths, so production code that uses key.Matches with `WithKeys(" ")` will work either way — but the test recipe for hand-building a `tea.KeyMsg` is shape-specific and the plan's current version is wrong.

This drift was introduced by the cycle-1 review (see `review-integrity-tracking-c1.md` finding 3). The pre-cycle-1 plan said `tea.KeyMsg{Type: tea.KeySpace}`, which was correct. The cycle-1 reviewer asserted the constant did not exist and replaced the correct shape. Six subsequent integrity cycles have re-read this line without catching the regression because the misleading parenthetical anchors the reader's interpretation. Reverting to the original shape (and removing the parenthetical altogether — the constant requires no defensive comment) restores plan accuracy and aligns with the existing in-tree usage.

The acceptance criteria on task 2-3 already list "no second binding for filter mode" and a literal Space passthrough during `SettingFilter()`, neither of which constrain the test-synthesis shape — the plan reads cleanly with the simpler line.

**Current** (Task 2-3 Tests, first entry):
```
- `"it transitions to pagePreview on Space when a session is highlighted"` — synthesise a `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}` (Bubble Tea has no `tea.KeySpace` constant — space is a runes key), drive `Update`, assert `m.activePage == pagePreview`.
```

**Proposed**:
```
- `"it transitions to pagePreview on Space when a session is highlighted"` — synthesise a `tea.KeyMsg{Type: tea.KeySpace}` (matches the runtime shape bubbletea produces for a standalone space keypress; see `internal/ui/browser_test.go` for the existing in-tree pattern), drive `Update`, assert `m.activePage == pagePreview`.
```

**Resolution**: Fixed
**Notes**: Reverts the cycle-1 regression. No production-code recipe changes — the Do section still uses `key.NewBinding(key.WithKeys(" "))`, which works regardless. Only the test-synthesis identifier and the no-longer-true parenthetical change. The new parenthetical points the implementer at the in-tree precedent (`internal/ui/browser_test.go:477`), which is the correct authority for "how Portal tests synthesise Space".

---

### 2. Task 2-7 Edge Cases names `XDG_STATE_HOME` but state.Dir consults `XDG_CONFIG_HOME`

**Severity**: Minor
**Plan Reference**: `phase-2-tasks.md` — Task 2-7 (Edge Cases — first bullet)
**Category**: Task Self-Containment

**Details**:
Task 2-7's Edge Cases section currently reads:

> The `stateDir` resolution helper may consult `XDG_STATE_HOME` or fall back to a default — preview must use the exact same source the daemon uses (so both sides resolve to the same directory, otherwise preview reads from an empty dir).

The actual resolution chain in `internal/state/paths.go::Dir()` (lines 34–44) is:

1. `$PORTAL_STATE_DIR` — used verbatim, no suffix appended.
2. `$XDG_CONFIG_HOME/portal/state` — when `XDG_CONFIG_HOME` is set.
3. `$HOME/.config/portal/state` — fallback.

The function does not consult `XDG_STATE_HOME` at any point. The Edge Cases bullet's claim is factually incorrect. The bullet's escape-hatch wording ("may consult ... or fall back to a default") and the surrounding Do-section direction ("Resolve `stateDir` via the existing `internal/state` paths helper ... review `internal/state/paths.go`") together steer an implementer toward the actual API — so the misleading line is unlikely to materially derail implementation — but the claim is wrong and Cycle 7 was asked to surface code-symbol drifts. Pinning the Edge Cases bullet to the actual env-var sources prevents an implementer from defensively threading `XDG_STATE_HOME` plumbing through preview.

**Current** (Task 2-7 Edge Cases — first bullet):
```
- The `stateDir` resolution helper may consult `XDG_STATE_HOME` or fall back to a default — preview must use the exact same source the daemon uses (so both sides resolve to the same directory, otherwise preview reads from an empty dir).
```

**Proposed**:
```
- The `stateDir` resolution helper consults `$PORTAL_STATE_DIR` first, then falls back through `$XDG_CONFIG_HOME/portal/state` to `$HOME/.config/portal/state` (per `internal/state/paths.go::Dir`) — preview must use this exact helper so both preview and the daemon resolve to the same directory; otherwise preview reads from an empty dir.
```

**Resolution**: Fixed
**Notes**: Mechanical alignment with `internal/state/paths.go`. No spec content changes; no acceptance criterion changes meaning; no architectural impact. Pin keeps the plan factually accurate without expanding scope.
