# Specification: Spectrum TUI Design

## Specification

## 1. Overview & Design Direction

### Goal
Portal's TUI is functional but personality-free. This redesign gives it a colourful, characterful visual identity that makes Portal nicer and more exciting to use **without overriding the user's terminal preferences**. The shipping bar is concrete: the result must be a genuine improvement over today's UI — both *objectively* (clears the contrast floor, §2) and on the user's *subjective* read. Bailing is a legitimate outcome if no direction clears that bar; this is an explicit anti-sunk-cost gate.

### Locked direction — Modern Vivid
The visual language is **Modern Vivid (MV)**: a restrained, modern palette (violet / cyan / green accents, plus an orange filter accent) with light retro touches grafted on (wordmark + caret + separator rule). It descends from a "ZX Spectrum" inspiration but is explicitly **not** a literal Spectrum reproduction. Two signature Spectrum motifs are **out**:

- **No forced black canvas** — Portal does not paint its own background (see canvas ownership below).
- **No rainbow / multi-hue spectrum motif** — the multi-hue rainbow is firmly excluded (unwanted pride-flag association). Colour is still used heavily; it is just never a rainbow.

Spectrum is loose inspiration only. The redesign keeps the structural/typographic ideas (wordmark, separators, spaced headers, chunky selector, honest loading screen) — which are theme-agnostic — and drops the literal colour scheme.

### Canvas ownership — respect the terminal background
Portal renders **foreground-only on the user's existing terminal background**, using adaptive (light/dark) accent colours so the redesign works on any terminal theme. Per-element backgrounds (selected-row tint, status strips, modal panels) are permitted — that is focus styling, not canvas ownership — but each must clear the contrast floor (§2) on **both** light and dark backgrounds. Portal never fills the full alt-screen with a bespoke colour.

### Nothing is sacred
The current UI carries no special claim. Today's pink cursor (`212`), green=attached (`76`), grey detail text (`#777777`), and blue preview border may all be replaced wholesale. The redesign may restructure colour, layout, and UI — and, where justified, UX — but only where "the juice is worth the squeeze." Code may change in service of good UI; gratuitous restructure is avoided. Every design decision is validated against how Portal actually works before being adopted.

---

## Working Notes

[Optional - capture in-progress discussion if needed]
