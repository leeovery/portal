# Investigation: Esc After Preview Hides Session List

## Symptoms

### Problem Description

**Expected behavior:**
After pressing `Esc` to dismiss the scrollback preview opened from a filtered sessions list, the TUI returns to the previously filtered session list — the committed filter text remains in effect and the matching rows are still visible.

**Actual behavior:**
The first `Esc` after returning from the preview puts the TUI into a hidden / empty-looking state (session list disappears). A second `Esc` is required to recover, and the committed filter is silently discarded — the reappearing list is unfiltered.

### Manifestation

- After `Esc` from preview on the filter→commit→preview path:
  - Session list visually vanishes / appears empty
  - Filter text committed earlier is gone
  - A second `Esc` brings the list back, but unfiltered
- Path is specific: filter (`/`) → type → `Enter` to commit → `Space` to preview → `Esc`
- Without the filter step (preview an unfiltered list and press `Esc`), preview dismisses straight back to the session list normally.

### Reproduction Steps

1. Launch the TUI: `portal open` (or `x`) so the Sessions page is showing.
2. Press `/` to enter filter mode.
3. Type characters until the list narrows to an available session.
4. Press `Enter` to commit the filter (filter input exits; matching session row highlighted).
5. Press `Space` — scrollback preview opens for the highlighted session.
6. Press `Esc` — **bug**: session list disappears entirely; committed filter is also gone.
7. Press `Esc` a second time — session list reappears, unfiltered.

**Reproducibility:** Always, on this specific keystroke path.

### Environment

- **Affected environments:** Local (TUI; not environment-specific)
- **Browser/platform:** N/A (terminal TUI)
- **User conditions:** Any sessions present; must be in `portal open`/`x` TUI

### Impact

- **Severity:** Low (UX friction)
- **Scope:** All TUI users who use filter + preview together
- **Business impact:** UX-only — no tmux state affected, nothing destroyed; user must re-type filter

### References

- Inbox: `.workflows/.inbox/.archived/bugs/2026-05-19--esc-after-preview-hides-session-list.md`
- Suspect area: `internal/tui/` — `pagePreview → pageSessions` dismiss handler and sessions-page Esc handling

---

## Analysis

### Initial Hypotheses

- Esc on `pagePreview` likely both dismisses the preview AND is then re-delivered to a sessions-page handler that interprets it as "clear filter / hide list."
- Alternatively, the dismiss transition may reset filter state but not re-apply the filter to the visible list, leaving the model in a state where the list renders empty until another event nudges it.

### Code Trace

_To be filled during code analysis._

### Root Cause

_To be filled during synthesis._

### Contributing Factors

_To be filled._

### Why It Wasn't Caught

_To be filled._

### Blast Radius

_To be filled._

---

## Fix Direction

_To be filled after findings review._

---

## Notes

- Related recently-completed work: `session-scrollback-preview`, `enter-attaches-from-preview`, `preview-visual-distinction`, `preview-keymap-discoverability`, `space-dismisses-preview` — preview pathway has had multiple iterations; this bug may be a regression introduced by one of them.
