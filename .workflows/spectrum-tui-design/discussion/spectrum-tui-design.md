# Discussion: ZX Spectrum-Inspired TUI Design

## Context

Portal's TUI is currently functional but personality-free. The proposal is a
single coherent retro visual pass giving it a ZX Spectrum aesthetic: bold
saturated rainbow primaries on a black canvas, a block-character `PORTAL` logo
(each letter a different colour), rainbow gradient separator lines, a coloured
block cursor (`▌`) that cycles colours on navigation, spaced uppercase 8-bit
headers (`S E S S I O N S`), heavy/double ZX-style borders framing the TUI, a
Manic Miner-inspired status bar, a small rainbow block accent on modals, and a
loading interstitial with the logo and a rainbow-block progress bar.

Discovery settled this as **one feature shipped as a unit** — the constituent
pieces all serve the one identity and do not split into independent
deliverables.

**Current state (baseline):**
- No visual identity. Pink cursor (`lipgloss.Color("212")`), grey detail text
  (`#777777`), green "attached" marker (`76`).
- Rounded borders used only on the modal (`modal.go`) and the scrollback
  preview chrome (`pagepreview.go`, adaptive blue `#3B5577`/`#7B95BD`).
- Loading page is a plain centered string `"Restoring sessions…"`
  (`viewLoading`), subject to `LoadingMinDuration = 1.2s`.
- Session-list title is plain text with mode suffixes (`Sessions` / `Sessions —
  by project` / `Sessions — by tag`) via `sessionListTitleForMode`.
- Footer is a manually-rendered three-column keymap (`renderKeymapFooter`); the
  bubbles/list built-in help renderer is disabled.
- Grouping renders real `HeaderItem` rows interleaved into the `bubbles/list`
  delegate — every row is exactly one delegate line (load-bearing for
  pagination; the grouped-viewport-overflow incident is documented in CLAUDE.md).

### References

- Seed: `seeds/2026-03-19-spectrum-tui-design.md` (inbox:idea)
- Discovery: `discovery/session-001.md`
- Stack: Bubble Tea (TUI) + Lipgloss (styling) — both support terminal colours,
  block characters, borders, and tick-based animation.

## Discussion Map

### States

- **pending** (`○`) — identified but not yet explored
- **exploring** (`◐`) — actively being discussed
- **converging** (`→`) — narrowing toward a decision
- **decided** (`✓`) — decision reached with rationale documented

### Map

  Discussion Map — ZX Spectrum TUI (11 subtopics, all pending)

  ├─ ○ Black-canvas assumption & terminal theming [pending]
  ├─ ○ Colour palette & accessibility [pending]
  ├─ ○ PORTAL block-character logo [pending]
  ├─ ○ Borders & whole-TUI framing [pending]
  ├─ ○ Spaced uppercase headers [pending]
  ├─ ○ Cycling block cursor [pending]
  ├─ ○ Manic Miner status bar [pending]
  ├─ ○ Loading interstitial [pending]
  ├─ ○ Modal rainbow accent [pending]
  ├─ ○ Animation infrastructure & performance [pending]
  └─ ○ Scope boundary (v1 vs deferred) [pending]

---

*Subtopics are documented below as they reach `decided` or accumulate enough
exploration to capture.*

---

## Summary

### Key Insights

*(to be populated as the discussion progresses)*

### Open Threads

- Black-background assumption across terminal themes flagged in discovery as the
  primary validation question.
- Animated cycling-colour border noted as possible-but-likely-overkill.

### Current State

- Nothing decided yet — discussion just initialized.

## Triage

(none)
