# Plan: Enter Attaches From Preview

## Phases

### Phase 1: Enter binding with pre-select and attach

status: approved
approved_at: 2026-05-15

**Goal**: Add the `Enter` keybinding to the preview page that commits an attach to the previewed session, honouring the `(window, pane)` focus the user navigated to with `]`/`[`/`Tab`. Implement the four-call sequence (`has-session` → `select-window` → `select-pane` → connector) with exact-match `=<session>` targets, the `*exec.ExitError` vs OS-error discriminator, best-effort silent-WARN handling for pre-select failures, and the chrome line `enter attach` token. Non-zero `has-session` exit dismisses to Sessions without attaching (placeholder bail — full flash UX lands in Phase 2).

**Why this order**: This is the core new capability and the foundation every other surface in the spec depends on. It establishes the connector wiring shape, the exact-match target convention applied uniformly across all four calls, the raw-tmux-index addressing contract, and the ExitError discriminator that Phase 2's flash path consumes. Without this phase, there is nothing for the bail UX to bail from.

**Acceptance**:

- [ ] Pressing `Enter` on the preview page invokes `has-session`, `select-window`, `select-pane`, then the connector in order, all using `-t '=<session>'` exact-match targets
- [ ] `select-window` and `select-pane` use raw tmux `window_index` / `pane_index` values (not slice positions) sourced from preview's existing `currentRawIndices()` helper or equivalent
- [ ] When the user has navigated with `]`/`[`/`Tab`, the connector lands them on the navigated `(window, pane)`; when they have not navigated, the connector lands them on the captured-at-open `(window, pane)`
- [ ] `select-window` / `select-pane` non-zero exits log at WARN through the `internal/state` structured logger with a greppable component string, are swallowed, and do not block the connector handoff
- [ ] `has-session` non-zero exit (`*exec.ExitError`) bails out of the attach sequence; OS-layer errors (non-`ExitError`) are treated as "session present" and the sequence proceeds
- [ ] `Enter` is intercepted by preview's `Update` handler and is NOT forwarded to the embedded `bubbles/viewport`
- [ ] The preview chrome line reads exactly `Window {w} of {wN} · Pane {p} of {pN} · win: {name}    ] next win · [ prev win · tab next pane · enter attach · esc back` regardless of viewport content state
- [ ] Connector targets remain session-only (no `:window.pane` suffix) — `attach-session -A -t '=<session>'` outside tmux, `switch-client -t '=<session>'` inside tmux
- [ ] Existing preview behaviour (open via Space, `]`/`[`/`Tab` navigation, viewport scroll, `Esc` dismiss with sessions-list refresh) is unchanged
- [ ] Test suite green; `go test ./...` passes

#### Tasks

status: approved
approved_at: 2026-05-15

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| enter-attaches-from-preview-1-1 | Add SelectWindow method to tmux.Client | window no longer exists (wrapped non-zero exit error) |
| enter-attaches-from-preview-1-2 | Apply exact-match `=` target prefix uniformly across HasSession, SelectWindow, SelectPane, SwitchClient, and AttachConnector | prefix-collision ("foo" vs "foo-2"), session names containing `=`, existing non-Enter callers unaffected |
| enter-attaches-from-preview-1-3 | Add ExitError discriminator for has-session probe | `*exec.ExitError` (absent → bail), non-`ExitError` OS-layer error (proceed), zero exit (present), existing boolean HasSession callers unchanged |
| enter-attaches-from-preview-1-4 | Build four-call attach pipeline tea.Cmd factory with bail message and WARN-swallow logger | select-window non-zero exit (log + proceed), select-pane non-zero exit (log + proceed), has-session ExitError (bail before selects), has-session OS-layer error (proceed), connector handoff error, inside-tmux vs outside-tmux connector branches |
| enter-attaches-from-preview-1-5 | Wire attach pipeline seam into tuiConfig, tui.Model, and openTUI production construction | inside-tmux selects SwitchConnector, outside-tmux selects AttachConnector, test fake injection via t.Cleanup |
| enter-attaches-from-preview-1-6 | Intercept tea.KeyEnter in previewModel.Update and dispatch pipeline with raw indices | user did not navigate (captured-at-open indices), user navigated via `]`/`[`/`Tab` (walked indices), Enter must not propagate to embedded viewport, raw indices used on non-contiguous index sessions |
| enter-attaches-from-preview-1-7 | Handle previewAttachBailMsg in top-level Model.Update with placeholder bail (transition + refresh, no flash) | bail arrives mid-preview, refresh cmd error tolerated silently, existing Esc dismiss path unchanged |
| enter-attaches-from-preview-1-8 | Update preview chromeLine to include `enter attach` token between `tab next pane` and `esc back` | token unconditional across viewport content states (real bytes / `(no saved content)` / `(unable to read scrollback)`), Sessions-page help bar untouched |

### Phase 2: Session-killed-externally bail path with inline flash

status: approved
approved_at: 2026-05-15

**Goal**: When `has-session` returns non-zero, dispatch the refresh-and-bail message that transitions `pagePreview → pageSessions`, triggers the existing dismiss-time sessions-list refresh, and emits a feature-local inline flash on the Sessions page reading exactly `session "<name>" no longer exists`. Add the flash state, render row, tick-based auto-clear, keystroke-based clear, and rapid-bail replacement semantics to the Sessions page model.

**Why this order**: Builds on Phase 1's `has-session` discriminator and bail signal. This phase is scoped to Sessions-page model state and chrome — a distinct architectural surface from Phase 1's preview/connector work. Separating it isolates the new persistent UI state (flash text, tick handle, replacement bookkeeping) from the attach mechanics, and keeps the Phase 1 deliverable shippable in its own right.

**Acceptance**:

- [ ] On `has-session` non-zero exit, preview transitions to the Sessions page, the existing sessions-list refresh dispatches on that transition, and a flash with the exact text `session "<captured-name>" no longer exists` (double quotes, no trailing punctuation, no paraphrase) is emitted from the same `Update` return
- [ ] The flash renders as a single chrome row between the filter input and the Sessions list; when no flash is active, no row is reserved (list sits directly under the filter input)
- [ ] Flash clears on the next actionable `tea.KeyMsg`; modifier-only events, resize events, and focus events do NOT clear the flash
- [ ] The first post-bail keystroke clears the flash AND applies to the filter input (one key, one intent — flash does not swallow the keystroke)
- [ ] Flash auto-clears via a tick `tea.Cmd` after a build-chosen short duration (`~3s` per spec guidance: long enough to read, short enough not to linger)
- [ ] A new bail while a prior flash is visible replaces the text and resets the tick; the prior pending tick must not clear the new flash early (verified by test)
- [ ] Flash render is not gated on refresh completion — a transient frame showing prior list state plus the flash is acceptable, killed-session row is removed by the next render
- [ ] The Sessions-page help bar is unaffected (still advertises Sessions-page `Enter` semantics, no preview-chrome propagation)
- [ ] Test suite green; `go test ./...` passes

#### Tasks

status: approved
approved_at: 2026-05-15

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| enter-attaches-from-preview-2-1 | Add flash state fields and setFlash/clearFlash helpers to Sessions page model | zero-value model has no flash, setFlash increments generation monotonically, clearFlash resets to zero-value |
| enter-attaches-from-preview-2-2 | Render conditional flash row between filter input and Sessions list | no row reserved when flashText empty, row appears between filter input and list when active, list shifts down by exactly one row, no overlay of existing chrome |
| enter-attaches-from-preview-2-3 | Add flashTickMsg with generation guard for tick-based auto-clear | stale-generation tick does NOT clear current flash, current-generation tick clears flash, tick after manual clear is no-op, build-chosen duration honours spec principle (~3s) |
| enter-attaches-from-preview-2-4 | Clear flash on actionable KeyMsg without swallowing keystroke | first keystroke clears flash AND lands in filter input, modifier-only key does not clear, WindowSizeMsg does not clear, focus events do not clear |
| enter-attaches-from-preview-2-5 | Replace placeholder previewAttachBailMsg handler with refresh + exact-text flash dispatch | exact wording `session "<name>" no longer exists` (double quotes, no trailing punctuation, no paraphrase), session name with special chars preserved verbatim, dispatch issued in single Update return, flash render not gated on refresh completion |
| enter-attaches-from-preview-2-6 | Rapid-bail replacement resets text and supersedes prior tick via generation bump | second bail replaces first text, prior in-flight tick does not clear new flash early, second bail's own tick still clears at its own deadline, N successive bails preserve only latest |

### Phase 3: Analysis (Cycle 1)

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| enter-attaches-from-preview-3-1 | Restructure preview-Enter to hand off connector after TUI quit | inside-tmux path leaves no orphan portal process, outside-tmux path unchanged (still terminates via syscall.Exec), pinning test for nil-cmd on success is updated, docstring reconciled with new shape |
| enter-attaches-from-preview-3-2 | Extract shared preview-teardown helper for dismiss + bail handlers | Esc-dismiss behaviour unchanged, bail still uses msg.Session for preserveName, no behavioural change visible to tests |

### Phase 4: Analysis (Cycle 2)

**Goal**: Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| enter-attaches-from-preview-4-1 | Close previewLogger to remove lifecycle asymmetry | repeated openTUI invocations would otherwise leak fd, Logger.Close is nil-safe, no other call sites change |
| enter-attaches-from-preview-4-2 | Delete redundant flashModelOnSessionsPage alias | all callers rewritten to flashModelWithSessions, function definition removed, tests pass unchanged |
| enter-attaches-from-preview-4-3 | Extract singlePaneGroups() test helper for repeated stubEnumerator fixture | 21 occurrences across 3 test files replaced, literal appears at most once outside helper, tests pass unchanged |
