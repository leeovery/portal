---
phase: 1
phase_name: Live-set filter in `mergeSkippedPanes` (Fix Component A)
total: 5
---

## daemon-merge-reintroduces-dead-sessions-1-1 | approved

### Task daemon-merge-reintroduces-dead-sessions-1-1: Replace codifying-bug test with session-level filter + add session-level filter

**Problem**: The current `mergeSkippedPanes` implementation in `internal/state/capture.go` re-introduces sessions from `prev` whose paneKey is in `skipSet`, with no validation against the freshly-built `idx.Sessions`. A stale `@portal-skeleton-<paneKey>` marker therefore causes a killed session to be re-committed to `sessions.json` on the next daemon tick. The existing unit test at `internal/state/capture_test.go:570-617` (`TestCaptureStructureMergeSkippedPanes/merges a skipped pane's session and window from prev when missing from fresh`) explicitly asserts the buggy behaviour as correct, codifying the wrong invariant and blocking any straight fix.

**Solution**: Replace the codifying-bug test with its inverse, then introduce a local fresh-structural map inside `mergeSkippedPanes` (built from `idx.Sessions` already in scope at the call site) and gate session-level merging on the session name being present in that map. Public signature of `mergeSkippedPanes` stays unchanged; `mergePane` and `findOrAppendSession` are not modified.

**Outcome**: A prev session whose name is not in the fresh `idx.Sessions` is NOT merged into the result, even when its paneKey is in `skipSet`. The replaced test asserts this inverted behaviour; the legitimate hydrate-in-progress merge (all three structural levels live in fresh) continues to work because session-name presence still passes the filter.

**Do**:
- Edit `internal/state/capture_test.go:570-617`. Rewrite the subtest `merges a skipped pane's session and window from prev when missing from fresh` so its name and assertions are inverted: rename it to e.g. `does not merge a skipped pane whose session is absent from fresh`, keep the same fixture (prev has `old`; fresh has `new`; `skipSet` marks `old:1.2`), and assert `findPane(idx, "old", 1, 2) == nil` and that `idx.Sessions` contains only `new`. Run the test to confirm it fails on the current implementation.
- Edit `internal/state/capture.go` `mergeSkippedPanes` (currently at lines 117-130). Add a small private helper (e.g. `buildLiveStructure(idx Index)`) returning a nested map shaped as `map[string]map[int]map[int]struct{}` keyed by session name → window index → pane index, populated by iterating `idx.Sessions` / `Windows` / `Panes`.
- In `mergeSkippedPanes`, call `buildLiveStructure(*fresh)` once at function entry. For each prev pane whose paneKey is in `skipSet`, check whether the prev session's name exists as a top-level key in the live-structure map; if not, `continue` (skip the merge for that pane).
- Do NOT add defensive checks inside `mergePane` or `findOrAppendSession` — the single point of enforcement is the entry-level filter inside `mergeSkippedPanes`.
- Re-run the rewritten test plus the rest of `TestCaptureStructureMergeSkippedPanes` to confirm the session-level filter passes the rewritten case and does not regress sibling subtests (notably the existing happy-path "skip set wins over fresh at matching coords" and "leaves the fresh capture unchanged when skip set is empty / prev is nil").

**Acceptance Criteria**:
- [ ] `internal/state/capture_test.go:570-617` (the codifying-bug subtest) is replaced with an inverted assertion: a prev session not present in fresh is NOT merged into the result, even when its paneKey is in `skipSet`.
- [ ] `mergeSkippedPanes` builds a local fresh-structural map from `idx.Sessions` at function entry and uses it to gate session-level merging on session-name presence.
- [ ] The public signature of `mergeSkippedPanes` is unchanged.
- [ ] `mergePane` and `findOrAppendSession` are not modified.
- [ ] All existing sibling subtests under `TestCaptureStructureMergeSkippedPanes` (skip-set wins, empty skip set, nil prev) still pass.
- [ ] `go test ./internal/state/...` passes.

**Tests**:
- `"does not merge a skipped pane whose session is absent from fresh"` (replacing the codifying-bug subtest): prev = session `old` with pane at `(1,2)`; fresh = session `new` only; `skipSet = {old__1.2}`; assert `findPane(idx, "old", 1, 2) == nil` and `len(idx.Sessions) == 1` with name `new`.
- `"merges a skipped pane when its session is still live in fresh"` (sibling preserved or extended): both fresh and prev contain session `work` with the same pane coords; assert prev's CWD/CurrentCommand wins (existing skip-set authority preserved).
- `"leaves the fresh capture unchanged when skip set is empty"` (existing): regression — passes unchanged.
- `"leaves the fresh capture unchanged when prev is nil"` (existing): regression — passes unchanged.

**Edge Cases**:
- Prev session name is the empty string or contains unusual characters — the live-structure map keyed by `Session.Name` handles this without special-casing because lookup is exact-string match.
- Multiple panes in prev under a session-name absent from fresh — every one of them is filtered out (not just the first).
- `mergePane` / `findOrAppendSession` signatures and behaviour are not changed; the filter is the single point of enforcement.

**Context**:
> From specification → Fix Component A → Data Flow / Signature Approach: "The structural map (session names → window indices → pane indices) is built locally inside `mergeSkippedPanes` from `idx.Sessions` — the freshly-built index that is already in scope at the call site (`internal/state/capture.go:100`, where `mergeSkippedPanes(&idx, *prev, skipSet)` is invoked). [...] Keeps the change internal to `mergeSkippedPanes` — no signature/caller updates required."
>
> From specification → Fix Component A → Filtering Levels: "**Session level** — A prev session whose name is not in fresh must NOT be merged, even when its paneKey is in `skipSet`."
>
> From specification → Testing Requirements → Existing Tests to Replace: "`internal/state/capture_test.go:570-617` — The test [...] codifies the buggy behaviour as correct and **must be replaced** with its inverse: A prev session whose name is not in fresh must NOT be merged, even when its paneKey is in `skipSet`."
>
> From specification → Fix Component A → Data Flow / Signature Approach: "Helper functions are untouched. `mergePane` and `findOrAppendSession` [...] appear in Files Touched only because the merge logic in the same file is being edited; their behaviour and contracts are unchanged. Implementers should not add belt-and-braces defensive checks inside `mergePane` / `findOrAppendSession` — the filter at the merge entry point is the single point of enforcement."

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` (Fix Component A; Testing Requirements → Existing Tests to Replace)

## daemon-merge-reintroduces-dead-sessions-1-2 | approved

### Task daemon-merge-reintroduces-dead-sessions-1-2: Add window-level filtering to `mergeSkippedPanes`

**Problem**: Session-level filtering alone leaves a parallel resurrection path open: when a prev pane's session is still live in fresh but its window index has been killed (`tmux kill-window`), a stale `@portal-skeleton-*` marker would still cause `mergePane` → `findOrAppendWindow` to re-append the dead window into the live session in fresh, polluting `sessions.json`. The merge filter must reject any prev pane whose window index is missing from the fresh session.

**Solution**: Extend the local fresh-structural map (introduced in task 1-1) so it carries window-index sets per session, and add a window-level gate in `mergeSkippedPanes`: a prev pane is only retained when its session name is in the live structure AND its window index is in that session's live window set.

**Outcome**: A prev pane in `skipSet` whose session is live in fresh but whose window index is not present in that fresh session is dropped from the merge result. Mixed prev sessions (some windows live, some stale) lose only the stale windows; the live-window panes still merge as before.

**Do**:
- Confirm the live-structure helper from task 1-1 already shapes its map as session → windows → panes (or extend it now to that shape if task 1-1 only populated the session-name layer). Each session's value is `map[int]map[int]struct{}` keyed by window index then pane index.
- In `mergeSkippedPanes`, after the session-level check passes, look up the window-index map for that session and continue if the prev pane's window index is not a key. Only after both session AND window checks pass should the prev pane be considered for merge.
- Preserve canonical ordering: the existing call to `resortIndex(fresh)` at the end of `mergeSkippedPanes` is sufficient — no extra sorting is required for the partial-drop case.
- Do NOT modify `mergePane` or `findOrAppendWindow` — the entry-level filter remains the single point of enforcement.

**Acceptance Criteria**:
- [ ] `mergeSkippedPanes` rejects prev panes whose window index is missing from the fresh session, even when the session itself is live and the paneKey is in `skipSet`.
- [ ] When a prev session has multiple windows and only some are live in fresh, the live-window panes still merge correctly while stale-window panes are dropped.
- [ ] Canonical ordering of `idx.Sessions` (and nested windows/panes) is preserved across partial drops.
- [ ] `mergePane`, `findOrAppendSession`, and `findOrAppendWindow` are not modified.
- [ ] `go test ./internal/state/...` passes.

**Tests**:
- `"does not merge a skipped pane whose window is absent from a live fresh session"`: prev session `work` has window indices `0` and `5`, each with one pane; fresh has session `work` with only window `0`; `skipSet` contains the paneKey for `work__5.0`; assert window `5` does not appear in `idx.Sessions[work].Windows` and only window `0` survives.
- `"drops only stale windows from a mixed prev session"`: prev session `work` has windows `0` (live in fresh) and `7` (absent from fresh), each pane in `skipSet`; assert window `0`'s pane merges from prev (CWD/CurrentCommand survive) while window `7` is absent.
- `"canonical ordering preserved after window-level drop"`: prev contributes a partially-merged session; assert windows in `idx.Sessions[*].Windows` are sorted ascending by index after merge.

**Edge Cases**:
- A prev session with zero live windows in fresh: the session is live (passes session-level filter) but every window is stale; result is the live session unchanged from fresh, with no panes appended from prev.
- Window index `0` in prev vs base-index drift: the comparison is exact-int match; if base-index drift has changed numbering, the affected windows are filtered out, which is the correct conservative behaviour.
- Helpers (`mergePane`, `findOrAppendWindow`) remain untouched — the spec explicitly forbids belt-and-braces defensive checks inside helpers.

**Context**:
> From specification → Fix Component A → Filtering Levels: "**Window level** — A prev window that exists in `skipSet` but whose window index is not present in the (otherwise-live) fresh session must be dropped from the merge result. Session-level filtering alone was rejected: the same defensive flaw exists at window and pane level — `kill-window` or `kill-pane` against a still-live session leaves the analogous resurrection path open."
>
> From specification → Fix Component A → Data Flow / Signature Approach: "The function may grow a small private helper (e.g. `buildLiveStructure(idx)` returning a nested map) but its public surface stays the same."

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` (Fix Component A → Filtering Levels)

## daemon-merge-reintroduces-dead-sessions-1-3 | approved

### Task daemon-merge-reintroduces-dead-sessions-1-3: Add pane-level filtering to `mergeSkippedPanes`

**Problem**: With session and window filters in place, one resurrection path remains: a prev pane whose session and window are both live in fresh but whose pane index has been killed (`tmux kill-pane`). Without a pane-level gate, `mergePane` will append the dead pane back into the live window — re-introducing the dead pane into `sessions.json` whenever a stale marker is present. The existing replacement-at-matching-coords contract (skip-set wins over fresh data when fresh has the same pane index) must be preserved untouched.

**Solution**: Use the existing pane-index keys in the local fresh-structural map (built at session 1-1; deepened at session 1-2) to gate pane-level merging on pane-index presence. A prev pane is retained only when session, window, AND pane index all exist in fresh.

**Outcome**: A prev pane in `skipSet` whose session and window are live in fresh but whose pane index is absent from fresh is dropped from the merge result. The existing replacement-at-matching-coords contract continues to work — when the pane index IS present in fresh, prev's CWD / CurrentCommand still wins (current behaviour, unchanged).

**Do**:
- Ensure the live-structure map produced by `buildLiveStructure` carries the pane-index keys (a `map[int]struct{}` per window). If task 1-2 already deepened the map to this shape, no additional work on the helper is needed.
- In `mergeSkippedPanes`, after the session and window gates pass, check whether the prev pane's index is a key in the fresh window's pane-index set. If not, `continue` (skip the merge for that pane).
- Do NOT alter the existing skip-set-wins behaviour for matching coords — `mergePane` line 142-145 (`if w.Panes[i].Index == pp.Index { w.Panes[i] = pp; return }`) handles that and remains the canonical replacement path.
- Confirm no defensive checks are added inside `mergePane` or `findOrAppendSession`.

**Acceptance Criteria**:
- [ ] `mergeSkippedPanes` rejects prev panes whose pane index is missing from the fresh window, even when the session and window are live and the paneKey is in `skipSet`.
- [ ] The existing skip-set-wins-over-fresh-data behaviour at matching coords is preserved (sibling subtest in `TestCaptureStructureMergeSkippedPanes` continues to pass).
- [ ] Canonical ordering survives the partial drop.
- [ ] `mergePane` and `findOrAppendSession` remain unmodified.
- [ ] `go test ./internal/state/...` passes.

**Tests**:
- `"does not merge a skipped pane whose pane index is absent from a live fresh window"`: fresh has session `work`, window `0`, pane `0`; prev has session `work`, window `0`, panes `0` and `1`; `skipSet` contains the paneKey for `work__0.1`; assert `findPane(idx, "work", 0, 1) == nil` and `idx.Sessions[work].Windows[0].Panes` contains only the fresh pane `0`.
- `"skip-set wins over fresh data at matching pane coords"` (existing or extended): both fresh and prev have pane `0` of session `work` window `0`; assert prev's CWD/CurrentCommand wins (existing replacement contract preserved).
- `"canonical ordering preserved after pane-level drop"`: prev contributes panes that partially merge; assert panes in the merged window are sorted ascending by index.

**Edge Cases**:
- Live window with zero matching live pane indices for a prev session's pane set: every prev pane is filtered out; the fresh window's pane list is returned unchanged.
- The replacement-at-matching-coords path (`mergePane` line 142) is reached only when the pane-level filter passes — i.e. the pane index IS in fresh. In that case the existing replacement contract executes unchanged.
- The new prev-pane-append path inside `mergePane` (line 147 `w.Panes = append(w.Panes, pp)`) is now structurally unreachable when the pane index is absent from fresh, because the entry-level filter rejects those cases. The line is left in place — it remains correct under future contracts and removing it would be belt-and-braces tampering with helper code.

**Context**:
> From specification → Fix Component A → Filtering Levels: "**Pane level** — A prev pane that exists in `skipSet` but whose pane index is not present in the (otherwise-live) fresh window must be dropped from the merge result. Session-level filtering alone was rejected: the same defensive flaw exists at window and pane level — `kill-window` or `kill-pane` against a still-live session leaves the analogous resurrection path open."
>
> From specification → Fix Component A → Data Flow / Signature Approach: "Helper functions are untouched. [...] Implementers should not add belt-and-braces defensive checks inside `mergePane` / `findOrAppendSession` — the filter at the merge entry point is the single point of enforcement."

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` (Fix Component A → Filtering Levels)

## daemon-merge-reintroduces-dead-sessions-1-4 | approved

### Task daemon-merge-reintroduces-dead-sessions-1-4: Add empirical-scenario regression test (kill-mid-flight self-heal)

**Problem**: The user observed three live-in-the-wild resurrecting sessions (`agentic-workflows-XXrJ3J`, `leeovery-Gi5NLG`, `leeovery-feqhpg`) with corresponding stale `@portal-skeleton-*` markers. The codifying-bug test inversion (task 1-1) and structural filters (tasks 1-2 / 1-3) are necessary but not sufficient evidence that the bug is fixed — the fix must also be exercised against a fixture that mirrors the empirical sequence: prev contains the session, marker is set in `skipSet`, fresh capture omits the session (kill), and the result must NOT reintroduce it. Additionally, the polluted `prev` from prior ticks must self-heal: a follow-up tick that uses the freshly-committed clean index as `prev` must continue to omit the dead session.

**Solution**: Add a regression test (or subtest) under `TestCaptureStructureMergeSkippedPanes` (or a new top-level test as appropriate) that seeds `prev.Sessions` with a session, drops it from the fresh enumeration, keeps its paneKey in `skipSet`, and asserts the killed session is NOT reintroduced. Then run a follow-up call using the just-returned clean index as `prev` (with the same `skipSet`) and assert the killed session remains absent — proving the self-heal path.

**Outcome**: A regression test exists that mirrors the empirical scenario end-to-end and would have caught the bug. The test seeds `prev.Sessions` directly (or via an initial tick) so it does not false-green via the `prev != nil` gate. Two-tick self-heal sequence is verified.

**Do**:
- Add a test in `internal/state/capture_test.go` (e.g. `TestCaptureStructureMergeSkippedPanes/kill_mid_flight_self_heal` or a new top-level `TestCaptureStructureKillMidFlightSelfHeal`).
- **Tick 1 setup (prev population is load-bearing)**: build a `state.Index` for `prev` containing the to-be-killed session (e.g. `agentic-workflows-XXrJ3J` with a single window/pane). Construct a `captureMock` where `listSessions` returns ONLY a survivor session (e.g. `survivor`) — i.e. the to-be-killed session is absent from fresh. Build `skipSet` containing the killed session's paneKey via `state.SanitizePaneKey`.
- Call `state.CaptureStructure(client, skipSet, &prev)`. Assert: the returned `idx.Sessions` does NOT contain the killed session (`findPane(idx, killedName, 0, 0) == nil`); the survivor session IS present.
- **Tick 2 (self-heal)**: re-use the just-returned `idx` as `prev` (mirrors the daemon's `deps.PrevIndex = &idx` line at `cmd/state_daemon.go:156`). Use the same `skipSet` (the marker is still leaked). Call `state.CaptureStructure` again with the same mock. Assert the killed session remains absent — `prev` no longer perpetuates it.
- Document via comment that prev-population is load-bearing — without it, the buggy implementation also passes (false-green) because `mergeSkippedPanes` is gated on `prev != nil` AND a non-empty prev pane set.
- Run `go test ./internal/state/...` to confirm the test passes after Phase 1's filter changes.

**Acceptance Criteria**:
- [ ] A regression test exists in `internal/state/capture_test.go` that seeds `prev.Sessions` with a session, drops it from fresh, keeps its paneKey in `skipSet`, and asserts the killed session is NOT in `idx.Sessions`.
- [ ] The test runs a follow-up `CaptureStructure` call with the just-returned clean index as `prev` (same `skipSet`) and asserts the killed session remains absent — verifying the self-heal path.
- [ ] The test would fail on the current (buggy) implementation — i.e. it is not false-green via the `prev != nil` gate. (If executed against a checkpoint of the code before the filter change, the assertion fails.)
- [ ] `go test ./internal/state/...` passes after the Phase 1 filter changes.

**Tests**:
- `"kill_mid_flight_self_heal"`: seeds `prev` with the killed session, asserts tick 1 result does not contain it, threads tick 1's result into tick 2 as `prev` and asserts tick 2 result also does not contain it. Uses `state.SanitizePaneKey` to build the `skipSet` entry; uses `findPane` (or equivalent) for the assertions.
- (Optional) `"kill_mid_flight_marker_remains_but_no_resurrection"`: same scenario but explicitly documents that the marker stays in `skipSet` across both ticks (mirrors a leaked marker that survives the daemon tick boundary).

**Edge Cases**:
- The prev-population precondition is load-bearing: a test that runs `CaptureStructure` with an empty prev (or a prev that does not contain the killed session) would pass on buggy code, providing no signal. The test must seed prev with the to-be-killed session.
- The `skipSet` must be non-empty AND contain the killed session's paneKey, otherwise `mergeSkippedPanes` is short-circuited at `internal/state/capture.go:100` and the test exercises a different code path.
- Tick 2's `prev` is the index returned by tick 1 (clean) — the self-heal assertion proves the daemon's `deps.PrevIndex = &idx` propagation no longer perpetuates dead sessions.

**Context**:
> From specification → Empirical Confirmation: "Live in-the-wild observation (2026-05-08): three specific sessions resurrected after kill — `agentic-workflows-XXrJ3J`, `leeovery-Gi5NLG`, `leeovery-feqhpg`. `tmux show-options -s` revealed exactly three matching stale `@portal-skeleton-*` markers. [...] Killing an unmarkered session (`game-ideas`) did NOT resurrect it. Marker presence is necessary AND sufficient (given a daemon tick) for the resurrection symptom."
>
> From specification → Acceptance Criteria #1: "**Test setup precondition:** `mergeSkippedPanes` is gated on `prev != nil` and only resurrects sessions present in `prev.Sessions`. The regression test must establish this state before kill — either by seeding `prev.Sessions` directly in the test harness, or by allowing one daemon tick to capture-and-commit the session before the kill. **Risk if skipped:** a test that runs kill-then-tick without prior `prev` population will pass on the buggy code (false-green), so this precondition is load-bearing for the regression test's value."
>
> From specification → Self-Healing Behavior: "Once `mergeSkippedPanes` no longer reintroduces dead sessions, `sessions.json` self-heals on the next daemon tick. The polluted `prev` from prior ticks is discarded when the dead session no longer survives the merge — `captureAndCommit` then commits the clean index, and `deps.PrevIndex = &idx` propagates clean state forward."

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` (Empirical Confirmation; Self-Healing Behavior; Acceptance Criteria #1)

## daemon-merge-reintroduces-dead-sessions-1-5 | approved

### Task daemon-merge-reintroduces-dead-sessions-1-5: Preserve hydrate-in-progress merge behaviour (positive test)

**Problem**: The structural filter must not regress the legitimate hydrate-in-progress flow: phase A of restore creates the session in tmux **before** setting the `@portal-skeleton-<paneKey>` marker, so a marker-protected pane in normal hydrate flow has its session, window, and pane all present in the fresh enumeration. The existing tests in `internal/state/capture_test.go` and `internal/restore/session_markers_test.go` already cover adjacent positive paths, but the spec calls out the need for explicit confirmation that prev's authoritative pane state (CWD / CurrentCommand) still wins at matching coordinates after the structural filter is in place — and that sessions present in both fresh and prev are not duplicated by the merge.

**Solution**: Add (or extend) a positive subtest under `TestCaptureStructureMergeSkippedPanes` that asserts the legitimate hydrate-in-progress path: fresh contains the session/window/pane (mirroring phase-A skeleton restore having created them in tmux), prev contains a different CWD/CurrentCommand for the same pane, the paneKey is in `skipSet`, and the merge result has prev's CWD/CurrentCommand at the same coords with no session duplication. Confirm the existing happy-path skeleton-marker tests in `internal/restore/session_markers_test.go` remain green.

**Outcome**: An explicit positive test verifies that the structural filter does not break the legitimate hydrate-in-progress merge — phase-A skeleton-restored panes (marker set, session/window/pane all live in fresh) still take prev's pane state. Existing happy-path tests continue to pass.

**Do**:
- Identify whether the existing subtest `TestCaptureStructureMergeSkippedPanes/skip set wins over fresh data at matching coords` (or its equivalent) already covers this case. If yes, leave it in place and verify it still passes after Phase 1's filter is applied. If the existing coverage is incomplete (e.g. it does not assert that `idx.Sessions` is not duplicated, or does not name the scenario as "hydrate-in-progress"), add a new subtest.
- New positive subtest scenario (if needed): `prev` contains session `work` window `0` pane `0` with `CWD=/old`, `CurrentCommand=vim`. Fresh has the same session/window/pane with `CWD=/new`, `CurrentCommand=zsh`. `skipSet` contains the paneKey for `work__0.0`. Call `state.CaptureStructure` and assert the returned pane has `CWD=/old`, `CurrentCommand=vim` (prev wins); `len(idx.Sessions) == 1` (no duplication); canonical ordering survives.
- Run `go test ./internal/restore/...` to confirm `internal/restore/session_markers_test.go` and adjacent tests stay green — these cover the broader bootstrap-level happy-path that this work unit must not regress.
- Run `go test ./...` end-to-end to confirm no other tests are broken by the filter.

**Acceptance Criteria**:
- [ ] A positive test exists (or has been confirmed adequate) under `TestCaptureStructureMergeSkippedPanes` covering: fresh has session/window/pane present (phase-A scenario); prev has different pane state at the same coords; `skipSet` marks the paneKey; result is prev's pane state at matching coords with no session duplication.
- [ ] `internal/restore/session_markers_test.go` and adjacent happy-path skeleton-marker tests pass unchanged.
- [ ] `len(idx.Sessions)` is asserted in the new/extended test to confirm no duplication.
- [ ] Canonical ordering is preserved.
- [ ] `go test ./...` passes.

**Tests**:
- `"hydrate_in_progress_pane_merges_from_prev_at_matching_coords"` (or extension of existing `skip set wins over fresh data at matching coords`): asserts prev's CWD/CurrentCommand wins, no session duplication, canonical ordering preserved.
- `"existing_skeleton_marker_tests_pass"`: smoke confirmation that `internal/restore/session_markers_test.go` runs green (executed via `go test ./internal/restore/...`).

**Edge Cases**:
- Sessions present in BOTH fresh and prev are not duplicated by the merge — `findOrAppendSession` already guards this via name lookup, but the test asserts `len(idx.Sessions) == 1` to lock the behaviour.
- Prev pane state at matching coords overrides fresh (the existing skip-set-wins contract); the new filter does NOT change this — the matching-coords replacement path inside `mergePane` (line 142-145) is reached only when session, window, and pane are all live in fresh, which is exactly the hydrate-in-progress case.
- Canonical ordering (`resortIndex` at the end of `mergeSkippedPanes`) survives merges that touch existing sessions/windows/panes.

**Context**:
> From specification → Fix Component A → Preserved Behavior: "The merge's intended use case — hydrate-in-progress panes briefly invisible to `list-sessions` — must remain correct. Phase A of restore creates the session in tmux **before** the marker is set, so legitimate hydrate-in-progress panes always have their session/window/pane visible in the fresh enumeration. The filter is structurally distinct from this case and does not affect it."
>
> From specification → Testing Requirements → Tests to Preserve: "Existing happy-path skeleton-marker tests in `internal/restore/session_markers_test.go` — the fix must not regress legitimate hydrate-in-progress merge behaviour."
>
> From specification → Acceptance Criteria #6: "The legitimate hydrate-in-progress flow remains correct — phase A skeleton-restored panes (marker set, session/window/pane present in tmux) are still merged from prev as expected."

**Spec Reference**: `.workflows/daemon-merge-reintroduces-dead-sessions/specification/daemon-merge-reintroduces-dead-sessions/specification.md` (Fix Component A → Preserved Behavior; Testing Requirements → Tests to Preserve; Acceptance Criteria #6)
