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

#### Tasks
status: approved
approved_at: 2026-05-07

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-scrollback-preview-1-1 | Tail-N reverse-scan helper happy path | file with fewer than N lines returns all lines, multi-chunk reverse scan when N lines span chunk boundary, file exactly N lines |
| session-scrollback-preview-1-2 | Tail-N helper no-content shape | ENOENT vs zero-byte file, file with content but no trailing newline, file with only unterminated partial line, file with terminated lines plus trailing partial |
| session-scrollback-preview-1-3 | Tail-N helper OS-error shape | permission-denied open, mid-scan read error, ensure ENOENT does NOT take this branch |
| session-scrollback-preview-1-4 | Tail-N performance benchmark | fixture generation cost excluded from measured region, benchmark resets timer after setup |
| session-scrollback-preview-1-5 | Window-grouped pane enumeration on tmux.Client | non-contiguous window_index (0, 2, 5), base-index 1 / pane-base-index 1, window names containing the pipe delimiter or whitespace, multiple panes per window |
| session-scrollback-preview-1-6 | Enumeration failure and empty-result handling | session-disappeared mid-call (tmux exit non-zero), empty stdout vs error, ensure no new capture wrapper introduced |

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

#### Tasks
status: approved
approved_at: 2026-05-07

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-scrollback-preview-2-1 | Define TmuxEnumerator and ScrollbackReader seam interfaces in internal/tui | Tail three-shape contract (bytes-nil / nil-nil / nil-err), stateDir hidden behind interface (closed over at construction) |
| session-scrollback-preview-2-2 | previewModel constructor with injected seams and initial-open flow | enumeration error signals no-open, empty enumeration (zero windows or window with zero panes) signals no-open, tail-N read failure does not block open, raw bytes passed verbatim to viewport.SetContent |
| session-scrollback-preview-2-3 | Add pagePreview arm to page state machine and bind Space on Sessions page | empty Sessions list Space is no-op, no highlighted item is no-op, enumeration failure stays on sessions silently, Loading and Projects/FileBrowser pages have no Space binding |
| session-scrollback-preview-2-4 | Esc dismiss returns to Sessions list preserving cursor and filter state | cursor preserved across open/dismiss, committed filter still applied on return, no filter case, re-opening preview constructs fresh model with no carried state |
| session-scrollback-preview-2-5 | Filter-mode Space passthrough integration | literal space inserted into filter text while SettingFilter is true, Space after Enter-commit opens preview on highlighted match, no second binding for open-while-filtering |
| session-scrollback-preview-2-6 | Viewport scroll keys and resize handling within preview | scroll-up at top silently no-ops, scroll-down at bottom silently no-ops, drag-resize incurs zero tail-N reads, scroll offset preserved across resize |
| session-scrollback-preview-2-7 | Production adapters wired at TUI construction | stateDir captured once for process lifetime, pane key derivation via state.SanitizePaneKey matching daemon writer call site verbatim, no tmuxtest import in pagepreview_test.go |

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

#### Tasks
status: approved
approved_at: 2026-05-07

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-scrollback-preview-3-1 | Focus state and pane-key resolution helpers | non-contiguous window_index (0, 2, 5), base-index 1 / pane-base-index 1, pane key uses raw tmux window_index / pane_index not ordinal position |
| session-scrollback-preview-3-2 | Tab cycle: next pane within current window with wrap and re-read | single-pane window Tab is silent no-op, last pane wraps to first, scroll position resets to tail on focus change, exactly one Tail call per Tab press |
| session-scrollback-preview-3-3 | Bracket cycles: next/previous window with pane-0 reset and re-read | single-window session keys silent no-op, `]` last wraps to first, `[` first wraps to last, per-window pane focus not preserved, scroll resets to tail |
| session-scrollback-preview-3-4 | Keymap precedence over embedded viewport | `]` `[` `Tab` never reach viewport.Update, scroll keys still passthrough, no double-handling |
| session-scrollback-preview-3-5 | Chrome rendering: counters, window name, and keystroke hints | non-contiguous window_index never leaks into M/N, base-index 1 still shows 1..N, window name with spaces or unusual characters, hint string visible |
| session-scrollback-preview-3-6 | Chrome layout integration with viewport sizing | small terminal heights, tea.WindowSizeMsg updates chrome+viewport atomically, resize triggers zero Tail calls, chrome line height stable across cycles |
| session-scrollback-preview-3-7 | Chrome stability under focus changes (no mid-preview re-enumeration) | full cycle of `]` `[` `Tab` produces one Enumerate call only, counters update from cached groups, no live tmux re-enumeration mid-preview |

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

#### Tasks
status: approved
approved_at: 2026-05-07

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| session-scrollback-preview-4-1 | Placeholder rendering for (nil, nil) Tail outcomes | ENOENT, zero-byte file, zero-line file with only an unterminated partial line, chrome integrity under placeholder, focused pane at open vs after cycle |
| session-scrollback-preview-4-2 | Error-string rendering for (nil, err) Tail outcomes with retry-on-refocus | uniform string across errno types (no EACCES vs EIO branch), no per-pane error cache, retry on Tab-away-and-back, retry on `]` / `[` and back |
| session-scrollback-preview-4-3 | Fewer-than-N lines renders all available lines (non-placeholder partial read) | 1-line file, exactly 999 lines, viewport opens at scroll-tail not scroll-top, scroll-up at top is silent no-op |
| session-scrollback-preview-4-4 | Brand-new-session traversal with placeholders on every pane | every pane (nil, nil), mixed session where one pane has bytes and others do not, chrome counts correct under all-placeholder state, cycle keys traverse all structural entries |
| session-scrollback-preview-4-5 | Sessions-list re-fetch on pagePreview → pageSessions transition | existing on-entry refresh path discovered vs gap requiring new dispatch, cursor lands on a neighbouring session when previous session is gone, no-op when nothing has changed |
| session-scrollback-preview-4-6 | Externally-killed-session in-preview stability with progressive placeholders | `.bin` deleted between focus events, all panes turn into placeholders, chrome counters do not change mid-preview, no errors crash the page |
| session-scrollback-preview-4-7 | Confirm _portal-saver exclusion at Sessions-list source | leak found and fixed at source vs already excluded (audit-only outcome), regression-pin test, no preview-layer name blacklist introduced |
| session-scrollback-preview-4-8 | Side-effect-free hermetic invariant test | exactly one TmuxEnumerator call across full cycle, exactly one ScrollbackReader.Tail call per focus event and zero other I/O, zero hooks.Store calls, zero state-package writes, zero FIFO creation or drain |
| session-scrollback-preview-4-9 | No-new-surface audit and regression guard | audit-only task, no incidental new tmux.Client wrappers, no daemon / restore / bootstrap / hooks / save-format changes |
