---
status: complete
created: 2026-05-06
cycle: 2
phase: Gap Analysis
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Gap Analysis

## Findings

### 1. Internal contradiction: window/pane indexing convention (0-based vs 1-based)

**Resolution**: Approved
**Notes**: Updated § Multi-pane Rendering Shape > Position on session re-entry to use "window 0 / pane 0 in enumeration order". Added "Indexing convention" paragraph pinning 0-based internal indices. Folded with Finding 11 — added "Counter semantics" paragraph in Chrome Floor pinning 1-based ordinal display, never raw `window_index`.

---

### 2. Pane-focus index after window cycle is unspecified

**Resolution**: Approved
**Notes**: Added "Pane focus on window cycle" paragraph in § Multi-pane Rendering Shape > Within-preview Key Bindings (resets to pane 0). Added acceptance criterion under Within-preview navigation.

---

### 3. `stateDir` source for `state.ScrollbackFile(stateDir, paneKey)` is not pinned

**Resolution**: Approved
**Notes**: Added "stateDir resolution" bullet in § Cross-cutting Seams > State Package API Reuse pinning that ScrollbackReader.Tail(paneKey) hides stateDir, captured once at TUI startup, stable for process lifetime. Reflected in § Architecture Summary > Test seams.

---

### 4. `state.SanitizePaneKey` argument types not pinned (string vs int for indices)

**Resolution**: Approved
**Notes**: Added clarification to the SanitizePaneKey bullet under § Cross-cutting Seams > State Package API Reuse: "arguments must match the daemon's call site verbatim (the goal is byte-identical pane keys)". Reflected in test seams that the TmuxEnumerator result type feeds SanitizePaneKey directly without adaptation.

---

### 5. Internal contradiction: OS-level read error is listed as both placeholder trigger and error-string trigger

**Resolution**: Approved
**Notes**: Restructured § Read-Failure Handling > Placeholder bullets — removed OS-error from triggering list and promoted to a sibling paragraph clarifying it routes to the error string instead.

---

### 6. Reverse-chunked tail-N scan: file-handle lifetime and torn-read invariant not pinned

**Resolution**: Approved
**Notes**: Added "Single-fd invariant" paragraph after the implementation-shape paragraph in § History Depth > Read Pipeline pinning the open-once / scan-fully / close-after invariant.

---

### 7. Tail-N "line" definition: trailing-newline edge case unspecified

**Resolution**: Approved
**Notes**: Added "Trailing-newline edge case" paragraph in § History Depth > Read Pipeline > Definition of "line": un-terminated trailing bytes excluded from tail; zero-line outcome from a single-line file with no terminator yields the placeholder.

---

### 8. `previewDeps` wiring shape is not pinned (package-level mutable vs constructor-injected)

**Resolution**: Approved
**Notes**: Refined § Architecture Summary > Test seams to pin constructor-injected wiring (not package-level *Deps), making `t.Parallel()` safe in pagepreview_test.go. Cmd-package convention preserved at cmd layer only.

---

### 9. Enumeration-returns-empty edge case (zero-window or zero-pane session) not handled

**Resolution**: Approved
**Notes**: Added step 3 to § Refresh Semantics > Initial-open ordering treating empty enumeration result identically to enumeration failure (silent return to Sessions list). Renumbered subsequent steps. Added acceptance criterion under Edge cases.

---

### 10. Window-resize during preview: re-read or pure re-flow not pinned

**Resolution**: Approved
**Notes**: Added "Resize is not a read trigger" paragraph in § Refresh Semantics > Read Trigger Events. Added acceptance criterion under Read pipeline.

---

### 11. Chrome counter under tmux base-index drift / non-contiguous window indices not pinned

**Resolution**: Approved
**Notes**: Folded into Finding 1's edit — "Counter semantics" paragraph in § Multi-pane Rendering Shape > Chrome Floor pins 1-based ordinal position over raw tmux indices. Acceptance criterion added under Within-preview navigation.

---

### 12. Acceptance criterion for side-effect-free contract is not mechanically testable as written

**Resolution**: Approved
**Notes**: Replaced single high-level bullet under § Acceptance Criteria > Side-effect-free contract with three mechanically-testable assertions framed against the TmuxEnumerator and ScrollbackReader mock recordings, plus snapshot-style verification for hooks/markers/FIFOs.

---
