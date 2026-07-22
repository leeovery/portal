---
phase: 1
phase_name: Banner Split by Identity Shape
total: 3
---

## persistent-no-host-terminal-banner-1-1 | approved

### Task 1.1: Add IsNull() discriminator to unsupportedBannerActive() so NULL keeps the standard header and signpost

**Problem**: The proactive unsupported-terminal banner permanently replaces the `Sessions ··· N` section header (count + grouping-mode suffix) for the *entire* picker session on **any** unsupported resolution — including NULL/remote (mosh/SSH) clients, where the banner (`⚠ no host-local terminal`) carries nothing actionable (no bundle id, no `see docs`). A remote user loses their session count and grouping indicator for the whole session in exchange for a dead label. The gate `unsupportedBannerActive()` reads the coarse `DetectUnsupported()` resolution and is blind to the NULL/named identity split that the *renderer* already knows.

**Solution**: Add an identity-shape discriminator so `unsupportedBannerActive()` is true **only for a named-unsupported identity**, false for NULL/remote. The predicate becomes `DetectUnsupported() && !multiSelectMode && !m.detectIdentity.IsNull()`. This single predicate is read by both consumers — `applySectionHeader` (swaps the banner in) and `activeNoticeBand` (suppresses the By-Tag "no tags yet" signpost while the banner is active) — so the one-line gate change fixes both surfaces coherently: NULL no longer claims the header row, and a NULL client with no tags shows the signpost again.

**Outcome**: On a resolved-unsupported **NULL/remote** client the standard `Sessions ··· N` header renders normally and the By-Tag "no tags yet" signpost behaves as on any supported client; on a resolved-unsupported **named** client (e.g. `com.apple.Terminal`) the banner and its `see docs` hint are unchanged; supported and in-flight-detection states are untouched.

**Do**:
- Edit `unsupportedBannerActive()` in `internal/tui/model.go` (~L4681). Change the return from `return m.DetectUnsupported() && !m.multiSelectMode` to `return m.DetectUnsupported() && !m.multiSelectMode && !m.detectIdentity.IsNull()`.
- Update the doc comment above `unsupportedBannerActive()` (~L4673-4680): it currently claims the banner owns the row for "a NULL remote/mosh identity OR a non-NULL undriven identity ... the resolution-based DetectUnsupported test, NOT IsNull". Flip it to state the banner is **named-only** — the added `!IsNull()` discriminator means NULL/remote no longer claims the header row (the standard `Sessions ··· N` header renders for NULL).
- Update the inline comment in `applySectionHeader` at the `if m.unsupportedBannerActive()` branch (`internal/tui/model.go` ~L4773-4779): it references the `⚠ no host-local terminal (NULL)` banner as a possible render — reword to reflect that only the named `⚠ unsupported terminal — <name> · <bundleID>` banner is reachable through this gate now (the NULL render branch's removal is Task 1.2 — reference it, do not do it here).
- Rework `TestApplySectionHeader_UnsupportedNullShowsHonestLine` in `internal/tui/unsupported_banner_test.go` (~L241): it currently asserts a resolved NULL identity renders `⚠ no host-local terminal` with `Sessions` absent. Invert it — a resolved NULL identity must now render the standard `Sessions` header (assert `Sessions` present; assert `no host-local terminal` / `unsupported terminal` / `see docs` absent). Keep the `unsupportedResolvedModel(t, spawn.Identity{})` setup and the `if !m.DetectUnsupported()` precondition (still true — the resolution is still unsupported; only the *banner* gate flips). Rename it to reflect the new contract (e.g. `TestApplySectionHeader_UnsupportedNullShowsStandardHeader`).
- Add a NULL-signpost-return test in `internal/tui/unsupported_banner_test.go` mirroring `TestActiveNoticeBand_SuppressesSignpostWhenUnsupported` (~L326, which uses a *named* identity and stays valid): seed the resolved-unsupported detection cache on `signpostModel(t)` with a **NULL** identity (`m.detectIdentity = spawn.Identity{}`, `m.detectResolution = spawn.ResolutionUnsupported`, `m.detectResolved = true`), assert `m.unsupportedBannerActive()` is now **false**, and assert `activeNoticeBand()` returns `ok == true` (the signpost owns the slot again).
- Do **not** touch the detection cache / dispatch latch, `rebuildSessionList`, or add any re-detection — the once-only detection cache is not the defect (see Context).

**Acceptance Criteria**:
- [ ] `unsupportedBannerActive()` returns `false` for a resolved-unsupported NULL identity (empty `BundleID`) and `true` for a resolved-unsupported named identity (non-empty `BundleID`), both outside multi-select mode.
- [ ] On a resolved-NULL model, `applySectionHeader` renders the standard `Sessions ··· N` header (no banner) at the section-header row.
- [ ] On a resolved-NULL By-Tag model with zero tags anywhere, `activeNoticeBand()` returns `ok == true` (the "no tags yet" signpost owns the slot).
- [ ] On a resolved-named model, the banner (`⚠ unsupported terminal — <name> · <bundleID>` + `see docs`) still replaces the header, and the signpost is still suppressed — existing named-path tests stay green (`TestApplySectionHeader_UnsupportedShowsBanner`, `TestActiveNoticeBand_SuppressesSignpostWhenUnsupported`).
- [ ] In-flight detection (`detectDispatched && !detectResolved`) and supported terminals still render the standard header — existing tests stay green (`TestApplySectionHeader_InFlightShowsStandardHeader`, `TestApplySectionHeader_SupportedShowsStandardHeader`).
- [ ] No new detection dispatch, cache invalidation, or re-detection is introduced; `rebuildSessionList` is unchanged.
- [ ] `go test ./internal/tui/...` passes (unit lane — no tmux daemon spawned).

**Tests**:
- `"it renders the standard Sessions header for a resolved NULL identity"` (reworked `TestApplySectionHeader_UnsupportedNullShowsHonestLine` → `..._ShowsStandardHeader`).
- `"it returns the By-Tag no-tags signpost for a resolved NULL identity"` (new — mirror of the named suppression test with `ok == true` for NULL).
- `"it keeps the named unsupported banner and see docs unchanged"` (existing `TestApplySectionHeader_UnsupportedShowsBanner` as regression guard).
- `"it suppresses the signpost for a resolved named identity"` (existing `TestActiveNoticeBand_SuppressesSignpostWhenUnsupported` stays valid).
- `"it shows the standard header while detection is in flight"` (existing `TestApplySectionHeader_InFlightShowsStandardHeader`).
- `"it shows the standard header for a supported terminal"` (existing `TestApplySectionHeader_SupportedShowsStandardHeader`).

**Edge Cases**:
- **Multi-select mode still outranks**: the `&& !m.multiSelectMode` clause is retained, so the predicate is false in multi-select mode for both shapes (the `N selected` banner owns the row). `TestApplySectionHeader_MultiSelectStepsUnsupportedAside` (named-in-mode) stays green; NULL-in-mode is trivially false (both clauses false).
- **In-flight / unresolved detection**: `DetectUnsupported()` is false until `detectResolved`, so the standard header renders regardless of identity shape during the async window (unchanged).
- **NULL + zero-tags By-Tag signpost**: with the banner gate now false for NULL, the signpost re-owns the notice slot (correct — nothing competes for it).
- **Named identity banner + `see docs` unchanged**: the named path still activates the banner and its docs hint; no renderer change here.
- **Detection cache left untouched**: no re-detection on rebuild; the once-only `detectDispatched`/`detectResolved` cache is not modified — the gate only decides *whether* the cached unsupported resolution renders as the banner.

**Context**:
> Spec §2 (Sub-fix 1 — Banner Split by Identity Shape): "Currently it is `DetectUnsupported() && !multiSelectMode`, which fires for *any* unsupported resolution; the new form additionally requires `!m.detectIdentity.IsNull()`." The renderer already branches on the NULL/named split (`bundleID == ""`); only the gate was blind to it.
> Spec §2 "Scope guard — the detection cache is not the defect": "The `!IsNull()` gate change alone fully resolves the NULL symptom; the once-only detection cache is **not** itself a defect and must be left untouched (do not add re-detection on rebuild)."
> `DetectUnsupported()` (`internal/tui/spawn_detect.go`) is TRUE for both a NULL and a non-NULL undriven identity — so `IsNull()` alone is not the resolution test; the discriminator is layered *on top of* `DetectUnsupported()`. `Identity.IsNull()` (`internal/spawn/identity.go`) is defined solely as `BundleID == ""`.
> `unsupportedBannerActive()` is the SINGLE predicate read by both `applySectionHeader` (banner swap) and `activeNoticeBand` (signpost suppression, `internal/tui/notice_band.go` ~L371) — one gate change fixes both surfaces.
> The now-unreachable NULL render branch inside `renderUnsupportedHeader` / `unsupportedLeftCluster` is deleted in Task 1.2, not here; after this task it remains present-but-unreachable-from-the-gate, and its direct render test (`TestUnsupportedHeader_NullIdentityNoHostLocal`) still passes.

**Spec Reference**: `/Users/leeovery/Code/portal/.workflows/persistent-no-host-terminal-banner/specification/persistent-no-host-terminal-banner/specification.md` §2 (Sub-fix 1), §7 (Testing Requirements — banner-split new coverage + rework of `TestApplySectionHeader_UnsupportedNullShowsHonestLine`).

## persistent-no-host-terminal-banner-1-2 | approved

### Task 1.2: Remove the unreachable NULL banner render branch (renderers named-only, see docs unconditional)

**Problem**: After Task 1.1 the banner gate (`unsupportedBannerActive()`) never fires for a NULL identity, so the NULL render path in the section-header renderers is unreachable from the picker (the gate is its only caller). The `unsupportedNullLabel = "no host-local terminal"` constant, the `bundleID == ""` branches in `unsupportedLeftCluster` / `renderUnsupportedHeader`, and their direct render test are dead code carrying the last live copy of the `no host-local terminal` jargon string — they would rot if retained.

**Solution**: Delete the NULL render branch so the two renderers are **named-only** (`bundleID != ""` always holds when reached), making the `see docs` hint unconditional. Remove the constant, the branch conditions, and the render-level test that exercised the dead branch; convert the ExactlyOneRow "null" table subcase to the named-only shape.

**Outcome**: `unsupportedLeftCluster` and `renderUnsupportedHeader` render only the named banner (amber `⚠ unsupported terminal — <name> · <bundleID>` + unconditional blue `see docs`); no `no host-local terminal` / `unsupportedNullLabel` reference remains anywhere in the tree; the named banner render tests stay green.

**Do**:
- In `internal/tui/section_header.go`, delete the `unsupportedNullLabel = "no host-local terminal"` constant (~L61-65, inside the `const (...)` block) and its doc comment.
- In `unsupportedLeftCluster` (`internal/tui/section_header.go` ~L223-232), remove the `if bundleID == "" { return amber.Render(flashWarningGlyph + " " + unsupportedNullLabel) }` branch. Keep only the named path (amber `⚠ unsupported terminal` label + dim `— <name> · <bundleID>` identity joined horizontally). Update the function doc comment to drop the NULL-branch description — the cluster is now named-only.
- In `renderUnsupportedHeader` (`internal/tui/section_header.go` ~L178-187), remove the `var hint string; if bundleID != "" { hint = ... }` conditional and render the `see docs` hint **unconditionally**: `hint := headerStyle(theme.MV.AccentBlue, mode, colourless).Render(unsupportedDocsHint)`. Update the function doc comment to drop the NULL branch and state the `see docs` hint is now always present (named-only renderer).
- In `internal/tui/unsupported_banner_test.go`, delete `TestUnsupportedHeader_NullIdentityNoHostLocal` (~L86-107) — it exercises the removed branch directly.
- In `internal/tui/unsupported_banner_test.go`, convert `TestUnsupportedHeader_ExactlyOneRow` (~L131-144): drop the `{"null", ""}` table entry so only the named case (`{"named", "com.apple.Terminal"}`) remains (the renderer is named-only — a `bundleID == ""` call is no longer a supported shape). Keep the single-row height assertion for the named case.
- Update the file-level comment block at the top of `internal/tui/unsupported_banner_test.go` (~L13-23) which describes the NULL branch (`⚠ no host-local terminal`) — reword so it describes the named-only banner (the NULL identity is no longer a banner case; it renders the standard header, covered by Task 1.1's test).
- Grep the tree to confirm no remaining reference: `no host-local terminal`, `unsupportedNullLabel`.

**Acceptance Criteria**:
- [ ] `unsupportedNullLabel` no longer exists in the codebase (`grep -rn 'unsupportedNullLabel' internal/` returns nothing).
- [ ] The string `no host-local terminal` no longer exists in the codebase (`grep -rn 'no host-local terminal' .` returns nothing).
- [ ] `unsupportedLeftCluster` has no `bundleID == ""` branch and `renderUnsupportedHeader` renders `see docs` unconditionally.
- [ ] `renderUnsupportedHeader("Apple Terminal", "com.apple.Terminal", …)` still produces the amber label + dim identity + blue `see docs`, single row — `TestUnsupportedHeader_NamedIdentityAmberDimSeeDocs`, `TestUnsupportedHeader_RightAlignedSeeDocs`, `TestUnsupportedHeader_ExactlyOneRow` (named-only), `TestUnsupportedHeader_NarrowDegradeDropsHint`, `TestUnsupportedHeader_ColourlessGlyphBacked`, `TestUnsupportedHeader_PaintsCanvasNoEdgeBleed` all stay green.
- [ ] `TestUnsupportedHeader_NullIdentityNoHostLocal` is removed and `TestUnsupportedHeader_ExactlyOneRow` no longer has a `null` subcase.
- [ ] `go build ./...` and `go test ./internal/tui/...` pass (unit lane — no tmux daemon).

**Tests**:
- `"it renders the named unsupported banner with an unconditional see docs hint"` (existing `TestUnsupportedHeader_NamedIdentityAmberDimSeeDocs` as regression guard).
- `"it renders exactly one row for the named banner"` (converted `TestUnsupportedHeader_ExactlyOneRow`, named-only).
- `"it drops the see docs hint on narrow width"` (existing `TestUnsupportedHeader_NarrowDegradeDropsHint`).
- `"it removes the dead NULL render branch and its direct test"` (verified by deletion + grep-clean acceptance).

**Edge Cases**:
- **Renderers only reached with non-empty bundleID**: after the gate change (Task 1.1) the picker only ever calls the renderers for a named identity, so removing the `bundleID == ""` branch cannot regress a live render path.
- **`see docs` always present**: the hint is now unconditional; the named banner always carries it (its actionable `terminals.json` pointer).
- **ExactlyOneRow "null" subcase converted/removed**: the table loses its `{"null", ""}` entry; a `bundleID == ""` render is no longer a supported call shape.
- **No remaining `no host-local terminal` / `unsupportedNullLabel` reference**: grep-clean is an explicit acceptance gate (aligns with the plain-language rewrite — this deletes the last live copy of the jargon string).

**Context**:
> Spec §6 (Dead NULL Banner Render Branch — Removed): "With the NULL banner gate dropped (Topic 2), the NULL render path is unreachable from the picker (the gate is its only caller) and is deleted: the `unsupportedNullLabel = "no host-local terminal"` constant; the `bundleID == ""` branches in `unsupportedLeftCluster` and `renderUnsupportedHeader`; the render-level test that exercises them directly (`TestUnsupportedHeader_NullIdentityNoHostLocal`). After removal the two renderers are **named-only** — `bundleID != ""` always holds when they are reached, so the `see docs` hint is unconditional. Retaining the branch as defensive dead code was rejected — it is genuinely unreachable and would rot."
> Spec §7 (Testing — Remove): "The render-level `TestUnsupportedHeader_NullIdentityNoHostLocal` — it exercises the deleted NULL render branch (Topic 6)."
> **Ordering dependency**: this task must land *after* Task 1.1. Task 1.1's gate change is what makes the NULL branch unreachable; removing the branch before the gate change would break a resolved-NULL render (it would emit a malformed named-shaped banner with empty identity) and fail Task 1.1's not-yet-reworked NULL test.

**Spec Reference**: `/Users/leeovery/Code/portal/.workflows/persistent-no-host-terminal-banner/specification/persistent-no-host-terminal-banner/specification.md` §6 (Dead NULL Banner Render Branch — Removed), §7 (Testing Requirements — Remove).

## persistent-no-host-terminal-banner-1-3 | approved

### Task 1.3: Add the sessions-unsupported-null capture fixture + committed reference PNG

**Problem**: The banner-split fix has no visual regression anchor for the NULL/remote case. There is a `sessions-unsupported-terminal` (named) fixture and reference PNG, but nothing captures the resolved-unsupported **NULL** seed path proving no banner intrudes — a future change re-adding a NULL banner would slip through the visual gate.

**Solution**: Add a `sessions-unsupported-null` capture fixture that seeds the detection cache with an empty `spawn.Identity{}` (`IsNull()` true) via the existing `InitialDetection` seed seam, register it in the fixture catalog, add a vhs tape modelled on the existing fixtures, and capture the committed reference PNG. The frame renders the standard `Sessions ··· N` header with no banner — visually identical to `sessions-flat` — and serves as the parity/regression anchor.

**Outcome**: `go run ./cmd/capturetool --fixture sessions-unsupported-null` renders the standard `Sessions ··· 12` header (no banner) from a resolved-unsupported NULL detection cache; the fixture is discoverable via `FixtureNames()` / `FixtureByName`; `testdata/vhs/sessions-unsupported-null.png` is committed and deterministically reproducible.

**Do**:
- Add a `sessionsUnsupportedNullFixture()` builder in `internal/capture/fixtures.go`, modelled on `sessionsUnsupportedTerminalFixture()` (~L462): reuse `sessionsFlatFixture()`, set `fx.name = "sessions-unsupported-null"`, and set `fx.initialDetection = &spawn.Identity{}` (empty `BundleID` → `IsNull()` true → resolves unsupported NULL). Write a doc comment noting it renders the **standard `Sessions ··· N` header with no banner** (visually identical to `sessions-flat`), seeded via the same `InitialDetection` seam the named fixture uses; the reference PNG is a regression anchor that the resolved-unsupported NULL seed path does not intrude a banner.
- Register the fixture: add `case "sessions-unsupported-null": return sessionsUnsupportedNullFixture(), nil` to `FixtureByName` (`internal/capture/fixtures.go` ~L155) and add `"sessions-unsupported-null"` to the `names` slice in `FixtureNames()` (~L200).
- Add a Go registry/seed-path test in `internal/capture/capture_test.go` (mirror the existing per-fixture tests, e.g. `TestSessionsEmptyFixture` / `TestFixtureNamesIncludesSessionsEmpty`): resolve `FixtureByName("sessions-unsupported-null")`, build the model via `tui.Build(fx.Deps())`, and assert `m.DetectResolved()` is true, `m.DetectUnsupported()` is true, `m.DetectedIdentity().IsNull()` is true, and `m.SessionListTitle() == "Sessions"` (Flat mode) — proving the resolved-unsupported NULL seed path is wired and the standard title holds. Also assert `FixtureNames()` includes `"sessions-unsupported-null"`.
- Add the tape `testdata/vhs/sessions-unsupported-null.tape`, modelled on `testdata/vhs/sessions-unsupported-terminal.tape`: same `FontFamily "JetBrains Mono"` / `FontSize 16` / `Width 1280` / `Height 800` / `Set Shell "bash"`; `Output "testdata/vhs/.gifcache/sessions-unsupported-null.gif"`; `Type "go run ./cmd/capturetool --fixture sessions-unsupported-null"` + `Enter`; `Sleep 4s` (cold-cache compile + first paint); `Screenshot "testdata/vhs/sessions-unsupported-null.png"` (quote all slashed paths).
- Capture the PNG: run `vhs testdata/vhs/sessions-unsupported-null.tape` from the project root with the sandbox disabled (vhs needs loopback networking for ttyd — Bash `dangerouslyDisableSandbox: true`). This writes `testdata/vhs/sessions-unsupported-null.png`.
- **Verify a fresh write before trusting the capture** (VHS silent-write flake): capture the file's hash, re-run the tape, and confirm the PNG hash *changed from any pre-run/placeholder state* and that two consecutive runs produce byte-identical PNGs (`shasum -a 256 testdata/vhs/sessions-unsupported-null.png` twice → equal). If the hash does not change on the first real run, re-run before trusting.
- Open the resulting PNG and confirm the frame shows the standard `Sessions ··· 12` header (count + no mode suffix, Flat) and **no** `⚠` banner — visually identical to `sessions-flat.png`.
- Do **not** add a `reference/*-mv.png` Paper oracle (this frame has no distinct Paper design — it is identical to `sessions-flat`, a design residual; the captured `sessions-unsupported-null.png` is itself the committed regression anchor). Do **not** add a NO_COLOR variant.

**Acceptance Criteria**:
- [ ] `FixtureByName("sessions-unsupported-null")` resolves without error and `FixtureNames()` includes `"sessions-unsupported-null"`.
- [ ] The fixture's built model has `DetectResolved() == true`, `DetectUnsupported() == true`, `DetectedIdentity().IsNull() == true`, and `SessionListTitle() == "Sessions"`.
- [ ] `testdata/vhs/sessions-unsupported-null.tape` exists and follows the established tape shape (quoted slashed paths, fixed font/dims, `go run ./cmd/capturetool --fixture sessions-unsupported-null`, `Sleep 4s`, `Screenshot`).
- [ ] `testdata/vhs/sessions-unsupported-null.png` is committed, freshly written (hash-verified), and two consecutive tape runs produce byte-identical PNGs (determinism gate).
- [ ] The captured frame renders the standard `Sessions ··· N` header with no `⚠` banner (visually identical to `sessions-flat`).
- [ ] `go test ./internal/capture/...` passes (unit lane — the harness opens no tmux server and reads no `~/.config/portal`).
- [ ] No `reference/sessions-unsupported-null-*.png` Paper oracle and no NO_COLOR variant are added.

**Tests**:
- `"it resolves the sessions-unsupported-null fixture and seeds a resolved NULL detection cache"` (new `capture_test` — `DetectUnsupported()` true + `DetectedIdentity().IsNull()` true + title `"Sessions"`).
- `"it includes sessions-unsupported-null in FixtureNames"` (new `capture_test`, mirror of `TestFixtureNamesIncludesSessionsEmpty`).
- `"it captures a deterministic, banner-free NULL frame"` (vhs determinism gate — two runs byte-identical; human visual check: standard header, no banner).

**Edge Cases**:
- **Empty `Identity{}` → `IsNull()` true seed path**: the fixture seeds `&spawn.Identity{}` (empty `BundleID`), which resolves unsupported-NULL through the production resolver — the exact seed shape the banner-split gate must treat as "no banner".
- **No banner intrudes on the resolved-NULL seed**: the frame must render the standard header, not the (removed) NULL banner — this is the whole point of the regression anchor.
- **Offline harness**: the capture opens no tmux server and reads no `~/.config` (in-memory fakes, `CWD: "/home/user"`) — determinism is the gate.
- **Fresh PNG write (VHS silent-write flake)**: verify the PNG hash changed / is freshly written and re-run to confirm byte-identical output before trusting/committing the capture.
- **No NO_COLOR variant required**: unlike `sessions-unsupported-terminal` (which has a `-nocolor` tape), the NULL frame is standard-header parity with `sessions-flat` and needs no colourless variant.

**Context**:
> Spec §7 (Testing — Visual): "add a NULL-identity capture fixture — name `sessions-unsupported-null`, seeded via the existing detection seed seam with `InitialDetection = &spawn.Identity{}` (empty `BundleID` → `IsNull()` true), the same seam the named fixture uses. It renders the **standard `Sessions ··· N` header with no banner** — visually identical to `sessions-flat`. The **render-level banner-split test (Task 1.1) is the primary NULL assertion**; the fixture + committed reference PNG are added for parity with `sessions-unsupported-terminal` and as a regression anchor that the resolved-unsupported NULL seed path does not intrude a banner. The existing `sessions-unsupported-terminal` (named) fixture stays valid."
> `tui.Build` with `InitialDetection` seeds the resolved detection cache synchronously (`build_test.go` "initial detection seeds the resolved unsupported cache"), so `DetectResolved()`/`DetectUnsupported()` are true immediately after `Build` — a NULL seed additionally has `DetectedIdentity().IsNull() == true`.
> `testdata/vhs/README.md` "Adding a new fixture / screen": add the `case` + `FixtureNames` entry + builder, add a tape modelled on `sessions-flat.tape`, commit the captured `<name>.png`. Determinism (two runs byte-identical) is the automated gate; the layout/no-banner check is human/agent-judged. The orchestrator copies the fresh capture into `trail/` at commit time.
> `MEMORY: reference_vhs_capture_flaky_write` — VHS captures silently fail to write the PNG; verify a fresh write (hash changed) + retry before trusting/pixel-checking a capture.

**Spec Reference**: `/Users/leeovery/Code/portal/.workflows/persistent-no-host-terminal-banner/specification/persistent-no-host-terminal-banner/specification.md` §7 (Testing Requirements — Visual).
