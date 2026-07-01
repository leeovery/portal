---
topic: session-rename-orphans-resume-hook
cycle: 1
total_proposed: 5
---
# Analysis Tasks: session-rename-orphans-resume-hook (Cycle 1)

## Task 1: Copy PortalID in the findOrAppendSession append branch
status: pending
severity: medium
sources: standards, architecture

**Problem**: `findOrAppendSession` (internal/state/capture.go:254-266) constructs a `Session` by hand when appending a prev session into the fresh index, copying only `Name`, `Environment`, and `Windows` and omitting the new `PortalID` field — despite its own doc-comment (capture.go:250-253) promising "a shallow copy of ps". The whole fix depends on `PortalID` riding `sessions.json` faithfully (spec § Cross-Reboot Persistence: losing the id → bare shell on next reboot + stale-cleanup deletes the just-fired hook). The append branch is UNREACHABLE today — `mergeSkippedPanes` (capture.go:187) `continue`s past any prev session absent from `live`, and `live` is built from `fresh.Sessions` (buildLiveStructure, capture.go:216-230), so any session reaching `mergePane` is already in `fresh.Sessions` and the "found" loop at capture.go:255-259 returns the existing index first. So there is no live bug. But the partial struct-copy is a latent re-orphan trap sitting directly on this fix's persistence seam: if the `sessionLive` gate at capture.go:187 is ever relaxed to reintroduce a prev session (the natural next step the surrounding code already reasons about), the reintroduced `Session` would carry `PortalID == ""`, erasing the id from the snapshot and re-orphaning the hook on the next reboot — the exact failure mode this fix exists to prevent, reintroduced silently.

**Solution**: Make the appended `Session` literal copy the whole struct so the "shallow copy of ps" the doc-comment promises is actually total. Add `PortalID: ps.PortalID` to the struct literal at capture.go:260-264 (keeping `Windows: []Window{}` empty, since the caller populates windows via subsequent merges).

**Outcome**: The `findOrAppendSession` append branch preserves `PortalID` across the copy, closing the seam structurally rather than relying on the branch's current unreachability (an invariant no test pins). A future relaxation of the `sessionLive` gate cannot silently re-orphan a hook via a `PortalID == ""` snapshot entry.

**Do**:
1. In `internal/state/capture.go`, locate the append branch of `findOrAppendSession` (currently capture.go:260-264): the `fresh.Sessions = append(fresh.Sessions, Session{ Name: ps.Name, Environment: ps.Environment, Windows: []Window{} })` literal.
2. Add `PortalID: ps.PortalID,` to that struct literal (alongside `Name` and `Environment`, before or after — field order is cosmetic).
3. Leave `Windows: []Window{}` as-is (windows are populated by the caller via subsequent `findOrAppendWindow`/`mergePane` calls; do not copy `ps.Windows`).
4. Do NOT change the `sessionLive` gate at capture.go:187 or any other merge logic — this task is strictly the struct-copy completeness fix.

**Acceptance Criteria**:
- The appended `Session` literal in `findOrAppendSession` includes `PortalID: ps.PortalID`.
- `Windows` remains an empty `[]Window{}` (unchanged copy semantics for windows).
- No change to `mergeSkippedPanes`, `buildLiveStructure`, `mergePane`, or the `sessionLive` gate.
- `go build ./...` and `go test ./internal/state/...` pass.

**Tests**:
- Add a focused unit test in `internal/state/capture_test.go` that drives the merge/append path directly (or via a lowered `sessionLive` condition in a test-only construction) so a prev session carrying a non-empty `PortalID` is appended into a fresh index, and assert the appended `Session.PortalID` equals the prev `PortalID`. This pins the trap closed independently of the branch's production reachability. Follow the existing `capture_test.go` conventions (no `t.Parallel()`).

## Task 2: Fix the stale ListAllPanes doc-comment on cleanStaleAdapter
status: pending
severity: medium
sources: standards, architecture

**Problem**: The `cleanStaleAdapter` doc-comment at cmd/bootstrap_production.go:71 states production wiring "passes a *tmux.Client which satisfies the interface via ListAllPanes." After this fix the `AllPaneLister` interface method is `ListAllPaneHookKeys` (cmd/clean.go:23) and `*tmux.Client` satisfies it via `ListAllPaneHookKeys`, not `ListAllPanes` — the adapter routes through `runHookStaleCleanup → lister.ListAllPaneHookKeys()`. The production wiring itself is correct (Stage 2 enumeration switched to `ListAllPaneHookKeys`); only the prose lies, pointing a future reader at the retired name-based enumeration. The spec made retiring these stale name-based-keying doc-comments an explicit deliverable precisely because a comment asserting the wrong enumeration "would invite a future caller back into name-based keying" (spec § Hook-Key Derivation; Risks → Missed key-producing site). This is the one residual on the hook-cleanup adapter. (The sibling reference at cmd/bootstrap/stale_marker_cleanup.go:43 is CORRECT and must NOT change — that path is the name-based skeleton-marker cleanup and legitimately still uses `ListAllPanes`.)

**Solution**: Change "satisfies the interface via ListAllPanes" to "satisfies the interface via ListAllPaneHookKeys" so the comment names the hook-key enumeration the interface actually requires.

**Outcome**: The `cleanStaleAdapter` doc-comment names the correct enumeration (`ListAllPaneHookKeys`), removing the residual pointer back toward name-based keying at the hook-cleanup adapter.

**Do**:
1. In `cmd/bootstrap_production.go`, on the `cleanStaleAdapter` doc-comment (around line 71), replace `ListAllPanes` with `ListAllPaneHookKeys` in the phrase "satisfies the interface via ListAllPanes".
2. Do NOT touch `cmd/bootstrap/stale_marker_cleanup.go:43` — its `ListAllPanes` reference is correct (name-based skeleton-marker cleanup).

**Acceptance Criteria**:
- cmd/bootstrap_production.go:71 names `ListAllPaneHookKeys`, not `ListAllPanes`.
- No code change (comment-only edit); `go build ./...` passes.
- cmd/bootstrap/stale_marker_cleanup.go:43 is unchanged.

**Tests**:
- No new test (documentation-only correction). Existing `cmd` tests must still pass (`go test ./cmd/...`).

## Task 3: Update the ListAllPanes prose in the shared stale-cleanup helper
status: pending
severity: low
sources: standards

**Problem**: The algorithm/policy prose in `runHookStaleCleanup` and `clean.go` describes the list step as `ListAllPanes` at cmd/run_hook_stale_cleanup.go:16, :31, :46, :66 and cmd/clean.go:118, :119, :144, :150 (e.g. "swallowListError (bool): how a non-nil err from ListAllPanes surfaces", "1. ListAllPanes. On error emit Warn"). The live call is now `lister.ListAllPaneHookKeys()` (run_hook_stale_cleanup.go:89). These are name-based structural-enumeration references left over from before the Stage 2 switch; the code is correct but the prose describes hook-key cleanup as if it still enumerated name-based structural keys, blurring the load-bearing distinction the spec draws between the name-based enumeration (`ListAllPanes` / `StructuralKeyFormat`) and the hook-key enumeration (`ListAllPaneHookKeys` / `HookKeyFormat`).

**Solution**: Update the eight prose references to `ListAllPaneHookKeys` (or a neutral phrasing like "the live-key enumeration") so the internal docs match the switched call and preserve the name-based-vs-hook-key distinction.

**Outcome**: The stale-cleanup helper and clean.go comments name the enumeration actually invoked, so a reader tracing the hook-cleanup algorithm is not steered back toward the name-based path.

**Do**:
1. In `cmd/run_hook_stale_cleanup.go`, update the `ListAllPanes` references in comments at lines 16, 31, 46, 66 to `ListAllPaneHookKeys` (or the neutral "the live-key enumeration").
2. In `cmd/clean.go`, update the `ListAllPanes` references in comments at lines 118, 119, 144, 150 the same way.
3. Comment-only edits — do NOT change any call site or logic.

**Acceptance Criteria**:
- All eight comment references name `ListAllPaneHookKeys` (or a neutral live-key-enumeration phrasing), consistent with the actual call at run_hook_stale_cleanup.go:89.
- No code/logic change; `go build ./...` and `go test ./cmd/...` pass.

**Tests**:
- No new test (documentation-only correction).

## Task 4: Add a fast static byte-identity guard for the three @portal-id literals
status: pending
severity: low
sources: architecture

**Problem**: The fix's correctness rests on three independent embeddings of the literal `@portal-id` staying byte-identical: `session.PortalIDOption` (the const at internal/session/create.go:29), `HookKeyFormat`'s `#{?@portal-id,#{@portal-id},#{session_name}}` conditional (internal/tmux/tmux.go:849), and `captureFormat`'s trailing `#{@portal-id}` column (internal/state/capture.go:42). The spec accepts that the two raw tmux format strings cannot share a single const, and consistency IS exercised end-to-end — but only by the real-tmux cross-site test (pins `HookKeyFormat`) and the durability integration test (round-trips `captureFormat` against a live stamped server). Those real-tmux tests carry NO build tag but are gated by `SkipIfNoTmux(t)`, so in a tmux-less environment they SKIP silently rather than run. A one-character typo in `captureFormat` (e.g. `@portal_id`) would therefore go uncaught wherever tmux is absent. There is no unit-level guard asserting `strings.Contains(HookKeyFormat, PortalIDOption)` and `strings.Contains(captureFormat, PortalIDOption)`. For the single most load-bearing invariant of this change, a fast static tripwire is cheap insurance.

**Solution**: Add a small unit test (no tmux) asserting that `HookKeyFormat` and `captureFormat` each contain the exact `@portal-id` literal, and that the canonical const equals `@portal-id`. Because `captureFormat` lives in `internal/state` and `HookKeyFormat` in `internal/tmux`, and `session.PortalIDOption` cannot be imported into either without an import cycle, place the guard where it can reach the strings it checks — a cross-package guard (e.g. an external `*_test.go` in a package that may import all three), or two co-located guards (one in `internal/tmux` asserting `HookKeyFormat` contains the literal, one in `internal/state` asserting `captureFormat` contains it), each also asserting the literal equals `"@portal-id"`.

**Outcome**: A typo in either raw tmux format string fails a millisecond-scale unit test on every `go test` run, independent of whether the integration / real-tmux suite executes, closing the gap where a tmux-less environment silently skips the only current byte-identity coverage.

**Do**:
1. Decide placement that avoids the import cycle. Preferred: co-located guards — one in `internal/tmux` (can reference `HookKeyFormat` directly and the literal `"@portal-id"`), one in `internal/state` (can reference `captureFormat`). If a single cross-package external test package can legally import `session`, `tmux`, and `state` without a cycle, a single guard is acceptable and preferable; confirm no cycle before choosing this.
2. In the `internal/tmux` guard: assert `strings.Contains(tmux.HookKeyFormat, "@portal-id")` (use the internal-package name if written as `package tmux`), and assert the literal is exactly `"@portal-id"`.
3. In the `internal/state` guard: assert `strings.Contains(captureFormat, "@portal-id")`.
4. If a cross-package guard is used instead, additionally assert `session.PortalIDOption == "@portal-id"` and that both format strings contain `session.PortalIDOption`.
5. Add a comment on each guard explaining WHY the literal is repeated (import-cycle avoidance) and that it must stay byte-identical to `session.PortalIDOption` — mirroring the existing comments on the real-tmux test constants.
6. No `t.Parallel()`.

**Acceptance Criteria**:
- A no-tmux unit test asserts `HookKeyFormat` contains the exact `@portal-id` literal.
- A no-tmux unit test asserts `captureFormat` contains the exact `@portal-id` literal.
- At least one assertion pins the canonical literal value (`session.PortalIDOption == "@portal-id"` if reachable without a cycle, otherwise a local literal-equality assertion).
- The guard runs and passes under plain `go test ./...` with no tmux present (does NOT depend on `SkipIfNoTmux`).
- No import cycle introduced; `go build ./...` passes.

**Tests**:
- This task IS the test. Verify it fails when the literal in either format string is mutated (e.g. temporarily change `captureFormat`'s trailing column to `#{@portal_id}` and confirm the guard fails), then revert.

## Task 5: Collapse the triplicated @portal-id test constant in the tmux_test package
status: pending
severity: low
sources: duplication

**Problem**: Three files in the SAME external test package (`package tmux_test`) each declare their own package-level constant for the identical literal `@portal-id`: `portalIDOption` (internal/tmux/hookkey_format_realtmux_test.go:43), `hookKeyPortalIDOption` (internal/tmux/list_all_pane_hookkeys_realtmux_test.go:47), and `crossSitePortalIDOption` (internal/tmux/hookkey_cross_site_realtmux_test.go:52). A fourth site (internal/tmux/resolve_hookkey_realtmux_test.go:64) inlines the bare `"@portal-id"` literal directly. Redeclaring the LITERAL across files (rather than importing `session.PortalIDOption`) is a genuine, correctly-documented import-cycle avoidance (`internal/session` imports `internal/tmux`) and must NOT change. What is redundant is that three distinct constants for the same value coexist within a single compilation unit — Go would let one shared const serve all four sites. The differing names invite a reader to think they might differ, when the load-bearing invariant is that they are byte-identical to `HookKeyFormat`'s embedded literal. A future edit to one (e.g. an option-name change) must be manually mirrored to the other two plus the inlined string.

**Solution**: Collapse the three package-level constants into a single shared `const portalIDOption = "@portal-id"` declared once in the `tmux_test` package (in any one of the existing files), reference it from all three tests plus the currently-inlined literal at resolve_hookkey_realtmux_test.go:64. Keep the "literal, not session.PortalIDOption, to avoid an import cycle; must stay byte-identical to HookKeyFormat" comment on the surviving declaration.

**Outcome**: One `@portal-id` test constant in the `tmux_test` package, referenced everywhere, with a single documented byte-identity contract — no divergent names, no inlined copy, one place to edit if the option name ever changes. The import-cycle avoidance is preserved.

**Do**:
1. Choose one surviving declaration (e.g. `const portalIDOption = "@portal-id"` already at hookkey_format_realtmux_test.go:43) and keep its explanatory comment (literal-not-import, byte-identical-to-HookKeyFormat).
2. Delete the `hookKeyPortalIDOption` const (list_all_pane_hookkeys_realtmux_test.go:47) and the `crossSitePortalIDOption` const (hookkey_cross_site_realtmux_test.go:52), updating their usages to the surviving `portalIDOption`.
3. Replace the inlined `"@portal-id"` literal at resolve_hookkey_realtmux_test.go:64 with the surviving `portalIDOption`.
4. Confirm no name collisions and that all four files reference the single const.

**Acceptance Criteria**:
- Exactly one `@portal-id` constant is declared in the `tmux_test` package.
- All previous usages (three consts + one inlined literal) reference the single surviving const.
- The surviving declaration keeps the import-cycle-avoidance + byte-identity comment.
- The import-cycle avoidance is unchanged (still a literal, still not `session.PortalIDOption`).
- `go test ./internal/tmux/...` compiles and passes (SkipIfNoTmux-gated tests skip cleanly where tmux is absent).

**Tests**:
- No new test — this is a test-scaffolding tidy. Verify the `tmux_test` package still compiles and the affected real-tmux tests still run/skip correctly.
