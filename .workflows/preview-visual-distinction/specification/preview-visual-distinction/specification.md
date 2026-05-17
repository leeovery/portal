# Specification: Preview Visual Distinction

## Specification

## Overview

When the quick preview opens (Space on a session in the TUI) the scrollback body fills the screen and reads identically to a fully-attached session. There is no visual signal that the surface is read-only and transient. This feature adds a paint-by-Portal visual frame around the preview body so the page is unmistakably identifiable as preview, independent of what the session's own scrollback bytes are doing.

### Goal

A user pressing `Space` on a session lands on a page that is *obviously* a preview at a glance — not at second glance, not after reading the chrome line. Distinction is content-independent: it holds when the scrollback is plain prose, a `bat`-rendered file, a vim session, or a colourful prompt.

### Non-goals

- **Dimming the preview body.** Rejected during discussion — the failure mode is content-dependent (works on plain prompts, breaks on coloured scrollback, which is precisely the case preview is most useful for).
- **A separate "preview unavailable" surface for degenerate terminal sizes.** Vertical degradation is not handled; the frame renders whatever falls out at very small heights.
- **Context-aware styling** (different render inside-tmux vs bare shell). A single unified treatment applies in both contexts.
- **Surfacing the session name in chrome.** Identity is anchored by the Sessions-page selection that triggered preview; chrome is dynamic-only.
- **Touching `pageSessions` rendering, bootstrap warning flush, or any other page's view.** The frame lives only in `pagePreview`'s `View()`.

### Boundary with prior work

This builds on three already-shipped pieces:

- `session-scrollback-preview` (feature) — the `pagePreview` arm of the TUI page state machine, with a `bubbles/viewport` rendering raw scrollback bytes.
- `preview-keymap-discoverability` (quick-fix) — added action labels to the chrome keymap tokens and the `win:` prefix on the window-name segment.
- `enter-attaches-from-preview` (feature) — added the `Enter` binding and `enter attach` chrome token; established the dismiss-refresh path from preview back to Sessions.

Those specs are frozen historical records of what they shipped. This feature *replaces* the verbose keymap token strings they introduced (the new glyph form is captured here as its own decision); they are not retroactively edited.

## Visual treatment

The preview body is wrapped in a visible frame painted by Portal. The body's rendering (raw ANSI scrollback bytes via the embedded `bubbles/viewport`) is **not touched** — distinction comes from the enclosure, not from modifying what the session emitted.

### Frame structure

The frame consists of four edges around the viewport:

- **Top edge** — manually composed (`╭─{chrome content}{filler}─╮`). Carries the chrome line as part of the top border row. See *Top edge composition*.
- **Left edge / right edge / bottom edge** — rendered by `lipgloss` using its `RoundedBorder()` preset with `BorderForeground` set to the design colour.

The chrome line (window/pane indicators + keymap) lives **inside the top border row**, not above it. The frame surrounds the viewport directly; there is no chrome row above the frame.

### Border style

`lipgloss.RoundedBorder()` — matching the existing modal precedent at `internal/tui/modal.go:24`. Portal's implicit rule is "rounded border = contextual surface, no border = main page"; preview is a contextual surface and fits that rule. Geometry differentiates preview from modals — modals are small centred overlays, preview is a full-width framed page — so identical border characters cause no visual confusion.

The manually-composed top edge **must source its corner and edge characters from the chosen `lipgloss` border value** rather than hardcoding them, so a future style switch is a single-point edit.

### Border colour

`lipgloss.AdaptiveColor{Light: "#3B5577", Dark: "#7B95BD"}` — a single unified colour across inside-tmux and bare-shell contexts. Applied to all four edges (the three `lipgloss`-rendered edges plus the hand-composed top edge's border parts).

Both variants sit at mid-luminance with recognisable blue saturation. The light variant is dark enough to be visible against pale terminal backgrounds; the dark variant is light enough to be visible against dark backgrounds. Neither competes with Portal's existing accents (pink-magenta cursor `ANSI 212`, green attached badge `ANSI 76`) — different hue families. This introduces a third accent colour to Portal's palette, owned by preview chrome.

### Colour robustness

The frame's **enclosure is the load-bearing distinction signal**. The blue tint is enhancement, and is allowed to degrade:

- **`NO_COLOR=1`** — `lipgloss`/`termenv` respects the convention and renders the border in default foreground. Blue is dropped; the frame remains visible. Distinction signal is preserved.
- **8/16-colour terminals or `TERM=dumb`** — `lipgloss`/`termenv` automatically downgrades the hex tones to the nearest palette colour. Design intent is approximated, not lost.
- **Truecolor terminals** — rendered as specified.

No explicit Portal handling is required beyond what `lipgloss`/`termenv` already provides. The hex values are not hard requirements at the implementation layer — they are the design target.

---

## Working Notes
