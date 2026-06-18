---
status: complete
created: 2026-06-18
cycle: 2
phase: Traceability Review
topic: Spectrum TUI Design
---

# Review Tracking: Spectrum TUI Design - Traceability

## Result: CLEAN

Cycle 2 re-ran both directions against the specification read in full, with fresh
context, and confirmed the cycle-1 fixes are present, correct, and introduced no
new gaps or drift. **No findings.** The plan is a faithful, complete translation
of the specification.

## Scope boundary confirmed (§16)

- **In scope (§16.1):** full MV reskin across every surface (Sessions flat/by-project/by-tag,
  Projects, Preview, Loading, all modals incl. two-mode edit/kill/rename/`?` help, two-mode
  filtering, every edge state); the cold-path startup flip (§10); the owned mode-matched canvas
  with explicit light/dark detection (OSC 11) + the `appearance` pref + `NO_COLOR` suppression.
  All present in the plan (Phases 1–5).
- **Deferred (§16.3):** user-overridable theme system + transparency opt-out; v2 tag features
  (per-session `@portal-tags`/`--tag=`, live-grouped filtering, tag exclusion). Confirmed ABSENT
  from the plan as work — not referenced even as commentary.
- **Cut (§16.4):** animated cycling-colour border. Confirmed ABSENT.

## Cycle-1 fixes — re-verified present and correct

- **(a) §12.3 caveat → task 1-9.** Acceptance criterion present (phase-1-tasks.md, the final
  `**(§12.3 validation caveat)**` bullet): confirm `Ctrl+↑`/`Ctrl+↓` are delivered (not swallowed
  by terminal/tmux passthrough) during the in-terminal validation pass, with a fallback page key
  flagged for the descriptor/keymap consumers (2-1 / 3-3 / 4-7). Correctly scoped to the §15
  in-terminal gate (1-9's domain). Records the *obligation to check + have a fallback* (spec
  content), not a pre-chosen key (an implementation detail the spec leaves open). No drift.
- **(b) §15.6 light-mode eyeball → tasks 3-4, 3-6, 3-9.** Each carries a `LIGHT-MODE EYEBALL (§15.6)`
  acceptance criterion: 3-4 (`?` help — two-column glyph/label + header wiring in light), 3-6
  (rename — panel/input/`was:` line in light), 3-9 (all three edit states — chip tint/outline/fill,
  violet field label, `+ add` faint slot, `◉ EDIT MODE` in light). Each correctly states "no
  further Paper mock required per §15.6 … not a frame compare" — faithful to §15.6's eyeball-not-mock
  rule. Kill (3-5) is correctly excluded from this residual set (it has a `Kill Confirm Modal (Light)`
  Paper frame and does a dark+light frame compare). No drift. The chain task 1-9 set up (foundation
  Sessions light wiring confirmed in 1-9; per-modal light eyeball delegated "to those surfaces in
  later phases") now lands concretely on each surface.
- **(integrity-cycle-1 carryovers, re-checked for traceability impact):** task 5-7 pins the post-load
  warning notice to the orange/warning role + transient/auto-clear lifetime — traces to §10.5
  ("soft warnings … post-load notice") + §11.2 (the `accent.orange` transient flash lifecycle), with
  a flagged deviation escape hatch; task 5-5's criterion+test pin the loading step-list to "exactly
  the five §10.4 friendly labels in order" — traces to §10.4 verbatim. Both spec-grounded, no
  invented content.

## Direction 1 (Spec → Plan, completeness)

Every specification element has plan coverage with implementer-sufficient depth. Section walk:

- **Verification mandate** — embedded per-UI-task (vhs capture + named Paper frame + behaviour
  parity); non-visual/plumbing tasks explicitly `vhs`-exempt (1-2, 1-3, 1-4, 1-5, 2-1, 3-3, 3-8,
  5-1..5-4, 5-6 (mock-not-frame), 5-7, 5-8). Never a standalone verification task/phase.
- **§1** (canvas ownership two-layer, reskin-not-rebuild, nothing-sacred, bail gate) → 1-3/1-4
  (tokens), 1-6 (two-layer canvas), 1-8 (NO_COLOR carve-out), 1-9 (anti-sunk-cost bail).
- **§2.1–§2.9** → 1-3 (dark tokens, roles, design-to-roles, tokenise-now), 1-4 (light variants +
  numeric floor + co-tuned tint/text pairs), 1-9 (light surface tints pinned+eyeballed), 1-7
  (OSC 11 detection / appearance override / detect-or-timeout / dark fallback / COLORFGBG weak
  hint / per-launch), 1-5 (appearance pref tolerant decode), 1-8 (NO_COLOR skip-detect + suppress
  canvas + glyph state), 1-2 (truecolor/256/16 downsample audit under v2). §2.7 narrow/per-dimension
  degrade → 2-2/2-3/2-4/2-6/5-5.
- **§3.1–§3.6** → 2-2 (header/wordmark/caret/subtitle/2px rule), 2-3 (section header+count+`/ to
  filter`), 3-2 (Projects header+count), 2-6/3-2 (violet left-bar selection), 2-4/3-2 (condensed
  footer + `? help`), 2-5 (centred pagination dots), 1-6/2-5/3-1 (no full-screen frame).
- **§4.1–§4.3** → 2-6 (row anatomy: name flex / fixed trailing slots / attached-on-selection
  keeps green / truncation / one-line invariant), 2-3 (section header), 2-6 (selection+nav).
- **§5.1–§5.5** → 2-7 (heading `text.detail` + `··· N` `text.dim`, indent levels, cursor-skip
  preserved, Pattern A/B, catch-alls, dir-resolution, mode persistence preserved, no `lipgloss/tree`).
- **§7–§7.3** → 2-8 (input-active / list-active, orange query, two footers, boundary, flatten-on-
  filter, no match-count), 2-9 (no-matches centred state, distinct from empty-sessions).
- **§8.1–§8.6** → 3-1 (blank-screen shared layer + §14.6 adapt-vs-rework + key-exclusive + NO_COLOR),
  3-8/3-9 (two-mode immediate-persist edit: state machine + render/chips/footers, behaviour-change
  not parity), 3-5 (kill destructive + drop `n`), 3-6 (rename), 3-4 (help new + descriptor-driven +
  header `esc close` exception), 3-7 (delete-project mirror + record-only consequence + drop `n`).
- **§9.1–§9.3** → 4-6 (cyan peek-mode chrome, content untouched, full-screen overlay not modal,
  width-cascade tiers, NO_COLOR), 4-7 (Preview `?` overlay-without-blanking + Preview descriptor).
- **§10.1–§10.5** → 5-1 (cold-vs-warm + TUI-vs-CLI gate), 5-2 (progress channel + goroutine), 5-3
  (restore N/M callback), 5-4 (11→5 step mapping incl. M=0 + resume-commands zero-item), 5-5 (honest
  loading render + five-labels-in-order + detect-or-timeout + LoadingMinDuration), 5-6 (fatal error
  state + tea.Quit non-zero exit + error frame mocked), 5-7 (soft warnings → post-load notice), 5-8
  (restore/daemon race review + startup-ordering tests).
- **§11 intro / §11.1–§11.4** → 4-1 (notice-band primitive + single-slot arbiter + F10 recompute),
  4-2 (inline flash warning+success), 4-3 (no-tags signpost), 4-4 (command-pending banner), 4-5
  (empty sessions/projects + replaced footer).
- **§12.1–§12.3** → 2-1 (Sessions keymap descriptor + revision), 3-3 (Projects descriptor + drop `s`
  alias), 4-7 (Preview descriptor), 1-9 (§12.3 swallow-check caveat).
- **§13.1–§13.6** → 3-6/3-8/3-9 (focus-vs-edit grammar), structural page model honoured throughout
  (Sessions one-page-three-views, Projects separate page, Preview overlay, Loading startup), 3-4/4-7
  (per-page `?` help), 2-3/2-8 (filter hint + `s switch view` footer-only), 3-1 (modals on blank
  screen), 2-3/3-2 (counts beside labels, same size dim-by-colour).
- **§14** (feasibility — sizing context, not scope) → 14.6 open question routed to 3-1; 14.4 descriptor
  to 2-1/3-3/3-4; cross-cutting foundation to Phase 1.
- **§15.1–§15.6** → 1-1 (harness + tapes + Paper reference export pipeline + agent-judged compare),
  per-task vhs+frame compare embedded; 15.6 light eyeball → 1-9 (foundation) + 3-4/3-6/3-9 (residual
  modals). Full frame map honoured, incl. both Light frames, the delete-modal "mirror Kill (MV)" and
  Preview-help "not separately mocked" cases.
- **§16** scope boundary — in-scope all present; deferred/cut all absent (see Scope boundary above).

Depth spot-checks: tasks carry the concrete spec strings (footer copy, consequence lines, signpost
wording, the five loading labels, the three edit-mode footers), the pinned §2.9 hexes (dark in 1-3,
light in 1-4/1-9), and the exact behaviour-preservation constraints (grouping machinery, filter
engine, CRUD, restore loop, daemon ordering) — an implementer would not need to return to the spec.

## Direction 2 (Plan → Spec, fidelity / anti-hallucination)

Clean. Every task's Problem / Solution / implementation detail / acceptance criteria / tests / edge
cases traces to a specific spec section. No content lacks a spec basis. Specific anti-hallucination
checks:

- **Bubble Tea v2 / Lipgloss v2 upgrade (1-2)** — grounded in §2.6 (`tea.RequestBackgroundColor` →
  `BackgroundColorMsg` "in Bubble Tea v2") and §14.5 ("Lipgloss v2 removed `AdaptiveColor`, so the
  light/dark choice is wired explicitly"). Carries the spec-faithful ambiguity escape hatch (collapse
  to "no upgrade" only if a v1 OSC 11 path suffices) — not hallucinated.
- **Inside-tmux `(current: %s)` decoration (2-3)** — an existing-behaviour parity-preserve of
  `sessionListTitleForMode`, explicitly flagged in-task as "the one documented spec divergence the
  function already carries." Legitimate reskin parity, not invented requirement.
- **Descriptor `RightAligned` field (2-1)** — traces to §3.4's right-aligned `? help` placement.
- **"No `bg.surface` token (there is none)" / transparent-fill inputs (3-1/3-6/3-9)** — verbatim §8.1
  ("border-defined panel — no distinct fill … not a `bg.surface` token (there is none)").
- **`accent.orange` command chip (4-4)** — verbatim §11.4 ("the command in an `accent.orange` chip"),
  the explicit per-banner exception to the general "chips are `text.primary` on a tint" rule.
- **§14.6 adapt-vs-rework default ("adapt if small") (3-1)** — §14.6 leaves it open; the task records
  the decision against the code and defaults conservatively — not over-reach.
- **Phase-5 plumbing specifics** (progress channel, goroutine, N/M callback, fatal-as-tea.Quit,
  post-load notice) — each traces to §10.2/§10.4/§10.5 named mechanisms.

No deferred (§16.3) or cut (§16.4) item appears as plan work or commentary. No acceptance criterion
tests a behaviour the spec does not require. No edge case is invented beyond the spec's stated set
(the few ambiguity notes — e.g. cross-project alias collision, Tab-in-edit-mode in 3-8 — are
explicitly flagged as ambiguities with conservative defaults and "do not invent silently" markers,
which is the correct handling, not hallucination).

## Findings

None.
