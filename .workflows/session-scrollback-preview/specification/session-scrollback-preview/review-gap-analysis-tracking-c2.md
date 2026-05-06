---
status: in-progress
created: 2026-05-06
cycle: 2
phase: Gap Analysis
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Gap Analysis

## Findings

### 1. Internal contradiction: window/pane indexing convention (0-based vs 1-based)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Multi-pane Rendering Shape > Position on session re-entry; § Refresh Semantics > Initial-open ordering; § Acceptance Criteria > Entry & dismiss

**Details**:
The spec uses two different conventions for "first window / first pane" without distinguishing them:

- § Position on session re-entry (line 73): "re-opens at **window 1 / pane 1**"
- § Initial-open ordering (line 152): "the focus indices are set to **window 0 / pane 0** (first window, first pane in enumeration order)"
- § Acceptance Criteria (line 397): "Preview opens at the first window's first pane (**window 0 / pane 0** in enumeration order)"
- § Acceptance Criteria (line 412): "Re-opening preview on the same session re-reads **window 0 / pane 0** fresh"
- § Chrome Floor (line 86): "**Window M of N** — Pane X of Y" (display, presumably 1-based)

Three distinct meanings are entangled: (a) tmux's underlying `window_index` (which can be any integer per `base-index`), (b) the preview model's internal enumeration-order indices (0-based), (c) the human-facing chrome counter ("Window 1 of 3"). The "window 1 / pane 1" sentence on line 73 reads as display-style language while everywhere else uses 0-based enumeration order. A planner reading the spec end-to-end will be unsure whether `M` in "Window M of N" is the tmux `window_index`, the 0-based enumeration position + 1, or the user's casual count.

This becomes load-bearing in two places:
- The chrome counter under base-index drift (e.g. `set -g base-index 1`, `renumber-windows on`, or after windows are killed leaving a gap like `0, 2, 5`). Does chrome show "Window 0 of 3" or "Window 1 of 3"?
- Acceptance test phrasing — "opens at window 0 / pane 0" is testable against internal indices but not against rendered chrome.

**Proposed Addition**:
Pin a single convention with a one-paragraph note in § Multi-pane Rendering Shape (or § Chrome Floor): preview maintains 0-based enumeration-order indices internally (windows ordered as returned by structural enumeration, then sorted by `window_index` ascending; panes within a window similarly by `pane_index`). Chrome renders human-facing 1-based counters: "Window {focusWindow + 1} of {len(windows)}", "Pane {focusPane + 1} of {len(panes in current window)}". The tmux `window_index` is **not** displayed in chrome (the window name carries identity; the counter carries position). Update line 73 to read "window 0 / pane 0 (the first entry in enumeration order)" for consistency with lines 152, 397, and 412.

**Resolution**: Pending
**Notes**:

---

### 2. Pane-focus index after window cycle is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Multi-pane Rendering Shape > Within-preview Key Bindings; § Refresh Semantics > Read Trigger Events; § Acceptance Criteria > Within-preview navigation

**Details**:
When the user presses `]` (next window) or `[` (previous window), the spec pins that the newly-focused pane is re-read — but does not pin **which** pane within the new window becomes focused. Two reasonable behaviours:

- (a) Reset to pane index 0 of the new window. Predictable; "first pane in window" is consistent across cycles.
- (b) Preserve current pane index and clamp to new window's pane count. E.g. on pane 2, cycling to a window with only 1 pane lands on pane 0; cycling to a window with 3 panes preserves pane 2.

This matters for correctness even in dominant 1-pane case (always pane 0 either way), but when windows have differing pane counts the spec is silent. The acceptance criterion on line 405 only asserts "advances to the next window" — not where within it.

There is also a related under-specified case: after `]` lands on a new window, does `Tab` start cycling from the focused pane index (whatever rule (a) or (b) gave us) or from pane 0? Implicitly "from the focused index" by the rule that Tab is "next pane within current window", but worth pinning.

**Proposed Addition**:
Add a subsection or bullet under § Multi-pane Rendering Shape > Within-preview Key Bindings:
"**Pane focus on window cycle.** After `]` or `[`, the focused pane within the new window resets to pane 0 (first pane in enumeration order). Per-window pane focus is not preserved across window cycles. `Tab` then cycles forward from pane 0. Rationale: with the dominant 1-pane case this rule is trivial; in the multi-pane case, resetting matches the position-on-session-re-entry rule (always start at the beginning) and avoids an invisible per-window state the user has no signal of."

Add to acceptance criteria: "After `]` or `[` lands on a new window, the focused pane is pane 0 of that window (first in enumeration order)."

**Resolution**: Pending
**Notes**:

---

### 3. `stateDir` source for `state.ScrollbackFile(stateDir, paneKey)` is not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Cross-cutting Seams > State Package API Reuse; § Architecture Summary > Read pipeline; § Architecture Summary > Test seams

**Details**:
The resolution chain (line 331) is `paneKey = state.SanitizePaneKey(...) → path = state.ScrollbackFile(stateDir, paneKey) → tail-N read on path`. But `stateDir` arrives from somewhere — it is not a constant, and `internal/tui` does not currently own state-path resolution.

The spec never pins:
- Which existing accessor produces `stateDir` for preview's purposes (e.g. an existing `state.Dir()`, `state.Paths().Dir`, or a value already threaded through `bootstrap.Orchestrator` that the TUI receives).
- Whether `stateDir` is captured at TUI construction time (and therefore stable across all preview opens within one Portal invocation) or resolved per-open.
- Whether the `ScrollbackReader` test seam (line 371) hides `stateDir` entirely behind the `paneKey`-keyed interface (so production wires `stateDir` once at construction, tests don't need it) or surfaces it.

A planner cannot wire the production adapter without this. It is also load-bearing for the `previewDeps` shape: if `ScrollbackReader.Tail(paneKey)` hides `stateDir`, the production adapter is closed over `stateDir` at construction; if it takes `(stateDir, paneKey)`, every caller must pass it.

**Proposed Addition**:
Add a paragraph to § Cross-cutting Seams > State Package API Reuse:
"**`stateDir` resolution.** Preview consumes `stateDir` via the `ScrollbackReader` seam, not directly. The production adapter for `ScrollbackReader.Tail(paneKey)` is constructed once at TUI startup with `stateDir` resolved from the existing `internal/state` paths helper (the same source the daemon and bootstrap orchestrator already use). The interface intentionally hides `stateDir` so tests can mock by `paneKey` alone and so preview never has its own state-path resolution policy. `stateDir` is captured once and stable for the Portal process lifetime; it is not re-resolved per preview-open."

Confirm in § Architecture Summary > Test seams that `ScrollbackReader.Tail(paneKey)` is the seam shape (no `stateDir` parameter).

**Resolution**: Pending
**Notes**:

---

### 4. `state.SanitizePaneKey` argument types not pinned (string vs int for indices)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Cross-cutting Seams > State Package API Reuse > Resolution chain

**Details**:
The resolution chain (line 331) calls `state.SanitizePaneKey(session, window_index, pane_index)`. Tmux returns indices as text from `list-panes -F` (`#{window_index}`, `#{pane_index}`). The spec does not state whether the helper takes raw strings (matching `tmux list-panes -F` output) or parsed integers, nor whether the structural enumeration result type uses strings or ints for these fields.

This is a small but real planning gap: the WindowGroup type referenced in the proposed `ListWindowsAndPanesInSession` method (line 94) is undefined in the spec. A planner cannot draft the type without knowing whether `Index` is `string` or `int` — which determines whether `SanitizePaneKey` callers parse, format, or pass through verbatim.

It is also relevant under tmux base-index drift: if indices are strings, `"0"` and `"1"` and `"5"` are all valid; if ints, parsing must handle gaps without renumbering them. The pane-key produced must match what the daemon writes — if the daemon uses string indices, preview must too, otherwise no `.bin` file will be found for any pane.

**Proposed Addition**:
Add a one-line clarification to § Cross-cutting Seams > State Package API Reuse > Resolution chain:
"The arguments to `state.SanitizePaneKey` use the same value types and string-form as the daemon's call site (per `internal/state` paneKey helpers). Build phase confirms the existing helper signature and uses it verbatim — the goal is byte-identical paneKeys to whatever the daemon wrote. The structural enumeration result type passes `window_index` and `pane_index` through in the form `SanitizePaneKey` expects (build-phase confirms whether that is `string` or `int`)."

Add to § Architecture Summary > Test seams that the `TmuxEnumerator` interface result type is shaped to feed `SanitizePaneKey` directly (no type adaptation required at the call site).

**Resolution**: Pending
**Notes**:

---

### 5. Internal contradiction: OS-level read error is listed as both a placeholder trigger and an error-string trigger

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Read-Failure Handling > Placeholder

**Details**:
Lines 185–189 list the placeholder triggering conditions. The third bullet reads:
> - OS-level read error (permissions, EIO, etc.) — see *Error string* below.

But § Read-Failure Handling line 193 explicitly says:
> OS-level read errors render a single short error string in the viewport **rather than** the placeholder.

So the bulleted "Triggering conditions for the placeholder" list contains an item that is **not** a placeholder trigger — it is an error-string trigger. The bullet and the prose contradict each other within the same subsection.

A planner enumerating placeholder cases will see this and either (a) follow the bullet list and conflate OS-error → placeholder, dropping the error-string distinction, or (b) follow the prose and ask why the bullet is in the wrong list.

The acceptance criteria (lines 419–422) get this right: missing → placeholder, zero-byte → placeholder, OS-error → error string. The defect is local to § Placeholder.

**Proposed Addition**:
Restructure § Read-Failure Handling > Placeholder bullets:

> **Triggering conditions for the placeholder.**
> - `.bin` file does not exist (ENOENT).
> - `.bin` file exists but is zero bytes (no captures yet).
>
> **OS-level read errors** (permissions, EIO, etc.) render the error string instead, not the placeholder — see *Error string* below.

Removes the OS-error bullet from the placeholder triggers list and promotes it to a sibling paragraph for clarity.

**Resolution**: Pending
**Notes**:

---

### 6. Reverse-chunked tail-N scan: file-handle lifetime and torn-read invariant not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § History Depth > Read Pipeline; § Read-Failure Handling (atomicity bullet)

**Details**:
The atomicity claim (line 177) rests on Unix rename(2) atomicity: a single read sees either the old content or the new content, never torn bytes. But the tail-N helper performs **multiple `read` syscalls** in a reverse chunked scan (line 122). Atomicity-of-rename only guarantees that whatever inode the `os.File` descriptor was opened against stays consistent — the reader continues to see the pre-rename inode's bytes for the lifetime of that file descriptor.

This is correct in practice (the standard Unix invariant), but the spec does not state the invariant load-bearing for the helper:

> The helper must `os.Open` the file once at the start of the scan, perform all `Seek` / `Read` calls against that single file descriptor, and `Close` only after the tail bytes are assembled. A close-and-reopen between chunks would expose a window where the daemon's atomic rename swaps the inode and the next chunk reads from a different file (or returns ENOENT).

A planner implementing the helper from a generic "reverse chunked scan" description could plausibly close-and-reopen per chunk (uncommon but not impossible), defeating the atomicity claim. Pinning the single-fd invariant closes that.

This also relates to the placeholder triggering for `.bin deleted between two consecutive focus events` (line 178): "between events" is the right framing — within a single focus event, the helper holds the fd, so a delete during the scan is also benign (reader keeps reading the unlinked inode until close).

**Proposed Addition**:
Add a sentence to § History Depth > Read Pipeline (after the "~30 LOC" implementation shape paragraph):
"**Single-fd invariant.** The helper opens the file once via `os.Open`, performs all `Seek` and `Read` calls against that single file descriptor, and closes only after the tail bytes are assembled. The atomic-rename guarantee in § Read-Failure Handling only holds across the helper's full scan if the fd is held; a close-and-reopen between chunks would expose a torn-read window."

**Resolution**: Pending
**Notes**:

---

### 7. Tail-N "line" definition: trailing-newline edge case unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § History Depth > Read Pipeline > Definition of "line"

**Details**:
The "Definition of 'line'" paragraph (line 126) pins "a `\n`-terminated record" and "the reverse scan counts newline bytes". This works cleanly when every line in the `.bin` ends with `\n`. The edge case it does not address: a `.bin` file whose final line lacks a trailing `\n`.

Two reasonable behaviours:
- (a) The trailing un-terminated bytes are treated as a partial/in-progress line and **discarded** from the tail (only `\n`-terminated records are returned). Strict newline counting.
- (b) The trailing un-terminated bytes are **included** as the last line in the tail (counted against the N budget once, even without their terminator).

This matters because:
- The daemon's writer may or may not always end with `\n`. If `tmux capture-pane -e` ever produces output without a trailing newline (e.g. on a partial line being typed), the `.bin` may have a trailing partial. Build-phase verification is needed.
- Acceptance criterion (line 410) "reads at most the last 1000 `\n`-terminated lines" reads as case (a) — but does not explicitly say what happens to a trailing un-terminated tail.
- The placeholder rule "fewer than N lines = partial read, success" (line 191) is silent on whether a single-line file with no trailing `\n` counts as 0 lines or 1.

**Proposed Addition**:
Append to § History Depth > Read Pipeline > Definition of "line":
"**Trailing-newline edge case.** The reverse scan counts only `\n` bytes. A file whose final bytes lack a trailing `\n` has those trailing bytes treated as a partial/in-progress record and **excluded** from the returned tail (the helper returns only fully-terminated records). A zero-byte file and a file containing only an unterminated partial line both render the placeholder under the zero-line outcome. In practice the daemon writes via `tmux capture-pane -e` whose output always terminates lines, so this edge case is defensive."

Tighten the corresponding acceptance criterion if needed.

**Resolution**: Pending
**Notes**:

---

### 8. `previewDeps` wiring shape is not pinned (package-level mutable vs constructor-injected)

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Architecture Summary > Test seams

**Details**:
The Test seams subsection (line 368) introduces a "`previewDeps`-shaped seam in `internal/tui`" following "the project's DI convention (small interfaces, package-level `*Deps` structs, `t.Cleanup()` restoration)". But the project's existing `*Deps` precedents (`bootstrapDeps`, `openDeps`, `attachDeps`, `hooksDeps`) all live in the **`cmd`** package — `internal/tui` has no existing `*Deps` precedent.

This produces two reasonable interpretations:
- (a) Add a `previewDeps` package-level variable to `internal/tui` mirroring the cmd convention. Tests mutate the package-level variable and `t.Cleanup()` restores. CLAUDE.md notes that tests **must not** use `t.Parallel()` for this reason.
- (b) Pass dependencies through the `previewModel` constructor (idiomatic for Bubble Tea models, which are usually constructed with their deps). Tests construct test models with mock deps directly. No package-level state, `t.Parallel()` is safe.

These are different test architectures with different implications for how `pagepreview_test.go` is written. A planner reading the Test seams paragraph cannot tell which one is required.

The CLAUDE.md note that "the cmd package injects mocks via package-level mutable state" is specific to the cmd package — extending the convention to `internal/tui` is a design choice the spec implicitly makes but does not state.

**Proposed Addition**:
Refine § Architecture Summary > Test seams to pin the wiring shape. Recommended:
"**Wiring shape.** Dependencies are passed through the `previewModel` constructor (constructor-injected), not via a package-level mutable `previewDeps` variable. This is idiomatic for Bubble Tea models. Tests construct `previewModel` directly with mock implementations of `TmuxEnumerator` and `ScrollbackReader`; no package-level state to restore, no `t.Cleanup()` plumbing required, and `t.Parallel()` is safe in `pagepreview_test.go`. The `cmd` package's package-level `*Deps` convention is preserved at the cmd-package level for compatibility with existing tests but is not extended into `internal/tui`."

Or: pick the package-level variable shape and note explicitly that `t.Parallel()` must not be used in `pagepreview_test.go`.

**Resolution**: Pending
**Notes**:

---

### 9. Enumeration-returns-empty edge case (zero-window or zero-pane session) not handled

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Refresh Semantics > Initial-open ordering; § Acceptance Criteria > Edge cases

**Details**:
Initial-open ordering (lines 148–155) handles the case where structural enumeration **fails** (returns to Sessions list silently). But it does not handle the case where enumeration **succeeds** with an empty result — a session that exists but currently has zero windows or a window with zero panes. This is implausible for a normal tmux session (a session always has at least one window with one pane) but is achievable in race conditions: the session is being torn down between the `Space` press and the enumeration call but hasn't disappeared yet.

If the enumeration returns `[]WindowGroup{}` (empty), step 3 ("focus indices are set to window 0 / pane 0") sets indices that point at no pane. The first frame would render chrome ("Window 0 of 0", "Pane 0 of 0") and a viewport with no content (and no placeholder, because there's no pane to read).

A planner faced with this hits an unresolved branch: should empty enumeration be treated as enumeration failure (return to Sessions list silently) or as a degenerate preview (chrome shows zero counts, viewport empty)?

**Proposed Addition**:
Add a step 2.5 to § Refresh Semantics > Initial-open ordering, or extend step 2:
"If enumeration returns an empty result (zero windows, or a window with zero panes — e.g. session is being torn down) → treated identically to enumeration failure: return to Sessions list silently; no preview page is shown."

Add to § Acceptance Criteria > Edge cases:
"A session that returns empty structural enumeration (zero windows / zero panes) is treated as enumeration failure: preview does not open and the user remains on the Sessions list."

**Resolution**: Pending
**Notes**:

---

### 10. Window-resize during preview: re-read or pure re-flow not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Interaction Shape > Layout; § Refresh Semantics > Read Trigger Events

**Details**:
§ Interaction Shape (line 26) pins that `tea.WindowSizeMsg` is forwarded to the embedded viewport for re-flow. § Refresh Semantics > Read Trigger Events (lines 142–146) lists exactly three read triggers: initial open, `]`/`[`, `Tab`. Window resize is not in that list — implying resize does **not** trigger a re-read.

This is the right behaviour (the loaded N-line buffer is independent of viewport dimensions; viewport handles re-flow internally), but it should be pinned explicitly. A planner could reasonably misread "every focus-changing event triggers a fresh disk read" + "viewport re-flows on resize" as licence to re-read on resize, especially because resize visually feels like a state change.

This is also load-bearing for the performance budget: if resize triggered a re-read on every `tea.WindowSizeMsg` (which can fire many times during a drag-resize), the p99 < 5ms budget would be hit repeatedly per drag.

**Proposed Addition**:
Add a sentence to § Refresh Semantics > Read Trigger Events (or as a new "Non-triggers" subsection):
"**Resize is not a read trigger.** `tea.WindowSizeMsg` is forwarded to the embedded viewport for content re-flow (per § Interaction Shape > Layout); it does not trigger a fresh disk read. The loaded N-line buffer is decoupled from viewport dimensions, so the viewport re-renders the existing buffer at the new size without re-reading from disk."

Add to § Acceptance Criteria > Read pipeline:
"Window resize during preview re-flows the viewport without triggering a re-read of the focused pane's `.bin`."

**Resolution**: Pending
**Notes**:

---

### 11. Chrome counter under tmux base-index drift / non-contiguous window indices not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Multi-pane Rendering Shape > Chrome Floor

**Details**:
Tmux supports `set -g base-index 1` and `set -g pane-base-index 1`, and windows can be killed leaving gaps (e.g. a session with `window_index` values `0, 2, 5`). The structural enumeration returns these as-is.

§ Chrome Floor (line 86) pins "**Window M of N**" without saying whether `M` is:
- (a) The 1-based ordinal position of the focused window in the enumeration order (always contiguous: 1 of 3, 2 of 3, 3 of 3).
- (b) The tmux `window_index` itself (could be `0 of 3`, `2 of 3`, `5 of 3` — confusing).

Same question for "**Pane X of Y**" under a `pane-base-index 1` setup.

This is genuinely ambiguous in the spec as written and load-bearing for chrome wording. Finding 1 covers indexing convention generally; this finding pins the user-facing display question separately because the answer affects testable acceptance criteria.

**Proposed Addition**:
Add to § Multi-pane Rendering Shape > Chrome Floor (or fold into Finding 1's edit if combined):
"**Counter semantics.** `M` and `X` are 1-based ordinal positions in enumeration order, **not** the tmux `window_index` / `pane_index` values. Under base-index drift or after window-kill gaps (e.g. `window_index` values `0, 2, 5`), chrome shows `Window 1 of 3`, `Window 2 of 3`, `Window 3 of 3` as the user cycles — never `Window 5 of 3`. The window name (`#W`) carries identity; the counter carries position."

Add a corresponding acceptance criterion: "Under a session with non-contiguous tmux `window_index` values, the chrome counter `M of N` shows ordinal position in enumeration order (always `1..N`), not the raw `window_index`."

**Resolution**: Pending
**Notes**:

---

### 12. Acceptance criterion for side-effect-free contract is not mechanically testable as written

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: § Acceptance Criteria > Side-effect-free contract

**Details**:
Line 431:
> A test that opens preview on session S, cycles through every pane in S, dismisses, and re-attaches to S observes session state (markers, FIFOs, hooks, scrollback content) byte-identical to a baseline that did not open preview at all.

This is a strong contract but the assertion vector is fuzzy:
- "byte-identical" against what — a tmux state dump? specific files on disk? specific markers via `show-options`?
- "scrollback content" — the `.bin` files on disk are written by the daemon, so they will differ between the preview-open-test and the baseline simply because time has passed and ticks have happened. "Byte-identical" is impossible against a live daemon.
- Markers (`@portal-skeleton-*`, `@portal-restoring`) — these are owned by bootstrap/restore, not by attach lifecycle. A baseline that "did not open preview" but did everything else still mutates these on bootstrap.

The intent is clear (preview does no writes), but the acceptance criterion as written cannot be mechanically tested. A planner asked to write the test will either reduce the assertion to "preview made no writes" (which is the actual intent) or be stuck on what "byte-identical" means.

The realistic mechanical assertions are:
- The `previewModel` does not invoke any tmux command other than the one structural enumeration call at open.
- The `previewModel` does not call `os.WriteFile`, `os.OpenFile` with any write flag, or any `state` package writer.
- After preview open + cycle + dismiss, no new tmux options are set on the session, no FIFOs are created or drained, no resume hooks fired.

**Proposed Addition**:
Replace the "Side-effect-free contract" acceptance criterion with a list of mechanically-testable assertions:
"**Side-effect-free contract.** Within a single test that opens preview on session S, cycles through every pane in S, and dismisses:
- The preview code path issues exactly one tmux read invocation (the structural enumeration at open) and zero tmux write invocations. Verifiable via the `TmuxEnumerator` mock recording calls.
- The preview code path issues only `os.Open` + read syscalls against `.bin` paths, never `os.OpenFile` with any write flag, `os.WriteFile`, `os.Remove`, or `os.Rename`. Verifiable via the `ScrollbackReader` mock recording calls.
- No FIFOs are created or drained, no hooks (`hooks.Store`) writes occur, no `@portal-*` markers are mutated on session S. Verifiable by snapshotting the relevant state before and after the preview interaction within the test."

Or: keep the higher-level wording but explicitly note "byte-identical" excludes daemon-driven `.bin` updates that happen in the background, and pin the assertion vector (mock recording) for the no-writes claim.

**Resolution**: Pending
**Notes**:

---
