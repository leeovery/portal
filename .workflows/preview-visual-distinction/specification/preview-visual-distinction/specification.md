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

## Chrome line content

The chrome line is the metadata strip that rides on the frame's top edge. It is *dynamic-only* — it describes what changes as the user navigates within preview, not what was established by opening preview. Session name is **not surfaced**; identity is anchored by the Sessions-page selection that triggered preview-open.

### Segments (left to right)

1. **Window indicator** — `Window M of N`
2. **Pane indicator** — `Pane X of Y`
3. **Window name** — `win: {name}`
4. **Keymap** — see *Keymap glyphs* below

Segments 1–3 are joined by `·` (middle dot, U+00B7) with spaces on either side. The keymap is separated from the preceding segments by whitespace padding so it visually right-aligns within the available chrome budget at wide widths and compresses toward the centre at narrow widths.

### Keymap glyphs

Verbose word tokens (`tab`, `enter`, `esc`) are replaced with macOS-convention keyboard glyphs. The bracket keys (`]` / `[`) stay as ASCII because they are literally the characters the user presses — no glyph is more accurate.

| Key   | Glyph | Codepoint |
|-------|-------|-----------|
| `]`   | `]`   | ASCII     |
| `[`   | `[`   | ASCII     |
| Tab   | `⇥`   | U+21E5    |
| Enter | `⏎`   | U+23CE    |
| Esc   | `⎋`   | U+238B    |

### Verbose form (default at typical widths)

```
] next win · [ prev win · ⇥ next pane · ⏎ attach · ⎋ back
```

### Compact form (cascade tier 3)

```
] [ ⇥ ⏎ ⎋
```

Compact uses **single-space separators** (no interpunct) — the entire point of tier 3 is character compression, and replacing the 4 separators saves 8 cells. Display-cell width of the compact form is **9 cells**.

**Token order matches across forms** — `] [ tab enter esc` left-to-right in both — so a user resizing the terminal sees the same sequence of keys, just with action labels added or removed.

### Constants

The two forms are baked into `internal/tui/pagepreview.go` as exported-or-package-level constants:

```go
const (
    verboseKeymap = "] next win · [ prev win · ⇥ next pane · ⏎ attach · ⎋ back"
    compactKeymap = "] [ ⇥ ⏎ ⎋"
)
```

Tests assert against these exact bytes.

### Font fallback

`⇥` and `⏎` are present in essentially every modern monospace font. `⎋` (U+238B) is the weakest link — present in SF Mono, Menlo, JetBrains Mono, Fira Code, Cascadia, and most modern terminal-targeted fonts. A user on an old terminal with a font lacking that codepoint sees a fallback box glyph. Acceptable degradation: bracket keys still render, the frame still delivers the "this is preview" signal, and the keys still work.

### Scope note on touching the verbose form

Replacing the word tokens with glyphs modifies what `preview-keymap-discoverability` and `enter-attaches-from-preview` shipped. Those prior specs remain accurate as records of what *they* shipped at the time; this feature's spec captures the new glyph form as its own decision.

## Width cascade

Terminal widths and window names vary unboundedly; chrome content that overflows the available top-edge budget would either clip the right corner or wrap to a second visual row, breaking the entire frame (the bottom corner would shift down by one row). The cascade is the mechanism that guarantees the top edge is always exactly one row, at any width ≥ 2.

### Algorithm shape: predicate-over-output

The four-tier cascade is **not** a stack of incremental transformations. Each tier produces a *candidate output*, measures it via `lipgloss.Width`, and returns the candidate if it fits. Otherwise it falls through to the next tier.

```
tier 1: compose with name truncated to fit + verbose keymap   → measure → if fits, return
tier 2: compose with name segment dropped  + verbose keymap   → measure → if fits, return
tier 3: compose with name segment dropped  + compact keymap   → measure → if fits, return
tier 4: corners + filler `─` (no chrome content)              → always fits any width ≥ 2 → return
```

Tier interactions:

- Tiers 1 and 2 are mutually exclusive — if tier 1's truncated name fits, tier 2 isn't reached; if tier 2 drops the segment, tier 1's work is discarded.
- Tiers 2 and 3 differ only in keymap form — tier 3 strictly compresses tier 2 further by swapping the verbose keymap for the compact one.
- Tier 4 supersedes whatever was attempted before.

### Tier-by-tier behaviour

**Tier 1 — truncate window name with `…` suffix.** When the budget for the window name segment is positive but smaller than the full name, the name is truncated to fit and a `…` (U+2026, 1 cell wide) suffix appended.

**Tier 2 — drop the `· win: {name}` segment entirely.** Reached when the budget for the window name segment falls below a sensible minimum (**target: ~8 display cells**). Below that minimum the truncation reads as garbage rather than as a recognisable name. The whole `· win: {name}` interpunct-prefixed segment is removed, not just the name string.

**Tier 3 — swap verbose keymap for compact form.** Reached when even with the window name segment dropped the verbose keymap still overflows. Saves ≈73 cells (verbose form is 82 cells, compact is 9). Action labels are not permanently lost from the product — once the user has seen the verbose chrome at wider widths, the keys-only form reads as a recognised compression rather than a fresh-eyes puzzle.

**Tier 4 — drop chrome entirely; render corners + filler.** A degenerate-terminal fallback (sub-40-col terminals, almost no real user terminal hits this). The top edge becomes `╭{─ × (width − 2)}╮`. Always fits any width ≥ 2.

### Load-bearing tier 4

Tier 4 is **load-bearing** — it is what guarantees the top edge always renders cleanly down to width 2 (the two corner glyphs). Without it, terminal widths narrow enough to fail tier 3 would either clip the chrome or wrap it to a second visual row, and wrapping in particular breaks the frame because the bottom corner shifts down by one row, destroying the visual integrity the cascade exists to protect. Tier 4 is rarely reached in practice, but its existence is what lets the cascade make a strong guarantee.

### Defending against pathological window names

A side benefit of the cascade: pathological window names — long file paths surfaced by vim, e.g. — no longer break rendering regardless of terminal width. The truncation-then-drop path applies the same regardless of whether the budget pressure came from the terminal being narrow or the name being long.

### Pure function

The cascade is implemented as a pure function:

```go
func composeChromeLine(width int, /* model fields */) string
```

Located in `internal/tui/pagepreview.go`. No I/O. Returns the unstyled top-edge chrome content as a single-line string. Width measurements use `lipgloss.Width`. Tested exhaustively at the cascade thresholds with table-driven cases.

## Display-cell-aware truncation

Tier 1 of the cascade ("truncate window name with `…` suffix") and the ~8-cell minimum in tier 2 are specified in **display cells**, not bytes or runes. Window names are arbitrary UTF-8 — tmux allows CJK, emoji, combining marks, and double-width glyphs. Naïve byte-slicing (`s[:n]`) can land mid-rune and produce invalid UTF-8 in the top border. Naïve rune-counting overcounts: a string of CJK glyphs is 1 rune per 2 cells, so "n runes" can be 2× the visual budget.

### Algorithm

Iterate codepoint by codepoint, accumulating `runewidth.RuneWidth(r)` (or equivalently `lipgloss.Width` of single-rune strings — `lipgloss` uses `go-runewidth` underneath). Stop when adding the next rune would exceed `budget − 1` (reserving 1 cell for the `…` suffix). Append `…` (1 cell wide).

### Where it applies

The display-cell primitive is the **same primitive** used wherever the cascade truncates anything — currently the window name, but the rule is the unit of measure, not the field. Any future truncation in this codepath uses the same primitive.

### Test coverage

Table-driven with at least these glyph classes:

- ASCII (1 cell per rune)
- CJK (2 cells per rune)
- Emoji (2 cells per rune, including ZWJ sequences)
- Combining marks (0 cells per combining rune; base+combiner = base's width)

Asserts:

- No mid-rune cuts (output is valid UTF-8).
- Final display-cell width ≤ budget.
- `…` is appended only when truncation actually occurred (full-string-fits case returns the original string).

---

## Working Notes
