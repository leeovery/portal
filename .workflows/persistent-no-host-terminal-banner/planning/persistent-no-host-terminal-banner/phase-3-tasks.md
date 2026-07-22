---
phase: 3
phase_name: Plain-Language UnsupportedNoopMessage Rewrite + Copy Coherence
total: 2
---

## persistent-no-host-terminal-banner-3-1 | approved

### Task 3-1: Rewrite UnsupportedNoopMessage (both shapes) + lockstep copy assertions

**Problem**: `spawn.UnsupportedNoopMessage` (`internal/spawn/message.go`) renders the shared N≥2 unsupported-terminal atomic-no-op sentence in jargon — the NULL/remote shape reads `no host-local terminal — nothing opened` and the named shape reads `unsupported terminal — <name> · <bundleID> — nothing opened`. Spec §5 replaces both with plain language that states what happened and why. Because this one function is rendered by *two* surfaces — the picker's reactive backstop flash (`internal/tui/burst_progress.go`'s `unsupportedFlashText`, which calls it verbatim) and the CLI open-burst (Task 3-2) — and the picker's reactive-backstop tests pin the old strings as byte-literal `want` values, the function edit and every dependent literal in the spawn suite and the tui reactive-backstop suite must change together or the build goes red.

**Solution**: Rewrite both return shapes of `UnsupportedNoopMessage` to the spec §5 plain-language copy, and in the same commit update every copy assertion in the `internal/spawn` message suite and the `internal/tui` reactive-backstop suite that encodes the old strings, so the function plus both suites land in one green commit.

**Outcome**: `UnsupportedNoopMessage(Identity{})` returns `can't open new windows over a remote connection — nothing opened`; `UnsupportedNoopMessage(Identity{Name:"Apple Terminal", BundleID:"com.apple.Terminal"})` returns `can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened`; `go test ./internal/spawn/... ./internal/tui/...` (unit lane, no tmux/daemon) passes.

**Do**:
- In `internal/spawn/message.go`, inside `UnsupportedNoopMessage`, change the NULL branch return from `"no host-local terminal — nothing opened"` to `"can't open new windows over a remote connection — nothing opened"`, and change the named branch `fmt.Sprintf` template from `"unsupported terminal — %s · %s — nothing opened"` to `"can't open new windows in %s · %s — nothing opened"` (keep the `id.Name`, `id.BundleID` args and the `·` U+00B7 middle-dot separator between them; keep the trailing ` — nothing opened` clause with its U+2014 em-dash; the apostrophe in `can't` is a straight ASCII apostrophe U+0027, not a typographic ’).
- Update the `UnsupportedNoopMessage` doc comment (the `// ...` block immediately above the function) so it describes the new plain-language copy instead of the old "honest 'no host-local terminal — nothing opened' line" / "unsupported terminal — ..." wording. Keep the doc's factual notes that both callers render through it and that the body carries no `spawn:` prefix and no ⚠ glyph.
- In `internal/spawn/message_test.go`, in `TestUnsupportedNoopMessage`, replace the NULL subtest `want` from `"no host-local terminal — nothing opened"` to `"can't open new windows over a remote connection — nothing opened"`, and the named subtest `want` from `"unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened"` to `"can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened"`. Adjust the two subtest name strings if they name the old copy (e.g. `"it renders the honest no-host-local body for a NULL identity"`) so they describe the new plain-language body.
- In `internal/tui/burst_unsupported_noop_test.go`, replace **every** occurrence of the two old literals with the new ones: the named literal `"unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened"` (appears in the `TestUnsupportedFlashText` table row, `TestBurstUnsupported_NonNullAtomicNoOp`, and `TestBurstUnsupported_DeferredThenUnsupported`) → `"can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened"`; and the NULL literal `"no host-local terminal — nothing opened"` (appears in the `TestUnsupportedFlashText` table row and `TestBurstUnsupported_NullFlash`) → `"can't open new windows over a remote connection — nothing opened"`. Update any adjacent stale comment prose in this file that names the old copy (e.g. "the honest no-host-local line", "the NULL form is the honest no-host-local line") to match the new wording. Do NOT change the test structure or entry-timing that Phase 2 established.
- Do NOT change `internal/tui/burst_progress.go` `unsupportedFlashText` — it delegates to `spawn.UnsupportedNoopMessage` and tracks the new copy automatically; it only needs the callee to change.
- Run `go build -o portal .` then `go test ./internal/spawn/... ./internal/tui/...` and confirm green.

**Acceptance Criteria**:
- [ ] `UnsupportedNoopMessage(spawn.Identity{})` returns exactly `can't open new windows over a remote connection — nothing opened`.
- [ ] `UnsupportedNoopMessage(spawn.Identity{Name:"Apple Terminal", BundleID:"com.apple.Terminal"})` returns exactly `can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened` (U+00B7 middle dot between name and bundle id; U+2014 em-dash before `nothing opened`).
- [ ] `internal/tui/burst_progress.go`'s `unsupportedFlashText(id)` (unchanged) returns the two new strings for the named and NULL inputs respectively, via delegation.
- [ ] `go test ./internal/spawn/...` passes: `TestUnsupportedNoopMessage` NULL + named subtests assert the new strings; `TestGoneMessage`, `TestPartialFailureMessage`, `TestQuoteJoin`, `TestGoneVerb` are unchanged and still pass.
- [ ] `go test ./internal/tui/...` passes: `TestUnsupportedFlashText`, `TestBurstUnsupported_NonNullAtomicNoOp`, `TestBurstUnsupported_NullFlash`, `TestBurstUnsupported_DeferredThenUnsupported` assert the new strings; `TestBurstUnsupported_SupportedStillDispatches` is unchanged and still passes.
- [ ] No production or test file still contains the literal `no host-local terminal — nothing opened` or `unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened` (the `— nothing opened`-suffixed user-facing forms are fully migrated).
- [ ] `internal/spawn/logemit.go` line `log.OrDiscard(logger).Info("unsupported terminal — nothing opened", …)` and its assertion in `internal/spawn/logemit_test.go` (`"INFO unsupported terminal — nothing opened resolution=unsupported …"`) are UNCHANGED — that is the observability log message, not the user-facing render.
- [ ] `GoneMessage` / `PartialFailureMessage` copy and the permission-guidance copy are UNCHANGED.
- [ ] Function edit + spawn-suite literals + tui reactive-backstop literals are all in one commit that builds and tests green (no intermediate red state committed).

**Tests**:
- `"it renders the plain remote-connection body for a NULL identity"` — `UnsupportedNoopMessage(Identity{})` == `can't open new windows over a remote connection — nothing opened` (folds in remote/mosh AND the transient-detection-error-to-`Identity{}` case: both are the same NULL input).
- `"it names the terminal and bundle id for a recognised identity"` — `UnsupportedNoopMessage(Identity{Name:"Apple Terminal", BundleID:"com.apple.Terminal"})` == `can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened`.
- `"TestUnsupportedFlashText"` (tui) — named + NULL rows return the two new strings through `unsupportedFlashText`, and neither embeds the ⚠ glyph (the band adds it).
- `"TestBurstUnsupported_NonNullAtomicNoOp"` / `"TestBurstUnsupported_DeferredThenUnsupported"` (tui) — `m.flashText` == the new named string after the atomic no-op / deferred-then-unsupported resolution.
- `"TestBurstUnsupported_NullFlash"` (tui) — `m.flashText` == the new NULL string after the atomic no-op.
- `"TestBurstUnsupported_SupportedStillDispatches"` (tui, guard) — supported (ghostty→native) still dispatches the burst and sets NO flash (unchanged).
- `"TestLogUnsupported"` (spawn, guard) — the observability line `unsupported terminal — nothing opened` remains asserted byte-identically (proves the log message was not swept into the user-facing rewrite).

**Edge Cases**:
- **NULL shape breadth.** `Identity{}` is the NULL identity for remote/mosh AND for a transient detection error folded to `Identity{}` (Phase 1 folds the error to the empty identity). One NULL input therefore pins both — no separate transient-error assertion is needed.
- **Named shape formatting.** Preserve the `<name> · <bundleID>` key with the U+00B7 middle dot and the single U+2014 em-dash before `nothing opened`. The named no-op message is the only place the CLI user sees the bundle id (the `terminals.json` key), so the identity must remain in the string.
- **Banner is not the no-op message.** The persistent named banner string `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal` (no `— nothing opened` suffix, rendered by the section-header path) is a DIFFERENT string, kept unchanged by spec §2/§5. Do not touch it. The `— nothing opened` suffix is the reliable discriminator between the no-op message (this task changes it) and the banner (untouched); a naïve `unsupported terminal — ` search-and-replace would wrongly hit the banner.
- **Observability log line is not the message.** `internal/spawn/logemit.go`'s `Info("unsupported terminal — nothing opened", …)` is a static log msg with structured attrs — no identity interpolation — and is explicitly out of scope (spec §5 "Out of scope (copy)"). Leave it and its `logemit_test.go` assertion alone.
- **Unrelated `no host-local terminal` occurrences stay.** After Phase 1 removed the dead NULL banner render branch, `no host-local terminal` still legitimately appears in DETECTION-layer strings and comments — e.g. `internal/spawn/detect.go` / `detect_test.go` (`"detection resolved no host-local terminal"`, a detection observability message), and conceptual comments in `detect_inside.go`, `walk.go`, `resolver.go`, `identity.go`. These are a different message and different concept; do NOT rewrite them. Only the two `— nothing opened`-suffixed user-facing forms migrate.
- **Apostrophe byte.** `can't` uses a straight ASCII apostrophe (U+0027), matching the golden spec bytes — not a typographic ’ (U+2019).

**Context**:
> Spec §5 copy set (golden): Reactive no-op (`spawn.UnsupportedNoopMessage`, async-race path + shared with the CLI open-burst) — NULL/remote: `can't open new windows over a remote connection — nothing opened`; named-unsupported: `can't open new windows in <name> · <bundleID> — nothing opened`.
>
> Spec §5 notes: "'can't open new windows' is the plain statement of what multi-select does and can't do here. 'nothing opened' stays — it is plain and honestly signals an attempt occurred (distinguishing the reactive no-op from the pre-emptive block, which says 'isn't available')." "The named reactive/CLI line keeps `<name> · <bundleID>` because in the CLI there is no banner — that line is the only place the user gets the bundle id (the `terminals.json` key)."
>
> Spec §5 scope: "`UnsupportedNoopMessage` is in scope. Rewriting it widens this bugfix into `internal/spawn`, and its wording is shared with the CLI open-burst (partly owned by `cli-verb-surface-redesign`)."
>
> Spec §5 out-of-scope: "Adjacent spawn messages for different scenarios — session killed mid-burst … a window failed to open … macOS permission guidance — are already plain and separately owned; not rewritten here."
>
> Sequencing (from the phase plan): This is the atomic copy rewrite. `unsupportedFlashText` (`internal/tui/burst_progress.go`) calls `UnsupportedNoopMessage`, and the tui reactive-backstop `want` literals go red the instant the string changes — so the function + spawn-suite literals + tui reactive-backstop literals MUST change in one green commit. Phase 2 already reworked the ENTRY TIMING of `TestBurstUnsupported_NonNullAtomicNoOp` / `TestBurstUnsupported_NullFlash` (enter multi-select before resolving detection) but kept their asserted copy strings byte-identical for Phase 3 to update now.
>
> Grounding note: line numbers cited in the phase table are against the pre-Phase-1/2 tree. Phases 1 and 2 execute before this task and shift lines in `internal/tui/burst_unsupported_noop_test.go`. Anchor the edits by grepping the exact old literals (`no host-local terminal — nothing opened` and `unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened`), not by line number.

**Spec Reference**: `.workflows/persistent-no-host-terminal-banner/specification/persistent-no-host-terminal-banner/specification.md` §5 (Unsupported-Terminal Copy — Plain-Language Rewrite), §7 (Testing Requirements — "Copy assertions" + rework of `burst_unsupported_noop_test.go`), §8 (Scope — `internal/spawn/message.go` in scope; adjacent spawn copy out of scope).

## persistent-no-host-terminal-banner-3-2 | approved

### Task 3-2: Lock CLI open-burst copy coherence with an explicit literal regression

**Problem**: The CLI open-burst renders the same shared `spawn.UnsupportedNoopMessage` on its N≥2 unsupported/NULL atomic no-op (`cmd/open_burst_run.go` returns `errors.New(spawn.UnsupportedNoopMessage(id))`). The existing CLI test `TestRunOpenBurst_UnsupportedTerminal_AtomicNoop` (`cmd/open_burst_run_test.go`) asserts the error text with `want := spawn.UnsupportedNoopMessage(id)` — it **self-references the shared function**, so it auto-tracks any wording change and silently accepts drift. After Task 3-1 rewrites the message, nothing in the CLI suite actually pins the rendered copy to the new plain-language strings, and the CLI's NULL shape has no copy coverage at all (the existing test uses a named identity). The coordination contract with `cli-verb-surface-redesign` (which shares ownership of this message and lands first) needs a real regression anchor on the CLI surface.

**Solution**: Add an explicit literal-copy regression to the CLI open-burst suite that pins the returned error string to the new plain-language literals — one assertion for the named shape and one for the NULL shape — independent of the shared function (byte-literal `want`, not `spawn.UnsupportedNoopMessage(id)`). No change to CLI block logic.

**Outcome**: The CLI open-burst suite fails if the rendered unsupported no-op copy drifts from the spec §5 strings on either shape; `go test ./cmd/...` (unit lane, no tmux/daemon) passes with the new literals.

**Do**:
- In `cmd/open_burst_run_test.go`, add a test (e.g. `TestRunOpenBurst_UnsupportedTerminal_CopyIsPlainLanguage`) that drives the same unsupported no-op path as `TestRunOpenBurst_UnsupportedTerminal_AtomicNoop` but asserts the returned error string against **byte-literal** `want` values, not `spawn.UnsupportedNoopMessage(id)`.
- Cover the **named** shape: build deps via `openBurstDepsForTest(appleTerminalIdentity(), spawn.ResolutionUnsupported, adapter, conn, mint.mint)`, run `runOpenBurstWithDeps(&cobra.Command{}, surfaces, nil, deps)` with two attach surfaces, and assert `err.Error() == "can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened"`.
- Cover the **NULL** shape (closes the gap called out in the phase table): build deps with `id := spawn.Identity{}` (empty — `IsNull()` true) and `spawn.ResolutionUnsupported`, run the same two-surface burst, and assert `err.Error() == "can't open new windows over a remote connection — nothing opened"`.
- Use a table or two subtests over `{id, wantError}` pairs so both shapes share one body; keep the surfaces as two `spawn.Surface{Kind: spawn.SurfaceAttach}` entries (N≥2). Keep the existing spies (NewBurster not built, no OpenWindow, no Connector.Connect, no LocalMint) or at minimum assert `err != nil` and the exact string.
- Leave `TestRunOpenBurst_UnsupportedTerminal_AtomicNoop` in place — it still validates the atomic-no-op behaviour (no burster, no half-connect). The new test adds the missing literal-copy pin; it does not replace the behaviour test.
- Do NOT change `cmd/open_burst_run.go` block logic — the CLI still detects, resolves, and returns `errors.New(spawn.UnsupportedNoopMessage(id))` on `ResolutionUnsupported`. This task only adds test coverage.
- Run `go build -o portal .` then `go test ./cmd/...` and confirm green.

**Acceptance Criteria**:
- [ ] A new CLI test asserts, with a byte-literal `want` (not `spawn.UnsupportedNoopMessage(id)`), that the named unsupported no-op returns exactly `can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened`.
- [ ] The same test (or its NULL branch) asserts the NULL unsupported no-op returns exactly `can't open new windows over a remote connection — nothing opened`.
- [ ] The new test would FAIL if `UnsupportedNoopMessage` copy drifted from spec §5 (it does not delegate to the function for its expected value) — verifiably distinct from the existing self-referencing assertion.
- [ ] `TestRunOpenBurst_UnsupportedTerminal_AtomicNoop` remains and still passes (behaviour: no burster, no half-connect, error names the identity).
- [ ] `cmd/open_burst_run.go` is unchanged (no block-logic edit).
- [ ] `go test ./cmd/...` passes on the unit lane (no tmux server, no daemon, no built binary).

**Tests**:
- `"it returns the plain named no-op copy for a recognised-but-undriven terminal"` — named identity → `err.Error()` == `can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened`.
- `"it returns the plain remote-connection no-op copy for a NULL identity"` — `spawn.Identity{}` → `err.Error()` == `can't open new windows over a remote connection — nothing opened`.
- `"it opens nothing and does not half-connect on the unsupported no-op"` — (existing `TestRunOpenBurst_UnsupportedTerminal_AtomicNoop`, guard) NewBurster not built, zero OpenWindow calls, zero Connect, zero LocalMint.

**Edge Cases**:
- **Self-reference trap.** The pre-existing assertion `want := spawn.UnsupportedNoopMessage(id)` silently tracks the function, so it cannot catch a wording regression. The new test MUST hardcode the expected literal to enforce the coordination contract — do not reuse the shared function to compute `want`.
- **NULL uncovered by the named-only existing test.** The existing CLI unsupported test uses `appleTerminalIdentity()` only; the NULL shape (remote/mosh, or transient detection error folded to `Identity{}`) had no CLI copy coverage. The new NULL branch closes that gap and reads correctly for the CLI's "something was attempted" case (`— nothing opened` honestly signals the attempt).
- **Coherence with `cli-verb-surface-redesign`.** That feature shares ownership of this message and is expected to land first; it changes the CLI *block logic*, not this shared renderer. This task adds no CLI logic change — only a literal-copy pin — so it stays compatible whichever order the two land.
- **Wording reads correctly for the CLI attempted-open case.** `can't open new windows … — nothing opened` is the reactive-no-op wording (an attempt was made and refused), correct for the CLI's N≥2 burst that detected an unsupported terminal after resolution — distinct from the TUI pre-emptive block ("isn't available", nothing attempted, Phase 2).

**Context**:
> Spec §5: "`UnsupportedNoopMessage` … its wording is shared with the CLI open-burst (partly owned by `cli-verb-surface-redesign`) — the rewrite must be coordinated with that feature so the CLI copy stays coherent." §8 Risks: "The rewritten wording must read correctly for the CLI's 'something was attempted' case and be coordinated with `cli-verb-surface-redesign`."
>
> Spec §8 Non-goals: "The CLI multi-target `portal open <a> <b> …` (N≥2) burst *block* — owned by the in-flight `cli-verb-surface-redesign` feature. This bugfix does not change the CLI's block logic; it only touches the shared message renderer."
>
> Production call site (`cmd/open_burst_run.go`, unchanged by this task):
> ```go
> if resolution == spawn.ResolutionUnsupported {
>     spawn.LogUnsupported(deps.Logger, id)
>     return errors.New(spawn.UnsupportedNoopMessage(id))
> }
> ```
>
> Existing test helpers in `cmd/open_burst_run_test.go`: `appleTerminalIdentity()` = `spawn.NewIdentity("com.apple.Terminal", "Apple Terminal")`; `openBurstDepsForTest(id, resolution, adapter, conn, mint)` assembles a fully-injected `*OpenBurstDeps` with a `fakeTerminalDetector{id}` and a fixed `Resolve` returning `(adapter, resolution)`; `runOpenBurstWithDeps(cmd, surfaces, command, deps)` runs the body. The NULL shape is `spawn.Identity{}` (empty BundleID → `IsNull()` true). `deps.Logger` is nil and `spawn.LogUnsupported` is nil-tolerant.
>
> Sequencing (from the phase plan): Task 3-1 is the atomic rewrite; this task is the prevention/coherence lock. The current CLI test self-references the shared function (auto-tracks, silently accepts drift); add an explicit literal regression pinning the CLI's rendered copy to the new plain strings. No CLI block-logic change.

**Spec Reference**: `.workflows/persistent-no-host-terminal-banner/specification/persistent-no-host-terminal-banner/specification.md` §5 (copy set — reactive no-op is shared with the CLI open-burst), §7 (Testing Requirements — "any test asserting the old `UnsupportedNoopMessage` strings in `internal/spawn` and the CLI open-burst suites updates to the new plain-language strings"), §8 (Non-goals — no CLI block-logic change; Risks — CLI copy coordination with `cli-verb-surface-redesign`).
