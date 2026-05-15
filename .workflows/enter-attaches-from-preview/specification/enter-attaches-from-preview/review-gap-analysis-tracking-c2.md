---
status: in-progress
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

A separate concern: the *Exact-match target syntax* subsection says the `=` prefix MUST apply to `attach-session` and `switch-client` too — but the Step 4 prose still shows `-t <session>` without the `=`. The exact-match section is the load-bearing one (recently added in cycle 1), but Step 4's example bullets contradict its own newly-added rule. An implementer copying Step 4's example would drop the `=` prefix on the connector calls.

The *Inside-tmux uniformity* out-of-scope item further hints at `switch-client -t session:win.pane` as a possible alternative shape, leaving the contract ambiguous between "always pre-select + session-only connector" and "may use combined target". Minor severity — primary risk is implementer copies Step 4's example verbatim and drops the exact-match prefix.

**Proposed Addition**:
Two clarifications: (a) Step 4's example commands MUST show the `=` exact-match prefix (`-A -t '=<session>'` and `-t '=<session>'`) to be consistent with the exact-match section; (b) one-line statement that pre-select state is what positions tmux on the focused (window, pane) and the connector target is intentionally session-only — the session's current window/pane resolves at connect time.

**Resolution**: Pending
**Notes**:

---

### 2. Distinguishing `has-session` non-zero exit from OS-layer error

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Pre-select + attach sequence > Step 1*

**Details**:
Step 1 defines three outcomes (zero exit, non-zero exit, OS-layer error) with materially different branches — non-zero exit triggers the bail path; OS-layer error proceeds as if the session exists. But the spec does not pin how the build phase distinguishes these two cases.

The `tmux.Commander` interface returns `error`. The standard Go shape is: non-zero exit produces `*exec.ExitError`; OS-layer failure (missing binary, fork failure, permission) produces a different error type (e.g. `*exec.Error`, `*fs.PathError`). Without the spec pinning the discriminator, an implementer could:

- Treat any non-nil error as "non-zero exit" → OS-layer error wrongly triggers the bail path with a misleading flash.
- Treat any non-nil error as "OS-layer error" → killed-session detection is missed.

The existing `tmux` package (per CLAUDE.md, `Commander.Run` / `RunRaw`) does not appear to surface this distinction explicitly in its current return shape; the spec should at least state the contract so build phase wires the discriminator (or adds a new commander shape) deliberately. Minor severity.

**Proposed Addition**:
Sub-bullet under Step 1's "OS-layer error" branch: build phase MUST discriminate `*exec.ExitError` (non-zero exit) from non-`ExitError` errors (OS-layer failure). The discriminator is a build-phase detail (e.g. extend `Commander` return shape, type-assert at call site); the spec-level contract is that the two cases are not collapsed into a single "any error" branch.

**Resolution**: Pending
**Notes**:

---

### 3. Log destination / component for swallowed pre-select failures

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Pre-select + attach sequence > Steps 2 and 3*

**Details**:
Steps 2 and 3 say non-zero exit on `select-window`/`select-pane` is "log and swallow". The codebase has a structured logger (per CLAUDE.md, `internal/state` ships one used under `ComponentBootstrap` etc.). The spec does not say:

- Which logger / component name the swallowed failures log under.
- What log level (WARN vs DEBUG vs INFO).
- Whether anything is logged at all in non-debug builds.

The cmd/bootstrap path uses WARN under `ComponentBootstrap`. A consistent component for preview-Enter failures (e.g. `ComponentPreview` or `ComponentTUI`) would let users grep state logs for these. Minor severity — failure is benign and self-correcting, but a silent-by-default policy diverges from how bootstrap handles similar best-effort failures.

**Proposed Addition**:
Sub-bullet in Steps 2 and 3: swallowed failures log at WARN under the existing structured logger, component name TBD by build phase (consistent with how `internal/state` logs are tagged). Build phase picks the exact component string.

**Resolution**: Pending
**Notes**:

---

### 4. Flash chrome line layout when no flash is active

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Session-killed-externally bail path > Inline flash — feature-local infrastructure > Shape > Render*

**Details**:
Cycle 1 pinned that the flash sits between the filter input and the Sessions list. The remaining gap is whether the flash row occupies space when no flash is active:

- **Always-reserved row** — the list y-position is constant; an empty/blank chrome row sits between filter and list even when no flash is active. Visually stable but adds a permanent gap.
- **Collapsed when empty** — the row is rendered only when flash is active; first bail pushes the list down by one row, tick expiry pops it back up.

Both are reasonable. The user's prior feedback on anchoring to visual outcomes suggests pinning the answer rather than leaving to build phase. Minor severity — purely visual.

The natural answer is "collapsed when empty" (no permanent gap for a rare edge case) but the spec should say so.

**Proposed Addition**:
Sub-bullet in *Render*: the flash row is rendered only when a flash is active. When no flash is active, no row is reserved between filter input and list — the list sits directly under the filter input as today. First bail visually pushes the list down by one row; tick expiry / next keystroke pops it back up.

**Resolution**: Pending
**Notes**:

---

### 5. Flash text wording — fixed or build-phase

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Session-killed-externally bail path > Inline flash — feature-local infrastructure*

**Details**:
The chrome line ("Discoverability" section) explicitly pins token wording: "Exact token placement and wording is fixed by this spec." The flash text, by contrast, is given as `e.g.`:

```
session "<name>" no longer exists
```

The example is qualified with "e.g." — suggesting wording is open. But the chrome-line precedent treats user-visible strings as spec-pinned. Without a pin:

- An implementer might write "session killed" or "session <name> not found" — semantically equivalent but inconsistent with the spec's tone.
- Future translation / re-wording invites repeat debate.

Minor severity. Either decision is fine; the spec should pick one.

**Proposed Addition**:
Either: (a) drop the `e.g.` and state "exact wording is fixed by this spec: `session \"<name>\" no longer exists`"; or (b) state "exact wording is a build-phase decision; the spec-level constraint is that the message names the killed session and conveys it no longer exists." Recommended: (a), matching the chrome-line precedent.

**Resolution**: Pending
**Notes**:

---

### 6. Viewport interception of Enter

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Enter binding behaviour*, *Keymap expansion policy > Owned preview keys*

**Details**:
The overview observes today "Enter falls through to the embedded viewport as a no-op". With the new `tea.KeyEnter` case added to preview's `Update`, the spec implies Enter is intercepted entirely — but does not state whether the handler:

- Returns early (Enter never reaches the viewport's Update), or
- Both fires the attach sequence AND forwards the key to the viewport.

`bubbles/viewport` treats Enter as a no-op today, so the observable difference is zero either way. But the *Keymap expansion policy* lists "viewport-native scroll keys (passed through to `bubbles/viewport`)" as a distinct category from `Enter` (listed separately as "commit attach"). The natural inference is Enter is intercepted, not forwarded. Spec should make this explicit.

Minor severity — observable behaviour is identical given viewport's current Enter handling; matters only if a future `bubbles/viewport` version assigns Enter a behaviour.

**Proposed Addition**:
One-line clarification in *Enter binding behaviour* (or in *Owned preview keys*): `Enter` is intercepted by preview's Update handler and is NOT forwarded to the embedded viewport. Future `bubbles/viewport` Enter behaviour cannot leak through.

**Resolution**: Pending
**Notes**:

---
