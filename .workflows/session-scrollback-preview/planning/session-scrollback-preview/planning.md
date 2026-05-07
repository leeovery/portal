# Plan: Session Scrollback Preview

## Phases

### Phase 1: Read pipeline and structural enumeration foundations
status: approved
approved_at: 2026-05-07

**Goal**: Establish the two production seams the preview page depends on — a tail-N scrollback reader in `internal/state` and a window-grouped pane enumeration call on `tmux.Client` — each independently testable.

**Why this order**: Preview's TUI logic is built against `TmuxEnumerator` and `ScrollbackReader` interfaces. Landing the production implementations first means Phase 2 can integrate against real adapters from day one, and the performance budget (p99 < 5ms tail-N read on a 4MB `.bin`) is verified before TUI code commits to synchronous reads in `Update`.

**Acceptance**:
- [ ] A tail-N helper exists in `internal/state` that returns at most the last 1000 newline-terminated records from a `.bin` path, using a single fd held across the reverse chunked scan.
- [ ] The helper returns `(bytes, nil)` for normal content, `(nil, nil)` for ENOENT / zero-byte / zero-terminated-line outcomes, and `(nil, err)` for OS-level read failures — exactly the three-shape contract in the spec.
- [ ] Files whose final bytes lack a trailing `\n` have those trailing bytes excluded from the returned tail.
- [ ] A benchmark guards p99 < 5ms tail-N read on a 4MB representative `.bin` file; CI fails if the budget regresses.
- [ ] A new read-only `tmux.Client` method returns window-grouped panes plus window names for a single session in one `list-panes -F` invocation, ordered by `window_index` then `pane_index`.
- [ ] The enumeration call surfaces tmux failures as errors (consumed by preview as the silent-no-open path) and returns an empty result faithfully when a session has zero windows or panes.
- [ ] Existing `tmux.Client` capture path is unchanged (no new capture wrappers, `CapturePane` signature untouched).

### Phase 2: Preview page entry, dismiss, and single-pane content rendering
status: approved
approved_at: 2026-05-07

**Goal**: Add the `pagePreview` arm to the TUI page state machine with constructor-injected `TmuxEnumerator` and `ScrollbackReader` seams. `Space` on the Sessions page opens preview for the highlighted session, the focused pane's tail-N bytes render verbatim into a `bubbles/viewport`, and `Esc` returns to the Sessions list at the original cursor.

**Why this order**: This is the smallest vertical slice that delivers a usable feature — for the empirically dominant ~95% single-window single-pane case, this phase alone covers the user need. Multi-pane cycling and chrome layer additively on top of a working preview page.

**Acceptance**:
- [ ] A new `pagePreview` arm exists in `internal/tui/model.go::Update`, peer of `pageFileBrowser`, with its own keymap.
- [ ] Pressing `Space` on a highlighted session in the Sessions page opens preview; pressing `Space` on an empty Sessions list is a no-op.
- [ ] The preview model is constructor-injected with `TmuxEnumerator` and `ScrollbackReader` interfaces — no package-level `previewDeps` variable; tests safely use `t.Parallel()` in `pagepreview_test.go` without touching mutable package state.
- [ ] On open, structural enumeration runs first; if it fails or returns empty (zero windows or zero panes), preview does not open and the user remains on the Sessions list silently.
- [ ] On successful enumeration, focus is set to window 0 / pane 0 (enumeration order) and the tail-N read for that pane runs synchronously in `Update`; the first frame renders viewport content atomically with the page transition.
- [ ] The viewport receives raw ANSI bytes verbatim via `SetContent` — no preprocessing, sanitisation, re-wrap, or escape stripping.
- [ ] `bubbles/viewport` native scroll keys (`Up`, `Down`, `PgUp`, `PgDn`, `Home`, `End`, `ctrl-u`, `ctrl-d`, `j`, `k`) scroll within the loaded N-line slice; scroll-up at the top silently no-ops.
- [ ] `tea.WindowSizeMsg` is forwarded to the embedded viewport for re-flow and does NOT trigger a fresh disk read.
- [ ] `Esc` returns to the Sessions list with the cursor at the same position it was on when `Space` was pressed; filter state (no filter / committed / mid-typing) is preserved per the Esc level tree.
- [ ] While the `bubbles/list` filter input is active (`SettingFilter`), `Space` inserts a literal space and does not open preview; after `Enter` commits the filter, `Space` opens preview on the highlighted match.
- [ ] Re-opening preview on the same session constructs a fresh `previewModel` — no focus, scroll, or content state survives.
- [ ] Loading and Projects/FileBrowser pages have no `Space` binding for preview.

### Phase 3: Multi-pane cycling, chrome, and focus-change reads
status: approved
approved_at: 2026-05-07

**Goal**: Layer the within-preview keymap (`]`, `[`, `Tab`) and chrome rendering (window M of N, pane X of Y, window name, keystroke hints) onto the preview page, with each focus-changing event triggering a fresh tail-N read of the newly-focused pane.

**Why this order**: Builds on the working single-pane preview from Phase 2 by adding the structural-disambiguation surface for the multi-pane minority. The cycling keymap and chrome are tightly coupled — chrome counters update in lockstep with focus indices — so they belong together. Focus-change reads reuse the synchronous read pipeline already validated in Phase 1's benchmark.

**Acceptance**:
- [ ] `]` advances to the next window with last→first wrap; `[` advances to the previous window with first→last wrap.
- [ ] `Tab` advances to the next pane within the current window with last→first wrap (forward-only).
- [ ] After `]` or `[` lands on a new window, focus resets to pane 0 of that window; per-window pane focus is not preserved across window cycles.
- [ ] In single-window single-pane sessions, `]`, `[`, and `Tab` silently no-op (no flicker, no error).
- [ ] Each focus-changing event (initial open, `]`, `[`, `Tab`) triggers exactly one fresh tail-N read of the newly-focused pane via `ScrollbackReader.Tail(paneKey)`; scroll position resets to tail on every focus change.
- [ ] Chrome shows window M of N, pane X of Y (1-based ordinal positions in enumeration order — never raw `window_index` / `pane_index`), window name (`#W`), and visible cycle-key hints (`]`, `[`, `Tab`, `Esc`).
- [ ] Under non-contiguous tmux `window_index` values (e.g. `0, 2, 5`) or base-index 1, the chrome counters stay `1..N` and never expose raw tmux indices.
- [ ] Chrome data is captured once at preview-open via the structural enumeration; no live re-enumeration runs mid-preview during cycling.
- [ ] Pane key resolution uses `state.SanitizePaneKey(session, window_index, pane_index)` → `state.ScrollbackFile(stateDir, paneKey)` for every read, with `stateDir` captured once at TUI startup and hidden behind the `ScrollbackReader` interface.
- [ ] Within-preview keymap collisions with `bubbles/viewport` defaults are resolved in preview's favour — `]`, `[`, and `Tab` are owned by preview and not consumed by the embedded viewport.

### Phase 4: Edge-case handling and cross-cutting integration
status: approved
approved_at: 2026-05-07

**Goal**: Pin down the three placeholder/error outcomes, the externally-killed-session re-fetch contract, the brand-new-session case, and the side-effect-free invariant — so preview behaves correctly across every scenario the spec acceptance criteria enumerate.

**Why this order**: These behaviours are observable refinements on top of a working multi-pane preview. They are easier to test once the core page exists, and they form the hardening surface that makes preview production-ready.

**Acceptance**:
- [ ] A pane whose `ScrollbackReader.Tail` returns `(nil, nil)` — covering ENOENT, zero-byte file, and zero-line file with only an unterminated partial — renders the "(no saved content)" placeholder in the viewport while chrome counts remain correct.
- [ ] A pane whose `Tail` returns `(nil, err)` renders a single short error string (uniform across errno types — no EACCES vs EIO branching); subsequent focus changes onto the same pane retry the read fresh (no per-pane error cache).
- [ ] A pane with fewer than 1000 lines renders all available lines with the viewport at scroll-tail — no placeholder.
- [ ] A brand-new session whose panes have no `.bin` yet shows the placeholder for each pane while `]`, `[`, and `Tab` continue to traverse the structural shape.
- [ ] When `Esc` returns from preview to the Sessions page, the Sessions list is re-fetched before rendering — verified by either confirming existing on-entry refresh or adding a refresh dispatch in the `pagePreview → pageSessions` transition handler. A killed session does not appear in the list on return.
- [ ] An externally-killed session whose preview is held open continues rendering with placeholders as `.bin` files are cleaned by the daemon; chrome stays stable (no live re-enumeration); cycle keys still traverse the captured shape.
- [ ] `_portal-saver` exclusion is confirmed at the Sessions-list source; if any leak is found, it is fixed at the source — no preview-layer name blacklist is introduced.
- [ ] A side-effect-free hermetic test opens preview on a session, cycles through every pane, and dismisses, asserting: exactly one `TmuxEnumerator` call, `ScrollbackReader.Tail` calls only (one per focus event, no other I/O), zero `hooks.Store` calls, zero `state` package writes (`SetSkeletonMarker`, `WriteScrollbackIfChanged`, etc.), and zero FIFO creation/drain.
- [ ] No new methods are added to `tmux.Client` beyond the single read-only listing method from Phase 1; no daemon, restore, bootstrap, hooks, or save-format changes are made.
