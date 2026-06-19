# Phase 1 lock-in / bail gate — light surface tints (task 1-9)

Spec refs: §1 (anti-sunk-cost gate), §2.3 (contrast floor), §2.9 (light surface
tints finalised at §15; two-knob co-tune; remedy = more contrast, never lower the
floor), §4.1 (foreground-on-tint pairings), §15.6 (light per-token eyeball), §16.5
(lock-in gate, bail legitimate).

This file is the committed lock-in/bail record for the four light surface tints.
It carries the four pinned hexes + each derivation + the numeric ratios. **The
lock-in/bail DECISION is the human's in-terminal eyeball** — the `DECISION`
section at the bottom is left PENDING for the human to fill.

## What to eyeball (artifacts)

- Swatch source: `internal/capture/swatch.go` (the `contrast-validation` fixture
  — a standalone `tea.Model`, NOT the production Sessions surface; the four tint
  *surfaces* are built in later phases — selection row → Phase 2, separator/footer
  borders → Phase 2, warning band → Phase 4, loading track → Phase 5).
- Captures (regenerate with `vhs <tape>`, sandbox disabled):
  - `testdata/vhs/contrast-validation-light.png` ← `contrast-validation-light.tape`
    (light tints against `#e1e2e7` — the wash-out risk)
  - `testdata/vhs/contrast-validation-dark.png`  ← `contrast-validation-dark.tape`
- Numeric floor gate: `internal/tui/theme/contrast_test.go`
  (`TestForegroundOnTintPairings`, `TestStateGreenOnSelectionRemedy` — the 1-9
  wash-out remedy gate, `TestLightSurfaceTintsPinned`,
  `TestLightTintFillsArePerceptible`, `TestBgWarningPairRule`, `TestBgTrackPairRule`).

Captures are byte-deterministic (two runs byte-identical). The existing Sessions /
nocolor captures are unchanged (`sessions-flat.png` `26cc4992…`, `sessions-flat-light.png`
`5374e479…`, `sessions-flat-nocolor.png` `feafa6fb…` all byte-identical) — the
on-selection green remedy below is **swatch-only**; the global `state.green` token
is untouched, so the canvas attached marker is unchanged.

## Pinned light hexes + derivations

Each tint is **derived from its dark anchor + the surface it renders on** (§2.9),
not invented. "Held" means the value derived at task 1-4 cleared its floor and read
as a distinct surface, so no more-contrast remedy was needed at the 1-9 gate.

| Token | Light (pinned) | Dark anchor | Derivation |
|---|---|---|---|
| `bg.selection` | `#D0C6F0` | `#28243a` | dark violet anchor lifted onto light canvas `#e1e2e7`; confirmed (was already the §2.9 light value) |
| `bg.warning` | `#E8D6A8` | `#241B10` | dark amber anchor lifted onto light canvas `#e1e2e7`; 1-4 derived value held |
| `bg.track` | `#D2D4DE` | `#26283A` | dark grey anchor lifted onto light canvas `#e1e2e7`; 1-4 derived value held |
| `border.separator` / `border.footer` | `#C9CDDB` (shared) | `#292E42` / `#20232E` | dark rules lifted onto light canvas `#e1e2e7`; confirmed |

No remedy was applied to any tint — every 1-4 derived value cleared the numeric
floor and reads as a distinct surface. (Had one dipped, the remedy would have been
*more contrast* — darken/saturate the light tint and/or move the on-band text
token, co-tuned so both clear — never a lowered floor.)

## Numeric ratios

### Tint fills vs their own canvas (perceptible ≥ 1.1; the §2.2 accent bar carries the 3:1 UI distinction, not the fill)

| Tint | Light vs `#e1e2e7` | Dark vs `#0b0c14` |
|---|---|---|
| `bg.selection` | 1.25 | 1.30 |
| `bg.warning` | 1.11 | 1.15 |
| `bg.track` | 1.14 | 1.34 |
| `border.separator` | 1.23 | 1.45 |
| `border.footer` | 1.23 (shared `#C9CDDB`) | 1.25 |

### §4.1 foreground-on-tint pairings (measured against the tint, not the canvas)

| Pairing | Light | Dark | Floor | Pass |
|---|---|---|---|---|
| `text.on-selection` on `bg.selection` | 10.50 | 14.96 | 4.5 (normal text) | ✅ |
| `text.strong` on `bg.selection` | 5.71 | 7.09 | 4.5 (normal text) | ✅ |
| `state.green-on-selection` `● attached` on `bg.selection` (the 1-9 remedy override) | **4.65** | 8.19 | **4.5 (normal text)** | ✅ |
| `text.on-warning` on `bg.warning` | 5.14 | 10.73 | 4.5 (normal text) | ✅ |

> The `● attached` marker row now uses the dedicated darker `state.green-on-selection`
> override (light `#3B5E18`, dark = global `state.green` dark `#9ECE6A`), not the
> global `state.green`. See the finding section below for the wash-out + remedy
> rationale. The global `state.green` (`#456E1C` light) measured **3.72** here — the
> washed-out value — and is **unchanged** (its canvas usages stay crisp).

### Accent bars vs canvas (§2.2 selector/warning bar — the 3:1 UI distinction)

| Bar | Light | Dark | Floor | Pass |
|---|---|---|---|---|
| `accent.violet` (selection `▌`) | 4.37 | 8.43 | 3.0 | ✅ |
| `accent.orange` (warning `⚠`) | 4.53 | 9.59 | 3.0 | ✅ |

## ⚠ Finding + remedy — the green-on-bg.selection pairing (light)

**1-9 human-eyeball finding (recorded):** the human eyeballed the light `● attached`
marker on the `bg.selection` band and found the global `state.green` (`#456E1C`) on
`bg.selection` (`#D0C6F0`) — measured **3.72** — **genuinely washed out**. (3.72
clears the 3:1 glyph+label UI bar but not 4.5; the wash-out is the human eyeball the
numeric floor alone cannot catch, §2.9 / §15.6.)

**Remedy applied (§2.8 defaulted override; MORE contrast, never lower the floor):**
a dedicated darker green — `theme.MV.StateGreenOnSelection` — used **ONLY** for the
`● attached` marker when it renders on the `bg.selection` tint:

- **Light variant `#3B5E18`** — the minimal HSL-darkening of `state.green` light
  `#456E1C` (hue preserved at H=90, the same yellow-green; G channel still dominates
  so it reads positive/green) that clears 4.5:1 with margin: **measured 4.65 vs
  `#D0C6F0`** (≥ 4.5 + 0.1).
- **Dark variant = the global `state.green` dark `#9ECE6A`** — on dark `bg.selection`
  `#28243a` it already clears comfortably (**8.19**), so **no dark override was
  needed**; the remedy is **light-only**. (Documented why: the dark pairing was never
  washed out.)
- **The global `state.green` token is UNCHANGED** (light `#456E1C` / dark `#9ECE6A`).
  Its canvas usages (Sessions count 4.64 light / clearing dark, Projects label, `✓`
  done-tick, success flash) stay crisp, and the foundation Sessions captures stay
  byte-identical. Only the on-selection attached marker uses the darker green.

Derivation comment (mirrored in `theme.go` + `contrast_test.go`):
`#3B5E18` — measured **4.65 vs `#D0C6F0`** — remedy for the 1-9 human-eyeball
wash-out finding; §2.8 defaulted override; global `state.green` unchanged.

**Phase 2 task 771c41 (Sessions flat row anatomy + violet selection) MUST use this
`StateGreenOnSelection` green for the attached marker on the SELECTED row** so the
real surface inherits the remedy (flagged in `theme.go` + `swatch.go`).

**Human: re-eyeball the light `● attached` marker on the `bg.selection` band in the
regenerated `contrast-validation-light.png`.** It now renders in the darker
`#3B5E18` (4.65). Confirm the wash-out is resolved and it still reads as positive
green; if not, record a further remedy/bail signal below.

## Ctrl+↑ / Ctrl+↓ paging-chord finding (tick-6b0f62 note / §12.3)

During the in-terminal pass, confirm `Ctrl+↑` / `Ctrl+↓` (the paging chords bound
in task 2-1) are actually delivered to Portal and not swallowed by the terminal or
tmux (notably tmux passthrough). If either chord is intercepted, record the finding
and choose a fallback page key, and flag that fallback for tasks 2-1 / 3-3 / 4-7 to
adopt.

> NOTE: the `contrast-validation` swatch binds no paging chords (it is a static
> swatch), so this chord check is a **separate** in-terminal step the human runs
> against a chord-bearing surface.

**DEFERRED to task 2-1.** The `Ctrl+↑` / `Ctrl+↓` paging keys are *bound* in task
2-1; this swatch binds no chords, so the chord-delivery check cannot be run at the
1-9 gate. Verify in-terminal once 2-1's paging lands: confirm each chord reaches
Portal (not swallowed by terminal / tmux passthrough), and if either is intercepted
choose a fallback page key and flag it for tasks 2-1 / 3-3 / 4-7 to adopt. Record
the result against task 2-1.

**Raw-terminal delivery verified at the 1-9 gate (2026-06-19, Lees-MacBook-Pro, via `command cat -v`):**
- Plain `↑` → `^[[A`
- **`Ctrl+↑` → `^[[1;5A`** — distinct, bindable (the `;5` is the Ctrl modifier). DELIVERED.
- **`Ctrl+↓` → `^[[1;5B`** — distinct, bindable. DELIVERED.

**Ctrl+↑ delivered (not swallowed):** YES — `^[[1;5A` arrives distinct from plain `↑`.
**Ctrl+↓ delivered (not swallowed):** YES — `^[[1;5B` arrives distinct from plain `↓`.
**Fallback page key:** NOT NEEDED — both chords are deliverable as bindable CSI sequences, so task 2-1 binds `Ctrl+↑`/`Ctrl+↓` directly.
**Residual check for 2-1:** the raw terminal delivers the chords; re-confirm they survive **tmux passthrough** when Portal runs *inside* a tmux session (the in-terminal step at 2-1, once the keys are bound). If tmux intercepts either, fall back then and flag for 3-3 / 4-7.

---

## DECISION: LOCK-IN ✅

> Eyeballed in a real terminal in both modes; the colour foundation clears the
> anti-sunk-cost bar (§16.5 / §1). Phase 2 proceeds to build the chrome on top.

- **Outcome (LOCK-IN | BAIL):** **LOCK-IN.**
- **Final pinned hexes:** `bg.selection #D0C6F0`, `bg.warning #E8D6A8`,
  `bg.track #D2D4DE`, borders `#C9CDDB` (separator/footer shared) — each eyeballed
  against `#e1e2e7` and confirmed to read as a distinct surface (not a wash-out),
  in both modes. Every foreground-on-tint pairing reads legibly on its tint.
- **On-selection green remedy (1-9 wash-out finding):** the human eyeballed the
  light `state.green ● attached` on `bg.selection` as washed out at 3.72:1. Remedied
  per the more-contrast rule with a dedicated `state.green-on-selection` token
  (light `#3B5E18`, 4.65:1 vs `#D0C6F0`; dark = global `#9ECE6A`, light-only) — the
  global `state.green` is UNCHANGED (canvas usages stay crisp; foundation captures
  byte-identical). Re-eyeballed: **reads well, wash-out resolved.** Phase 2 task
  771c41 MUST use `state.green-on-selection` for the attached marker on the selected
  row.
- **Ctrl+↑/↓ chord finding:** **deliverable** — `Ctrl+↑`→`^[[1;5A`, `Ctrl+↓`→`^[[1;5B`,
  both distinct/bindable (verified at the gate, see above). Task 2-1 binds them
  directly; no fallback needed. Re-confirm tmux-passthrough at 2-1.
- **Eyeballed by / date:** Lee Overy / 2026-06-19.
