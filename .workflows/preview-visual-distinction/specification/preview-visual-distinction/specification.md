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

---

## Working Notes
