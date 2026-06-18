# Plan: Spectrum TUI Design

## Phases

### Phase 1: Verification harness + colour-token/canvas/detection foundation (lock-in gate)
status: approved
approved_at: 2026-06-18

**Goal**: Stand up the `vhs` capture harness and build the cross-cutting Modern Vivid foundation — the ~20 §2.9 role tokens (light + dark variants), the owned mode-matched canvas paint, explicit light/dark detection (OSC 11 + `appearance` pref + detect-or-timeout first-paint gate), contrast-floor adherence against the exact canvas, and `NO_COLOR` handling — then lock the direction at the in-terminal validation gate before any broad rollout.

**Why this order**: This is the foundation the rest builds on (§14.5): every later surface references the tokens, renders on the canvas, and is verified through the harness. The colour direction is a hypothesis until prototyped in a real terminal (§16.5 / §1), so the lock-in/bail gate must sit here — before broad reskin investment. The harness (§15.2) is a one-time prerequisite every later task depends on.

**Acceptance**:
- [ ] `vhs` is installed and verified (`vhs --version`); the harness dir (e.g. `testdata/vhs/`) holds runnable tapes plus committed Paper reference PNG exports (§15.5), and at least one tape captures a foundation screen deterministically from seeded fixture state
- [ ] The §2.9 closed vocabulary (~20 named role tokens) exists with light + dark variants; no literal hex survives at the call sites the foundation centralises
- [ ] The owned canvas paints every cell — leaf `.Background(canvas)` plus the outer full-terminal fill (§1) — without perturbing the one-row-per-delegate pagination invariant
- [ ] Light/dark detection via OSC 11 (`tea.RequestBackgroundColor`), the `appearance: auto|light|dark` pref in `prefs.json`, and the detect-or-timeout first-paint gate land the correct canvas from frame one with no canvas flip; fallback resolves to dark
- [ ] `NO_COLOR` skips detection, suppresses the canvas, and renders colourless on the terminal's native fg/bg legibly (the documented carve-out, §2.5)
- [ ] Every foreground token, per-element tint/band, and foreground-on-tint pairing clears the contrast floor against its exact canvas (dark on `#0b0c14`, light on `#e1e2e7`, resolved independently); the in-terminal validation/lock-in gate is passed — light surface tints (`bg.selection`, `bg.warning`, `bg.track`, light borders) pinned and eyeballed against `#e1e2e7` — or a bail outcome is recorded (§16.5)

#### Tasks
status: draft

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| spectrum-tui-design-1-1 | `vhs` capture harness: install/verify, committed tapes dir, fixture-seeded foundation capture, Paper reference PNG export pipeline | vhs not installed / non-Homebrew install path, deterministic fixture-seeded state, agent/user-judged compare (not pixel-diff) |
| spectrum-tui-design-1-2 | Upgrade to Bubble Tea v2 / Lipgloss v2 (OSC 11 API + AdaptiveColor removal) with full parity | existing `AdaptiveColor` (`previewBorderColor`) migrated unchanged, all TUI tests stay green, no render/key behaviour drift |
| spectrum-tui-design-1-3 | MV role-token colour layer — closed ~20-token vocabulary with pinned dark variants, centralising scattered dark literals | `state.green` live/positive-only, `state.red` destructive-only, no raw hex at centralised call sites, `text.faint` decorative-only |
| spectrum-tui-design-1-4 | Light token variants + independent contrast-floor numeric verification | variants resolve independently, `text.dim` 3:1 floor, `text.faint` exempt, text-carrying tints co-tuned with on-band text token (pair clears) |
| spectrum-tui-design-1-5 | `appearance: auto\|light\|dark` pref in prefs.json (default `auto`, tolerant decode) | missing/unrecognised/corrupt/empty → `auto`, no `session_list_mode` regression, prefs stays a leaf (no log import) |
| spectrum-tui-design-1-6 | Owned mode-matched canvas paint — leaf `.Background(canvas)` + outer full-terminal fill as last layer | fill outside list height budget, re-pads to `termH` on vertical change, no edge bleed / empty rows painted, zero-size fallback, pagination invariant preserved |
| spectrum-tui-design-1-7 | Light/dark detection (OSC 11) + `appearance` override + detect-or-timeout first-paint gate (dark fallback) | never paint-then-flip, no-answer/timeout → dark, `light`/`dark` pin skips detection+wait, mis-detection cosmetic-not-broken, `COLORFGBG` weak hint only |
| spectrum-tui-design-1-8 | `NO_COLOR` carve-out — skip detection, suppress canvas, colourless native fg/bg path | skips detection + first-paint wait, no canvas painted, state via glyph + bold/dim, legible-by-construction on terminal defaults |
| spectrum-tui-design-1-9 | In-terminal contrast-floor validation & lock-in/bail gate — pin + eyeball light surface tints against `#e1e2e7` | light-tint-on-light-canvas recurring failure (numeric insufficient), text-on-tint pairs verified vs tint, remedy = more contrast never lower floor, bail is legitimate |

### Phase 2: Shared chrome + Sessions surfaces (flat, grouped, filtering)
status: approved
approved_at: 2026-06-18

**Goal**: Restyle the shared chrome (header/wordmark + violet caret + subtitle + 2px rule, section header + count, condensed footer + `? help`, centred pagination dots) and the Sessions page across all three views (flat, by Project, by Tag) plus two-mode filtering — all onto the Phase 1 foundation, preserving behaviour.

**Why this order**: Sessions Flat is the default view and the baseline every other view derives from (§4); the shared chrome (§3) wraps every page, so it must exist before Projects and modals consume it. Depends only on Phase 1 (tokens, canvas, harness).

**Acceptance**:
- [ ] Shared chrome — header block, section header + count, condensed footer (Sessions core keys + right-aligned `? help`), and centred pagination dots — renders in tokens and matches `Sessions — Modern Vivid v2` (dark) and `Sessions — Modern Vivid (Light)` for layout/structure/colour-role
- [ ] Sessions flat, by-Project, and by-Tag views render the MV treatment (violet left-bar selection, fixed trailing slots, indented grouped rows, dimmed `heading ··· N`); grouping machinery (`HeaderItem`, cursor-skip, Pattern A/B, catch-alls, directory resolution, mode persistence) stays behaviourally identical
- [ ] Two-mode filtering renders the `accent.orange` query (input-active vs list-active vs no-matches), correct contextual footers, and flatten-on-filter; the underlying `bubbles/list` filter engine is unchanged
- [ ] Each Sessions/grouped/filtering surface has its `vhs` capture produced and checked against its named Paper frame (§15.1) for layout/structure/colour-role
- [ ] Behaviour parity verified against the pre-reskin implementation for every touched render path (read it, trace paths, diff logic — provably cosmetic)

### Phase 3: Projects page + modal layer (kill · rename · delete · two-mode edit · ? help)
status: approved
approved_at: 2026-06-18

**Goal**: Restyle the Projects page, introduce the shared blank-screen modal layer, parity-restyle the kill / rename / delete-project modals, build the new two-mode immediate-persist edit-project modal, and add the new per-page `?` help modal driven by a per-page keymap descriptor — applying the keymap revision (§12.2).

**Why this order**: Modals depend on the shared chrome (Phase 2) and the owned canvas (blank-screen clears to canvas, §8.1/§13.5); the `?` help modal is generated from the per-page keymap descriptor that also drives the footers introduced in Phase 2 (single source of truth, §8.5/§14.4). Projects reuses the same shared chrome. Depends on Phases 1–2.

**Acceptance**:
- [ ] Projects page (two-line rows, full-height `accent.violet` left bar over `bg.selection`, `state.green` header + count, `/ to filter` hint) matches `Projects (MV)`; project CRUD behaviour stays identical
- [ ] The blank-screen modal layer clears the page behind to the owned canvas and centres the panel (border-defined, no fill); kill / rename / delete-project are restyled with confirm/input logic preserved (parity) and `n` dropped so cancel is `Esc`-only
- [ ] The two-mode (navigate/edit) immediate-persist edit-project modal is implemented per §8.2 — chips grammar, `+ add` slot, falling-out rules (empty=delete, empty-name reverts, duplicate no-op), contextual footers — as a deliberate behaviour change (parity does not apply)
- [ ] `?` is bound on every page and opens a per-page help modal generated from the keymap descriptor (the single source driving footer + help); the keymap revision is applied (arrows-only nav, `k`=kill, `x` toggles Sessions⟷Projects, `s` Sessions-only, no uppercase, `?` newly bound)
- [ ] Each modal/Projects surface `vhs` capture is checked against its named Paper frame; the `NO_COLOR` modal blank-screen clears to native bg
- [ ] Behaviour parity verified for Projects and every restyled (non-edit) modal render path

### Phase 4: Preview chrome + edge/UX states
status: approved
approved_at: 2026-06-18

**Goal**: Restyle the read-only Preview overlay's chrome to the `accent.cyan` peek-mode treatment, and restyle every edge/UX state — empty sessions/projects, inline flash (warning + success), no-tags signpost, command-pending banner — under the shared left-bar notice convention and single-slot rule.

**Why this order**: Preview and the edge states reuse the foundation (canvas, tokens), the shared chrome, and the band/notice conventions established in earlier phases; grouping them completes the broad visual reskin before the isolated startup flip. Depends on Phases 1–3.

**Acceptance**:
- [ ] The Preview overlay renders `accent.cyan` chrome (`⊙ preview` top bar + cyan content frame) with the captured ANSI content left untouched (the documented palette exception); scroll/nav, pane/window keys, attach, and `?`-overlay-without-blanking behaviour are unchanged
- [ ] Empty sessions / empty projects, inline flash (`⚠` warning on `bg.warning`, `✓` success in `state.green`), no-tags signpost (violet left-bar), and command-pending banner (violet left-bar + orange command chip) match their named Paper frames (§11)
- [ ] The left-bar notice convention, placement (under the title separator, above the section header), single-slot rule, flash↔persistent-band interaction, and the flash-driven viewport-height recompute (F10) are all honoured
- [ ] Each surface `vhs` capture is checked against its named Paper frame; under `NO_COLOR` notice bands drop tint/bar colour while keeping the `▌` bar, glyph, and message state
- [ ] Behaviour parity verified for the preview chrome and every edge-state render path

### Phase 5: Cold-path startup flip (concurrent bootstrap + honest loading screen)
status: approved
approved_at: 2026-06-18

**Goal**: Restructure cold-boot so the 11-step bootstrap runs concurrently with the TUI on the cold + TUI path, streaming live per-step progress to an honest determinate loading screen (thick violet bar + ticking step-list), with the cold-path error/warning contract — while the warm path keeps today's synchronous fast path untouched.

**Why this order**: This is the single biggest engineering item and is explicitly its own phase/PR (§10 / §14.7) — plumbing isolated from the visual reskin, carrying genuine startup-path risk and prior-incident history. It is gated behind the in-terminal validation already passed in Phase 1, and the loading screen reuses the Phase 1 canvas/tokens. Sequencing it last contains the race surface and keeps the broad reskin shippable independently. Depends on Phase 1 (canvas + detect-or-timeout gate).

**Acceptance**:
- [ ] On the cold + TUI path (scoped via `isTUIPath`), Bubble Tea launches immediately on the loading page; the orchestrator runs in a goroutine streaming a `tea.Msg` per real bootstrap step and per restored session over a progress channel carrying `serverStarted` + progress, transitioning to Sessions on complete
- [ ] The loading screen matches `Loading 6 — Combined (thick bar)`: `PORTAL ▌`, thick `accent.violet` bar on `bg.track`, real ticking step-list (`✓`/`◐`/`·`); the 11 real steps map to the 5 friendly labels; only `Restoring sessions` carries the `N/M` counter; empty restore (M=0) ticks `✓` immediately without stalling
- [ ] The warm path (`serverStarted=false`) keeps today's synchronous fast path with no loading screen and zero new risk
- [ ] A fatal cold-boot step failure produces an in-TUI error state on the loading page (`state.red` marker + one-line message) and quits with a non-zero exit; soft warnings ride the progress channel and surface as a post-load notice
- [ ] The first real paint gates on detect-or-timeout so the correct canvas shows from frame one; the restore/daemon race is reviewed against the live event loop; integration tests are updated for the new startup ordering
- [ ] The loading-screen `vhs` capture is checked against its Paper frame; the loading-page error frame is mocked at implementation (§10.5)
