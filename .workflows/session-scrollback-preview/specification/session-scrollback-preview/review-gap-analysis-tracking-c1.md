---
status: in-progress
created: 2026-05-06
cycle: 1
phase: Gap Analysis
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Gap Analysis

## Findings

### 1. Multi-pane sessions: structural enumeration source is under-specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Chrome Floor" (Chrome data source), "Cross-cutting Seams / State Package API Reuse", "Architecture Summary / Chrome rendering"

**Details**:
The spec says chrome data comes from "tmux structural enumeration (e.g. `list-panes -F`)" and is "computed once at preview-open". It does not say:
- Which specific `tmux` invocation is used (single `list-windows` + `list-panes` per session, one combined `-a` query, or per-window enumeration).
- Which existing `tmux.Client` methods are reused vs whether a new one is required. The CLAUDE.md inventory lists `ListPanesInSession`, `ListAllPanes`, `ListAllPanesWithFormat` as candidates. The spec's "No new methods on `tmux.Client`" claim in the Cross-cutting Seams section conflicts with the absence of a named existing method that returns window-grouped pane listing for a single session.
- Whether window names are obtained via the same call or a second `list-windows` call.
- What happens if the structural enumeration call itself fails (the only failure mode discussed is per-pane content reads).

A planner cannot turn this into a task without picking one path and confirming a method exists on `tmux.Client` that returns the needed shape, or specifying that a new accessor is needed. The "No changes to tmux.Client" claim should be either confirmed by naming the reused method, or relaxed.

**Proposed Addition**:

---

### 2. Pane-key resolution from tmux enumeration to disk path is not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Cross-cutting Seams / State Package API Reuse", "Read Pipeline", "Source of Preview Bytes"

**Details**:
Preview reads from `state.ScrollbackFile(stateDir, paneKey)`. The structural enumeration produces tmux session/window/pane identifiers; the daemon writes `.bin` files keyed by what CLAUDE.md calls "structural pane key". The spec says preview "must look up structural pane keys consistent with how the daemon writes them (see `internal/state` paneKey helpers)" but does not name the helper, define the mapping, or specify whether the daemon's keying is addressable from tmux runtime fields alone (session name + window index + pane index) or requires markers (`@portal-skeleton-<paneKey>`).

The bootstrap notes in CLAUDE.md mention "structural key is preserved across base-index drift" — meaning the runtime indices may not match the saved key directly. This is a load-bearing detail for whether preview can read the right `.bin` file at all. Without a named helper or explicit mapping rule, an implementer would have to derive the resolution policy from reading daemon code.

**Proposed Addition**:

---

### 3. Initial-open chrome enumeration timing vs first content read is not ordered

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Refresh Semantics / Read Trigger Events", "Chrome Floor", "Interaction Shape"

**Details**:
The spec says:
- "Chrome is computed once at preview-open and cycled in place."
- "Initial preview-open (Space) — lazy per focus: reads only the currently-focused pane (window 1 / pane 1 by reset rule)."

It does not specify whether structural enumeration happens before or after the first `.bin` read on `Space`, or whether both are done synchronously in the `Update` handler for the Space keypress. This matters for:
- Latency budget on `Space` (two synchronous calls — tmux enumeration + tail-N read — vs deferred enumeration).
- Whether the preview page can render chrome before content arrives, or whether both are required to render the first frame.
- What the user sees if enumeration fails but the disk read would have succeeded (or vice versa).

**Proposed Addition**:

---

### 4. Empty / very-short `.bin` content is not differentiated from missing

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Read-Failure Handling / Placeholder", "Brand-new-session Edge Case"

**Details**:
The placeholder "(no saved content)" is specified for missing or unreadable `.bin`. It does not say what is rendered when:
- The `.bin` file exists but is empty (zero bytes) — e.g. daemon created the file but no capture has populated it yet.
- The `.bin` file exists but has fewer than N lines (e.g. 5 lines from a brand-new pane).

The reasonable behaviour for the second case is "render whatever's there" (it's the tail of a smaller file), but for the first case it could be "(no saved content)" or an empty viewport. Without an explicit rule the implementation could land either way, and the user-visible affordance differs (placeholder text vs blank pane).

**Proposed Addition**:

---

### 5. Tail-N "lines" definition is ambiguous around long/wrapped lines

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "History Depth / Slice size", "Read Pipeline"

**Details**:
"The last N lines (N=1000)" is defined in terms of newlines counted in the reverse chunked scan. The spec also notes:
- tmux `capture-pane -e` output is hard-wrapped to the source pane width at capture time.
- `bubbles/viewport` does not auto-wrap; long lines get ANSI-aware horizontal cut.

So a "line" in the `.bin` file already corresponds to a display line at capture-time pane width, not a logical/source line. This is fine, but the spec doesn't pin whether the tail counts:
- Newline characters (`\n`) — straightforward.
- Display lines after re-rendering — not what the helper does, but a reader unfamiliar with the daemon might assume so.

The memory budget claim ("typical line widths × 1000 ≪ a few MB") implicitly assumes newline counting. Pinning "N=1000 means the last 1000 `\n`-terminated records in the `.bin` file" removes interpretive ambiguity for the helper implementer.

**Proposed Addition**:

---

### 6. Acceptance criteria / observable success states are implicit, not enumerated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Whole specification (no dedicated section)

**Details**:
The spec is rich on behaviour but never enumerates "this feature is done when…" criteria. A planner breaking this into phases/tasks has to derive the acceptance set from prose. Examples of testable assertions that are present-but-scattered or implicit:
- Pressing Space on Sessions page enters preview at window 1 / pane 1.
- Pressing Esc returns to Sessions list with cursor at pre-Space position.
- `]` / `[` / `Tab` cycle correctly with wraparound; no-op cleanly on degenerate single-window single-pane.
- Preview leaves session state byte-identical (no marker mutation, no FIFO drain, no hooks fired).
- Missing `.bin` renders placeholder; chrome stays correct.
- Tail read is sub-millisecond on a 3.7MB file (perf budget).
- Filter behaviour: Space inserts literal space while filtering; Space opens preview only after Enter.

Without a grouped "Acceptance Criteria" or "Observable Behaviours" section, planning has to invent the test surface.

**Proposed Addition**:

---

### 7. Test seam guidance for `internal/tui` page-state additions is missing

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Architecture Summary", "Cross-cutting Seams"

**Details**:
The spec calls out new `pagePreview` arm in the TUI page state machine and a tail-N helper in `internal/state`. It does not document:
- How preview's reads should be testable without a real tmux server (e.g. injecting a `ScrollbackFile`-like resolver, or a `tailN(path string, n int) ([]byte, error)` seam).
- How structural enumeration should be testable (mock `tmux.Client` interface vs real socket fixtures via `tmuxtest`).
- Whether existing TUI tests (which exercise `Update` directly) cover the new page or whether a new test file convention is expected.

Given the project's strong DI / mock convention (CLAUDE.md: "All external dependencies use small interfaces (1-3 methods). Commands expose package-level `*Deps` structs"), this is a project-shaped gap — planners will need to know whether `pagePreview` introduces a new `previewDeps`-style seam or composes existing ones.

**Proposed Addition**:

---

### 8. Inconsistency: "No new tmux wrapper" vs structural enumeration call

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Source of Preview Bytes / Single read path consequences", "Cross-cutting Seams / State Package API Reuse", "Architecture Summary"

**Details**:
"Source of Preview Bytes" justifies the always-disk decision partly by saying "No new tmux wrapper. The existing `tmux.Client.CapturePane` ... a bounded variant ... would have been net-new code. Always-disk avoids that addition entirely." Then "Chrome Floor" says chrome data comes from `list-panes -F`-style enumeration, and Cross-cutting Seams says "No new methods on `tmux.Client`."

Either:
(a) An existing `tmux.Client` method already returns the needed enumeration shape (one of `ListPanesInSession` / `ListAllPanesWithFormat` etc.) — should be named explicitly.
(b) A new method is required for the chrome enumeration shape — contradicts "No new methods on `tmux.Client`".

This is the same family of issue as #1 but framed as an internal contradiction: the rejection-of-bounded-capture rationale and the "no new tmux wrapper" claim need to be cleanly compatible with whatever enumeration call chrome uses.

**Proposed Addition**:

---

### 9. Viewport scroll keymap is not bound or named

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Architecture Summary / Within-preview keymap", "Refresh Semantics / Viewport-internal Scroll Does Not Re-read", "History Depth / Scroll within bounds"

**Details**:
The spec repeatedly refers to "the viewport's native scroll keymap" / "viewport's native scroll keys (passed through to the embedded `bubbles/viewport`)" without naming them. `bubbles/viewport`'s defaults include PgUp/PgDn, Up/Down, ctrl-u/ctrl-d, etc. The spec's "Open Items Handed to the Build Phase" mentions confirming that `]` `[` `Tab` don't collide with viewport bindings, but doesn't say which viewport bindings preview keeps vs which it shadows.

For example, `bubbles/list` may also have arrow-key behaviour the user expects in lists; preview's viewport bindings need to be enumerated (or the policy "all `bubbles/viewport` defaults pass through unchanged" stated explicitly) so a planner can write a keymap test.

**Proposed Addition**:

---

### 10. "No between-session keymap" leaves arrow/j/k behaviour TBD inline

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "No In-preview Between-Session Stepping"

**Details**:
The spec says: "Arrow keys, `j`/`k`, `n`/`p`, etc. inside preview are unbound or no-op (TBD by build phase keymap design); they do not advance to the next session."

But arrow keys also overlap with `bubbles/viewport`'s default scroll bindings (Up/Down). Saying they're "unbound or no-op" while elsewhere saying viewport native scroll keys pass through creates a possible collision. An implementer would need to choose:
(a) Up/Down scroll within the focused pane viewport (passes through).
(b) Up/Down do nothing (explicitly no-op'd to prevent any between-session interpretation).

Given "Scroll within bounds" is part of the feature, (a) seems intended. But the "TBD by build phase keymap design" leaves it unresolved. This is the kind of decision spec should pin so planning can write a single keymap table.

**Proposed Addition**:

---

### 11. Preview-open with zero sessions / cursor-on-empty-list is not addressed

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Trigger and Entry Point", "Filter Behaviour with Preview"

**Details**:
The spec assumes a session is highlighted when `Space` is pressed. It does not specify behaviour when:
- The Sessions list is empty (no sessions exist — possible after `portal clean` or first-ever run inside tmux where the only session is excluded by self-exclusion).
- A filter narrows the list to zero matches and `Space` is pressed.

Reasonable behaviour: `Space` no-ops because there's no item to highlight. But this should be pinned, since the empty-list edge for the Sessions page is not otherwise obvious from the rest of the document.

**Proposed Addition**:

---

### 12. "Brief error string" for OS-level read failure is unspecified in shape and lifetime

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Read-Failure Handling", "Open Items Handed to the Build Phase"

**Details**:
The spec acknowledges OS-level read errors ("permissions, disk full, etc.") and says "the viewport renders a brief error string in place of content" with the wording deferred to build. Beyond wording, it does not pin:
- Whether the same string is shown for every error type or whether (say) ENOENT is rendered as the placeholder while EACCES gets a different string.
- Whether the chrome counts still increment past the failed pane (presumably yes, by analogy with placeholder behaviour, but not stated).
- Whether subsequent focus changes retry the read or stick with the cached error string for that pane (the "no content cache" rule says yes, retry — but the error case should confirm).

**Proposed Addition**:

---

### 13. Sessions list refresh on Esc is asserted but not located

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Cross-cutting Seams / Externally-Killed Session During Preview"

**Details**:
"Esc back to list. The Sessions list re-fetches the live session list on return — the killed session simply isn't there anymore." This asserts a behaviour that may or may not be present in the current `internal/tui` Sessions page. The spec doesn't say whether:
- This is existing behaviour that preview inherits (and therefore the Sessions page already re-fetches whenever it gains focus).
- Or new behaviour the build phase must add (re-fetch on `pagePreview → pageSessions` transition).

If it's the latter, this is a small but real implementation task that planning would otherwise miss because it's framed as "nothing new is needed".

**Proposed Addition**:

---

### 14. Implicit assumption: preview viewport sizing relative to terminal

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Interaction Shape", "Cross-cutting Seams / ANSI Passthrough vs Viewport Width"

**Details**:
The spec says preview is "a full-screen page" and discusses preview-viewport-width vs source-pane-width. It does not pin:
- Where chrome lives in the layout (top, bottom, both?) — though "header vs footer placement" is in Open Items.
- How much vertical space chrome consumes vs the viewport content area.
- Whether the viewport adjusts on terminal resize (`tea.WindowSizeMsg`) and what happens to scroll position on resize (assumption: bubbles/viewport handles this, but worth pinning).

The width comparison rule ("If preview viewport ≥ source pane width: no truncation; If preview viewport < source pane width: clean ANSI-aware horizontal cut") presumes the implementer has decided viewport width = terminal width minus chrome margin, but no such derivation is stated.

**Proposed Addition**:

---

### 15. "Position state not retained across re-opens" — what about within a single bootstrap session

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Multi-pane Rendering Shape / Position on session re-entry"

**Details**:
"Stepping out via `Esc` and re-opening preview on the same session re-opens at window 1 / pane 1, not at the last viewed position. Per-session position state is not retained."

This is unambiguous for the same session, but unclear whether the preview model itself is rebuilt each time `Space` is pressed (a new `previewModel` per open) or whether a singleton model is reused with reset state. This affects:
- Whether structural enumeration is repeated on every open (probably yes, per spec).
- Whether the tail-N read for window 1 / pane 1 is repeated on every open of the same session (probably yes — per "fresh disk read on every focus-changing event", and initial-open is one of them).

A "preview model lifecycle" sentence would close this gap explicitly.

**Proposed Addition**:

---

### 16. Atomicity claim assumes daemon write strategy that may not match current implementation

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "Read-Failure Handling / Daemon mid-write while preview reads"

**Details**:
"The daemon writes via `fileutil.AtomicWrite0600` (tempfile + rename), so the reader observes either the previous full content or the new full content — never torn bytes."

The spec asserts this as a property of the daemon. CLAUDE.md mentions `fileutil.AtomicWrite` as a hooks-store helper but doesn't list `AtomicWrite0600` or claim the state daemon uses it for `.bin` writes. If the daemon currently appends to `.bin` files (which would be more efficient than full rewrite for scrollback) rather than rewrites atomically, the torn-read mitigation collapses.

The build phase needs to either (a) verify the daemon's current write strategy matches the assertion and document it, or (b) pin a different mitigation (e.g. accept rare brief torn reads as transient and let the next focus change recover). Without a confirmed write-side behaviour, the read-side claim is unverifiable.

**Proposed Addition**:

---

### 17. Performance budget is implied ("sub-millisecond") but not framed as a target

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: "History Depth / Read Pipeline", "Refresh Semantics / Read is synchronous"

**Details**:
The spec asserts "sub-millisecond cost" and "bounded and sub-millisecond regardless of file size" as the basis for synchronous in-`Update` reads. This is a claim, not a target — there is no "if measured tail-N read on a 3.7MB file exceeds X ms, the implementation must be revised" line. For planning:
- A planner should know whether to write a bench test guarding the assertion.
- A reviewer should know what threshold causes "the synchronous-in-Update decision is invalidated".

Pinning a soft target (e.g. "tail-N read budget: < 5ms p99 on 4MB file") would make the synchronous-read decision auditable rather than asserted.

**Proposed Addition**:

---
