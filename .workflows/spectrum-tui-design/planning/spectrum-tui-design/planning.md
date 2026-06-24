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
status: approved
approved_at: 2026-06-18

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

#### Tasks
status: approved
approved_at: 2026-06-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| spectrum-tui-design-2-1 | Sessions keymap descriptor (footer data source) + §12.2 keymap revision (arrows-only, drop `p`, de-overload `s`/`x`, no uppercase) | drop `p` (s/x single-meaning), `s` literal while filter focused (unchanged), filter-mode bindings excluded from core footer, no uppercase aliases, parity of every dispatched action |
| spectrum-tui-design-2-2 | Header block — PORTAL wordmark + violet `▌` caret + right-aligned `session manager` subtitle + full-width 2px separator rule | narrow degrade (drop subtitle → compact wordmark), one-row-per-delegate pagination invariant unperturbed, owned-canvas render without edge bleed |
| spectrum-tui-design-2-3 | Section header + count — `Sessions` accent.cyan + state.green count + mode suffix text.detail + right-aligned `/ to filter` hint | count same cap-height as label (dim not smaller), mode suffix from existing sessionListTitleForMode (parity), narrow degrade drops right hint, inside-tmux `(current: …)` decoration preserved |
| spectrum-tui-design-2-4 | Condensed footer + right-aligned `? help` — single row of core keys from the keymap descriptor (glyphs accent.blue / labels text.detail / `?` accent.violet) | `s switch view`/`x projects` on all session views incl. Flat, paging/`n`/`r`/`k`/`q` excluded (help-only), single-row height-budget recompute, narrow truncation |
| spectrum-tui-design-2-5 | Centred pagination dots — built-in paginator restyled (active accent.violet / inactive text.faint), no full-screen frame | single-page suppresses dots, centred across width, no-full-frame rule, page count unchanged (parity) |
| spectrum-tui-design-2-6 | Sessions Flat row anatomy + violet left-bar selection — name flex / fixed window-count + attached-marker slots, `▌` bar + bg.selection tint + text.on-selection | attached marker keeps state.green on selection (fg-on-tint floor), columns aligned regardless of name length, empty attached slot preserves alignment, over-long name `…` truncation, selected-row count text.strong |
| spectrum-tui-design-2-7 | Sessions grouped reskin — `heading ··· N` (text.detail + text.dim) + indented rows (cursor col 2 / name col 4) for By Project & By Tag, pure Lipgloss | cursor never lands on header (skip preserved), catch-all (Unknown/Untagged) headings same style, flat rows still flush col 2, no lipgloss/tree, pagination exactness preserved, no-tags signpost reskin out of scope (Phase 4) |
| spectrum-tui-design-2-8 | Filtering input-active + list-active reskin — accent.orange `/` query + contextual footers + flatten-on-filter | never both cursor+selection (§7.1), `↵`/`↓` commits input→list (§7.2), `Esc` clears from either mode, flatten-on-filter via empty HeaderItem FilterValue (parity), `s` literal while input-active, no match-count shown |
| spectrum-tui-design-2-9 | Filtering no-matches state — centred `⌀` text.faint + `No sessions match "<query>"` text.primary + widen-search hint text.detail, footer stays input-active | renders only when query matches zero, query interpolated into message, footer stays input-active (not list-active), distinct from empty-sessions state (Phase 4) |

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

#### Tasks
status: approved
approved_at: 2026-06-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| spectrum-tui-design-3-1 | Blank-screen modal layer (shared) — clear page behind an open modal to the owned canvas (mode-matched) + centre the border-defined panel; resolve the §14.6 adapt-vs-rework decision against renderModal/renderListWithModal | adapt-vs-rework decision recorded (§14.6), confirm/input logic of all modals preserved (parity), NO_COLOR clears to native bg (inherited Phase 1 carve-out §2.5), no full-screen frame, centred on canvas not dimmed overlay, one-row-per-delegate pagination unperturbed underneath |
| spectrum-tui-design-3-2 | Projects page reskin (MV) — two-line rows (name text.primary / path text.detail), full-height accent.violet left bar over bg.selection on selection (name text.on-selection / path text.muted-bright), state.green Projects header + text.detail count, condensed footer | full-height bar spans both lines (uniform height keeps pagination exact), CRUD behaviour identical (reskin parity), fg-on-tint floor for selected name/path, empty-projects state out of scope (Phase 4), count same cap-height as label (dim not smaller) |
| spectrum-tui-design-3-3 | Projects keymap descriptor + §12.2 Projects-side keymap revision — add the Projects descriptor to the single-source type from Phase 2 task 2-1; drop the Projects-side `s`→Sessions alias so `x` toggles both directions, no uppercase | drop `s` alias (x single-meaning both directions), Sessions-side x toggle already done in 2-1 (no duplication), descriptor drives footer + help (single source §8.5/§14.4), command-pending keymap unaffected, parity of every dispatched Projects action |
| spectrum-tui-design-3-4 | `?` help modal (new) — new help-modal type + generic two-column renderer (glyph accent.blue / action text.strong) over the per-page descriptor, header `? Keybindings` + right-aligned `esc close` (§8.1 help exception); bind `?` on Sessions & Projects (un-swallow), toggle-close on ?/Esc, key-exclusive | ? un-swallowed (was swallowed so bubbles/list can't self-toggle help), key-exclusive (Esc dismisses, no fall-through to clear-filter/quit), generated from descriptor not hand-authored, lists complete keymap incl. footer keys, Preview descriptor + help-from-Preview wiring deferred/flagged for Phase 4, only Sessions help mocked |
| spectrum-tui-design-3-5 | Kill confirm modal reskin (MV) — state.red header `▲ Kill session?`, name state.red, `· N window(s)` text.detail, consequence line text.detail, footer `y kill · esc cancel`; confirm logic preserved, drop `n`, inherits blank-screen | drop `n` key (cancel Esc-only §8.1), confirm action unchanged (parity), state.red destructive-only, inherits blank-screen + NO_COLOR native bg, both Kill Confirm Modal (MV) + (Light) frames |
| spectrum-tui-design-3-6 | Rename modal reskin (MV) — header `Rename session`, labelled NEW NAME input (focused label accent.violet, value text.primary + violet `▌` cursor), `was: <old>` line text.detail, footer `↵ rename · esc cancel`; rename flow preserved, inherits blank-screen | rename flow unchanged (parity), Enter/Esc keys preserved, focus=outline vs edit=fill grammar (§13.1), inherits blank-screen + NO_COLOR native bg |
| spectrum-tui-design-3-7 | Delete-project confirm modal reskin (MV) — mirrors kill frame: state.red header `▲ Delete project?`, name state.red, path text.detail, distinct record-only consequence line, footer `y delete · esc cancel`; confirm logic preserved, drop `n`, inherits blank-screen | drop `n` key (cancel Esc-only), distinct consequence line vs kill (record-only, sessions/files untouched), mocked mirroring Kill Confirm Modal (MV), confirm action unchanged (parity), inherits blank-screen + NO_COLOR native bg |
| spectrum-tui-design-3-8 | Two-mode edit-project modal — interaction core (⚠ behaviour change) — uniform navigate/edit immediate-persist across NAME/ALIASES/TAGS: Tab/Shift+Tab + ←/→ nav, Enter/e/+ enter edit, Enter commits & persists, Esc backs out one level; falling-out rules | per-item immediate persist (no dirty/save/batch), Esc never discards saved work, Tab into chip field lands on + add slot, + add spawns empty chip in edit mode (landing via Tab/←→ is navigate-only), empty Name reverts to prior, duplicate-on-commit silent dedupe (case-sensitive tags), brand-new empty chip vanishes on Esc, x deletes focused chip immediately, NOT a parity-preserve |
| spectrum-tui-design-3-9 | Two-mode edit-project modal — MV render + chip grammar + contextual footers — three visual states (chips normal/focused/editing §13.1, + add slot text.faint, focused field label accent.violet, ◉ EDIT MODE indicator) + the three per-mode footers; matches the three edit-modal frames | chips one neutral style (text.primary on tint, never green §13.1), focus=outline / editing=fill grammar, per-mode footer matches focus/mode (name / chip / editing), ◉ EDIT MODE only while editing in place (empty otherwise — no standing navigate label), inherits blank-screen, three named frames (navigate name / chip focused / edit in place) |

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

#### Tasks
status: approved
approved_at: 2026-06-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| spectrum-tui-design-4-1 | Notice-band primitive (`▌` left-bar, orange/green/violet role variants, under title separator above section header) + single-slot arbiter (persistent owns slot, transient flash temporarily wins then persistent returns) + band appears/clears viewport-height recompute (F10) | at-most-one band (replace today's independent signpost+flash double-insert), persistent↔transient hand-off, flash auto-clear on next keypress / short timeout (existing flashGen guard preserved), band present↔absent recomputes list height (one-row-per-delegate invariant, no overflow/miscount), NO_COLOR drops tint/bar colour but keeps `▌` bar + position |
| spectrum-tui-design-4-2 | Inline flash band reskin — `accent.orange` left-bar + `⚠` on `bg.warning` tint with `text.on-warning` (warning) + `state.green` + `✓` success variant, routed through single-slot arbiter + F10 recompute | success glyph-distinct from warning (`✓` vs `⚠`, not colour-only §2.2), warning text-on-tint pair clears floor (`text.on-warning` on `bg.warning`), auto-clears on next actionable keypress / timeout (parity with `formatSessionGoneFlash`), takes slot over persistent band then restores it, NO_COLOR keeps `▌` + glyph + bold/dim, list height recomputed on appear/clear |
| spectrum-tui-design-4-3 | "No tags yet" signpost reskin — `accent.violet` left-bar band (`text.strong`) over the flat list, routed through the single-slot arbiter (degrade-with-message) | shows only in By-Tag with zero tags anywhere (parity, grouping machinery untouched), persistent band yields slot to transient flash then returns, renders over flat list (zero pane reads preserved §5.4), NO_COLOR keeps `▌` + position, message wording sourced as a constant |
| spectrum-tui-design-4-4 | Command-pending banner reskin — `accent.violet` left-bar (`Pick a project to run`) + command in `accent.orange` chip, footer → `⏎ run here · n run in cwd · esc cancel`, over full Projects chrome | keeps full Projects chrome (green header + `/ to filter`, not stripped), command joined into orange chip, footer drawn from `commandPendingHelpKeys` (parity of dispatched actions), banner routed through single-slot arbiter as persistent violet band, NO_COLOR keeps `▌` + chip glyph/position |
| spectrum-tui-design-4-5 | Empty sessions + empty projects states reskin — centred `▌ ▌ ▌` text.faint + `No sessions yet` text.primary + hint text.detail, footer REPLACED by `n new in cwd · x projects · / filter · ? help` (from keymap descriptor); empty projects mirrors | renders only when list is empty, footer fully replaced (not a subset) drawn from keymap descriptor §12.1, reuses the 2-9 centred-empty-state pattern (distinct from no-matches), empty projects mirrors with own message/hint (not separately mocked), one-row-per-delegate invariant holds |
| spectrum-tui-design-4-6 | Preview overlay chrome reskin (peek mode) — `accent.cyan` top bar (`⊙ preview` cyan + `<session>` text.primary + `Window x/y · Pane x/y` text.detail + right-aligned nav hints text.detail) + cyan content frame; captured ANSI content left untouched | full-screen overlay NOT a modal (no §8.1 blank-screen), content area real ANSI untouched (only chrome themed §9.2), scroll/pane/window-nav/attach/back parity (`Space`/`Esc`, `]`/`[`, `Tab`, `Enter`), `previewBorderColor` AdaptiveColor → `accent.cyan` token (Phase 1 v2 migration), width-cascade tiers preserved, NO_COLOR colourless chrome on native bg, cyan-on-canvas contrast (§2.9) |
| spectrum-tui-design-4-7 | Preview `?` help wiring (Phase-3 carry) — Preview keymap descriptor (§12.1) added to single-source descriptor type + bind `?` on Preview to overlay the generic help renderer (3-4) WITHOUT blanking the preview | help overlays preview without blanking (distinct from blank-screen modal path), reuses 3-4 generic descriptor-driven renderer (no hand-authored Preview copy), toggle-close on `?`/`Esc` key-exclusive (Esc dismisses help, not fall-through to preview back), descriptor lists Preview's complete keymap §12.1, Preview help not separately mocked (follows audited keymap) |

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

#### Tasks
status: approved
approved_at: 2026-06-18

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| spectrum-tui-design-5-1 | Cold-vs-warm gating — `tmux has-server` decision scoping the concurrent flip to cold + TUI path only (`serverStarted` gates the loading page) | warm path (`serverStarted=false`) keeps today's untouched synchronous fast path (zero new risk), CLI/direct-path (`!isTUIPath`) keeps synchronous bootstrap, `tmux has-server` is the cheap cold/warm decider, no loading page on warm |
| spectrum-tui-design-5-2 | Progress channel + goroutine orchestrator wrapper — run the 11-step bootstrap concurrently on the cold/TUI path, stream a `tea.Msg` per real step, channel carries `serverStarted` + progress | channel carries `serverStarted` + per-step progress (replaces context/package-memo on cold/TUI path), TUI inert during loading contains race surface, msg-per-real-step ordering, package-memo `sync.Once` warm path unaffected, channel drain/close on complete without goroutine leak |
| spectrum-tui-design-5-3 | Restore per-session progress callback (the N/M source) injected at the restore per-session loop (`internal/restore/restore.go`) | callback fires per restored session (live-skips still advance N against M), M=0 fires zero callbacks, callback nil-tolerant for the synchronous warm/CLI path, per-session restore parity unchanged (additive) |
| spectrum-tui-design-5-4 | Step mapping — 11 real bootstrap steps → 5 friendly labels; bar advances on every real step, active label is the current step's group | only `Restoring sessions` carries the `N/M` counter, M=0 suppresses `(N/M)` and ticks ✓ immediately without stalling, `Running resume commands` ticks ✓ with no per-item work, cleanup steps 8–11 fold under final label, bar advances through every real step |
| spectrum-tui-design-5-5 | Honest loading-screen render (VISUAL — `Loading 6 — Combined (thick bar)`) — `PORTAL ▌` over thick `accent.violet` bar on `bg.track` + real ticking step-list (✓/◐/·), consuming the step-mapped stream; carries `vhs` capture + frame compare | first paint gates on Phase 1 task 1-7 detect-or-timeout gate (correct canvas frame one, no flip), real list not in-place text swap, tick glyphs `state.green`/`accent.cyan`/`text.faint` + label tokens, narrow/short degrade, `NO_COLOR` colourless on native bg, `LoadingMinDuration` interaction with honest progress |
| spectrum-tui-design-5-6 | Fatal cold-boot error contract — in-TUI error state on the loading page (`state.red` marker + one-line message) + fatal-error-as-`tea.Quit` with non-zero exit; error frame mocked at implementation | fatal-error-as-`tea.Quit` (was `PersistentPreRunE` error return), `q`/`Esc` → non-zero exit, failed step `state.red` marker + one-line message, no drop into half-restored picker, only the fatal steps abort (best-effort steps warn-and-continue), error frame mocked at implementation (§10.5) |
| spectrum-tui-design-5-7 | Soft warnings ride the progress channel → post-load notice after the picker appears (reworks `bootstrapWarnings` package-sink delivery on the cold/TUI path) | warnings surface only after picker appears (not over loading page / alt-screen), reworks package-memo delivery on cold/TUI path only, warm/CLI warning delivery unchanged, zero-warnings produces no notice, reuses Phase 4 notice-band primitive |
| spectrum-tui-design-5-8 | Restore/daemon race review against the live event loop + startup-ordering integration-test updates (prior-incident surface) | prior-incident surface (slow-open / zombie-session) reviewed against live event loop, daemon spawned under `IsolateStateForTest` discipline (no leaked test daemon), startup-ordering integration tests updated for concurrent boot, warm-path synchronous ordering parity asserted, no `t.Parallel()` (cmd mutable mock state) |

### Phase 6: Analysis (Cycle 1)

Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| spectrum-tui-design-6-1 | Extract one shared footer key-hint helper and collapse the parallel footer-group types | byte-identical footer/modal-footer output across all five-to-six call sites, empty-key label-only fast path preserved (editFooterGroup), `footerGroup`/`previewFooterGroup` unified into one type, confirm/cancel footer-row helper routes the three modal footer rows, light/dark + colourless/NO_COLOR carve-out unchanged |
| spectrum-tui-design-6-2 | Consolidate the kill / delete-project destructive-confirm modals behind one parameterised builder | shared `renderDestructiveConfirm`/spec owns the destructive panel once, delete's project-path extra body row passed as data not a forked path, body-width 52 / `▲` glyph / state.red / text.detail / `y verb · esc cancel` each defined once, update/state logic untouched, byte-identical render in both modes + colourless |
| spectrum-tui-design-6-3 | Extract shared row-style and left-bar-column helpers for the Session/Project list delegates | `rowBg`/`rowToken` style logic in one place (both delegates route through it), §3.3 left-bar selector-column (selected + unselected) in one place, both `renderSessionRow`/`renderRowLine` call it, byte-identical selected/unselected rows in both modes + colourless |
| spectrum-tui-design-6-4 | Remove the stale post-detection documentation and the dead dark-pinned cursorStyle var | theme.go package doc + `Token.Color()` comment no longer claim detection "lands in 1-7"/hard-pins dark (describe `canvasMode → ColorFor(mode)`), package-level `cursorStyle` var removed, edit_modal.go:170 shadowing local untouched, no behavioural change, build + `internal/tui` tests pass |

### Phase 7: Analysis (Cycle 2)

Address findings from Analysis (Cycle 2).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| spectrum-tui-design-7-1 | Projects list must drop vim/uppercase/page-jump keys (§12.2 arrow-only nav) — call `pinArrowOnlyNav` in `newProjectList` mirroring `newSessionList` | `j/k/h/l/g/G/pgup/pgdown/home/end/b/u/f` inert for cursor/page on Projects (live dispatch, not descriptor display), uppercase `G` no longer navigates, `↑/↓` move + `Ctrl+↑/↓` page preserved, `updateProjectsPage` interception list untouched, dispatch-layer test (not descriptor-layer) drives live `bubbles/list` |
| spectrum-tui-design-7-2 | Remove dead per-modal footer/key-hint wrappers and consolidate the accent.blue key-hint path | five dead wrappers (`killModalFooterRow`/`deleteModalFooterRow`/`killModalKeyHint`/`deleteModalKeyHint`/`renameModalKeyHint`) + their sub-tests removed, two live wrappers (`editFooterGroup`/`previewFooterHint`) collapse to one canonical blue-hint path, `renameModalFooterRow` STAYS (live caller), edit+preview footers render byte-identically, surviving `renderConfirmCancelFooter`/`renderKeyHint` golden cases retained |
| spectrum-tui-design-7-3 | Scope the keymap descriptor "single source of truth" framing to display and guard descriptor↔dispatch drift | doc comments rescoped to "footer + help *display*" only, guard test asserts every non-help descriptor `Key` honoured by dispatch and every bound dispatch key present in descriptor, descriptor + dispatch stay separate (no behaviour change), display-only/help allow-list documented, guard passes on post-Task-1 tree |
| spectrum-tui-design-7-4 | Close residual enforcement and DRY gaps (colour guard, command-pending footer, separator constants) | `centralisedColourSites` grown to cover every token-referencing render file in internal/tui (Phase 3-5 blind spot closed), command-pending footer sourced from descriptor/entry vocabulary (no inline `enter→⏎` rewrite), `editFooterSep` deleted in favour of shared `footerEntrySeparator`, `helpColumnGap`/`modalFooterGap` pair left as-is, footers render byte-identically |

### Phase 8: Analysis (Cycle 3)

Address findings from Analysis (Cycle 3).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| spectrum-tui-design-8-1 | Collapse `BootstrapProgressMsg`'s duplicate friendly-label computation to a single authority — drop the producer-side `Label`/`Name` so the §10.4 11-step→5-label mapping lives only in `loading_progress.go` | only consumer folds through `LoadingProgress.Apply` (reads `Index`/`RestoreN`/`RestoreM` only), grep confirms no `internal/tui` reader of `msg.Label`/`msg.Name`, `Name` retained only if a consumer reads it, producer in `bootstrap_progress.go:247-254` no longer computes/assigns dropped field, `model.go` placeholder-field doc updated, loading page renders identical 5-label progression incl. restore N/M, tests constructing the msg compile against reduced field set |
| spectrum-tui-design-8-2 | Switch Sessions/Projects footer to §3.4 ⏎/␣ glyphs (spec-owner ratified PATH (b)) — `Key` forms → `⏎`/`␣`, nav → `↑↓` (no slash) to match §3.4 verbatim + the Preview footer convention | DECISION ratified path (b) glyphs (path (a) words NOT chosen), keymap descriptor (`keymap.go:88-90`,`:128`) + `footer.go:64-66` special-case + byte-exact `footer_test.go:41` all updated to glyph form, VISUAL change — re-capture every affected fixture (sessions-flat/by-project/by-tag/empty/no-tags-signpost, projects, projects-command-pending, filtering-* + light/nocolor) vs committed reference frame, Preview footer (already glyphs) untouched, render-level test confirms `⏎`/`␣` + `↑↓` |
| spectrum-tui-design-8-3 | Model the no-matches footer membership structurally instead of by magic label string — `noMatchesFooterEntries` must not drop by `Label == "browse results"` | structural model (identifying flag mirroring keymap Core-flag pattern, or compose directly from shared `type`/`esc` entries), no cross-file display-string coupling to `"browse results"`, §7.3 no-matches footer renders byte-identical (browse-results hint absent), unit test passes even if browse-results copy reworded, existing filtering-footer render tests byte-identical |
| spectrum-tui-design-8-4 | Remove dead `SessionItem`/`ProjectItem` `Title()`/`Description()` (or derive the attached marker from the const) — kill the hard-coded `"● attached"` re-spelling outside `attachedMarker` | only `FilterValue()` consumed off items (rows render via `renderSessionRow`/`renderRowLine`), grep confirms `Title()`/`Description()` test-only (no production caller), deletion path updates tests to assert live render OR derivation path sources marker from `attachedMarker`, literal `"● attached"` gone from `Description()`, live `renderSessionRow` marker render unchanged, colour-literal guard blind to plain string (latent stale seam) |
| spectrum-tui-design-8-5 | Point `loading_view.go` at the shared `header.go` leaf canvas-style helpers — `loadingFg`/`loadingStyle` are byte-identical to `headerStyle`/`headerCanvasBg` | third forked copy of the leaf-paint rule (role-token fg over `Background(canvas)`, bare under NO_COLOR), replace call sites with `headerStyle`/`headerCanvasBg` OR convert to one-line delegating aliases (mirroring `SessionDelegate.rowBg`/`rowToken` delegation), forked bodies deleted, leaf canvas-paint rule in exactly one source (`header.go`), loading screen renders byte-identical in light/dark/NO_COLOR |
| spectrum-tui-design-8-6 | Extract a single cleared-canvas modal placement helper — five `render*ModalOnClearedCanvas` wrappers repeat the §8.1/§13.5 `lipgloss.Place(... Center, Center ...)` line verbatim | `placeModalOnClearedCanvas(panel, width, height) string` owns the centring, each wrapper builds its panel then calls it (or call sites call it directly dropping wrappers), differs only by content builder (`renderHelpModalContent`/`renderKillModalContent`/`renderDeleteModalContent`/`renderRenameModalContent`/`m.renderEditProjectContent`), centring appears in exactly one place, help/kill/delete/rename/edit render byte-identical, test confirms routing through the single helper |
| spectrum-tui-design-8-7 | Extract the right-anchored footer row assembler shared by `footer.go` and `filter_footer.go` — `footerKeyRow` and `filterFooterRow` mirror the same fit-test/narrow-degrade/flex-spacer geometry | `assembleRightAnchoredRow(left, leftWidth, rightSeg, rightWidth, w, mode, colourless) string` owns fit test (`leftWidth+1+rightWidth > w`) + `headerPadRight` narrow-degrade + flex-spacer join, both rows render own left cluster + resolve shared `? help` anchor then call it, `fitLeftCluster` ellipsis logic stays footer.go-specific (not merged), both footers byte-identical at wide widths and at/below the narrow-degrade boundary, boundary test through shared assembler |

### Phase 9: Analysis (Cycle 4)

Address findings from Analysis (Cycle 4).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| spectrum-tui-design-9-1 | Delete dead modal-box scaffolding and scrub misleading "not-yet-reskinned" comments — remove dead `renderModalOnClearedCanvas` (modal.go:67) + transitively-dead `modalBorderStyle` (modal.go:32) | both symbols have no production caller (all five modals route through `render*ModalOnClearedCanvas` + `renderJoinedPanel`), two orphaned tests in `help_modal_frame_test.go`/`edit_modal_test.go` driving the dead path removed (live joined-panel tests retained), five wrapper comments (modal.go:94,:105,:118,:130,:143) scrubbed of "left intact for the OTHER (not-yet-reskinned) modals" framing, repo-wide grep returns no matches, `go build`/`go test ./...` pass with no unused-symbol/import errors |
| spectrum-tui-design-9-2 | Remove the unconsumed `bubbles/list` help-styling layer and correct its stale doc comment — drop `l.Help.Styles.*` writes from `brightenHelpStyles`/`canvasHelpStyles`/`colourlessHelpStyles` (footer is descriptor-driven via `renderCondensedFooter`) | built-in help disabled (`SetShowHelp(false)` model.go:1037/1075), no live render reads `l.Help`, retain load-bearing `l.Styles.HelpStyle` bg + delegate/pagination styling, `brightenHelpStyles` removed if fully emptied (pagination dots handled by `canvas`/`colourlessPaginationDots`), model.go:1023-1029 comment rewritten to name `renderCondensedFooter` (no `help.Model.FullHelpView` claim), dark-only `Token.Color()` (theme.go:60-62) retired if no other callers, footer byte-identical Sessions/Projects × canvas/colourless |
| spectrum-tui-design-9-3 | Make `SessionDelegate.canvasBg`/`tokenStyle` delegate to the canonical header leaf-style helpers — `canvasBg` (session_item.go:197-202) re-states `headerCanvasBg`, `tokenStyle` (session_item.go:214-221) re-states `headerStyle` + base | `canvasBg` → `headerCanvasBg(d.Mode, d.Colourless)`, `tokenStyle` → `headerStyle(fg, d.Mode, d.Colourless)` with caller base composited via `.Inherit(base)`, mirrors existing `loadingStyle`/`loadingFg` (loading_view.go:251,259) + `rowBg`/`rowToken` (session_item.go:337,344) delegations, colourless-fallback + canvas-resolution logic lives only in header leaf helpers, session-row render byte-identical canvas/colourless (grouped + flat), no-behaviour-change collapse |
| spectrum-tui-design-9-4 | Move and rename the shared joined-panel frame primitives off the use-site `help*` prefix — `renderJoinedPanel` (panel.go:34-61) chrome for all five modals + preview overlay is built from `help*`-prefixed primitives living in help_modal.go | shared primitives (`helpFrameStyle`/`helpFrameTop`/`Bottom`/`Divider`/`ContentLine`/`helpInsetRow`/`helpRowInset`/`helpRuleGlyph` + `helpFrame*` consts at help_modal.go:56-78,124-166) moved into panel.go beside `renderJoinedPanel` + renamed to frame-neutral `panel*` prefix, genuinely help-specific names (`helpModalHeader`/`helpModalRow`/`helpTitle`) stay in help_modal.go, every call site (modals/preview/tests) updated in one pass, repo-wide grep for old names returns no matches, all five modals + preview overlay render byte-identical, mechanical rename + relocation |
