# Spectrum-Inspired TUI Design

## Idea

Redesign Portal's TUI with a ZX Spectrum aesthetic — bold colours on black, block characters, retro typography, and personality. Inspired by games like Manic Miner and Dizzy. The goal is a tool that's fun to look at, not just functional.

## Inspiration

- **ZX Spectrum loading screen** — the iconic cyan/red cycling border bars, `load ""` prompt
- **Manic Miner** — information-dense status bar at the bottom (room name, air gauge, scores), bright colours on black, double-line borders
- **Dizzy series** — bold saturated colour blocks against black, charming personality, Codemasters title screens

The common thread: **black canvas, saturated primaries, chunky block graphics, dense-but-readable info bars, and warmth/charm**.

## Design Elements Explored

### Block-Character Logo

PORTAL spelled out in Unicode block characters, each letter a different rainbow colour:

```
█▀█ █▀█ █▀█ ▀█▀ █▀█ █
█▀▀ █ █ █▀▄  █  █▀█ █
▀   ▀▀▀ ▀ ▀  ▀  ▀ ▀ ▀▀▀
```

Or the larger ASCII art variant:

```
██████╗  ██████╗ ██████╗ ████████╗ █████╗ ██╗
██╔══██╗██╔═══██╗██╔══██╗╚══██╔══╝██╔══██╗██║
██████╔╝██║   ██║██████╔╝   ██║   ███████║██║
██╔═══╝ ██║   ██║██╔══██╗   ██║   ██╔══██║██║
██║     ╚██████╔╝██║  ██║   ██║   ██║  ██║███████╗
╚═╝      ╚═════╝ ╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝╚══════╝
```

### Rainbow Separator Lines

Horizontal bars of rainbow-coloured blocks spanning the full width. Could be thin (`▃▃▃▃`) or chunky (`▄▄▄▄`). Used to frame content top and bottom.

```
▐█▌█▌█▌█▌  P O R T A L                        ▐█▌█▌█▌█▌
```

Small rainbow blocks bookending the header is another option — brand mark at the edges.

### Block Cursor

Instead of `>` for selection, a coloured block `▌` that could cycle through rainbow colours as you navigate up/down. Each item you land on gets a different accent colour.

### Spaced Uppercase Headers

`S E S S I O N S` / `P R O J E C T S` — the retro 8-bit typography feel from spaced-out capitals.

### Manic Miner Status Bar

Dense, colourful bottom bar inspired by Manic Miner's HUD:

```
██████████████████░░░░░░  3 sessions
High Score  042          Score  003
```

The "air gauge" as a session count or progress indicator. "High Score" / "Score" as a playful data display. Tongue-in-cheek but functional.

### Heavy/Double Borders

ZX-style framing around the entire TUI:

```
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃                                               ┃
┃  content                                      ┃
┃                                               ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
```

### Animated Loading Border

The Spectrum loading screen had cycling coloured bars around the border. Could do this for the boot interstitial in BubbleTea via tick-based animation. Probably too much for a 2-5 second screen, but noted as an option.

## Mockups

### Sessions View

```
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓

   █▀█ █▀█ █▀█ ▀█▀ █▀█ █
   █▀▀ █ █ █▀▄  █  █▀█ █
   ▀   ▀▀▀ ▀ ▀  ▀  ▀ ▀ ▀▀▀

   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

   ▌ portal              3 windows        ● attached
     dotfiles            1 window
     blog                2 windows



   ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
   Sessions              ↑↓ nav  ⏎ attach  k kill  p proj
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
```

### Projects View

Same frame and language, swapping content and help bar keys.

### Loading Interstitial

```
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃                                                      ┃
┃                                                      ┃
┃   █▀█ █▀█ █▀█ ▀█▀ █▀█ █                             ┃
┃   █▀▀ █ █ █▀▄  █  █▀█ █                             ┃
┃   ▀   ▀▀▀ ▀ ▀  ▀  ▀ ▀ ▀▀▀                          ┃
┃                                                      ┃
┃         Starting tmux server...                      ┃
┃                                                      ┃
┃   ██████████░░░░░░░░░░░░░░░░░░░░                     ┃
┃                                                      ┃
┃                                                      ┃
┃   ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄   ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
```

Progress bar fills with rainbow-coloured blocks left to right.

### Modal

```
       ┌──────────────────────────────────┐
       │ ▐█▌  Kill session?               │
       │                                  │
       │      portal  (3 windows)         │
       │                                  │
       │         y confirm   n cancel     │
       └──────────────────────────────────┘
```

Rainbow block accent on modals.

## Emerging Design System

| Element | Treatment |
|---|---|
| Background | Black — the ZX canvas |
| Logo | Block-character PORTAL, each letter a different rainbow colour |
| Separators | Rainbow gradient lines across full width |
| Cursor | Coloured block `▌`, cycles colour per item |
| Borders | Heavy/double — ZX framing |
| Status bar | Manic Miner style, bottom of screen, info-dense, colourful |
| Headers | Spaced uppercase — 8-bit typography feel |
| Modals | Small rainbow block accent |

## Status

No decisions made. This is a design exploration that came out of discussing the auto-start-tmux-server loading interstitial. Would be a separate feature/initiative from the bootstrap work.

## Implementation Notes

- Lipgloss supports all of this — terminal colours, block characters, borders
- BubbleTea tick system could handle any animation (progress bars, cycling colours)
- Current TUI has no branding/logo — this would be a significant visual identity shift
- Would need to work across terminal themes (light/dark) — black background assumption may need to be validated
