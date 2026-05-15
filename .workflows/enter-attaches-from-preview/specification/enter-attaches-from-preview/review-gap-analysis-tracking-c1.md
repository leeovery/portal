---
status: in-progress
created: 2026-05-15
cycle: 1
phase: Gap Analysis
topic: enter-attaches-from-preview
---

# Review Tracking: enter-attaches-from-preview - Gap Analysis

## Findings

### 1. Flash render position relative to existing Sessions-page chrome

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Session-killed-externally bail path > Inline flash — feature-local infrastructure*

**Details**:
The spec says the flash renders as "a single chrome line rendered above the Sessions list". The Sessions page already carries chrome — at minimum a filter input row. The spec does not pin whether the flash line sits:

- above the filter input,
- between the filter input and the list,
- or replaces/overlays an existing chrome row.

An implementer would have to make a layout call that affects visual outcome. Given the user's prior feedback on anchoring to visual outcomes, this seems worth pinning explicitly rather than leaving to "build phase". Minor severity — there is a most-natural answer (between filter and list, so the flash sits adjacent to the list it is describing) but the spec should say it.

**Proposed Addition**:
Update the *Inline flash — feature-local infrastructure > Shape > Render* bullet to: "a single chrome line rendered between the filter input and the Sessions list. The flash sits adjacent to the list it is describing — the filter input remains in its existing position above the flash, and no existing chrome row is replaced or overlaid."

**Resolution**: Approved
**Notes**: Flash sits between filter input and list — visually adjacent to the list it describes.

---

### 2. Flash replacement semantics on rapid successive bails

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Session-killed-externally bail path > Inline flash — feature-local infrastructure*

**Details**:
If a user bails (sees flash for session A), opens preview on session B, and B is also killed externally before they press Enter, a second bail fires while the first flash's tick may still be pending. The spec defines clear conditions (next actionable KeyMsg, tick expiry) but does not address:

- Does the second bail replace the first flash's text and reset the tick?
- Does the pending tick from flash 1 fire and clear flash 2 early?

The natural answer is "latest bail wins, tick resets" — anything else would feel wrong (the visible message must match the most recent bail). But because the spec describes "state: an active flash text string and an associated timestamp or tick handle" without specifying replacement, an implementer could leak the old tick and prematurely clear the new flash. Minor severity — easy to spec, easy to miss in implementation.

**Proposed Addition**:
[Leave blank until discussed]

**Resolution**: Pending
**Notes**:

---

### 3. Captured window/pane index — tmux index vs array position

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Pre-select + attach sequence > Steps 2 and 3*, *Out of scope > Captured coordinate provenance*

**Details**:
Steps 2 and 3 issue `tmux select-window -t <session>:<window_index>` and `tmux select-pane -t <session>:<window_index>.<pane_index>`. Tmux's `-t` target syntax uses tmux's own window/pane *index* (which can be non-contiguous and is not a 0-based array position), not the position of the entry in `ListWindowsAndPanesInSession`'s returned slice.

The spec defers "captured coordinate provenance" (which struct field backs the captured values) to build phase. That deferral is fine, but the spec does not state the contract that the captured value MUST be the tmux index (the value returned by `list-windows -F '#{window_index}'` / `list-panes -F '#{pane_index}'`), not the slice position. If the implementer interprets `]`/`[`/`Tab` as moving a cursor through slice positions and stores the position, the pre-select calls would target the wrong window/pane (or fail) on any session with non-contiguous indices.

The prior spec's enumeration probably already uses tmux indices, but this spec should pin the contract explicitly so the build phase cannot inadvertently regress it. Minor-to-important severity depending on how the captured field is shaped today.

**Proposed Addition**:
[Leave blank until discussed]

**Resolution**: Pending
**Notes**:

---

### 4. Dispatch ordering of refresh + transition + flash on bail

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Session-killed-externally bail path > Behaviour*

**Details**:
The bail dispatches three things: a `pagePreview → pageSessions` transition, the existing sessions-list refresh on that transition, and the flash emission. The spec describes them as a unit ("dispatches a refresh-and-bail message that ...") but does not pin ordering / atomicity:

- If the refresh is async (a `tea.Cmd` returning a `sessionsLoadedMsg`), the user could observe one render where preview is gone, list is stale (killed session still listed), flash is present — then a second render where list is fresh.
- Alternatively the implementer could buffer the transition until the refresh resolves, but that delays the visible response to Enter.

The spec's intent ("user lands back on Sessions page with the killed session already absent and a single-line message") suggests the killed-session row should not be visibly present alongside the flash. The implementer needs the contract pinned: either "flash render is gated on refresh completion" or "best-effort, transient stale row is acceptable". Minor severity — visible only in a brief render frame.

**Proposed Addition**:
[Leave blank until discussed]

**Resolution**: Pending
**Notes**:

---

### 5. `has-session -t <session>` prefix-matching behaviour

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: *Pre-select + attach sequence > Step 1*

**Details**:
`tmux has-session -t <name>` matches by prefix by default (tmux's standard target resolution). If a session "foo" was killed but "foo-2" still exists, `has-session -t foo` returns zero (matches "foo-2"), step 1 proceeds, then `select-window -t foo:0` fails, then connector targets "foo" which auto-creates outside tmux or errors inside tmux.

The connector path uses the same `-t <name>` semantics, so behaviour is at least consistent — the proactive check and the connector agree. But the spec frames step 1 as "session has been killed externally" detection, and prefix matching can mask exactly that case for users with related session names.

Two possible resolutions:
- Pin `-t=<name>` exact-match syntax (tmux's `=` prefix on the target forces exact match).
- Explicitly accept prefix matching as inherited tmux behaviour and document it as out-of-scope to defend against.

Minor severity — narrow edge case, but worth pinning the answer.

**Proposed Addition**:
[Leave blank until discussed]

**Resolution**: Pending
**Notes**:

---
