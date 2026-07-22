# Implementation Review: Persistent No Host Terminal Banner

**Plan**: persistent-no-host-terminal-banner
**QA Verdict**: Approve

## Summary

All nine plan tasks across four phases are implemented faithfully, tested adequately, and meet the project's Go conventions. The bugfix cleanly splits the unsupported-terminal banner by identity shape (named-only via an `IsNull()` discriminator), proactively blocks multi-select entry on any resolved-unsupported terminal with an honest per-shape flash, suppresses `m` in the Sessions help at the call site while keeping the static descriptor intact, and rewrites the shared `UnsupportedNoopMessage` to plain language byte-exactly — with the CLI open-burst copy pinned by an independent byte-literal regression and a duplication cleanup collapsing the two CLI no-op tests onto a shared helper. The load-bearing constraints (reactive backstop retained, detection cache untouched, `WithInitialMultiSelect` ungated, `keymap_dispatch_guard_test` kept green, named two-row non-repetition) were all respected. No blocking issues; one cosmetic non-blocking note.

## QA Verification

### Specification Compliance

Implementation aligns with the specification across all sub-fixes:

- **§2 Sub-fix 1 (banner split):** `unsupportedBannerActive()` gains `&& !m.detectIdentity.IsNull()` (model.go:4736–4737); both consumers (`applySectionHeader` banner swap and `activeNoticeBand` signpost suppression) fixed coherently by the single gate. Detection cache / `rebuildSessionList` left untouched, per the §2 scope guard.
- **§6 (dead NULL render branch removed):** `unsupportedNullLabel` constant and both `bundleID == ""` branches deleted; `see docs` now unconditional; grep-clean gates satisfied (residual `no host-local terminal` matches are out-of-scope detection-layer strings/comments, correctly left alone).
- **§3 Sub-fix 2 (proactive `m`-entry block):** gate at the top of `handleMultiSelectToggle`'s entry branch with the TUI-local `multiSelectBlockedFlashText` helper (no `⚠`, no `— nothing opened`); reactive `decideBurst` backstop retained for the async in-flight window (A1); inline guard-coupling source note present.
- **§4 Sub-fix 3 (help `m`-suppression):** call-site `sessionsHelpKeymap()` filter gated on `DetectUnsupported() && !m.multiSelectMode`; `sessionsKeymap()` stays a pure static constant; Projects call site untouched.
- **§5 (plain-language copy):** `UnsupportedNoopMessage` both shapes byte-exact (verified via hexdump — U+0027 apostrophe, U+00B7 middle dot, U+2014 em-dash); observability log line and persistent named banner correctly untouched.
- **§7 (testing/visual):** `sessions-unsupported-null` fixture + registration + Go seed-path test + vhs tape + committed reference PNG (byte-identical to `sessions-flat`); reactive-backstop tests reworked onto the in-flight entry path; CLI byte-literal copy regression added for both shapes.
- **§8 CLI coordination:** cross-work-unit check confirms `cli-verb-surface-redesign` owns the verb surface, not the shared message copy; no coordination conflict. `cmd/open_burst_run.go` block logic unchanged.

### Plan Completion

- [x] Phase 1 (banner split) acceptance criteria met
- [x] Phase 2 (entry block + help suppression) acceptance criteria met
- [x] Phase 3 (shared copy rewrite) acceptance criteria met
- [x] Phase 4 (analysis cleanup) acceptance criteria met
- [x] All 9 tasks completed
- [x] No scope creep (all touched files map to plan tasks; test-only chores touched no production code)

### Code Quality

No issues found. New helpers mirror existing shapes (`multiSelectBlockedFlashText` follows `unsupportedFlashText`; the cmd-side `assertOpenBurstAtomicNoOp` mirrors the tui-side `assertAtomicNoOp`). Complexity is low — single guarded early-returns and a one-branch identity selector. Comments match each file's density, including the required guard-coupling note.

### Test Quality

Tests adequately verify requirements without over-testing. Each new test targets a distinct acceptance criterion (per-shape flash strings, clear-on-next-key, named two-row co-render with both `⚠` and non-repetition, double-`m` re-block, in-flight-still-enters, ungated construction seam, supported-unaffected; help cases a/b/c + footer guard; byte-literal CLI copy for both shapes). The two divergent CLI assertions (computed `spawn.UnsupportedNoopMessage(id)` vs byte-literal want) were confirmed independently load-bearing. Test-only reworks (2-1, 4-1) touched no production code.

### Required Changes (if any)

None.

## Recommendations

### Quick-fixes

1. `internal/tui/multi_select_entry_block_test.go:230` — `TestMultiSelectEntryBlock_SupportedEntersNoFlash` builds its model via `unsupportedResolvedModel(t, ghosttyIdentity())`; the helper name reads "unsupported" while the case is deliberately the supported path. Functionally correct (the helper just wraps `warmResolvedModel`), but calling `warmResolvedModel(...)` directly or adding a neutrally-named `resolvedModel` wrapper would remove the misleading-name read at this one call site. Cosmetic only. (Report 2-2)
