---
phase: 4
phase_name: Edge-case handling and cross-cutting integration
total: 9
---

## session-scrollback-preview-4-1 | approved

### Task 4-1: Placeholder rendering for (nil, nil) Tail outcomes

**Problem**: Phase 2 wired `ScrollbackReader.Tail` into the preview model but rendered raw bytes verbatim without finalising the `(nil, nil)` "no content available" branch. The spec collapses ENOENT, zero-byte file, and zero-line-file (only an unterminated partial) into one observable outcome — a single placeholder string in the viewport — and that wording must be pinned now or the user sees an empty viewport with no signal that the file exists-but-is-empty vs is-being-disambiguated.

**Solution**: Finalise the `(nil, nil)` branch at the call site in `internal/tui/pagepreview.go` (the preview model's read-and-render path). When `Tail` returns `(nil, nil)`, set the viewport content to the placeholder string `(no saved content)` (the spec's working label, build-phase pinned). Chrome (window M of N, pane X of Y, window name, hint) is unaffected — it comes from the structural enumeration, not from `.bin` content. Define the placeholder as a package-level constant in `internal/tui` so the wording is one place.

**Outcome**: For every focus event whose `Tail` returns `(nil, nil)` — at preview-open or after `]` / `[` / `Tab` — the viewport renders the literal string `(no saved content)` on its own line, while the chrome line continues to display correct counts and window name. No empty viewport, no error noise, no crash.

**Do**:
- In `internal/tui/pagepreview.go` (or the preview model's render/read helper, depending on Phase 2 layout), define `const previewPlaceholder = "(no saved content)"`.
- In the read-and-render path that handles `Tail` outcomes, branch on `(bytes == nil && err == nil)` and call `viewport.SetContent(previewPlaceholder)` for that branch. Keep the verbatim-bytes branch and the error-branch (Task 4-2) untouched.
- Ensure the placeholder branch fires at both initial open (window 0 / pane 0 read in the constructor flow) and post-cycle reads (the `]` / `[` / `Tab` handlers that already call `Tail`).
- Confirm the chrome-line render code in the `View` method does not consult `bytes`/`err` — chrome is purely a function of the captured enumeration plus current focus indices.

**Acceptance Criteria**:
- [ ] When `Tail` returns `(nil, nil)`, viewport content equals `(no saved content)`.
- [ ] When `Tail` returns `(nil, nil)`, chrome shows correct `Window M of N`, `Pane X of Y`, and window name from the captured enumeration.
- [ ] The placeholder branch fires identically at initial-open and post-cycle reads.
- [ ] The placeholder string is a single package-level constant — the wording exists in exactly one place.
- [ ] No code path attempts to interpret a `(nil, nil)` outcome as "error" — it is treated as the spec's "no content available" semantic.

**Tests**:
- `"placeholder renders when Tail returns (nil, nil) at initial open"` — mock `ScrollbackReader.Tail` returns `(nil, nil)` for the focused pane key; assert `viewport.View()` (or the model's rendered output) contains `(no saved content)`.
- `"placeholder renders when Tail returns (nil, nil) after Tab cycle"` — multi-pane session, first pane returns bytes, second pane returns `(nil, nil)`; press `Tab`; assert viewport now shows the placeholder.
- `"placeholder renders when Tail returns (nil, nil) after ] cycle"` — multi-window session, first window pane 0 returns bytes, second window pane 0 returns `(nil, nil)`; press `]`; assert placeholder.
- `"chrome counts remain correct when placeholder is shown"` — placeholder pane focused; assert rendered output contains `Window 1 of N` / `Pane 1 of M` and the window name from the captured enumeration.
- `"placeholder is the canonical wording '(no saved content)'"` — pin exact string; guards against drift.
- `"ENOENT, zero-byte, and zero-line outcomes are indistinguishable at the call site"` — three mock variants all returning `(nil, nil)`; all render identical placeholder output.

**Edge Cases**:
- ENOENT (`.bin` missing entirely — pane killed, daemon hasn't ticked, brand-new pane).
- Zero-byte `.bin` (file created by daemon but no captures yet).
- Zero-line file (only an unterminated partial line — the helper excludes the partial and returns `(nil, nil)`).
- Chrome integrity under placeholder — counts and window name unaffected.
- Focused pane at open vs after cycle — same code path, same outcome.

**Context**:
> Spec § Read-Failure Handling > Placeholder: "A single placeholder shape is used across all 'no content available' cases — read failures, deleted `.bin`, zero-byte `.bin`, and the brand-new-session edge case (covered separately below). Working label: '(no saved content)'." Triggering conditions: `.bin` does not exist (ENOENT) or `.bin` exists but is zero bytes. The Phase 1 helper unifies all three "no content" cases into the `(nil, nil)` shape, so the call site has exactly one branch to handle.
>
> Spec § Architecture Summary > Test seams > ScrollbackReader: "The helper unifies the three 'no content' cases by design — the placeholder/error decision lives at the call site in `internal/tui`, not in the helper."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § Read-Failure Handling > Placeholder; § Architecture Summary > Test seams; § Acceptance Criteria > Edge cases.

## session-scrollback-preview-4-2 | approved

### Task 4-2: Error-string rendering for (nil, err) Tail outcomes with retry-on-refocus

**Problem**: Phase 2 left the `(nil, err)` branch stubbed. The spec requires OS-level read failures (EACCES, EIO, etc.) to render a single short error string — uniform across errno types, no per-error branching — and explicitly forbids a per-pane error cache, so re-focusing the pane after a transient error must issue a fresh `Tail` call. Without finalising this, a defensive failure path (mode 0600 / same-user setup makes it rare in practice) is the kind of latent bug that surfaces only when something else has gone wrong.

**Solution**: Finalise the `(nil, err)` branch at the call site in `internal/tui/pagepreview.go`. When `Tail` returns `(nil, err)`, set viewport content to a single short error string (e.g. `(unable to read scrollback)` — pin as a package-level constant alongside the placeholder). Do not store the error against the focused pane's state; do not cache the outcome by pane key. The next focus event on the same pane re-issues `Tail` exactly as if the previous outcome had been forgotten.

**Outcome**: A pane whose `Tail` returns an error renders the canonical error string in the viewport. Cycling away (`Tab` / `]` / `[`) and cycling back issues a brand-new `Tail` call — verifiable by the mock recording exactly one call per focus event regardless of previous outcome. Chrome remains correct.

**Do**:
- In `internal/tui/pagepreview.go`, define `const previewReadError = "(unable to read scrollback)"` (single short error string; same wording for every errno).
- In the read-and-render path, branch on `(bytes == nil && err != nil)` and call `viewport.SetContent(previewReadError)`.
- Audit the focus-cycle handlers (`]`, `[`, `Tab`) and the initial-open path: confirm none of them store, cache, or short-circuit on prior `Tail` outcomes — every focus event must funnel through the same call into `ScrollbackReader.Tail(paneKey)`.
- Explicitly do NOT introduce any `errorByPaneKey` map, `lastTailErr` field, or similar memoisation on the `previewModel`.

**Acceptance Criteria**:
- [ ] When `Tail` returns `(nil, err)`, viewport content equals `(unable to read scrollback)`.
- [ ] The error string is uniform — EACCES, EIO, and any other errno produce byte-identical viewport output.
- [ ] After landing on an error, cycling away and back to the same pane issues a fresh `Tail` call (mock records two calls for that paneKey across the away/back round-trip).
- [ ] No field on `previewModel` stores per-pane error state.
- [ ] Chrome (`Window M of N`, `Pane X of Y`, window name) is unaffected by the error branch.

**Tests**:
- `"error string renders when Tail returns (nil, err) at initial open"` — mock returns `(nil, errors.New("permission denied"))`; assert viewport shows `(unable to read scrollback)`.
- `"error string is uniform across errno types"` — three mock variants returning EACCES, EIO, and a generic error; all produce byte-identical output.
- `"error string differs from placeholder string"` — assert `previewReadError != previewPlaceholder`; confirms the two no-content shapes are visually distinguishable.
- `"refocus after error issues a fresh Tail call (Tab away and back)"` — multi-pane session, pane 0 returns error; press `Tab` (pane 1 returns bytes); press `Tab` again to wrap back to pane 0; assert mock recorded two `Tail` calls for pane 0's key.
- `"refocus after error issues a fresh Tail call (] away and back)"` — multi-window session, w0p0 returns error; press `]` then `]` again to wrap back; assert two `Tail` calls for w0p0's pane key.
- `"second Tail call after error sees the new outcome"` — first call returns error, second call returns bytes; after refocus the viewport shows the bytes (not stale error).
- `"no per-pane error state is stored on previewModel"` — reflection or struct-field audit confirms no field whose name suggests caching errors; alternatively, this is enforced by the test above (which would fail if the model short-circuited refocus).
- `"chrome counts unaffected by error branch"` — error pane focused; assert chrome still reflects captured enumeration.

**Edge Cases**:
- Uniform error string across errno types — no EACCES vs EIO vs generic-error branching at the call site.
- No per-pane error cache — refocus always re-issues `Tail`.
- Retry on `Tab`-away-and-back (within-window cycle).
- Retry on `]` / `[` and back (cross-window cycle).
- Transient error followed by success on second read — verified by the mock returning different outcomes per call.

**Context**:
> Spec § Read-Failure Handling: "OS-level read error (permissions, disk full, etc.). The viewport renders a brief error string in place of content. Should never occur given mode 0600 / same-user guarantees from the save daemon, but handled defensively rather than crashing the TUI."
>
> Spec § Read-Failure Handling > Placeholder > Error string: "OS-level read errors render a single short error string in the viewport rather than the placeholder. The wording is build-phase TBD; the same string is used for every error type (no per-errno differentiation, no EACCES vs EIO branching). Future focus changes onto the same pane retry the read fresh — there is no per-pane error cache."
>
> Phase 4 layer note: "4-2 must explicitly verify that re-focusing a pane after error does NOT use a per-pane error cache — the `Tail` call is fresh on every focus change."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § Read-Failure Handling > Placeholder > Error string; § Acceptance Criteria > Edge cases ("An OS-level read error renders the error string; subsequent focus changes onto the same pane retry the read.").

## session-scrollback-preview-4-3 | approved

### Task 4-3: Fewer-than-N lines renders all available lines (non-placeholder partial read)

**Problem**: A common preview case is a recently-created pane whose `.bin` has, say, 5 or 50 lines — well below N=1000. The spec is explicit that this is a successful read (a partial read is a successful read) and must NOT trigger the placeholder; the viewport must render whatever lines exist, anchored at scroll-tail. Without an explicit test pinning this behaviour, a future refactor could plausibly conflate "fewer than N" with "no content" and silently regress.

**Solution**: Add a regression-pinning test layer in `internal/tui/pagepreview_test.go` that exercises the fewer-than-N path. The mock `ScrollbackReader.Tail` returns a non-nil byte slice containing fewer than 1000 lines (e.g. 1 line, 50 lines, exactly 999 lines). Assert the viewport renders exactly those bytes, and that the viewport opens at scroll-tail (not scroll-top), and that scrolling up to the top boundary then attempting another scroll-up is a silent no-op. No production code change is expected — Phase 1 already returns "all lines in file" when the file has fewer than N — this task pins that contract at the TUI layer.

**Outcome**: A pane whose `Tail` returns `(bytes, nil)` for a small number of lines renders those lines verbatim in the viewport, with the viewport positioned at scroll-tail. The placeholder branch is not taken. Scrolling up reveals all the loaded lines; scrolling further at the top is a silent no-op.

**Do**:
- In `internal/tui/pagepreview_test.go`, add tests covering fewer-than-N outcomes: 1 line, 50 lines, exactly 999 lines.
- Construct the `previewModel` with a mock `ScrollbackReader` returning the test fixture bytes.
- Assert `viewport.View()` (or the rendered output) contains the fixture bytes.
- Assert the placeholder string `(no saved content)` is NOT present in the rendered output.
- Verify scroll-tail position by inspecting `viewport.AtBottom()` (or equivalent) immediately after open.
- Send a sequence of `Up` key messages to scroll up to the top, then send another `Up` and assert no panic / no crash / viewport content unchanged.
- If Phase 1's helper or Phase 2's call-site code does not already correctly route fewer-than-N to the verbatim-bytes branch, audit the branching logic and fix at the call site (no helper signature change expected).

**Acceptance Criteria**:
- [ ] A `Tail` outcome of `(bytes, nil)` with bytes containing 1 line renders those bytes in the viewport.
- [ ] A `Tail` outcome with 50 lines renders all 50 lines.
- [ ] A `Tail` outcome with 999 lines renders all 999 lines.
- [ ] The placeholder string `(no saved content)` does NOT appear in the rendered output for any of the above.
- [ ] The viewport is at scroll-tail (bottom) on initial open / focus change.
- [ ] Scroll-up at the top boundary is a silent no-op (no panic, no error, viewport content unchanged).

**Tests**:
- `"1-line file renders the single line and not the placeholder"` — mock returns `[]byte("only line\n")`; assert rendered output contains "only line" and not `(no saved content)`.
- `"50-line file renders all 50 lines"` — mock returns 50 newline-terminated lines; assert all are present in the loaded buffer.
- `"exactly 999 lines renders all 999 lines"` — mock returns exactly 999 newline-terminated lines (one below N); assert all are present.
- `"viewport opens at scroll-tail not scroll-top for fewer-than-N content"` — assert `AtBottom()` immediately post-construction.
- `"scroll-up at top boundary is silent no-op"` — load fewer-than-N content; send enough `Up` messages to reach the top; send one more; assert no error and rendered output is unchanged from the previous frame.
- `"fewer-than-N never triggers the placeholder branch"` — explicit pinning test: one line, ten lines, hundred lines, all assert placeholder absent.

**Edge Cases**:
- 1-line file (smallest non-empty fewer-than-N case).
- Exactly 999 lines (one below N=1000 boundary).
- Viewport opens at scroll-tail not scroll-top — even when content is short.
- Scroll-up past the top is silently no-op (`bubbles/viewport` default).

**Context**:
> Spec § Read-Failure Handling > Placeholder > Non-triggering condition: "A `.bin` file with **fewer than N lines** (e.g. 5 lines from a brand-new pane that has had a few captures but not 1000 of them) does **not** trigger the placeholder. Preview simply renders whatever lines are present, with the viewport at scroll-tail. The tail-N read returns 'all lines in file' when the file has fewer than N — a partial read is a successful read."
>
> Spec § Acceptance Criteria > Edge cases: "A pane with fewer than 1000 lines renders all available lines (no placeholder)."
>
> Spec § History Depth > Scroll within bounds: "the viewport (`bubbles/viewport`) renders the tail by default; the user can scroll up within the loaded N lines using the viewport's native scroll keymap. The top boundary of the slice is a hard edge — pressing scroll-up at the top silently no-ops."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § Read-Failure Handling > Placeholder > Non-triggering condition; § History Depth; § Acceptance Criteria > Edge cases.

## session-scrollback-preview-4-4 | approved

### Task 4-4: Brand-new-session traversal with placeholders on every pane

**Problem**: A session created within the last save tick, or one with a freshly-split pane the daemon hasn't ticked yet, is a real and common case. The spec demands that preview traverse the structural shape (cycle keys still work) and render the placeholder on each pane independently while chrome continues to show correct structural counts. The Phase 4-1 placeholder task pins the per-pane render; this task pins the traversal-under-all-placeholders behaviour as a system-level test.

**Solution**: Add an end-to-end test in `internal/tui/pagepreview_test.go` that constructs a `previewModel` for a multi-window multi-pane structural enumeration where every pane's `ScrollbackReader.Tail` returns `(nil, nil)`. Drive the model with synthetic `tea.KeyMsg` events covering `]`, `[`, `Tab`, and assert: (a) every focus lands on the placeholder; (b) chrome counters update correctly across cycles; (c) cycle keys traverse all structural entries — no entry is skipped because of the all-placeholder content state. Add a mixed-content variant where one pane has bytes and others do not.

**Outcome**: A synthetic brand-new session is fully navigable in preview: every pane shows the placeholder, every cycle key advances focus correctly, and chrome reflects accurate structural counts on every frame. The mixed case shows that `bytes != nil` panes render content while `(nil, nil)` panes render the placeholder, all within the same preview session, all addressable by the same cycle keymap.

**Do**:
- In `internal/tui/pagepreview_test.go`, add a test fixture: a `TmuxEnumerator` mock returning, e.g., 2 windows × 2 panes (4 panes total), and a `ScrollbackReader` mock returning `(nil, nil)` for every pane key.
- Construct the `previewModel`, assert initial focus is window 0 / pane 0 and the viewport shows the placeholder, and chrome shows `Window 1 of 2` / `Pane 1 of 2` plus the captured window name.
- Drive `Tab` twice (wrapping back to pane 0 within window 0 — the spec's forward-only Tab with wrap), asserting placeholder + chrome at each step.
- Drive `]` to advance to window 1, assert focus resets to pane 0 of window 1, assert chrome shows `Window 2 of 2` / `Pane 1 of 2`, and viewport shows placeholder.
- Drive `]` again to wrap back to window 0, assert chrome and placeholder.
- Drive `[` from window 0 to wrap to window 1 (last), assert chrome and placeholder.
- Add a second test fixture: 2 windows × 2 panes where window 0 / pane 0 returns `(bytes, nil)` and the other three return `(nil, nil)`. Drive a full cycle, asserting the bytes pane renders content and the others render placeholder.

**Acceptance Criteria**:
- [ ] Initial open on a brand-new session shows the placeholder on the first pane and correct chrome counts.
- [ ] `Tab`, `]`, and `[` all advance focus correctly when every pane's content is `(nil, nil)`.
- [ ] After each cycle, chrome reflects the new focus position via 1-based ordinal counters.
- [ ] All structural entries are reachable via cycle keys regardless of content state.
- [ ] In the mixed variant, panes with bytes render bytes; panes with `(nil, nil)` render the placeholder; cycle keys work uniformly.
- [ ] No code path treats `(nil, nil)` as "skip this pane" or "abort the cycle".

**Tests**:
- `"brand-new session: every pane renders placeholder"` — 2x2 fixture, all `(nil, nil)`; cycle through all 4 focus positions; assert each frame shows placeholder.
- `"brand-new session: chrome counts accurate across all-placeholder cycles"` — 2x2 fixture; cycle through; assert chrome shows `Window M of N` / `Pane X of Y` correctly at each step.
- `"brand-new session: ] advances and Tab cycles within window under all placeholders"` — same fixture; assert `]` lands on window 1 / pane 0 and `Tab` cycles to window 1 / pane 1.
- `"brand-new session: cycle keys do not skip placeholder panes"` — assert that the count of distinct `(window_idx, pane_idx)` focuses observed during a full traversal equals the total pane count from the enumeration.
- `"mixed session: bytes pane and placeholder panes coexist"` — fixture with one bytes pane and three placeholder panes; cycle through; assert appropriate render at each focus.
- `"mixed session: focus from bytes pane to placeholder pane and back issues fresh Tail calls"` — verifies no caching short-circuit; mock records two calls for the bytes pane across the round-trip.

**Edge Cases**:
- Every pane returns `(nil, nil)` — full all-placeholder traversal.
- Mixed session — one pane has bytes, others do not; cycle keys must work uniformly.
- Chrome counts stay correct under all-placeholder state.
- Cycle keys traverse all structural entries regardless of content availability.

**Context**:
> Spec § Brand-new-session Edge Case: "Two related 'no `.bin` content yet' scenarios are handled by the same placeholder: Whole-session — A session created within the last save tick — the daemon hasn't captured any of its panes yet. Every pane reads as missing-content; preview shows the placeholder for each pane. Per-pane — A multi-pane session where one pane was just split-windowed in but the daemon hasn't ticked it yet. Other panes have `.bin` and render normally; the new one shows the placeholder."
>
> Spec § Brand-new-session Edge Case > Chrome integrity: "Window M of N, pane X of Y, and window name come from tmux structural enumeration, so the user always sees the correct structure even when content is missing. Cycle keys (`]` / `[` / `Tab`) work normally and traverse all structural entries regardless of which have content."
>
> Spec § Brand-new-session Edge Case > No live capture fallback: "Preview never falls back to `tmux capture-pane` for missing-`.bin` panes. That would contradict the always-disk decision and would only succeed for hydrated panes anyway."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § Brand-new-session Edge Case; § Acceptance Criteria > Edge cases ("A brand-new session whose panes have no `.bin` yet shows the placeholder for each pane; chrome remains correct.").

## session-scrollback-preview-4-5 | approved

### Task 4-5: Sessions-list re-fetch on pagePreview → pageSessions transition

**Problem**: When the user dismisses preview with `Esc`, the Sessions list must re-fetch live tmux state so a session killed externally during the preview window does not appear in the returned-to list. The spec explicitly says preview owns the re-fetch contract on this transition; the build phase must audit whether the existing Sessions page already refreshes on entry from another page, and only add a refresh dispatch if there is a gap. This is audit-first work — adding an unconditional refresh on every transition would be over-shooting; missing one would silently fail the externally-killed-session contract.

**Solution**: Audit `internal/tui/model.go::updateSessionsPage` and the page transition handlers for any existing on-entry refresh path (e.g. a `loadSessionsCmd` already dispatched on `pageProjects → pageSessions` or `pageFileBrowser → pageSessions`). Two outcomes:
1. **Existing refresh found and applies to `pagePreview → pageSessions`**: add a regression-pinning test that exercises the dismiss path with a mock session lister whose return changes between the open and the dismiss, confirming the post-dismiss list reflects the new state.
2. **Gap found**: add a refresh dispatch in the dismiss handler (the `Esc`-from-preview branch in `Update`) that returns a `tea.Cmd` re-fetching the sessions list before re-rendering `pageSessions`. Re-test against the same scenario.

The dismiss handler must continue to preserve the `bubbles/list` cursor and filter state per Phase 2 task 2-4 — the refresh updates the list items, but cursor positioning falls back to `bubbles/list`'s default behaviour when the previously-selected session is gone (lands on a neighbouring entry).

**Outcome**: After preview is dismissed, the Sessions list shows current live tmux sessions. A session killed externally during preview no longer appears. The cursor lands on a neighbouring session via `bubbles/list`'s default behaviour. When nothing has changed, the refresh is a no-op from the user's perspective.

**Do**:
- Audit `internal/tui/model.go` for existing refresh dispatches: search for `loadSessionsCmd`, `listSessionsCmd`, or equivalent commands wired into the page state machine; trace their entry conditions.
- Document the audit outcome in the test (a code comment is sufficient — this is a planning-traceability artefact, not external doc).
- If a gap exists, add the refresh dispatch in the dismiss handler — the same `Esc`-from-preview code path that returns the model to `pageSessions` returns a `tea.Cmd` (likely `tea.Batch(existingDismissCmd, refreshSessionsCmd)`).
- Add a test in `internal/tui` that:
  - Constructs the model on `pagePreview` with a session A in the list.
  - Mocks the session lister to return only sessions {A, B} on first call and only {B} on second call.
  - Sends `Esc`.
  - Drains the resulting commands (calling the returned `tea.Cmd` to produce the next message).
  - Sends the resulting message back through `Update`.
  - Asserts the rendered Sessions list no longer contains A.
- Add a second test confirming cursor preservation when the previously-selected session still exists post-refresh.
- Add a third test confirming graceful cursor fallback when the previously-selected session is gone (cursor lands on a neighbouring entry; assert no panic, assert a valid cursor index).

**Acceptance Criteria**:
- [ ] Audit recorded: documents whether an existing refresh path covers `pagePreview → pageSessions` or whether a new dispatch was needed.
- [ ] After `Esc` from preview, the Sessions list reflects current live tmux state via a fresh enumeration.
- [ ] A session killed externally between preview-open and preview-dismiss does not appear in the post-dismiss list.
- [ ] When the previously-selected session still exists, the cursor remains on it.
- [ ] When the previously-selected session is gone, the cursor lands on a valid neighbouring entry without panic.
- [ ] The refresh is a no-op observably when nothing has changed (list contents identical, cursor unchanged).
- [ ] Filter state (committed / mid-typing / no filter) is preserved across the dismiss-with-refresh transition per Phase 2 task 2-4.

**Tests**:
- `"audit: existing on-entry refresh path"` — comment-documented test that captures whichever finding the audit produced (existing path vs gap fixed).
- `"Esc from preview re-fetches sessions list"` — mock lister returns different sets on first vs second call; after `Esc`, list reflects second call.
- `"externally-killed session not in list after dismiss"` — set up A and B in initial list; preview opened on A; mock lister returns {B} on dismiss; assert A is gone from rendered list.
- `"cursor preserved when previous session still exists"` — A and B in list; preview on A; lister returns {A, B} again on dismiss; cursor is on A post-dismiss.
- `"cursor falls back to neighbour when previous session is gone"` — A, B, C in list; preview on B; lister returns {A, C} on dismiss; cursor is on a valid index, no panic.
- `"no-op refresh when list unchanged"` — same set both calls; rendered output unchanged from pre-preview state (modulo any refresh-cycle frame artefacts).
- `"filter state preserved across dismiss-with-refresh"` — committed filter present pre-preview; after `Esc`, filter still committed.

**Edge Cases**:
- Existing on-entry refresh path discovered (audit-only outcome — no new dispatch needed).
- Gap discovered (new dispatch required in `pagePreview → pageSessions` transition).
- Cursor lands on neighbouring session when previously-selected is gone.
- No-op refresh when nothing has changed.
- Filter state preservation interacts with refresh.

**Context**:
> Spec § Cross-cutting Seams > Externally-Killed Session During Preview: "Esc back to list. The Sessions list re-fetches the live session list on return — the killed session simply isn't there anymore. Cursor lands on a neighbouring session via `bubbles/list`'s default behaviour."
>
> Spec § Cross-cutting Seams > Externally-Killed Session During Preview > Re-fetch contract: "Preview owns the re-fetch on the `pagePreview → pageSessions` transition. The build phase confirms during implementation whether the existing Sessions page already re-fetches on entry from another page; if not, the transition handler dispatched by `Esc` from preview must trigger a Sessions-list refresh before rendering the Sessions page. Either path satisfies the contract — what matters is that the transition does not show a stale list containing a killed session."
>
> Phase 4 layer note: "4-5 may discover existing on-entry refresh on Sessions page (audit-first); only add a refresh dispatch if there is a gap."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § Cross-cutting Seams > Externally-Killed Session During Preview; § Acceptance Criteria > Edge cases.

## session-scrollback-preview-4-6 | approved

### Task 4-6: Externally-killed-session in-preview stability with progressive placeholders

**Problem**: When session A is killed externally (another tmux client, `tmux kill-session`, `portal clean`) while the user holds preview open on A, two things must remain true: (1) chrome stays stable — counts and window names captured at preview-open continue to drive the chrome line, no live re-enumeration is performed, no error frame is shown; (2) `.bin` files persist briefly then get cleaned by the daemon, so cycle keys land on increasingly more placeholders as the user navigates. Without a regression-pinning test, a future change to "be helpful" by re-enumerating mid-preview, or to error-out when `.bin` reads start failing, would silently break the spec.

**Solution**: Add a test in `internal/tui/pagepreview_test.go` simulating the killed-session-mid-preview scenario. The `TmuxEnumerator` mock is called exactly once at preview-open (already enforced by Phase 3 task 3-7). The `ScrollbackReader.Tail` mock is configured to return progressively more `(nil, nil)` outcomes as the user cycles — initially all panes return bytes; after several cycle events, some return `(nil, nil)` simulating the daemon's cleanup. Assert chrome remains stable across the entire cycle (`Window M of N` and window name from the captured enumeration unchanged), placeholders appear in lockstep with the mock's progression, and no panic / no error frame is raised.

**Outcome**: An externally-killed session held in preview continues to render usefully: chrome reflects the at-open structural snapshot; placeholders progressively appear as `.bin` files are cleaned; the user can dismiss with `Esc` and return to a refreshed Sessions list (per Task 4-5) where the killed session is gone.

**Do**:
- In `internal/tui/pagepreview_test.go`, set up a 2-window 2-pane fixture (4 panes total).
- The `TmuxEnumerator` mock returns the captured shape on its first call. Track the call count and assert it stays at 1 across the test.
- The `ScrollbackReader.Tail` mock is stateful: first round of focus events returns `(bytes, nil)` for all panes; after, e.g., 2 focus events, it starts returning `(nil, nil)` for unfocused panes; eventually returns `(nil, nil)` for all panes.
- Drive a full cycle through all 4 panes, then cycle again.
- Assert chrome on every frame: `Window M of N`, `Pane X of Y`, window name — all match the captured enumeration regardless of content state.
- Assert the rendered viewport content shifts from bytes to placeholder in lockstep with the mock progression.
- Assert no `tea.Cmd` is dispatched that would re-call `TmuxEnumerator`.
- Assert no panic, no error frame, no crash.

**Acceptance Criteria**:
- [ ] `TmuxEnumerator.ListWindowsAndPanesInSession` is called exactly once across the entire test (regression-pin against mid-preview re-enumeration).
- [ ] Chrome `Window M of N`, `Pane X of Y`, and window name remain stable across all cycle events, regardless of content availability.
- [ ] As the mock progressively returns `(nil, nil)`, the viewport renders placeholder for those panes.
- [ ] No panic / no error frame is raised when content reads start failing.
- [ ] Cycle keys (`]`, `[`, `Tab`) continue to traverse the captured shape — no entry is skipped because content has become unavailable.
- [ ] `Esc` still works and returns to the Sessions list (with refresh per Task 4-5).

**Tests**:
- `"chrome stable when .bin files disappear mid-preview"` — stateful mock, full cycle, assert chrome unchanged across all frames.
- `"placeholders appear progressively as content vanishes"` — assert viewport content shifts from bytes to placeholder in lockstep with mock progression.
- `"no live re-enumeration mid-preview when session is killed"` — assert `TmuxEnumerator` call count == 1 across the entire test.
- `"cycle keys continue to traverse after content vanishes"` — assert all 4 pane positions are reachable even when all return `(nil, nil)`.
- `"no panic when all panes return (nil, nil) mid-preview"` — assert no error / no crash after full cycle through all-placeholder state.
- `"Esc dismisses cleanly from a fully-degraded preview"` — after all panes return `(nil, nil)`, send `Esc`; assert transition to `pageSessions` succeeds.

**Edge Cases**:
- `.bin` deleted between two consecutive focus events (the spec's specific scenario).
- All panes turn into placeholders (full progression).
- Chrome counters do not change mid-preview (regression-pin against re-enumeration).
- No errors crash the page.
- Cycle keys still work over the captured shape.

**Context**:
> Spec § Cross-cutting Seams > Externally-Killed Session During Preview: "Content reads. The `.bin` files persist briefly, then get cleaned by the daemon. Read-failure / deleted-`.bin` is already covered by the placeholder behaviour. `]` / `[` / `Tab` will increasingly land on placeholders as files are cleaned. Chrome. Window/pane structural counts and names were captured at preview-open. Cycle keys cycle the captured shape; no live re-enumeration is performed mid-preview, so chrome stays stable."
>
> Spec § Acceptance Criteria > Edge cases: "An externally-killed session whose preview is open continues to render with placeholders as `.bin` files are cleaned; `Esc` returns to the Sessions list, which re-fetches and the killed session is gone."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § Cross-cutting Seams > Externally-Killed Session During Preview; § Acceptance Criteria > Edge cases.

## session-scrollback-preview-4-7 | approved

### Task 4-7: Confirm _portal-saver exclusion at Sessions-list source

**Problem**: The `_portal-saver` detached session that hosts `portal state daemon` must never appear in the Sessions list — and therefore never be a preview target. The spec is explicit that existing list-population logic and the inside-tmux invariant should already exclude it; preview must NOT introduce a name-based blacklist as a defensive layer. This task is audit-first: confirm the existing exclusion holds at the list-population source. If a leak is found, fix at the source. If the existing path is sound, add a regression-pin test so future refactors of list population can't silently leak `_portal-saver` into the user-facing list.

**Solution**: Audit the Sessions-list population path in `internal/tui/model.go` (the `loadSessionsCmd` or equivalent), tracing back to the `tmux.Client.ListSessions` call site, identifying every filter step. Confirm `_portal-saver` is excluded by name at the canonical filter point. Two outcomes:
1. **Already excluded at source**: add a regression-pinning test that constructs a mock list including `_portal-saver` and asserts the rendered Sessions list omits it. Test lives at the same layer as the exclusion (TUI list-population, not preview).
2. **Leak found**: fix the exclusion at the source (in the list-population code path, NOT in the preview layer). Add the same regression-pin test against the now-fixed code. Document the fix location.

Preview must NOT add any name-based check on `_portal-saver` — the spec explicitly forbids a preview-layer suppression layer.

**Outcome**: The Sessions list never contains `_portal-saver`. A regression-pin test guards this contract at the list-population layer. Preview itself contains no `_portal-saver` string reference and no name-based blacklist.

**Do**:
- Search `internal/tui` for `_portal-saver` references — locate every filter site.
- Search `internal/tmux` and `internal/session` similarly for the source of the filtered list (likely `ListSessions` is unfiltered; the TUI-side population applies the exclusion).
- Confirm the exclusion is applied at exactly the layer where the list is handed to the `bubbles/list` model.
- If the exclusion is missing or applied at the wrong layer, fix at the source — do not introduce a preview-layer check.
- Add a regression-pin test in the relevant test file (e.g. `internal/tui/model_test.go` or wherever list-population is tested):
  - Mock the session source to return a list including `_portal-saver` plus several real sessions.
  - Construct/load the model.
  - Assert the rendered Sessions list does not contain `_portal-saver`.
- Search the preview code (`internal/tui/pagepreview.go`) for any reference to `_portal-saver` and assert there is none.

**Acceptance Criteria**:
- [ ] Audit recorded — documents whether `_portal-saver` was already excluded or was leaking.
- [ ] If a leak was found, the fix is in the list-population code path, NOT in the preview layer.
- [ ] A regression-pin test asserts `_portal-saver` is absent from the rendered Sessions list when the upstream source contains it.
- [ ] No string reference to `_portal-saver` exists in `internal/tui/pagepreview.go` (or wherever the preview model lives).
- [ ] The Phase 1 `ListWindowsAndPanesInSession` call is unchanged — preview does not need to filter the enumeration result for `_portal-saver` because preview can never be opened against `_portal-saver` (it isn't in the list).

**Tests**:
- `"_portal-saver not in rendered Sessions list when upstream source contains it"` — mock returns `[real-A, _portal-saver, real-B]`; assert rendered list contains `real-A` and `real-B` only.
- `"audit: exclusion applied at list-population source not preview layer"` — comment-documented test recording the audit finding (existing path vs gap fixed).
- `"preview model file contains no _portal-saver references"` — grep-style or file-content audit: read `pagepreview.go` and assert the substring `_portal-saver` is absent.
- `"regression-pin: refactor of list population must keep _portal-saver excluded"` — same test as the first, framed explicitly as a regression guard.

**Edge Cases**:
- `_portal-saver` already excluded by existing logic (audit-only outcome).
- Leak found and fixed at source.
- No preview-layer name blacklist introduced.
- Multiple sessions with similar prefixes (e.g. `_portal-saver-test`) — confirm exact-match exclusion, not prefix-match (re-read the existing code to see whether name comparison is exact or pattern-based; pin behaviour to whatever the existing implementation is).

**Context**:
> Spec § Cross-cutting Seams > `_portal-saver` Self-Reference: "The `_portal-saver` detached session that hosts `portal state daemon` must not appear in the Sessions list at all. Existing list-population logic and the inside-tmux invariant should already exclude it. The build phase must confirm during implementation that `_portal-saver` is excluded from the list passed to the Sessions page; if any code path ever leaks it into the list, no preview-layer suppression is required as long as the list filter is fixed at its source. Preview itself does not introduce a name-based blacklist."
>
> Spec § Out of Scope (v1): "Preview-layer `_portal-saver` suppression (excluded at list-population layer instead)."
>
> Phase 4 layer note: "4-7 is audit-first: spec says `_portal-saver` is already excluded by existing logic; verify and add a regression-pin test."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § Cross-cutting Seams > `_portal-saver` Self-Reference; § Out of Scope (v1).

## session-scrollback-preview-4-8 | approved

### Task 4-8: Side-effect-free hermetic invariant test

**Problem**: The spec's central design promise is that preview is read-only — opening and dismissing leaves byte-identical state, no hydration, no resume-hook firing, no tmux marker mutation, no FIFO consumed. Without an explicit hermetic test asserting these invariants, the promise is implicit and easy to break with a future refactor. The acceptance criterion enumerates exactly what to verify; this task operationalises it as a single test.

**Solution**: Add a hermetic test in `internal/tui/pagepreview_test.go` that exercises the full preview lifecycle against mocked seams — no real tmux server, no live daemon, no real filesystem. The test:
- Records every call into `TmuxEnumerator` (call count, arguments).
- Records every call into `ScrollbackReader.Tail` (call count, paneKey).
- Snapshots the `hooks.Store` (or its mock) before and after the preview interaction.
- Snapshots any `state` package writer (`SetSkeletonMarker`, `WriteScrollbackIfChanged`, FIFO creation/drain) before and after — asserted via mocks-with-recording or via direct assertion that no such writer is reachable from preview's call graph.
- Drives a full cycle through every pane in a multi-window multi-pane session, then dismisses with `Esc`.
- Asserts: exactly one `TmuxEnumerator` call; exactly one `ScrollbackReader.Tail` call per focus event with no other I/O on `.bin` paths; zero `hooks.Store` calls; zero `state`-package writes; zero FIFO creation or drain.

**Outcome**: A single test pins the side-effect-free contract end-to-end. Any future change that introduces a hidden write — e.g. a "warm cache" that calls `SetSkeletonMarker`, or a "helpful" hook firing on preview-dismiss — breaks this test.

**Do**:
- In `internal/tui/pagepreview_test.go`, build a recording `TmuxEnumerator` mock: tracks `ListWindowsAndPanesInSession(session)` call count and arguments.
- Build a recording `ScrollbackReader` mock: tracks `Tail(paneKey)` call count and arguments.
- For `hooks.Store`, the preview model should not import or accept a `hooks.Store` at all — assert this structurally (the `previewModel` constructor signature does not include `hooks.Store` or any hook-firing dependency). If somehow it does, the test fails the spec.
- For `state`-package writers: preview's only `state` dependency is the read-side helper (closed over inside the `ScrollbackReader` adapter). Assert the preview model's package imports do not include any `state` writer — this can be a static assertion via `go/parser` or a grep-style test that reads `pagepreview.go` and asserts the strings `SetSkeletonMarker`, `WriteScrollbackIfChanged`, `os.Create`, `os.Mkdir`, `mkfifo` are absent.
- Drive the model: construct, send synthetic key messages (`Tab`, `Tab`, `]`, `Tab`, `Tab`, `[`, `Esc` — covering all cycle keys and dismiss).
- After each cycle event, record what the mocks observed.
- Assert at end of test:
  - `TmuxEnumerator` call count == 1.
  - `ScrollbackReader.Tail` call count == number of focus-changing events (initial open + each cycle press that landed on a new focus).
  - No `Tail` calls other than for focus events (i.e. no spurious eager reads).
  - The static-import audit confirms zero references to write-side state functions, zero hooks references, zero FIFO references.

**Acceptance Criteria**:
- [ ] Test builds against fully-mocked seams; no real tmux, no live daemon, no real filesystem.
- [ ] After a full open + cycle + dismiss flow, `TmuxEnumerator.ListWindowsAndPanesInSession` was called exactly once.
- [ ] `ScrollbackReader.Tail` was called exactly once per focus event; no other reads.
- [ ] Static audit (or constructor signature check) confirms preview has zero `hooks.Store` dependency.
- [ ] Static audit confirms preview has zero `state`-package writer references.
- [ ] Static audit confirms preview has zero FIFO creation / drain references.
- [ ] Test runs in `internal/tui/pagepreview_test.go` and does NOT import `tmuxtest` or `restoretest` (no real-tmux fixture).

**Tests**:
- `"hermetic preview cycle: exactly one TmuxEnumerator call across full lifecycle"` — drive full cycle + dismiss; assert call count.
- `"hermetic preview cycle: one ScrollbackReader.Tail call per focus event"` — drive full cycle; assert per-focus call count and no spurious calls.
- `"hermetic preview cycle: zero hooks.Store calls"` — preview model constructor signature does not accept a `hooks.Store`; alternatively a recording mock asserts zero method invocations.
- `"hermetic preview cycle: zero state-package writes"` — static audit / import check confirms no write-side state references in preview source.
- `"hermetic preview cycle: zero FIFO creation or drain"` — static audit confirms no `mkfifo` / `os.MkdirAll` / FIFO-path references in preview source.
- `"hermetic preview cycle: no tmuxtest dependency"` — file-import audit on `pagepreview_test.go` confirms `tmuxtest` is not imported.

**Edge Cases**:
- Exactly one `TmuxEnumerator` call across full cycle (not zero, not two).
- Exactly one `ScrollbackReader.Tail` call per focus event and zero other I/O.
- Zero `hooks.Store` calls.
- Zero `state`-package writes (no `SetSkeletonMarker`, no `WriteScrollbackIfChanged`).
- Zero FIFO creation or drain.

**Context**:
> Spec § Overview > Side-effect-free contract: "Opening and dismissing the preview leaves session state byte-identical: no hydration, no resume-hook firing, no tmux marker mutation, no FIFO consumed. Preview is read-only with respect to portal state and tmux."
>
> Spec § Acceptance Criteria > Side-effect-free contract: "Within a single test that opens preview on session S, cycles through every pane in S, and dismisses (no live daemon, no real tmux server — the seams are mocked): The preview code path issues exactly one `TmuxEnumerator` call (the structural enumeration at open) and zero further tmux invocations. Verifiable via the `TmuxEnumerator` mock recording calls. The preview code path issues only `ScrollbackReader.Tail(paneKey)` calls — one per focus event — and never any other I/O on `.bin` paths. Verifiable via the `ScrollbackReader` mock recording calls (no write methods exist on the interface). The preview code path makes zero calls into the `hooks.Store`, no writes via `state` package writers (`SetSkeletonMarker`, `WriteScrollbackIfChanged`, etc.), and no FIFO creation or drain. Verifiable by snapshotting the relevant state before/after the preview interaction. The intent is a hermetic, no-write code path — the assertions above are the operationalisation of 'side-effect-free'."
>
> Spec § Architecture Summary > Wiring shape: "Tests construct `previewModel` directly with mock implementations of `TmuxEnumerator` and `ScrollbackReader`; no package-level state to restore, no `t.Cleanup()` plumbing required, and `t.Parallel()` is safe in `pagepreview_test.go`. ... A new `pagepreview_test.go` (or equivalent) houses the new tests; it must not require a real tmux server (no `tmuxtest` import)."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § Overview > Side-effect-free contract; § Acceptance Criteria > Side-effect-free contract; § Architecture Summary > Wiring shape.

## session-scrollback-preview-4-9 | approved

### Task 4-9: No-new-surface audit and regression guard

**Problem**: The spec's bottom-line constraint is that preview is **purely additive on top of existing surfaces** — no new methods on `tmux.Client` beyond the single read-only listing method added in Phase 1, no daemon work, no restore changes, no bootstrap changes, no hooks changes, no save-format changes. With nine tasks across four phases, scope creep is plausible: a "tiny" tmux helper here, a "harmless" daemon flag there. This task is the non-modification audit — verify nothing has incidentally crept in.

**Solution**: Audit the diff produced by Phases 1–4 and assert no out-of-scope surface was introduced. Concrete checks:
- Diff `internal/tmux/client.go` (and any other files in `internal/tmux/`): the only new method is the Phase 1 window-grouped pane enumeration. No new capture wrappers (no `CapturePaneTail`, no `CapturePaneN`, etc.). The existing `CapturePane` signature is unchanged.
- Diff `internal/state/`: only the Phase 1 tail-N read helper and `ScrollbackFile` reuse. No new daemon writers, no new save-format fields, no new marker helpers.
- Diff `internal/restore/`: no changes.
- Diff `cmd/bootstrap/`: no changes (no preview-aware steps, no new orchestrator hooks).
- Diff `internal/hooks/`: no changes.
- Diff save-format constants (the `.bin` file shape, daemon writer paths): no changes.

This is an audit-only task — no production code is modified. The deliverable is either a clean audit (passing) or a list of unauthorised additions that must be reverted before phase close.

**Outcome**: A documented audit confirms preview's footprint is exactly: (a) one new `tmux.Client` listing method, (b) one new `internal/state` tail-N helper, (c) the new `pagePreview` arm and `previewModel` in `internal/tui`, (d) constructor wiring in TUI startup. No other production package has any modification beyond what these four imply.

**Do**:
- In `internal/tui/pagepreview_audit_test.go` (or equivalent — a test file is the durable form of an audit), add tests that operate against fixed file contents:
  - Assert `internal/tmux/client.go` does not contain new symbols matching `CapturePaneTail`, `CapturePaneN`, or other capture-wrapper-shaped names — substring or regex on the file contents.
  - Assert `internal/state/` exposes exactly the writers it had before (a fixed list of expected functions; any new writer fails the test).
  - Assert `internal/restore/` source is unchanged (file-hash or substring-pin against any preview-related additions).
  - Assert `cmd/bootstrap/` source contains no `preview` or `pagePreview` references.
  - Assert `internal/hooks/` source contains no `preview` references.
- Optionally cross-check by looking at the Phase 1 task surface — the only `tmux.Client` method that should be new is `ListWindowsAndPanesInSession` (or whatever Phase 1-5 named it).
- If the audit fails on any axis, document the unauthorised addition in the test failure message and stop — do not silently absorb scope creep into the audit.

**Acceptance Criteria**:
- [ ] Audit test exists and passes.
- [ ] `internal/tmux/client.go` has exactly one new method beyond pre-feature baseline (the Phase 1 enumeration method); the existing `CapturePane` signature is unchanged.
- [ ] `internal/state/` adds only the tail-N helper; existing daemon writers are byte-identical to pre-feature baseline.
- [ ] `internal/restore/` is unchanged.
- [ ] `cmd/bootstrap/` is unchanged.
- [ ] `internal/hooks/` is unchanged.
- [ ] Save-format constants and `.bin` file shape are unchanged.
- [ ] No new package was added in service of preview (the feature lives in `internal/tui` plus the Phase 1 helpers in `internal/state` and `internal/tmux`).

**Tests**:
- `"audit: tmux.Client has no new capture wrapper"` — assert source of `internal/tmux/client.go` does not contain `CapturePaneTail` / `CapturePaneN` / similar.
- `"audit: tmux.Client capture method signature unchanged"` — assert the existing `CapturePane` signature line is present in source verbatim.
- `"audit: internal/state exposes only the expected writers"` — enumerate expected writer names; assert the package's exported writer surface is a subset.
- `"audit: internal/restore source has no preview references"` — substring search for `preview` / `pagePreview` returns no matches in `internal/restore/`.
- `"audit: cmd/bootstrap source has no preview references"` — same substring search in `cmd/bootstrap/`.
- `"audit: internal/hooks source has no preview references"` — same substring search in `internal/hooks/`.
- `"audit: no new package added for preview"` — assert no new directory under `internal/` named `preview/` (or similar) was introduced; preview lives inside `internal/tui`.
- `"audit: save-format constants unchanged"` — pin the relevant constants (e.g. `.bin` extension, daemon write path scheme) to known values; any change fails.

**Edge Cases**:
- Audit-only task — no production-code modification expected as part of this task.
- A failure on any axis means scope creep crept in via an earlier phase and must be reverted before Phase 4 closes.
- The audit is durable: it stays in the test suite as a permanent regression guard, not a one-shot review checklist.

**Context**:
> Spec § Cross-cutting Seams > State Package API Reuse: "No new methods on `tmux.Client`. No new daemon work. No marker mutations. No hook firing. The feature is purely additive on top of existing surfaces."
>
> Spec § Architecture Summary > No changes to: "`tmux.Client` capture path (no new capture wrappers; one new read-only listing method is permissible per § *Multi-pane Rendering Shape > Concrete enumeration call*). `state` daemon (no new save/replay logic). `restore` engine. `cmd/bootstrap` orchestrator. `hooks` store or hydrate helper. Save format or `.bin` file shape."
>
> Spec § Multi-pane Rendering Shape > Concrete enumeration call: "The earlier claim 'No new methods on `tmux.Client`' is qualified by this concrete enumeration shape: a single new read-only listing method is permissible. ... a new listing method is a different category and does not contradict it."
>
> Phase 4 layer note: "4-9 is a non-modification audit — verify no incidental scope creep."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § Cross-cutting Seams > State Package API Reuse; § Architecture Summary > No changes to; § Multi-pane Rendering Shape > Concrete enumeration call; § Acceptance Criteria ("No new methods are added to `tmux.Client` beyond the single read-only listing method from Phase 1; no daemon, restore, bootstrap, hooks, or save-format changes are made.").
