# Plan: Persistent No Host Terminal Banner

## Phases

### Phase 1: Banner Split by Identity Shape
status: approved
approved_at: 2026-07-22

**Goal**: A NULL/remote unsupported client keeps its standard `Sessions ··· N` section header (count + grouping-mode suffix) and its By-Tag "no tags yet" signpost, while the named-unsupported banner remains unchanged. The now-unreachable NULL banner render branch is deleted, and a NULL visual fixture anchors the regression.

**Why this order**: This is the strongest foundation — it introduces the identity-shape discrimination (`IsNull()` vs the coarse `DetectUnsupported()`) at the single `unsupportedBannerActive()` gate that both `applySectionHeader` and `activeNoticeBand` read, so both surfaces are fixed coherently in one place. It resolves the higher-visibility defect (the user permanently losing their header/count on remote), it is fully self-contained with no dependency on later phases, and the `IsNull()` split it lands is conceptually reused by Phase 2's flash copy. The dead-branch removal (Topic 6) is gated on this phase's gate change (the drop is what makes the NULL render path unreachable), so it belongs here.

**Acceptance**:
- [ ] `unsupportedBannerActive()` returns `false` for a resolved NULL/remote identity and `true` for a named-unsupported identity — the added `!m.detectIdentity.IsNull()` discriminator is the only gate change.
- [ ] A resolved NULL/remote client renders the standard `Sessions ··· N` header (count + grouping-mode suffix), not a banner.
- [ ] A NULL/remote client with no tags shows the By-Tag "no tags yet" signpost again (the named-unsupported signpost-suppression case stays unchanged).
- [ ] The named-unsupported banner (`⚠ unsupported terminal — <name> · <bundleID>` + right-anchored `see docs`) renders exactly as before.
- [ ] The dead NULL render path is removed: the `unsupportedNullLabel = "no host-local terminal"` constant, the `bundleID == ""` branches in `unsupportedLeftCluster` and `renderUnsupportedHeader`, and `TestUnsupportedHeader_NullIdentityNoHostLocal` are deleted; the two renderers are named-only with `see docs` unconditional.
- [ ] `TestApplySectionHeader_UnsupportedNullShowsHonestLine` is inverted to assert the NULL case now renders the standard `Sessions ··· N` header.
- [ ] A new `sessions-unsupported-null` capture fixture (seeded via the existing detection seam with `InitialDetection = &spawn.Identity{}`) renders the standard header with no banner (visually identical to `sessions-flat`), registered in the fixture list with a committed reference PNG; the existing `sessions-unsupported-terminal` fixture stays valid.
- [ ] The once-only detection cache is left untouched — no re-detection is added on `rebuildSessionList`.
- [ ] Full unit suite green; no regression in the named-banner, supported-header, or multi-select-steps-aside tests.

#### Tasks
status: approved
approved_at: 2026-07-22

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| persistent-no-host-terminal-banner-1-1 | Add IsNull() discriminator to unsupportedBannerActive() so NULL keeps the standard header and signpost | multi-select mode still outranks (predicate false in mode), in-flight/unresolved detection renders standard header, NULL + zero-tags By-Tag signpost returns, named identity banner + see docs unchanged, detection cache left untouched (no re-detection on rebuild) |
| persistent-no-host-terminal-banner-1-2 | Remove the unreachable NULL banner render branch (renderers named-only, see docs unconditional) | renderers only reached with non-empty bundleID, see docs always present, ExactlyOneRow "null" subcase converted/removed, no remaining `no host-local terminal` / unsupportedNullLabel reference |
| persistent-no-host-terminal-banner-1-3 | Add the sessions-unsupported-null capture fixture + committed reference PNG | empty Identity{} → IsNull() true seed path, no banner intrudes on resolved-NULL seed, offline harness (no tmux server / no ~/.config), verify fresh PNG write (VHS silent-write flake), no NO_COLOR variant required |

### Phase 2: Proactive Multi-Select Entry Block + Help Suppression
status: approved
approved_at: 2026-07-22

**Goal**: On any resolved-unsupported terminal (NULL or named), pressing `m` fails immediately with an honest transient flash instead of opening a walkable dead-end multi-select mode, and `m` is omitted from the `?` help while it is unavailable. The reactive burst-time no-op backstop is retained for the async in-flight window.

**Why this order**: This fixes the second, independent defect. It comes after Phase 1 because it reuses the `IsNull()` identity-shape split for its per-shape flash copy, and because sequencing the two independent defects lets each be validated at its own checkpoint. Every change is TUI-local (`internal/tui`), keeping the phase cohesive and free of cross-package coordination — which is deliberately deferred to Phase 3.

**Acceptance**:
- [ ] Pressing `m` on a resolved-unsupported terminal (both NULL and named) does not enter multi-select mode (`multiSelectMode` stays false, nothing marked) and sets a transient blocked-entry flash; the entry branch of `handleMultiSelectToggle` gains the proactive `DetectUnsupported()` gate.
- [ ] The blocked-entry flash copy is a new TUI-local helper (`multiSelectBlockedFlashText`) selecting shape via `IsNull()`: `multi-select isn't available over a remote connection` (NULL) / `multi-select isn't available on this terminal` (named) — intent-only, no bundle id, no `see docs`, no `— nothing opened`.
- [ ] The flash self-clears on the next actionable key (existing `setFlash`/`isActionableKey` lifecycle); repeated `m` while it shows clears then re-blocks + re-flashes.
- [ ] Named co-render: a blocked `m` on a named-unsupported terminal yields the two-row state (persistent banner on the header row + block flash on the notice-band row), with both rows carrying the `⚠` glyph (block flash not special-cased to drop it).
- [ ] `WithInitialMultiSelect` (construction-time capture-harness seam) is not gated; existing multi-select fixtures are unaffected.
- [ ] The `?` help omits the `m` row iff `DetectUnsupported() && !m.multiSelectMode`, covering all three cases: (a) unsupported + not in mode → omitted; (b) supported → listed; (c) unsupported + in mode (A1 in-flight-entered) → listed. The filter is applied at the call site in `renderHelpModalOnClearedCanvas`; `sessionsKeymap()` stays a pure static constant; the footer is unchanged.
- [ ] `keymap_dispatch_guard_test` stays green (runs with detection unwired → filter inert); an inline source note near the entry-block gate / guard probe records the latent guard-coupling dependency on `sessionsGuardModel` keeping detection unwired.
- [ ] The reactive `decideBurst` unsupported no-op backstop is retained; `burst_unsupported_noop_test.go` is reworked so the two post-resolve-entry tests (`TestBurstUnsupported_NonNullAtomicNoOp`, `TestBurstUnsupported_NullFlash`) enter multi-select before resolving detection (the in-flight path), while the deferred-Enter → reactive no-op and supported-dispatch coverage stay valid.
- [ ] Supported terminals are unaffected: `m` enters, `?` help lists `m`, and the burst dispatches; full unit suite green.

#### Tasks
status: approved
approved_at: 2026-07-22

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| persistent-no-host-terminal-banner-2-1 | Rework reactive-backstop no-op tests onto the in-flight entry path | in-flight entry before resolve (NonNull + Null), deferred-Enter no-op retained, supported-still-dispatches unchanged, backstop copy unchanged (Phase 3), ordered before the entry block to keep the suite green |
| persistent-no-host-terminal-banner-2-2 | Proactive multi-select entry block + TUI-local blocked-entry flash helper | NULL vs named flash copy via IsNull, intent-only copy (no bundle id / no see docs / no "— nothing opened"), named two-row co-render both rows carry ⚠, self-clears on next actionable key, repeated m re-blocks + re-flashes, in-flight window still enters mode, WithInitialMultiSelect not gated, supported terminal unaffected, inline guard-coupling source note |
| persistent-no-host-terminal-banner-2-3 | Help-modal m-suppression at the call site | unsupported + not in multi-select omits m, supported lists m, unsupported + in multi-select (A1 in-flight-entered) lists m, footer unchanged (m non-Core), sessionsKeymap() stays static (call-site filter only), keymap_dispatch_guard_test stays green (detection unwired), Projects help call-site untouched |

### Phase 3: Shared Reactive/CLI Copy Rewrite (plain-language `UnsupportedNoopMessage`)
status: approved
approved_at: 2026-07-22

**Goal**: Rewrite the shared `UnsupportedNoopMessage` (both shapes) in `internal/spawn/message.go` to plain language, and update every copy assertion across the spawn, picker reactive-flash, and CLI open-burst suites so the two surfaces stay coherent.

**Why this order**: This is the one cross-package, coordination-sensitive dimension of the fix — it widens into `internal/spawn` and shares its copy with the CLI open-burst (partly owned by `cli-verb-surface-redesign`). Isolating it in a final checkpoint keeps Phases 1-2 purely TUI-local and lets the CLI coordination be validated once, after the TUI behaviour is settled. It comes last because it edits the copy strings within the reactive-backstop tests that Phase 2 already restructured (Phase 2 adapts structure keeping current copy; Phase 3 updates the copy), and because `cli-verb-surface-redesign` is expected to land first.

**Acceptance**:
- [ ] `UnsupportedNoopMessage` returns the rewritten plain strings: `can't open new windows over a remote connection — nothing opened` (NULL) / `can't open new windows in <name> · <bundleID> — nothing opened` (named); the `— nothing opened` clause and the `<name> · <bundleID>` key are preserved.
- [ ] The picker reactive no-op flash (`decideBurst` → `unsupportedFlashText` → `UnsupportedNoopMessage`) renders the rewritten copy per shape.
- [ ] All copy assertions asserting the old strings are updated to the new strings across the spawn suite, the tui reactive-backstop suite (`burst_unsupported_noop_test.go`), and the CLI open-burst suite.
- [ ] The rewritten wording reads correctly for the CLI's "something was attempted" case and is coherent with `cli-verb-surface-redesign`.
- [ ] Adjacent spawn copy (gone-session, failed-to-open, permission guidance) is unchanged.
- [ ] Full unit suite and the CLI open-burst copy tests green.

#### Tasks
status: approved
approved_at: 2026-07-22

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| persistent-no-host-terminal-banner-3-1 | Rewrite UnsupportedNoopMessage (both shapes) + lockstep copy assertions | NULL shape covers remote/mosh + transient-detection-error folded to Identity{}; named shape preserves `<name> · <bundleID>` (U+00B7) and the `— nothing opened` clause; adjacent spawn copy (gone / failed-to-open / permission) untouched; observability log line `unsupported terminal — nothing opened` (logemit) is not the message and stays untouched; function + spawn-suite + tui-reactive-backstop literals must land in one green commit |
| persistent-no-host-terminal-banner-3-2 | Lock CLI open-burst copy coherence (regression + wording check) | CLI assertion self-references the shared function so it auto-tracks and silently accepts drift (literal regression enforces the coordination contract); `can't open new windows … — nothing opened` must read correctly for the CLI's attempted-open case; cli-verb-surface-redesign shares ownership and lands first (no CLI block-logic change); NULL shape uncovered by the current named-only CLI test |

### Phase 4: Analysis (Cycle 1)
status: approved
approved_at: 2026-07-22

**Goal**: Address findings from Analysis (Cycle 1).

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| persistent-no-host-terminal-banner-4-1 | Extract a shared cmd-side unsupported-burst no-op test helper | shared helper pair owns arrange + Execute + structural no-op invariants; both tests keep their divergent err assertions at the call site (computed `spawn.UnsupportedNoopMessage(id)` for AtomicNoop vs byte-literal `want` + NULL row for CopyIsPlainLanguage); no production (non-test) code changes; both divergent assertions stay independently load-bearing; `go test ./cmd` passes |
