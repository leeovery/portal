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

## 2. Colour System & Terminal Robustness

### 2.1 Design to roles, not fixed hex
Every renderer references a small fixed set of **semantic role tokens**, never scattered literal hex. The redesign is built on this role-token colour layer; concrete values are pinned in §2.9 (MV token table).

Roles:
- **primary accent** — cursor / selection / active title / header caret. *(MV: violet.)*
- **detail** — paths / secondary text / dim metadata / group counts. *(MV: dim grey.)*
- **state — attached** — the live/attached marker. *(MV: green; reserved for the attached state only — never reused for chips or decoration.)*
- **state — destructive** — kill/delete confirmation emphasis. *(MV: red; reserved for destructive actions only.)*
- **state — warning / transient** — inline flash / warnings. *(MV: amber.)*
- **filter / search** — the live filter query text. *(MV: bright orange.)*
- **preview mode-chrome** — the read-only preview frame, deliberately distinct from the primary violet to signal "peek mode." *(MV: cyan.)*

Each role has **light and dark variants** (`AdaptiveColor`). A direction instantiates the roles; the roles are the stable interface the rest of the spec refers to.

### 2.2 State is never carried by hue alone
Every state is conveyed by **glyph + colour**, never colour alone: `●` attached, `▌` selector bar, `✕` removable/destructive, spaced uppercase headers, `⚠` warning. Colour, where available, only *reinforces* the glyph. This makes the monochrome / `NO_COLOR` path work for free and protects colour-blind users. (Consistent with the prior `preview-visual-distinction` rule: don't rely on colour alone.)

### 2.3 Contrast floor (hard gate)
Every direction must clear a contrast gate **before taste is judged**. Functional foreground — session names, paths, footer, status text — must meet **WCAG AA**:
- **4.5:1** for normal text,
- **3:1** for large/bold text and UI accents (cursor, border, selection highlight),

measured against **both** a canonical light background (≈ white) and a canonical dark background (≈ black). With `AdaptiveColor` this means the **light variant tested on white** AND the **dark variant tested on black**, each ≥ its ratio. Purely decorative glyphs (the wordmark) are exempt from the text ratio but must stay visible. Arbitrary mid-tone custom backgrounds are out of scope for the hard gate — we target the standard light/dark cases `AdaptiveColor` flips between. A direction that can't hit the floor on both extremes is **disqualified** before looks are judged.

### 2.4 Colour-capability ladder (truecolor / 256 / 16)
Portal **imposes its own exact hues via truecolor `AdaptiveColor`** — it does **not** inherit the terminal's 16 ANSI colours. Rationale: a recognisable identity needs consistent hues across machines; inheriting the user's palette means no identity and possible clashes. Respecting the *background* (§1) plus honouring `NO_COLOR` (§2.5) already covers "don't fight the user"; imposing *hues* does not conflict with that distinction. Lipgloss/termenv **auto-downsamples** to 256/16 on weaker terminals — accepted as graceful degradation (a hue may approximate, but the contrast floor still governs legibility). Matches existing repo practice (`previewBorderColor`).

### 2.5 NO_COLOR / monochrome
Portal **honours `NO_COLOR`** and monochrome terminals: it renders colourless, leaning on the glyph-backed state (§2.2) plus bold/dim attributes. Because state is never colour-only, the UI stays fully usable without colour.

### 2.6 AdaptiveColor binary classification — best-effort + override
`AdaptiveColor` makes a **binary** light/dark choice from terminal-background detection; the real world is continuous. Two accepted-best-effort risks:
- **Mid-tone backgrounds** (e.g. Solarized base03, Gruvbox soft-dark — mainstream, not exotic) get classified to an extreme they aren't on, so a variant tuned for near-white/near-black may dip below the floor on their actual bg.
- **Detection failure** (no OSC response over SSH / tmux passthrough; `COLORFGBG` unset) → termenv defaults (often dark), so a *light*-terminal user can be served the *dark* variant — a cross-pairing the floor never tests. Acute because Portal runs **inside tmux**, where bg-detection passthrough is unreliable.

**Decision:** choose token variants that also hold up on mid-tone backgrounds **where possible**; the eventual manual theme / light-dark override (the deferred user-theme initiative, §16) is the ultimate escape hatch; exotic backgrounds and detection-miss cross-pairing are **accepted as best-effort**, mitigated by the override.

### 2.7 Narrow / short terminal — degrade, never break
Define a **minimum supported terminal size**; below it the UI **degrades** rather than breaks: drop the wordmark → compact wordmark, drop the right-side header hint, truncate names with `…`, and let height drive pagination. It must **never overflow** — the one-row-per-delegate pagination invariant always holds (every list row, header or session, is exactly one delegate line). Exact thresholds are pinned as an implementation detail.

### 2.8 Theming — tokenise now, user-override deferred
- **In scope:** structure the role-token colour layer as a single named built-in theme ("Modern Vivid"). Every renderer references **tokens**, not scattered hex. This locks layout and delegates colour, making the app theme-*ready* at near-zero extra cost.
- **Deferred to its own initiative:** a **user-overridable theme system** (external theme file e.g. `~/.config/portal/theme.{json,toml}`, merge-over-default, validation when a user picks unreadable colours — contrast floor becomes advisory + warn/clamp — multiple built-in themes, a `theme` setting, docs). Bigger surface; ships independently after the redesign. (See §16.)

---

## Working Notes

[Optional - capture in-progress discussion if needed]
