---
phase: 1
phase_name: Verification harness + colour-token/canvas/detection foundation (lock-in gate)
total: 9
---

## spectrum-tui-design-1-1 | approved

### Task spectrum-tui-design-1-1: `vhs` capture harness — install/verify, committed tapes dir, fixture-seeded foundation capture, Paper reference PNG export pipeline

**Problem**: This is a visual reskin, so correctness is *visual* (spec verification mandate, §15.4). Every later task must self-verify by capturing the live TUI and comparing it to its named Paper frame, but there is no capture mechanism, no committed tapes directory, no deterministic fixture-seeding for captures, and no committed Paper-reference export to compare against. Without this harness the per-task implement↔review loop has no objective visual gate and can never terminate (§15.4).

**Solution**: Stand up the `vhs` (charmbracelet/vhs) capture harness as the one-time prerequisite every later visual task depends on: document/verify the install, create the committed tapes directory, build a deterministic fixture-seeding path so a tape produces the *same* PNG every run, author the first tape that drives Portal to a foundation screen and writes a PNG, and stand up the Paper-reference export pipeline (export the named frame via the `paper` MCP, commit the PNG in-repo so there is no live-MCP dependency at implementation/CI time).

**Outcome**: Running `vhs <tape>` from a clean checkout produces a deterministic PNG of a foundation Sessions screen under the committed harness dir, and the corresponding Paper reference PNG (`Sessions — Modern Vivid v2`) sits committed beside it — so any later task can place its capture next to the committed reference and judge layout/structure/colour-role match (§15.5), and any reviewer can re-capture without re-running the MCP.

**Do**:
- Document the one-time install in the harness README/doc-comment: `brew install vhs` (pulls `ttyd` + `ffmpeg`); non-Homebrew fallback `go install github.com/charmbracelet/vhs@latest` with `ttyd` + `ffmpeg` installed separately; verify with `vhs --version` (§15.2). Treat the install as developer-machine setup, not a Go dependency.
- Create the committed harness directory `testdata/vhs/` with two subdirs: tapes at the top level (e.g. `testdata/vhs/sessions-flat.tape`), captures committed and overwritten in place (e.g. `testdata/vhs/sessions-flat.png`), and `testdata/vhs/reference/` for the committed Paper exports (e.g. `testdata/vhs/reference/sessions-modern-vivid-v2.png`) (§15.4 screenshot storage, §15.5).
- Build a deterministic fixture-seeding mechanism the tape can invoke before launching Portal: a fixed set of sessions/projects (e.g. a known `projects.json` plus a known set of tmux sessions) seeded into an isolated config/state dir so captures never depend on the developer's live install. Reuse `portaltest.IsolateStateForTest`-style isolation env (a scrubbed `XDG_CONFIG_HOME` + isolated state dir) and a fixed seed script so the same sessions/projects appear in every run. Fixture-seeding mechanics are a harness implementation detail (§15.2) — the load-bearing requirement is *determinism*.
- Author `testdata/vhs/sessions-flat.tape`: set a fixed `FontFamily`, `Width`, `Height`; seed the fixture; launch Portal; `Sleep` long enough for first paint; `Screenshot sessions-flat.png`. Model it on the §15.2 example tape.
- Stand up the Paper-reference export step: export the `Sessions — Modern Vivid v2` frame from the Paper file via the `paper` MCP (`get_screenshot` / `export` by the frame's node-id) and commit it to `testdata/vhs/reference/sessions-modern-vivid-v2.png`. Document the refresh command in the harness doc so when the frame changes in Paper its export is re-committed (§15.5).

**Acceptance Criteria**:
- [ ] **(harness-is-the-gate-enabler — this task's acceptance is "the harness produces a deterministic capture", NOT a frame compare)** Running `vhs testdata/vhs/sessions-flat.tape` twice from a clean checkout produces byte-comparable foundation captures (deterministic: same seeded sessions/projects, same layout) — determinism is the pass criterion, not a Paper match.
- [ ] The committed `testdata/vhs/` dir holds the runnable tape, the captured PNG (committed, overwritten in place), and the committed Paper reference export `testdata/vhs/reference/sessions-modern-vivid-v2.png`.
- [ ] The capture is seeded from a known fixture in an isolated config/state dir — it never reads or writes the developer's real `~/.config/portal/` (verified by the isolation env applied to the spawned Portal process).
- [ ] The harness doc records the install + verify steps (Homebrew and non-Homebrew) and the `paper`-MCP re-export command for the reference.
- [ ] The compare mechanism is documented as **agent/user-judged layout/structure/colour-role match, NOT a pixel-diff CI gate** (Paper is an approximation; an exact diff would always fail — §15.2).

**Tests**:
- `"it produces a deterministic PNG: two vhs runs of the same tape yield the same seeded screen"`
- `"it seeds a known fixture set of sessions/projects into an isolated config/state dir, not the developer's live install"`
- `"it documents both the Homebrew and the non-Homebrew (go install + ttyd + ffmpeg) install paths and vhs --version verify"`
- `"it commits the Paper reference export beside the tape so comparison needs no live MCP"`
- `"it records the compare as agent/user-judged (not pixel-diff)"` — guard against a future pixel-diff CI gate being added

**Edge Cases**:
- `vhs` not installed / non-Homebrew install path — the harness doc must cover both; the tape itself fails fast with a clear "vhs not found" if absent (developer-machine prerequisite, not a Go dependency).
- Deterministic fixture-seeded state — a tape that depended on ambient sessions/projects would produce non-reproducible captures; the seed must fully determine the screen.
- Agent/user-judged compare (not pixel-diff) — Paper is an HTML approximation; the real terminal uses the user's font + the §2.9 token hexes, so an exact-pixel gate would always fail. Compare is layout/structure/colour-role only.

**Context**:
> §15.2: vhs is the prescribed verification tool (Portal is a Bubble Tea / charm app). One `.tape` per canonical screen, committed under a fixed harness dir (e.g. `testdata/vhs/`); each tape sets a fixed terminal size, seeds a known fixture state, launches Portal, sends keys, then `Screenshot <name>.png`. Pass criterion: layout/structure/colour-role match — agent/user-judged, NOT a pixel-diff CI gate.
> §15.4: each task's latest vhs PNG is committed in-repo under the harness dir, named per frame/task, overwritten in place so "latest" is always current.
> §15.5: the reference is a committed PNG export of the named Paper frame, exported via the `paper` MCP and committed alongside the tapes (e.g. `testdata/vhs/reference/<frame>.png`) — in-repo and durable, no live-MCP dependency at implementation/CI time.
> Codebase: `internal/portaltest/isolated_env.go` provides `IsolateStateForTest(t)` returning isolation env + an isolated state dir, and `SpawnIsolatedDaemon` / `RegisterSubprocessCleanup` for daemon-spawning subprocesses — reuse this isolation discipline for the seeded capture so a leaked test daemon cannot corrupt the live install (the slow-open/empty-previews/zombie-session incident is the canonical failure).
> The foundation screen this first tape targets is `Sessions — Modern Vivid v2` (dark) per §15.1 — but note Phase 1 produces only the canvas + tokens, so the *full* MV Sessions treatment lands in Phase 2; this task's deterministic-capture acceptance does not require the screen to already match the frame.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §15.1 (frame map), §15.2 (vhs harness + one-time setup + tapes dir + fixture seeding), §15.3 (per-task manual review), §15.4 (implement/review/human verification loop + screenshot storage), §15.5 (committed Paper reference export via the `paper` MCP).

## spectrum-tui-design-1-2 | approved

### Task spectrum-tui-design-1-2: Upgrade to Bubble Tea v2 / Lipgloss v2 (OSC 11 API + `AdaptiveColor` removal) with full parity

**Problem**: The detection mechanism the foundation needs (§2.6, §10.2) is written against Bubble Tea v2 (`tea.RequestBackgroundColor` → `BackgroundColorMsg`) and Lipgloss v2 (which removed `AdaptiveColor`, forcing *explicit* light/dark wiring — §14.5). The repo is currently on Bubble Tea v1.3.10 / Lipgloss v1.1.0 (`go.mod`), and bubbles v1.0.0. Detection task 1-7 cannot be written against the spec's v2 API surface until the deps are bumped and the API migrated.

**Solution**: Bump `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, and `github.com/charmbracelet/bubbles` to their v2 lines, then migrate every affected API call site to the v2 surface with **full behaviour parity** — no render or key-handling drift. Parity *is* the whole test for this task: nothing visual changes here, only the framework version under the same behaviour.

**Outcome**: The project compiles and all existing TUI tests pass against Bubble Tea v2 / Lipgloss v2 / bubbles v2, the one `AdaptiveColor` call site is migrated to the v2 equivalent with the same resolved colours, and no rendered output or key behaviour differs from the v1 build.

**Do**:
- Bump `go.mod`: `bubbletea` → v2, `lipgloss` → v2, `bubbles` → v2 (the bubbles `list`/`viewport`/`key` packages the TUI uses must match the bubbletea major). Run `go get` for each, then `go mod tidy`.
- Migrate the one existing `AdaptiveColor` call site — `previewBorderColor` in `internal/tui/pagepreview.go` (`lipgloss.AdaptiveColor{Light: "#3B5577", Dark: "#7B95BD"}`). Lipgloss v2 removed `AdaptiveColor`, so resolve the light/dark choice explicitly at render time using the same two hexes (the canvas-aware token layer that *centralises* this lands in tasks 1-3/1-4; here the goal is a parity-only migration — keep the exact `#3B5577`/`#7B95BD` values, do **not** change the colour).
- Sweep every other v1→v2 API break across the cmd/tui surface: `tea.NewProgram` options, `tea.KeyMsg`/`tea.Key` shape, `bubbles/list` and `bubbles/viewport` method/field signatures (e.g. the `viewport.SetSize` TODO noted in `pagepreview.go`), `bubbles/key` bindings, and any `Update`/`View`/`Init` signature changes. Migrate each to its v2 equivalent preserving behaviour.
- Audit the v2 colour-profile / renderer model: v2 changed how the default renderer and colour profile are configured. Confirm the existing `NO_COLOR` / palette-downsample behaviour (lipgloss/termenv auto-downsampling, §2.4) still holds under v2 before later tasks lean on it.
- Run the full suite (`go test ./...`); fix any v2-driven test breakage that is purely an API-shape change, and treat any behaviour difference as a parity bug to resolve, not accept.

**Acceptance Criteria**:
- [ ] **(non-visual / framework upgrade — exempt from the per-task vhs+frame check; parity IS the test)** `go build ./...` succeeds and `go test ./...` passes on Bubble Tea v2 / Lipgloss v2 / bubbles v2.
- [ ] `go.mod` shows the v2 lines for bubbletea, lipgloss, and bubbles; `go mod tidy` leaves a clean module graph.
- [ ] The `previewBorderColor` migration resolves to the **same** light `#3B5577` / dark `#7B95BD` colours as before (no hue change) under the v2 explicit-resolution model.
- [ ] No render output or key-handling behaviour differs from the v1 build — verified by reading every touched path, tracing it, and diffing the logic (provably cosmetic-or-nil change per §1).
- [ ] The `NO_COLOR` / palette-downsample behaviour is confirmed intact under v2 (§2.4).

**Tests**:
- `"it builds and the full existing test suite passes on Bubble Tea v2 / Lipgloss v2 / bubbles v2"`
- `"it migrates the previewBorderColor AdaptiveColor site to explicit light/dark with the same #3B5577/#7B95BD resolution"`
- `"it preserves preview chrome rendering byte-for-byte after the migration"` (parity check on the one styled surface that used AdaptiveColor)
- `"it preserves every Sessions/Projects key-handling path under the v2 KeyMsg shape"` (parity)
- `"it confirms NO_COLOR still suppresses colour under the v2 renderer"`

**Edge Cases**:
- The one existing `AdaptiveColor` call site (`previewBorderColor`) must migrate **unchanged** in resolved colour — it is a parity migration, not a restyle.
- All TUI tests must stay green — v2 API-shape breakage is expected and must be fixed; a behaviour difference hiding behind an API change is a parity bug.
- No render or key behaviour drift — the v2 KeyMsg / renderer model differs in shape; the *observable* behaviour must not.

**Context**:
> §2.6: detection is via OSC 11 — `tea.RequestBackgroundColor` → `BackgroundColorMsg` in Bubble Tea v2. Lipgloss v2 removed `AdaptiveColor`, so detection is explicit, not framework-implicit.
> §14.5: "Lipgloss v2 removed `AdaptiveColor`, so the light/dark choice is wired explicitly, not via a framework adaptive type."
> Codebase: `go.mod` is currently bubbletea v1.3.10 / lipgloss v1.1.0 / bubbles v1.0.0. The single `AdaptiveColor` site is `previewBorderColor` in `internal/tui/pagepreview.go` (`{Light: "#3B5577", Dark: "#7B95BD"}`); `pagepreview.go` also carries a TODO to switch to `viewport.SetSize` once bubbles exposes it — the v2 upgrade is the moment to resolve that TODO if v2's viewport exposes the method.
> **AMBIGUITY (note, do not invent):** the spec language is firmly v2-specific (§2.6/§14.5), but if an implementation audit finds Bubble Tea v1 already exposes a working OSC 11 background-colour query sufficient for §2.6, this task may collapse to "no upgrade needed" — record that finding explicitly rather than upgrading reflexively. The default, per the spec's v2-specific wording, is to upgrade.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §2.6 (OSC 11 detection mechanism, v2 API), §14.5 (cross-cutting foundation, AdaptiveColor removal), §10.2 (detect-or-timeout first-paint gate — consumes the v2 API).

## spectrum-tui-design-1-3 | approved

### Task spectrum-tui-design-1-3: MV role-token colour layer — closed ~20-token vocabulary, pinned DARK variants, centralising scattered dark literals

**Problem**: The redesign is built on a role-token colour layer (§2.1, §2.8): every renderer must reference a small fixed set of *semantic* tokens, never scattered literal hex. Today the colours are scattered raw literals — cursor `212` and detail `#777777` and attached `76` in `session_item.go`; `#777777` in `project_item.go`; help/keymap `#999999`/`#777777`/`#555555`, signpost `#888888`, flash `#888888` in `model.go`. Without a centralised token layer the spec's theme-readiness (§2.8), the closed vocabulary (§2.9), and the role rules (state.green live-only, state.red destructive-only, text.faint decorative-only) cannot be enforced, and the contrast-floor and light-variant tasks have nothing to operate on.

**Solution**: Introduce the closed MV token vocabulary (~20 named role tokens from the §2.9 table) as a single centralised colour layer, pin the **dark** variants exactly to the §2.9 hexes, and replace the scattered dark literals at the call sites the foundation centralises with token references. Light variants are added in task 1-4; this task lands the dark-only token layer and the role-correct re-pointing.

**Outcome**: A single token-layer package/file exposes every §2.9 named token with its pinned dark hex; the scattered dark literals in `session_item.go`, `project_item.go`, and `model.go` are replaced by the matching role token; no raw hex survives at those centralised call sites; the role rules hold (green only on live/positive signals, red only on destructive emphasis, faint only on decorative text).

**Do**:
- Create a centralised token layer (e.g. `internal/tui/theme/` or `internal/tui/tokens.go`) declaring the closed §2.9 vocabulary as named tokens. Greys/text ramp: `text.primary` `#C0CAF5`, `text.strong` `#A9B1D6`, `text.muted-bright` `#828BB8`, `text.detail` `#737AA2`, `text.dim` `#535C86`, `text.faint` `#3B4261`, `text.on-selection` `#FFFFFF`. Accents: `accent.violet` `#BB9AF7`, `accent.blue` `#7AA2F7`, `accent.cyan` `#7DCFFF`, `state.green` `#9ECE6A`, `state.red` `#F7768E`, `accent.orange` `#FF9E64`. Surfaces: `canvas` `#0b0c14`, `bg.selection` `#28243a`, `bg.warning` `#241B10`, `bg.track` `#26283A`, `border.separator` `#292E42`, `border.footer` `#20232E`, `text.on-warning` `#E8C9A0`. (Dark column of §2.9; light column is task 1-4.)
- Structure it so each token will carry a light + dark variant in task 1-4 — choose a representation (e.g. a `Token` with `Dark`/`Light` fields, resolved by a mode parameter) that 1-4 can extend without re-pointing every call site. Dark-only is acceptable in *this* task; design for the second variant.
- Re-point the scattered dark literals to tokens, by role: `session_item.go` `cursorStyle` `212` → `accent.violet` (cursor/selector role), `detailStyle` `#777777` → `text.detail`, `attachedStyle` `76` → `state.green` (the live `● attached` marker — a legitimate green role); `headingStyle` Faint(true) stays glyph/dim per §5.1 but its colour, if any, maps to `text.detail`/`text.dim` per the heading/count split. `project_item.go` `projectPathStyle` `#777777` → `text.detail`. `model.go` `brightenHelpStyles` `#999999`/`#777777`/`#555555` → the footer key-hint/label/separator roles (`accent.blue` key glyph, `text.detail` label — but DO NOT change footer *structure* here, only the colour source; the condensed-footer restyle is Phase 2), `byTagSignpostStyle` `#888888` → `text.strong` (signpost role, §11.3), `flashRowStyle` `#888888` → the flash token (transient — but the full flash band restyle is Phase 4; re-point colour only).
- Enforce the role rules at the centralised sites: `state.green` is used **only** for the attached marker / Sessions count / Projects label / `✓` done / success flash — never chips or decoration; `state.red` only for destructive emphasis; `text.faint` only for decorative text (inactive dots, `+ add`, mode indicator, hints) and must never carry functional text. Where a former literal's role does not match these rules, choose the role-correct token (e.g. a grey that was decorative → `text.faint`, a grey that was functional metadata → `text.detail`).
- Add a guard test asserting no raw `lipgloss.Color("#...")` / ANSI-index literal survives at the centralised call sites (the closed-vocabulary enforcement — model it on the codebase's existing source-walking guard pattern, e.g. the `internal/log` single-owner guard test).

**Acceptance Criteria**:
- [ ] **(non-visual / data layer — exempt from the per-task vhs+frame check; its visual effect is only Paper-comparable once proven on a foundation screen in 1-6/1-9. Acceptance is "tokens defined with pinned dark hex + role-correct re-pointing + no raw hex at centralised sites")** Every §2.9 dark token exists in the token layer with the exact pinned hex.
- [ ] The scattered dark literals in `session_item.go`, `project_item.go`, and `model.go` (the centralised call sites) are replaced by role-correct token references; a guard test confirms no raw hex / ANSI-index literal survives at those sites.
- [ ] `state.green` appears only on live/positive signals, `state.red` only on destructive emphasis, `text.faint` only on decorative text — verified by inspecting each re-pointed site.
- [ ] The token representation is structured so task 1-4 can add the light variant without re-pointing call sites.
- [ ] `go build ./...` and `go test ./...` pass; existing TUI render behaviour is unchanged in dark (this is a colour-source centralisation, not a restyle — the hexes may differ from the old literals, but the *role/role-mapping* is the change; any visible delta is the intended move to the MV dark palette and is verified against §2.9, not the old screen).

**Tests**:
- `"it defines every §2.9 named token with its exact pinned dark hex"`
- `"it re-points session/project/model dark literals to the role-correct token"`
- `"it leaves no raw hex or ANSI-index literal at the centralised call sites"` (guard test)
- `"it uses state.green only on live/positive signals (attached marker), never on a chip or decoration"`
- `"it uses state.red only on destructive emphasis and text.faint only on decorative text"`
- `"it structures each token to carry a light variant added later without re-pointing call sites"`

**Edge Cases**:
- `state.green` live/positive-only — the attached marker re-point is legitimate; any temptation to colour a chip or decorative element green must be rejected (green is attached-only, §2.9 rules).
- `state.red` destructive-only — no non-destructive site may adopt red.
- No raw hex at centralised call sites — the guard test is the enforcement; the closed vocabulary forbids a literal surviving outside the token layer.
- `text.faint` decorative-only — it must never carry functional text (2.1:1, exempt from the floor); a former functional-metadata grey maps to `text.detail`, not `text.faint`.

**Context**:
> §2.1: every renderer references a small fixed set of semantic role tokens, never scattered literal hex.
> §2.8: semantic role tokens, not per-element — reuse on a genuine role-match, promote a new named role where the value genuinely differs, never raw hex at a call site. A small role set (~20 tokens) re-themes coherently.
> §2.9 dark column (the pinned values listed in Do). Rules: closed vocabulary, state.green live/positive-only, state.red destructive-only, chips are `text.primary` on a tint never green, no stray hex (`#15131F` → `bg.selection`, `#2B3050` → `border.separator`).
> Codebase scattered literals: `internal/tui/session_item.go` (cursor `212`, detail `#777777`, attached `76`, headingStyle Faint), `internal/tui/project_item.go` (`#777777`), `internal/tui/model.go` (`brightenHelpStyles` `#999999`/`#777777`/`#555555` at ~L634, `byTagSignpostStyle` `#888888` at ~L2376, `flashRowStyle` `#888888` at ~L2395). The `internal/log` package's single-owner source-walking guard test is the model for the no-raw-hex guard.
> Reskin guardrail (§1): re-pointing colour to tokens is in-bounds; the dark *hex values change* to the MV palette, but layout/behaviour does not — verify against §2.9, not against preserving the old literal.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §2.1 (design to roles), §2.8 (tokenise now), §2.9 (MV token table — dark variants + rules).

## spectrum-tui-design-1-4 | approved

### Task spectrum-tui-design-1-4: Light token variants + independent contrast-floor numeric verification

**Problem**: Portal owns a mode-matched canvas (§1), so every §2.9 token needs a **light** variant (Tokyo Night Day derived) in addition to the dark one (task 1-3). The contrast floor (§2.3) is a hard gate that must be checked *numerically* before any taste judgement, and the two variants resolve **independently** — each measured only against its own mode-canvas (dark vs `#0b0c14`, light vs `#e1e2e7`). Without the light variants and a numeric floor check, the canvas paint (1-6), detection (1-7), and the in-terminal lock-in gate (1-9) have nothing to render in light mode or to validate against.

**Solution**: Add the LIGHT variant to every §2.9 token (the light column hexes), and write a numeric contrast-floor verification (an automated test) that measures each variant against *only its own* mode-canvas, applying the per-token floors (4.5:1 normal text, 3:1 large/bold/UI accents, `text.dim` held to 3:1, `text.faint` exempt), and co-tunes text-carrying tints with their on-band text token so the pair clears simultaneously.

**Outcome**: Every token carries both a dark and a light variant; an automated test computes WCAG contrast for each foreground token against its own mode-canvas and for each text-carrying tint as a *pair* (tint-vs-canvas ≥3:1 and text-vs-tint ≥ its text floor), and the test passes for every in-scope token — with the light SURFACE tints recorded as *provisional* pending the in-terminal eyeball in task 1-9.

**Do**:
- Add the light variant to each token from the §2.9 light column. Greys/text ramp (light, on `#e1e2e7`): `text.primary` `#2E3C64`, `text.strong` `#3F4760`, `text.muted-bright` `#515A80`, `text.detail` `#5A6296`, `text.dim` `#7C84AA`, `text.faint` `#AEB2C6`, `text.on-selection` `#1A1B2E`. Accents (light): `accent.violet` `#8A3FD1`, `accent.blue` `#2E5FD0`, `accent.cyan` `#0E7490`, `state.green` `#4C7A1F`, `state.red` `#C32647`, `accent.orange` `#9A5200`. Surfaces (light): `canvas` `#e1e2e7`, `border.separator`/`border.footer` `#C9CDDB`, `text.on-warning` `#7A4B12`; `bg.selection` `#D0C6F0`; `bg.warning` "light amber (§15)"; `bg.track` "light grey (§15)" — the latter two and the surface tints are PROVISIONAL here (pinned/eyeballed in task 1-9 — §2.9 "Light surface tints finalised at §15").
- Write the numeric contrast-floor verification as a Go test that, for each token, computes the WCAG contrast ratio of its variant against the matching mode-canvas (`canvas` dark `#0b0c14`, light `#e1e2e7`) and asserts ≥ the token's floor: 4.5 for normal-text tokens (`text.primary`/`strong`/`muted-bright`/`detail`/`on-selection`, `accent.blue`/`cyan`, `state.green`/`red`, `accent.orange`, `text.on-warning`), 3.0 for large/bold/UI-accent tokens (`accent.violet`, and `text.dim` held to 3:1), and **exempt** `text.faint` (decorative, ~2.1:1 — must NOT be asserted to the floor, only assert it stays in the decorative band so a future edit can't accidentally promote it to functional). The two variants resolve **independently** — assert each only against its own canvas; no single value need clear both.
- For text-carrying tints, verify the **pair**: `bg.selection` vs `canvas` ≥3:1 AND `text.on-selection`/the on-band text vs `bg.selection` ≥ its text floor (both clear simultaneously); same for `bg.warning` + `text.on-warning`. When a tint can't satisfy both with the current text token, the remedy is to move the text token too (two knobs — §2.9 co-tuning rule). Encode the pairings the test checks: selection band (name/count/attached on `bg.selection`), warning band (`text.on-warning` on `bg.warning`).
- Where a numeric check fails, apply the remedy rule (§2.9): brighten a dark variant on `#0b0c14`, darken/saturate a light variant on `#e1e2e7` — never lower the floor. Record any adjustment in a comment beside the token with the resulting ratio.
- Mark the light SURFACE tints (`bg.selection`, `bg.warning`, `bg.track`, light borders) as provisional-pending-1-9 (a comment + a note in the test) since a numeric pass alone is insufficient for the light-tint-on-light-canvas case (§2.9 / §15.6).

**Acceptance Criteria**:
- [ ] **(non-visual / data layer — exempt from the per-task vhs+frame check. Acceptance is "light variants defined + numeric contrast-floor check passes")** Every §2.9 token carries a light variant matching the §2.9 light column (surface tints provisional).
- [ ] The numeric contrast-floor test passes: each foreground token clears its floor against *its own* mode-canvas, measured independently (dark vs `#0b0c14`, light vs `#e1e2e7`).
- [ ] `text.dim` is asserted to the 3:1 floor; `text.faint` is exempt from the floor (asserted only to stay in the decorative band, never promoted to functional).
- [ ] Text-carrying tints are verified as a pair (tint-vs-canvas AND text-vs-tint) and both clear simultaneously for `bg.selection`+on-band text and `bg.warning`+`text.on-warning`.
- [ ] The light surface tints are flagged provisional-pending the in-terminal eyeball (task 1-9); any numeric remedy applied is the more-contrast direction (never a lowered floor) and is recorded.

**Tests**:
- `"it defines a light variant for every §2.9 token"`
- `"it clears the contrast floor for each foreground token against its own mode-canvas, independently (dark on #0b0c14, light on #e1e2e7)"`
- `"it holds text.dim to the 3:1 floor"`
- `"it exempts text.faint from the floor but asserts it stays in the decorative band (never functional)"`
- `"it verifies bg.selection + on-band text as a pair that both clear simultaneously"`
- `"it verifies bg.warning + text.on-warning as a pair that both clear simultaneously"`
- `"it flags the light surface tints provisional pending the in-terminal eyeball"`

**Edge Cases**:
- Variants resolve independently — a value need only clear against its own canvas; asserting one variant against the *other's* canvas is wrong.
- `text.dim` 3:1 floor — deliberately de-emphasised but legible; held to the large/UI floor, not 4.5.
- `text.faint` exempt — must never carry functional text; the test guards against accidental promotion, not against being under 4.5.
- Text-carrying tints co-tuned — the pair (tint + on-band text) must clear *together*; when no single tint value satisfies both, the text token moves too (two knobs).
- Light-tint-on-light-canvas — a numeric pass is insufficient for surface tints; they are provisional here and finalised by eyeball in task 1-9.

**Context**:
> §2.3: contrast floor is a hard gate before taste — 4.5:1 normal text, 3:1 large/bold/UI accents, measured against the exact owned canvas for its mode (dark on `#0b0c14`, light on `#e1e2e7`), each ≥ its ratio, resolving independently. Scope is every rendered element: all foreground tokens, all per-element tints/bands, and every foreground-on-tint pairing.
> §2.9 light column (the hexes listed in Do) + rules: contrast re-verification (the canvas pass) measures each variant against the exact canvas, independently; remedy = adjust toward more contrast (brighten dark on `#0b0c14`, darken/saturate light on `#e1e2e7`), never drop the floor. Text-carrying tints co-tuned with their on-band text token — pinned by two ratios, both clear simultaneously. "Light surface tints finalised at §15" — `bg.selection` (`#D0C6F0`), `bg.warning`, `bg.track`, light borders (`#C9CDDB`) pinned and eyeballed against `#e1e2e7` at the validation gate.
> §15.6: light-mode coverage is per-token; a numeric pass is insufficient for light tints (the recurring failure class). This task does the numeric layer; the eyeball is task 1-9.
> Codebase: `github.com/lucasb-eyer/go-colorful` is already an indirect dependency (`go.mod`) and provides colour math usable for a WCAG-ratio helper in the test.
> **AMBIGUITY (note, do not invent):** §2.9 leaves `bg.warning` light and `bg.track` light as "light amber (§15)" / "light grey (§15)" — concrete hexes are deferred to task 1-9. Use a clearly-marked provisional placeholder derived from the dark anchor + the light canvas; do not invent a final value here. The footer/separator light value `#C9CDDB` is shared by `border.separator` and `border.footer` in the light column (the 2-tone dark split collapses to one light value per §2.9).

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §2.3 (contrast floor), §2.9 (MV token table — light variants + re-verification rules), §15.6 (light-mode per-token check).

## spectrum-tui-design-1-5 | approved

### Task spectrum-tui-design-1-5: `appearance: auto|light|dark` pref in prefs.json (default `auto`, tolerant decode)

**Problem**: The owned-canvas mode is auto-detected (§2.6), but OSC 11 misdetects on some terminals (notably tmux passthrough). The spec's recourse is an `appearance` preference (`auto | light | dark`, default `auto`) in `prefs.json`, beside `session_list_mode`, that pins the mode and skips detection when set to `light`/`dark` (§2.6, §16.1). Without this pref field plumbed through, detection task 1-7 has no override to honour.

**Solution**: Extend `internal/prefs/store.go` to carry an `appearance` field mirroring its tolerant-decode pattern (missing/empty/corrupt/unrecognised → `auto`), with an `Appearance` enum, and wire it into the TUI at construction via the existing `cmd/open.go` option path (alongside `WithInitialMode`/`WithModePersister`). Prefs stays a leaf (no `internal/log` import).

**Outcome**: `prefs.json` round-trips an `appearance` field beside `session_list_mode`; `Store.Load` returns the parsed appearance with every degenerate input collapsing to `auto`; the value is read once at TUI construction and injected into the model via a new option, available for detection task 1-7 to honour — and the existing `session_list_mode` behaviour is unchanged.

**Do**:
- In `internal/prefs/store.go`, add an `Appearance` enum mirroring `SessionListMode`: `AppearanceAuto` (iota/default), `AppearanceLight`, `AppearanceDark`, with canonical on-disk strings `"auto"`/`"light"`/`"dark"` and a `String()` method that maps out-of-range to `"auto"`.
- Add a `parseAppearance(s string) Appearance` that collapses any unrecognised value to `AppearanceAuto` (tolerant decode, mirroring `parseMode`).
- Add `Appearance string \`json:"appearance"\`` to the `prefsFile` struct (keeping `SessionListMode` intact). A missing field decodes to the empty string → `parseAppearance` → `AppearanceAuto`.
- Add a `LoadAppearance() (Appearance, error)` method (or extend the existing load to return both) that reads `prefs.json` and returns `parseAppearance(f.Appearance)` with the same error policy as `Load`: missing file / empty / corrupt / unrecognised → `(AppearanceAuto, nil)`; only a non-ErrNotExist read error propagates alongside `AppearanceAuto`. Decide whether to keep two single-purpose loaders or one combined loader — prefer the minimal change that does not regress the `Load() (SessionListMode, error)` signature its existing callers depend on (a separate `LoadAppearance` is the lowest-risk option; note the choice).
- Ensure `Save` round-trips both fields: when writing `session_list_mode` it must not drop a previously-set `appearance` (and vice versa). If `Save` currently writes only `session_list_mode`, extend it to preserve/write `appearance` too (read-modify-write or carry both in the in-memory struct) — verify a save of one field does not blank the other.
- Wire into the TUI: in `cmd/open.go`'s `openTUI`, read the appearance from the same `prefsStore` instance (`LoadAppearance`, tolerant) alongside the existing `initialMode` read, add an `appearance` field to `tuiConfig`, and pass it through a new `tui.WithAppearance(...)` option (sibling to `WithInitialMode`) so the model holds the resolved/pinned-mode preference for detection task 1-7. The model just *stores* it here; honouring it (skip detection + wait) is task 1-7.
- Keep prefs a leaf: do NOT add an `internal/log` (or `internal/storelog`) import — prefs.json is outside the closed audit-trail vocabulary (its package doc and the `prefs.json`→empty-component mapping in `cmd/config.go` already enforce this).

**Acceptance Criteria**:
- [ ] **(non-visual / data round-trip — exempt from the per-task vhs+frame check. Acceptance is "the pref round-trips and tolerant-decodes")** `prefs.json` round-trips `appearance` beside `session_list_mode`; saving one field does not blank the other.
- [ ] `Store.LoadAppearance` (or the combined loader) returns `AppearanceAuto` for missing file, empty file, corrupt/unparseable content, and an unrecognised `appearance` value; only a non-ErrNotExist read error propagates.
- [ ] The `appearance` value is read once at TUI construction in `cmd/open.go` and injected into the model via the new option alongside `WithInitialMode`.
- [ ] `session_list_mode` load/save behaviour is unchanged (no regression).
- [ ] `internal/prefs` imports only stdlib + `internal/fileutil` (no `internal/log` / `internal/storelog`) — prefs stays a leaf.

**Tests**:
- `"it round-trips appearance=light through Save then Load"`
- `"it defaults appearance to auto when the field is missing"`
- `"it collapses an unrecognised appearance value to auto (tolerant decode)"`
- `"it collapses an empty/corrupt prefs.json to auto for appearance"`
- `"it preserves session_list_mode when saving appearance, and vice versa"` (no cross-field blanking)
- `"it propagates only a non-ErrNotExist read error, returning auto alongside"`
- `"it does not regress the existing session_list_mode load/save"`
- `"it keeps internal/prefs a leaf (no internal/log import)"` (import guard)

**Edge Cases**:
- Missing / unrecognised / corrupt / empty → `auto` — every degenerate input collapses, no hard error (mirrors `session_list_mode`).
- No `session_list_mode` regression — extending `prefsFile` and `Save` must not drop or change the existing field's behaviour; a one-field save must preserve the other.
- Prefs stays a leaf — no `internal/log` import; an import-guard test enforces it.

**Context**:
> §2.6: `prefs.json` carries `appearance: auto | light | dark` (default `auto`), beside `session_list_mode`. `auto` detects with the dark fallback; `light`/`dark` pin the mode and skip detection (also skipping the startup detection wait). Recourse for terminals where OSC 11 misdetects. Not a second render path — both light and dark are owned-canvas paths regardless.
> §16.1: in scope — the `appearance: auto | light | dark` pref in `prefs.json`.
> Codebase: `internal/prefs/store.go` already implements the tolerant-decode pattern for `session_list_mode` (`parseMode`, `prefsFile`, `Load`, `Save` via `fileutil.AtomicWrite`) and its package doc forbids importing `internal/log`/`internal/storelog`. `cmd/open.go` `openTUI` loads `prefsStore` once (~L453), reads `initialMode` tolerantly (~L461), and injects it via `tui.WithInitialMode` (~L386); the `tuiConfig` struct is at ~L332. `cmd/clean.go` has `loadPrefsStore`/`prefsFilePath`. `cmd/config.go` maps `prefs.json` to the empty log component (suppressing its migrate log).

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §2.6 (light/dark detection & canvas selection — `appearance` pref), §16.1 (in scope — `appearance` pref in prefs.json).

## spectrum-tui-design-1-6 | approved

### Task spectrum-tui-design-1-6: Owned mode-matched canvas paint — leaf `.Background(canvas)` + outer full-terminal fill as the last layer

**Problem**: Portal must own a mode-matched canvas painted on **every cell** (§1) so MV's colours always sit on the surface they were tuned for and the contrast floor is guaranteed. Today there is no canvas: rows render on the terminal's native background, leaving edge bleed and unpainted mid-screen rows, and the floor is measured against an arbitrary terminal bg. The canvas is the surface every later reskin surface renders on.

**Solution**: Paint the owned canvas in two layers per §1: (1) leaf styles carry `.Background(canvas)` so every text/accent run paints its own cells, and (2) an **outer-layer full-terminal fill** — a container sized `Width=termW · Height=termH · Background=canvas` (or `lipgloss.Place` + `WithWhitespaceBackground`) — that wraps the already-composed view as the **last** layer, padding every line to full width and filling full height. This is a foundation visual task: it carries the full vhs capture + named-Paper-frame compare. It may use a temporary/injected mode source so it lands before detection task 1-7 supplies the real resolved mode.

**Outcome**: Every cell of the TUI carries the `canvas` colour — no edge bleed, no unpainted mid-screen rows — and the one-row-per-delegate pagination invariant (§3.5, §4.1) is preserved because the outer fill never participates in the list's height budget; a dynamic vertical change re-pads to `termH` underneath the fill. The captured foundation screen matches `Sessions — Modern Vivid v2` (dark) / `Sessions — Modern Vivid (Light)` for the canvas paint (full-bleed, flat fill, no frame).

**Do**:
- Apply the leaf layer: ensure the foundation-screen leaf styles (the token-backed styles from tasks 1-3/1-4 used by the row delegates, header, footer) carry `.Background(canvas)` so each rendered run paints its own cells against the canvas. (The full per-surface restyle is later phases; here, prove the canvas on the foundation Sessions screen.)
- Add the **single outer wrap point** in `internal/tui/model.go` `View()`. `View()` already dispatches per page and `m.termWidth`/`m.termHeight` already feed `viewLoading`'s `lipgloss.Place`. Compose each page's view as today, then wrap the assembled result in the outer full-terminal fill (`Width=m.termWidth`, `Height=m.termHeight`, `Background=canvas`, or `lipgloss.Place(termW, termH, ..., WithWhitespaceBackground(canvas))`) as the **last** layer over the assembled view (header + any notice band + list + footer).
- Keep the fill **outside the list's height budget**: the list's width/height budget is unchanged; the outer fill is a wrap, not per-delegate-row painting. A dynamic vertical change (e.g. a notice band appearing/clearing) drives the list's height recompute *underneath* the fill, which simply re-pads to `termH`. The fill must NOT perturb the one-row-per-delegate pagination invariant — no extra uncounted lines (the original cursor-invisible / missing-title / left-shift overflow bug class, §5.1).
- Source the canvas colour from a mode parameter. Detection (1-7) is not landed yet, so use a temporary/injected mode source (e.g. default to dark, or a test-injectable mode) — structure it so 1-7 can swap in the real resolved mode without changing the wrap point.
- Handle the zero-size edge: when `m.termWidth`/`m.termHeight` are 0 (pre-first-`WindowSizeMsg`), fall back to safe defaults exactly as `viewLoading` does today (`w=80`, `h=24`) so the fill never sizes to zero and blanks the screen.
- Produce the vhs capture: extend/author the foundation tape (from task 1-1) to drive the TUI to the Sessions screen and `Screenshot` it; place the capture beside the committed `Sessions — Modern Vivid v2` reference and judge canvas paint (full-bleed, flat fill, no frame), and the `Sessions — Modern Vivid (Light)` reference in light mode (using the injected light mode source).

**Acceptance Criteria**:
- [ ] **(VISUAL — full vhs capture + named-Paper-frame compare required)** A vhs capture of the Sessions foundation screen shows every cell painted the `canvas` colour (`#0b0c14` dark / `#e1e2e7` light) — no edge bleed at line ends, no unpainted mid-screen rows — and matches `Sessions — Modern Vivid v2` (dark) / `Sessions — Modern Vivid (Light)` (light) for the flat full-terminal fill with **no frame** (§3.6).
- [ ] The outer fill is a single wrap point in `model.go` `View()`, applied as the last layer over the assembled per-page view; the leaf styles carry `.Background(canvas)`.
- [ ] The one-row-per-delegate pagination invariant is preserved: the fill is outside the list's height budget; a dynamic vertical change re-pads to `termH` without changing the list's row count or overflowing the viewport.
- [ ] Zero-size (`termWidth`/`termHeight` == 0) falls back to safe defaults so the fill never blanks the screen.
- [ ] Behaviour parity vs the pre-reskin Sessions implementation: navigation, selection, filtering, and key handling are byte-for-byte identical underneath the new fill (read the path, trace it, diff — provably cosmetic).

**Tests**:
- `"it paints the canvas background on every cell — no edge bleed, no unpainted mid-screen rows"` (vhs capture vs `Sessions — Modern Vivid v2`)
- `"it renders the light canvas full-bleed matching Sessions — Modern Vivid (Light)"` (vhs capture, injected light mode)
- `"it keeps the outer fill outside the list height budget so pagination row count is unchanged"`
- `"it re-pads to termH when a dynamic vertical change recomputes the list height"`
- `"it preserves the one-row-per-delegate pagination invariant under the fill (no overflow, no extra uncounted lines)"`
- `"it falls back to safe defaults at zero terminal size so the fill never blanks the screen"`
- `"it preserves Sessions navigation/selection/filter behaviour parity under the fill"`

**Edge Cases**:
- Fill outside the list height budget — the fill is an outer wrap, not per-delegate painting; it must not enter the list's height computation.
- Re-pads to `termH` on vertical change — a band appearing/clearing recomputes the list height underneath; the fill simply re-pads, never miscounts.
- No edge bleed / empty rows painted — every line padded to full width, full height filled.
- Zero-size fallback — pre-first-`WindowSizeMsg` `termWidth`/`termHeight` are 0; fall back to `80×24` (matching `viewLoading`) so the fill never sizes to zero.
- Pagination invariant preserved — the original overflow bug (uncounted extra lines scrolling title/cursor off the top, §5.1) must not regress.

**Context**:
> §1 Canvas ownership: painted on every cell, two layers — (1) leaf styles carry `.Background(canvas)`, (2) an outer-layer full-terminal fill (`Width=termW · Height=termH · Background=canvas`, or `lipgloss.Place` + `WithWhitespaceBackground`) pads every line to full width and fills full height. The fill is the **last** layer: it wraps the already-composed view (header + notice band + list + footer summed to `termH`), so a dynamic vertical change drives the list's height recompute underneath the fill, which re-pads to `termH`. The fill never participates in the list's height budget and must not perturb the one-row-per-delegate pagination invariant (§3.5, §4.1).
> §3.6: no full-screen frame — the owned canvas is a flat full-terminal fill, not a frame; it paints every cell the same `canvas` colour but draws no border around the UI.
> §15.1: foundation frames are `Sessions — Modern Vivid v2` (dark) and `Sessions — Modern Vivid (Light)`, painted on the owned canvas.
> Codebase: `internal/tui/model.go` `View()` (~L2209) dispatches per page (`PageLoading`→`viewLoading`, `PageProjects`→`viewProjectList`, `pagePreview`→`preview.View()`, default→`viewSessionList`); `m.termWidth`/`m.termHeight` are set on `tea.WindowSizeMsg` (~L1336) and `viewLoading` (~L2232) already uses `lipgloss.Place(w, h, Center, Center, text)` with the `w==0→80`/`h==0→24` fallback — the canonical pattern to mirror for the outer fill.
> May use a temporary/injected mode source so this lands before 1-7 (which supplies the real resolved mode).

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §1 (Canvas ownership — two layers, outer fill as last layer), §3.5/§4.1 (one-row-per-delegate pagination invariant), §3.6 (flat fill, no frame), §15.1 (foundation frames).

## spectrum-tui-design-1-7 | approved

### Task spectrum-tui-design-1-7: Light/dark detection (OSC 11) + `appearance` override + detect-or-timeout first-paint gate (dark fallback)

**Problem**: Owning a canvas means deciding *which* canvas — light or dark — to paint (§2.6). The reply to the OSC 11 background-colour query is async, so a naive implementation paints one canvas then flips to the other (a visible, jarring flip). The canvas paint (1-6) currently uses a temporary mode source; it needs the *real* resolved mode, gated so the correct canvas lands on frame one with no flip.

**Solution**: Implement explicit light/dark detection via OSC 11 (`tea.RequestBackgroundColor` → `BackgroundColorMsg`, Bubble Tea v2), honour the `appearance` pin from task 1-5 (light/dark skip detection AND its wait), and gate the **first real paint** on "detection resolved OR a short timeout (tens of ms)" so Portal never paints one canvas then flips. A no-answer resolves to the **dark** fallback. `COLORFGBG` is a weak secondary hint only. Integration point is the `cmd/open.go` `tea.NewProgram` launch; 1-6 supplies the wrap, this task supplies the resolved mode. This is a foundation visual task: it carries the full vhs capture + named-Paper-frame compare.

**Outcome**: On launch in `auto` mode, Portal queries OSC 11, gates the first real paint on detect-or-timeout, and paints the correct canvas from frame one with no flip; a non-responding terminal falls through to the dark fallback after a brief invisible wait; `appearance: light`/`dark` pins the mode and skips detection + the wait entirely. The captured Sessions screen matches `Sessions — Modern Vivid v2` (dark-detected) and `Sessions — Modern Vivid (Light)` (light-detected), each landing the correct canvas on frame one.

**Do**:
- Wire the OSC 11 query at the `cmd/open.go` `tea.NewProgram` launch path. In the model's `Init` (or the program command set), request the background colour via `tea.RequestBackgroundColor` (Bubble Tea v2). Handle the `BackgroundColorMsg` reply in `Update`: compute light/dark from the reported background luminance and resolve the canvas mode.
- Implement the **first-paint gate**: hold the first *real* paint until "detection resolved OR a short timeout (tens of ms)". Use a `tea.Tick`-based timeout (tens of ms) racing the `BackgroundColorMsg`; whichever resolves first sets the mode, and the model gates its first canvas paint on that resolution. Portal must never paint one canvas then flip to the other — paint the resolved canvas from frame one. The cold-path loading page (§10, Phase 5) will gate the same way; structure the gate so Phase 5 reuses it.
- Honour the `appearance` pin (from task 1-5, now held on the model): if `appearance == light` or `dark`, **skip the OSC 11 query and the first-paint wait entirely** — set the canvas mode directly and paint immediately. Only `appearance == auto` runs detection.
- Fallback: a no-answer (timeout fires before `BackgroundColorMsg`) resolves to the **dark** canvas (most users run dark; termenv defaults dark; MV is dark-first). A mis-detected light-terminal user gets a legible-but-wrong-mode screen — cosmetic, not broken — with the `appearance` override as recourse.
- `COLORFGBG`: treat as a **weak secondary hint only** — OSC 11 is the real signal. Do not let `COLORFGBG` override an OSC 11 answer; use it (if at all) only as a tie-break/early hint, never as the primary decision.
- Swap the canvas mode source: replace the temporary/injected mode source from task 1-6 with the real resolved mode, keeping the single outer wrap point in `View()` unchanged.
- Produce the vhs capture: drive the TUI under a dark-reporting and a light-reporting terminal (or the `appearance` pins), capture the Sessions screen in each, and compare against `Sessions — Modern Vivid v2` (dark) / `Sessions — Modern Vivid (Light)` (light) — confirming the correct canvas lands on frame one with no observable flip.

**Acceptance Criteria**:
- [ ] **(VISUAL — full vhs capture + named-Paper-frame compare required)** Captured Sessions screens match `Sessions — Modern Vivid v2` (dark-detected) and `Sessions — Modern Vivid (Light)` (light-detected); the correct canvas is present on the first frame with no paint-then-flip.
- [ ] In `auto` mode the OSC 11 query (`tea.RequestBackgroundColor`/`BackgroundColorMsg`) drives the canvas mode; the first real paint gates on detect-resolved-OR-short-timeout.
- [ ] `appearance: light`/`dark` pins the mode and skips detection AND the wait; only `auto` runs detection.
- [ ] A no-answer / timeout resolves to the dark fallback; mis-detection is cosmetic-not-broken (the floor holds against whichever canvas is painted).
- [ ] `COLORFGBG` is a weak secondary hint only and never overrides an OSC 11 answer.
- [ ] Behaviour parity: detection adds startup messages but does not alter Sessions navigation/selection/filter/key behaviour once resolved.

**Tests**:
- `"it detects dark via OSC 11 and paints the dark canvas from frame one"` (vhs vs `Sessions — Modern Vivid v2`)
- `"it detects light via OSC 11 and paints the light canvas from frame one"` (vhs vs `Sessions — Modern Vivid (Light)`)
- `"it never paints one canvas then flips to the other"` (gate on detect-or-timeout)
- `"it falls back to dark when no OSC 11 answer arrives before the timeout"`
- `"it pins the mode and skips detection + wait when appearance is light"`
- `"it pins the mode and skips detection + wait when appearance is dark"`
- `"it treats COLORFGBG as a weak hint and never overrides an OSC 11 answer"`
- `"it leaves a mis-detected terminal legible-but-wrong-mode, not broken (floor holds)"`

**Edge Cases**:
- Never paint-then-flip — the first real paint gates on detect-or-timeout; a flip is a defect.
- No-answer / timeout → dark — the fallback default; the wait is brief and invisible.
- `light`/`dark` pin skips detection + wait — no OSC 11 query, no timeout, immediate paint.
- Mis-detection cosmetic-not-broken — with an owned canvas a wrong guess is a light/dark surprise, never illegibility; `appearance` is the recourse.
- `COLORFGBG` weak hint only — must not override OSC 11.

**Context**:
> §2.6: detect terminal background luminance via OSC 11 (`tea.RequestBackgroundColor` → `BackgroundColorMsg` in Bubble Tea v2) → light/dark. `COLORFGBG` is a weak secondary hint only; OSC 11 is the real signal. Run once per launch, no caching. Flip avoidance: the reply is async, so gate the first real paint on "detection resolved OR a short timeout (tens of ms)" — Portal never paints one canvas then flips. Fallback default — dark. Override — `appearance` pref: `auto` detects with the dark fallback; `light`/`dark` pin the mode and skip detection (also skipping the startup detection wait). `NO_COLOR` skips detection entirely (task 1-8).
> §10.2: canvas-flip avoidance — the first real paint gates on light/dark detection-or-timeout so the loading page paints the correct canvas from frame one; the cold-path loading page (Phase 5) gates the same way (structure the gate for reuse).
> Codebase: `cmd/open.go` `openTUI` builds the model and launches `tea.NewProgram(m, tea.WithAltScreen())` (~L534); task 1-5 added the `appearance` pref to the model. Task 1-6 added the single outer canvas wrap in `model.go` `View()` with a temporary mode source — replace that source here with the real resolved mode.
> **AMBIGUITY (note, do not invent):** the spec says "tens of ms" for the timeout without pinning an exact value (it is an implementation detail). Choose a value in the tens-of-ms range (e.g. 50ms) that terminals answering in single-digit ms beat comfortably while keeping the wait invisible against the multi-hundred-ms bootstrap; record the chosen value and rationale.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §2.6 (light/dark detection & canvas selection), §10.2 (canvas-flip avoidance — detect-or-timeout first-paint gate).

## spectrum-tui-design-1-8 | approved

### Task spectrum-tui-design-1-8: `NO_COLOR` carve-out — skip detection, suppress canvas, colourless native fg/bg path

**Problem**: Portal honours `NO_COLOR` (§2.5): it imposes no hues, so painting an owned canvas would be wrong — the one documented carve-out to the single owned-canvas render path (§1). Under `NO_COLOR`, Portal must render colourless on the terminal's native fg/bg, leaning on glyph-backed state (§2.2) + bold/dim, and it must skip detection (and its first-paint wait) since there is no canvas to select. Without this carve-out, `NO_COLOR` users get a painted canvas that fights their explicit opt-out and an unnecessary detection wait.

**Solution**: Implement the `NO_COLOR` carve-out: when `NO_COLOR` is set, skip OSC 11 detection and its first-paint wait entirely, suppress the canvas paint (both the leaf `.Background(canvas)` and the outer full-terminal fill), and render colourless on the terminal's native fg/bg — relying on the glyph-backed state (§2.2) plus bold/dim attributes. This is the one documented second render path, legible by construction (the terminal's own defaults). It applies to every canvas-dependent surface; here it is proven on the foundation Sessions screen. This is a foundation visual task: it carries the full vhs capture + named-Paper-frame compare (the `NO_COLOR` foundation variant).

**Outcome**: With `NO_COLOR` set, the Sessions screen renders on the terminal's native fg/bg with no painted canvas and no colour, state conveyed by glyphs (`●` attached, `▌` selector, spaced headers) + bold/dim; detection and its wait are skipped; the screen stays fully usable and legible by construction. The capture confirms the colourless native-bg path against the foundation Sessions layout (structure/glyph match — colour-role is N/A under `NO_COLOR`).

**Do**:
- Detect `NO_COLOR` at the same decision point as canvas-mode selection (the `cmd/open.go`/model construction path, before the OSC 11 query). When `NO_COLOR` is set: skip the OSC 11 query and the detect-or-timeout first-paint wait entirely (no canvas to select — §2.6 / §2.5), and set a "colourless" render mode on the model.
- Suppress the canvas in colourless mode: the outer full-terminal fill must NOT paint a `canvas` background (render on the terminal's native bg), and the leaf styles must NOT carry `.Background(canvas)` — i.e. the single outer wrap point from task 1-6 becomes a no-op-background pass-through under `NO_COLOR`.
- Render colourless: under `NO_COLOR`, foreground tokens drop their hue and lean on the glyph-backed state (§2.2) + bold/dim attributes. Confirm lipgloss/termenv's `NO_COLOR` handling (verified intact in task 1-2) already strips foreground colour; the work here is ensuring the *canvas backgrounds* are also suppressed (lipgloss would otherwise still emit the bg) and that state remains glyph-distinct (the `●`/`▌`/`✓`/`⚠` glyphs + bold/dim carry state without colour — already true per §2.2).
- Keep this a single carve-out that every canvas-dependent surface inherits: structure the colourless decision so later phases' surfaces (modals' blank-screen, notice bands, preview chrome) read the same flag rather than each re-deriving `NO_COLOR` handling. Here, prove it only on the foundation Sessions screen.
- Produce the vhs capture: drive the foundation tape with `NO_COLOR=1` in the seeded environment, capture the Sessions screen, and confirm the colourless native-bg path — no painted canvas, state via glyphs + bold/dim, layout/structure matching the foundation Sessions frame (colour-role match is N/A; verify structure + glyph-backed state).

**Acceptance Criteria**:
- [ ] **(VISUAL — full vhs capture + named-Paper-frame compare required, as the `NO_COLOR` foundation variant)** A `NO_COLOR=1` vhs capture of Sessions shows no painted canvas (terminal native bg), no colour, and state carried by glyphs (`●`/`▌`/spaced headers) + bold/dim — structurally matching the foundation Sessions layout and legible by construction.
- [ ] Under `NO_COLOR`, OSC 11 detection and its first-paint wait are skipped entirely.
- [ ] The outer full-terminal fill suppresses the canvas background and the leaf styles drop `.Background(canvas)` under `NO_COLOR`.
- [ ] State stays glyph-distinct without colour (the `NO_COLOR` path works for free off §2.2's glyph-backed state).
- [ ] The colourless decision is a single carve-out flag every canvas-dependent surface can inherit (not re-derived per surface).
- [ ] Behaviour parity: `NO_COLOR` changes only rendering; navigation/selection/filter/key behaviour is identical.

**Tests**:
- `"it suppresses the canvas (terminal native bg) under NO_COLOR"` (vhs capture, `NO_COLOR=1`)
- `"it skips OSC 11 detection and the first-paint wait under NO_COLOR"`
- `"it renders colourless, conveying state via glyphs + bold/dim"` (no colour-only state)
- `"it stays legible by construction on the terminal's native fg/bg"`
- `"it exposes a single colourless flag that canvas-dependent surfaces inherit"`
- `"it preserves Sessions navigation/selection/filter behaviour under NO_COLOR"`

**Edge Cases**:
- Skips detection + first-paint wait — there is no canvas to select under `NO_COLOR`, so the OSC 11 query and its timeout are skipped.
- No canvas painted — both the leaf `.Background(canvas)` and the outer fill suppress the background; render on native bg.
- State via glyph + bold/dim — because state is never colour-only (§2.2), the colourless path is usable; the `✓`/`⚠` glyphs keep success/warning distinct without colour.
- Legible-by-construction — the legibility guarantee is the terminal's own default fg-on-bg; this is the one carve-out to the single owned-canvas path.

**Context**:
> §2.5: Portal honours `NO_COLOR` and monochrome terminals — renders colourless, leaning on glyph-backed state (§2.2) + bold/dim. Under `NO_COLOR`, Portal paints no canvas at all — it renders on the terminal's native fg/bg. This is the one documented carve-out to the single owned-canvas render path (§1): a second, distinct, colourless render path whose legibility guarantee is the terminal's own defaults. The carve-out applies to every canvas-dependent surface — the modal blank-screen clears to native bg, notice bands drop tint/bar colour (band stays present via `▌` + position + glyph + bold/dim), preview chrome renders colourless on native bg.
> §2.6: `NO_COLOR` skips detection and its first-paint wait entirely (no canvas to select).
> §1: opaque-only in v1; `NO_COLOR` is the one carve-out.
> Codebase: task 1-2 verified lipgloss/termenv `NO_COLOR` foreground-stripping is intact under v2; this task adds canvas-background suppression + the skip-detection branch. The outer wrap point is the one added in task 1-6 in `model.go` `View()`.
> Later phases inherit this carve-out; here prove it on the foundation Sessions screen only.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §2.5 (NO_COLOR / monochrome carve-out), §1 (opaque-only v1, NO_COLOR the one carve-out), §2.2 (state never carried by hue alone), §2.6 (NO_COLOR skips detection).

## spectrum-tui-design-1-9 | approved

### Task spectrum-tui-design-1-9: In-terminal contrast-floor validation & lock-in/bail gate — pin + eyeball light surface tints against `#e1e2e7`

**Problem**: The colour direction is a **hypothesis until prototyped in a real terminal** (§16.5, §1) — the numeric contrast check (task 1-4) is necessary but insufficient, especially for the recurring failure class of a **light tint on a light canvas** (§2.9 / §15.6). The light surface tints (`bg.selection` `#D0C6F0`, `bg.warning`, `bg.track`, light borders `#C9CDDB`) are explicitly left provisional by task 1-4 and must be **pinned and eyeballed** against `#e1e2e7` before implementation closes. This is the anti-sunk-cost lock-in gate: bail is a legitimate recorded outcome if the direction doesn't clear the bar.

**Solution**: Run the in-terminal validation/lock-in gate: pin the four light surface tints (derived from their dark anchor + the surface they render on, not invented), eyeball each against `#e1e2e7` in a real terminal in both modes, confirm every foreground-on-tint pairing clears the floor, and record the lock-in (or bail) decision. This task's acceptance **is** the eyeball pass in a real terminal in both modes; it carries its own vhs capture + Paper-frame compare in BOTH modes.

**Outcome**: The four light surface tints are pinned to concrete hexes (`bg.selection` `#D0C6F0` confirmed or remedied; `bg.warning` and `bg.track` light values pinned; light borders `#C9CDDB` confirmed), each eyeballed against `#e1e2e7` and confirmed legible (not just numerically); every foreground-on-tint pairing clears the floor; and a lock-in (or bail) decision is recorded with its rationale.

**Do**:
- Pin the provisional light surface tints from task 1-4 to concrete hexes, each **derived from its dark anchor + the surface it renders on** (not invented): `bg.selection` `#D0C6F0` (selected-row tint), `bg.warning` (warning-flash band — the light-amber value §2.9 deferred), `bg.track` (loading-bar empty track — the light-grey value §2.9 deferred), and the light borders `#C9CDDB` (`border.separator`/`border.footer`).
- Eyeball each pinned light tint against `#e1e2e7` in a **real terminal** (not just numerically): the recurring failure class is a light tint on a light canvas — a numeric pass is insufficient (§2.9, §15.6). Render each surface (selected row, warning band, loading track, the separator/footer rules) on the light canvas and visually confirm the tint reads as a distinct surface, not a wash-out.
- Confirm every **foreground-on-tint** pairing clears the floor in-terminal: selected-row foregrounds (name `text.on-selection`, count `text.strong`, attached `state.green` — the attached-only rule holds and green-on-`bg.selection` must clear the floor) on `bg.selection`; `text.on-warning` on `bg.warning`. Where a pairing dips, apply the remedy rule (§2.9): adjust toward more contrast (darken/saturate the light tint or move the on-band text token) — **never lower the floor**; co-tune the pair so both clear simultaneously.
- Also eyeball the remaining light per-screen token wiring against `#e1e2e7` on the foundation screen (per §15.6's "per-screen token wiring in light mode" — the full per-modal light eyeball of rename/edit/help lands with those surfaces in later phases, but the foundation Sessions screen's light wiring is confirmed here).
- Record the **lock-in/bail decision** explicitly (a committed note/log in the harness or the planning artefact): either "direction locked — every tint pinned and eyeballed, every pairing clears the floor" with the final pinned hexes, or "bail — <which tint/pairing failed and why no remedy clears the bar>". Bail is a legitimate recorded outcome (§16.5, the anti-sunk-cost gate, §1).
- Produce the vhs captures in BOTH modes: capture the foundation Sessions screen (with a selected row showing the selection tint + on-tint foregrounds) in dark and light, place each beside its committed Paper reference (`Sessions — Modern Vivid v2` dark / `Sessions — Modern Vivid (Light)` light), and judge layout/structure/colour-role match.

**Acceptance Criteria**:
- [ ] **(VISUAL — this task's acceptance IS the in-terminal eyeball pass in BOTH modes; carries its own vhs capture + Paper-frame compare in dark AND light)** The four light surface tints (`bg.selection` `#D0C6F0`, `bg.warning`, `bg.track`, light borders `#C9CDDB`) are pinned to concrete hexes and **eyeballed** against `#e1e2e7` in a real terminal — each reads as a distinct surface, not a wash-out (numeric pass alone is insufficient).
- [ ] Every foreground-on-tint pairing (selected-row name/count/attached on `bg.selection`; `text.on-warning` on `bg.warning`) clears the contrast floor in-terminal; any remedy applied is the more-contrast direction, never a lowered floor, with the pair co-tuned to clear simultaneously.
- [ ] Each pinned tint is derived from its dark anchor + the surface it renders on (recorded), not invented.
- [ ] The lock-in (or bail) decision is recorded explicitly with the final pinned hexes (lock-in) or the failing tint/pairing and rationale (bail).
- [ ] vhs captures of the foundation Sessions screen in dark and light each match `Sessions — Modern Vivid v2` / `Sessions — Modern Vivid (Light)` for layout/structure/colour-role.
- [ ] **(§12.3 validation caveat)** During the in-terminal validation pass, confirm `Ctrl+↑`/`Ctrl+↓` (the paging chords bound in task 2-1) are actually delivered to Portal and not swallowed by the terminal or tmux (notably tmux passthrough); if either chord is intercepted, record the finding and choose a fallback page key, and flag that fallback for tasks 2-1 / 3-3 / 4-7 (the descriptor + keymap consumers) to adopt.

**Tests**:
- `"it pins the four light surface tints to concrete hexes derived from their dark anchor + surface"`
- `"it eyeballs each light tint against #e1e2e7 in a real terminal (light-tint-on-light-canvas not a wash-out)"`
- `"it confirms selected-row name/count/attached on bg.selection clear the floor in light mode"`
- `"it confirms text.on-warning on bg.warning clears the floor in light mode"`
- `"it applies the more-contrast remedy (never a lowered floor) where a pairing dips, co-tuning the pair"`
- `"it records the lock-in/bail decision with final pinned hexes or a documented bail rationale"`
- `"it captures the foundation Sessions screen in both modes vs the named Paper frames"`

**Edge Cases**:
- Light-tint-on-light-canvas (recurring failure, numeric insufficient) — the central risk; each light surface tint must be eyeballed against `#e1e2e7`, not just numerically cleared.
- Text-on-tint pairs verified vs the tint — selected-row foregrounds and warning-band text are measured against the tint they sit on, co-tuned to clear simultaneously (the §2.9 two-knob rule).
- Remedy = more contrast, never lower the floor — darken/saturate the light tint or move the on-band text token; never relax the gate.
- Bail is legitimate — if no remedy clears the bar, recording a bail is a valid outcome (§16.5 / §1 anti-sunk-cost gate), not a failure to complete the task.
- `state.green` attached marker on `bg.selection` — the attached-only rule holds even on the selected row; green-on-`bg.selection` must clear the floor (the §4.1 foreground-on-tint requirement).

**Context**:
> §2.9: light surface tints finalised at §15 — `bg.selection` (`#D0C6F0`), `bg.warning`, `bg.track`, light borders (`#C9CDDB`) pinned and eyeballed against `#e1e2e7`, each derived from its dark anchor + the surface it renders — not invented. A numeric pass alone is insufficient; the light-tint-on-light-canvas case is the recurring risk. Contrast re-verification (the canvas pass): remedy = adjust toward more contrast (darken/saturate a light variant on `#e1e2e7`), never drop the floor. Text-carrying tints co-tuned with their on-band text token — the pair (tint + on-band text) measured, both clear simultaneously.
> §15.6: two residual light-mode checks are an explicit implementation task at the §15 gate — (1) pin + eyeball each light surface tint against `#e1e2e7` (numeric insufficient), (2) eyeball the remaining light modal/edit states and the per-screen token wiring in light mode (the full per-modal eyeball lands with those surfaces; the foundation screen wiring is confirmed here).
> §16.5: the colour direction is a hypothesis until prototyped in a real terminal — the in-terminal validation gate is the final lock before implementation closes; bail remains a legitimate outcome if the direction doesn't clear the bar.
> §1: bailing is a legitimate outcome if no direction clears the bar — an explicit anti-sunk-cost gate (objective floor + subjective read).
> §4.1: selection/warning tints must keep every selected-row foreground (name, count, attached bullet) above the floor; verified against the tints in addition to the §2.3 canvas gate.
> This task depends on tasks 1-3/1-4 (tokens + numeric floor), 1-6 (canvas paint), and 1-7 (detection) so a real terminal can render both modes for the eyeball.

**Spec Reference**: `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md` §2.3 (contrast floor), §2.9 (canvas-pass re-verification + "Light surface tints finalised at §15"), §15.6 (light-mode per-token check), §16.5 (lock-in gate, bail legitimate), §1 (anti-sunk-cost gate), §4.1 (foreground-on-tint pairings).
