---
status: complete
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

**Details**: Spec says chrome data comes from "tmux structural enumeration (e.g. `list-panes -F`)" and is "computed once at preview-open" without saying which existing method is used. No existing method returns window-grouped panes plus window names for one session.

**Resolution**: Approved
**Notes**: Added "Concrete enumeration call" subsection in § Multi-pane Rendering Shape > Chrome Floor pinning either (a) new `ListWindowsAndPanesInSession` method (preferred) or (b) composition. Added enumeration failure handling. Qualified the "no new tmux methods" claim as scoped to capture wrappers.

---

### 2. Pane-key resolution from tmux enumeration to disk path is not pinned

**Resolution**: Approved
**Notes**: Added explicit resolution chain in § Cross-cutting Seams > State Package API Reuse: `(session, w, p) → state.SanitizePaneKey → state.ScrollbackFile → tail-N read`. Named `SanitizePaneKey` as the canonical helper. Documented marker-independence.

---

### 3. Initial-open chrome enumeration timing vs first content read is not ordered

**Resolution**: Approved
**Notes**: Added "Initial-open ordering" subsection under § Refresh Semantics > Read Trigger Events with 4-step sequence (enumerate → fail-fast or proceed → tail-N read → atomic first frame).

---

### 4. Empty / very-short `.bin` content is not differentiated from missing

**Resolution**: Approved
**Notes**: Refined § Read-Failure Handling > Placeholder with explicit triggering conditions (ENOENT, zero-byte, OS error) and non-triggering condition (fewer-than-N lines = partial read, success). Documented error-string lifetime as part of the same edit.

---

### 5. Tail-N "lines" definition is ambiguous around long/wrapped lines

**Resolution**: Approved
**Notes**: Added "Definition of 'line'" paragraph in § History Depth > Read Pipeline pinning `\n`-terminated record counting (no logical-line reconstruction).

---

### 6. Acceptance criteria / observable success states are implicit, not enumerated

**Resolution**: Approved
**Notes**: Added § Acceptance Criteria section before § Open Items, grouping observable assertions under Entry & dismiss / Within-preview navigation / Read pipeline / Chrome / Edge cases / Filter integration / Side-effect-free contract.

---

### 7. Test seam guidance for `internal/tui` page-state additions is missing

**Resolution**: Approved
**Notes**: Added "Test seams" subsection in § Architecture Summary specifying TmuxEnumerator and ScrollbackReader interfaces under `previewDeps` per project DI convention. Pinned that tests must not require a real tmux server.

---

### 8. Inconsistency: "No new tmux wrapper" vs structural enumeration call

**Resolution**: Approved
**Notes**: Refined the Single-read-path bullet to scope "no new wrapper" to capture wrappers specifically; structural enumeration listing method handled by Finding 1.

---

### 9. Viewport scroll keymap is not bound or named

**Resolution**: Approved
**Notes**: Added an explicit "Viewport scroll" row in the keymap table enumerating bubbles/viewport defaults (Up/Down/PgUp/PgDn/Home/End/ctrl-u/ctrl-d/j/k) and a "Keymap policy" paragraph stating preview-owned keys vs pass-through.

---

### 10. "No between-session keymap" leaves arrow/j/k behaviour TBD inline

**Resolution**: Approved
**Notes**: Replaced the "TBD by build phase keymap design" line under § No In-preview Between-Session Stepping with a concrete policy referencing the keymap table — Up/Down/j/k pass through to viewport scroll; Left/Right/n/p unbound.

---

### 11. Preview-open with zero sessions / cursor-on-empty-list is not addressed

**Resolution**: Approved
**Notes**: Added "Empty list / no highlighted item" bullet to § Trigger and Entry Point pinning Space as a no-op when no item is highlighted.

---

### 12. "Brief error string" for OS-level read failure is unspecified in shape and lifetime

**Resolution**: Approved
**Notes**: Folded into Finding 4's edit — single error string for all OS errors, no per-errno differentiation, no per-pane error cache (subsequent focus changes retry), chrome unaffected.

---

### 13. Sessions list refresh on Esc is asserted but not located

**Resolution**: Approved
**Notes**: Added "Re-fetch contract" paragraph under § Cross-cutting Seams > Externally-Killed Session During Preview pinning the contract to preview's transition handler with build-phase confirmation of the existing path.

---

### 14. Implicit assumption: preview viewport sizing relative to terminal

**Resolution**: Approved
**Notes**: Added "Layout" bullet in § Interaction Shape pinning full-terminal occupation, single-line chrome, viewport fills remainder, `tea.WindowSizeMsg` propagation.

---

### 15. "Position state not retained across re-opens" — what about within a single bootstrap session

**Resolution**: Approved
**Notes**: Added "Model lifecycle" paragraph to § Multi-pane Rendering Shape > Position on session re-entry pinning fresh `previewModel` per Space press, fresh enumeration, fresh first-pane read.

---

### 16. Atomicity claim assumes daemon write strategy that may not match current implementation

**Resolution**: Approved
**Notes**: Verified `state.WriteScrollbackIfChanged` calls `fileutil.AtomicWrite0600` in `internal/state/scrollback.go`. Updated the bullet under § Read-Failure Handling to name `WriteScrollbackIfChanged` directly and note the verification + future-change caveat.

---

### 17. Performance budget is implied ("sub-millisecond") but not framed as a target

**Resolution**: Approved
**Notes**: Added "Performance budget" paragraph in § History Depth > Read Pipeline pinning a target of tail-N p99 < 5ms on a 4 MB file with a build-phase benchmark and an explicit revisit trigger if exceeded.

---
