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
