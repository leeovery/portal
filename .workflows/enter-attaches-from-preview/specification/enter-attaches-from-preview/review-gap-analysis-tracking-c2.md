---
status: complete
created: 2026-05-15
cycle: 2
phase: Gap Analysis
topic: enter-attaches-from-preview
---

# Review Tracking: enter-attaches-from-preview - Gap Analysis

## Findings

### 1. Connector target string — session-only vs session:win.pane

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Pre-select + attach sequence > Step 4. Connector handoff*, *Out of scope > Inside-tmux uniformity*

**Details**:
Step 4 specifies connector commands as `tmux attach-session -A -t <session>` (outside tmux) and `tmux switch-client -t <session>` (inside tmux) — session-only targets. The pre-select calls in steps 2 and 3 are the mechanism that positions tmux on the right `(window, pane)` before the connector fires.

This is internally consistent only if pre-select state survives the connector handoff. For outside-tmux `attach-session -A -t <session>`, that is fine — `select-pane` mutates server state, and `attach-session` resolves the session's current pane on attach. For inside-tmux `switch-client -t <session>`, the same holds (switch-client honours the target session's current window/pane). Implicit but unstated.

A separate concern: the *Exact-match target syntax* subsection says the `=` prefix MUST apply to `attach-session` and `switch-client` too — but the Step 4 prose still shows `-t <session>` without the `=`. The exact-match section is the load-bearing one (recently added in cycle 1), but Step 4's example bullets contradict its own newly-added rule.

**Proposed Addition**:
Step 4 example commands updated to `-t '=<session>'` for consistency with exact-match section. Added one-line note that pre-select state positions tmux on the focused `(window, pane)` and the connector target is intentionally session-only — the session's current window/pane resolves at connect time.

**Resolution**: Approved
**Notes**: Exact-match prefix applied to connector examples; pre-select-positioning contract made explicit.

---

### 2. Distinguishing `has-session` non-zero exit from OS-layer error

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Pre-select + attach sequence > Step 1*

**Details**:
Step 1 defines three outcomes (zero exit, non-zero exit, OS-layer error) with materially different branches — non-zero exit triggers the bail path; OS-layer error proceeds as if the session exists. But the spec does not pin how the build phase distinguishes these two cases.

Without the spec pinning the discriminator, an implementer could treat any non-nil error as "non-zero exit" (OS-layer error wrongly triggers the bail path) or as "OS-layer error" (killed-session detection is missed).

**Proposed Addition**:
Sub-bullet under Step 1's "OS-layer error" branch: build phase MUST discriminate `*exec.ExitError` (non-zero exit — bail) from non-`ExitError` errors (OS-layer failure — proceed). Discriminator mechanism is a build decision; spec contract is the two cases are not collapsed.

**Resolution**: Approved
**Notes**: Discriminator contract pinned; build phase picks mechanism (extend Commander, type-assert, etc.).

---

### 3. Log destination / component for swallowed pre-select failures

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Pre-select + attach sequence > Steps 2 and 3*

**Details**:
Steps 2 and 3 say non-zero exit on `select-window`/`select-pane` is "log and swallow". The spec does not say which logger, what level, or whether anything is logged at all. The cmd/bootstrap path uses WARN under `ComponentBootstrap`; a consistent component for preview-Enter failures would let users grep state logs for these.

**Proposed Addition**:
Sub-bullet in Steps 2 and 3: swallowed failures log at WARN through the existing structured logger (`internal/state`), consistent with bootstrap. Build phase picks the exact component string; spec-level contract is WARN + structured logger + greppable component, not silent.

**Resolution**: Approved
**Notes**: WARN-level + structured logger pinned; component string is build decision.

---

### 4. Flash chrome line layout when no flash is active

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Session-killed-externally bail path > Inline flash — feature-local infrastructure > Shape > Render*

**Details**:
Cycle 1 pinned that the flash sits between the filter input and the Sessions list. Remaining gap: whether the flash row occupies space when no flash is active (always-reserved vs collapsed-when-empty).

**Proposed Addition**:
Render bullet extended: flash row rendered only when active; no row reserved between filter input and list when inactive; first bail visually pushes the list down one row, tick expiry / clearing keystroke pops it back up.

**Resolution**: Approved
**Notes**: Collapsed-when-empty — list sits directly under filter today; bail pushes it down by one row temporarily.

---

### 5. Flash text wording — fixed or build-phase

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Session-killed-externally bail path > Inline flash — feature-local infrastructure*

**Details**:
The chrome line ("Discoverability" section) explicitly pins token wording: "Exact token placement and wording is fixed by this spec." The flash text, by contrast, is given as `e.g.` — suggesting wording is open. Without a pin, an implementer might paraphrase.

**Proposed Addition**:
Drop `e.g.`; state exact wording is fixed by this spec: `session "<name>" no longer exists`. Specify the `<name>` placeholder is the captured session name, double-quoted, no trailing punctuation. Matches the chrome-line precedent.

**Resolution**: Approved
**Notes**: Wording fixed by spec; matches chrome-line precedent for spec-pinned user-visible strings.

---

### 6. Viewport interception of Enter

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Enter binding behaviour*, *Keymap expansion policy > Owned preview keys*

**Details**:
With the new `tea.KeyEnter` case added to preview's `Update`, the spec implies Enter is intercepted entirely but does not state whether the handler returns early or also forwards the key to the viewport. Observable difference is zero today (viewport treats Enter as no-op), but matters if a future `bubbles/viewport` version assigns Enter a behaviour.

**Proposed Addition**:
One-line clarification in *Enter binding behaviour*: `Enter` is intercepted by preview's Update handler and is NOT forwarded to the embedded viewport. Future `bubbles/viewport` Enter behaviour cannot leak through.

**Resolution**: Approved
**Notes**: Enter is intercepted, not forwarded — future-proofs against viewport behaviour changes.

---
