---
topic: restore-host-terminal-windows
cycle: 2
total_proposed: 7
---
# Analysis Tasks: restore-host-terminal-windows (Cycle 2)

## Task 1: Extract the spawn log-emission shapes into internal/spawn shared helpers
status: approved
severity: high
sources: duplication

**Problem**: The two spawn surfaces each carry a hand-written copy of the same closed-vocabulary `spawn`-component log emission. `cmd/spawn.go`'s `tallyWindowResults` + `logSpawnSummary`/`logSpawnPermission`/`logSpawnUnsupported`/`logSpawnGone` (cmd/spawn.go:206-280) are byte-for-byte mirrored by `internal/tui/burst_observability.go`'s `emitBurstSummary`/`emitPermission`/`emitUnsupportedNoop`/`emitPreflightAbort` (internal/tui/burst_observability.go:32-102) — identical message strings and closed attr-key lists duplicated in full. `burst_observability.go`'s own header comment states the emit methods "MIRROR cmd/spawn.go's logSpawnSummary / tallyWindowResults / logSpawnUnsupported / logSpawnGone so the two emission paths stay byte-identical" — the duplication is acknowledged and kept in sync by hand. Because the `spawn` log component is a closed, spec-governed vocabulary (Observability & State Footprint §"Attr keys (closed set)"), an edit to a message string or attr key in one file silently drifts the taxonomy between the CLI (test seam) and the picker (dominant production path). A latent drift already exists: `emitBurstSummary` hand-rolls the confirmed count inline (`if r.Confirmed() { opened++ }`, burst_observability.go:48) while `tallyWindowResults` derives it from the shared `spawn.PartitionResults` chokepoint — so the two "byte-identical" paths already compute `opened` by different mechanisms. Cycle 1 extracted the renderers (`spawn/message.go`) and count-semantics (`spawn/classify.go`) precisely "so the two paths cannot drift"; the log emission is the one shared concern left duplicated instead of extracted.

**Solution**: Extract the four emission shapes into `internal/spawn` as shared helpers taking a `*slog.Logger` (e.g. `spawn.LogBatchSummary`, `spawn.LogPermission`, `spawn.LogUnsupported`, `spawn.LogGone`), mirroring the existing `message.go` / `classify.go` extraction pattern. Route the summary's opened/failed count through `spawn.PartitionResults` inside the shared helper so the count chokepoint is honoured on both paths. Have both `cmd/spawn.go` and `internal/tui/burst_observability.go` call them.

**Outcome**: One source of truth for the closed `spawn` log vocabulary; the manual "keep byte-identical" burden collapses; the opened/total count derives from the single `PartitionResults` chokepoint on both the CLI and picker paths.

**Do**:
1. In `internal/spawn` add a log-emission file (e.g. `logemit.go`) with four helpers taking the component logger both callers already hold:
   - `LogBatchSummary(logger, results []WindowResult, id Identity, batch string)` — the per-window `Debug("external window", "session", …, "ack", …, "detail", …)` loop then `Info(fmt.Sprintf("opened %d/%d", opened, total), "resolution", "terminal", "bundle_id", …, "opened", …, "total", …, "batch", …)`, deriving `opened`/`total` via `spawn.PartitionResults`.
   - `LogPermission(logger, id Identity, detail string)` — `Info("permission required — nothing self-attached", "resolution", "terminal", "bundle_id", …, "detail", …)`.
   - `LogUnsupported(logger, id Identity)` — `Info("unsupported terminal — nothing opened", "resolution", string(ResolutionUnsupported), "terminal", "bundle_id", …)`.
   - `LogGone(logger, gone []string)` — `Info(GoneMessage(gone))`.
   Preserve the exact message strings and closed attr-key list emitted today.
2. Replace `cmd/spawn.go`'s `tallyWindowResults`/`logSpawnSummary`/`logSpawnPermission`/`logSpawnUnsupported`/`logSpawnGone` bodies with calls to the shared helpers (keep the CLI's component logger binding).
3. Replace `internal/tui/burst_observability.go`'s `emitBurstSummary`/`emitPermission`/`emitUnsupportedNoop`/`emitPreflightAbort` bodies with calls to the same helpers; remove the inline `if r.Confirmed() { opened++ }` count in favour of the helper's `PartitionResults`-derived count.
4. Update/remove `burst_observability.go`'s "MIRROR …" header comment to point at the single shared source.

**Acceptance Criteria**:
- The four spawn log-emission shapes exist once in `internal/spawn`; neither `cmd/spawn.go` nor `internal/tui/burst_observability.go` hand-rolls the message strings or attr lists.
- The summary's opened/total is derived from `spawn.PartitionResults` on BOTH the CLI and picker paths (no residual inline confirmed-count loop in `emitBurstSummary`).
- Rendered log output (message + closed attr keys/values) at every emission site is byte-identical to today for the summary, permission, unsupported, and gone events.
- Only the closed `spawn` attr keys appear; all existing spawn unit + integration tests remain green.

**Tests**:
- Unit (spawn): each helper emits its exact message + closed attr set into a logtest sink for a representative results/identity input; `LogBatchSummary`'s opened/total matches `spawn.PartitionResults` over a mixed `AckConfirmed`/`AckTimeout`/`AckFailed` slice.
- Cross-caller parity: the CLI path and picker path produce byte-identical rendered records (message + attrs) for the same results/identity/batch across summary, permission, unsupported, and gone.
- Regression: existing `cmd/spawn.go` and `internal/tui` burst-observability log assertions pass unchanged.

## Task 2: Suppress `n` (new-session-in-cwd) while in multi-select mode
status: approved
severity: medium
sources: standards

**Problem**: The spec (Multi-Select Mode → Key coexistence within the mode) fixes a closed live-set — "Live in mode: Space (preview), / (filter), s (regroup)" — and suppresses everything else, explicitly naming "Suppressed in mode: k (kill), x (page-toggle), r (rename), and other row actions." The implementation gates `k`, `r`, and `x` behind `if m.multiSelectMode { return m, nil }` (internal/tui/model.go:3383-3425) but leaves `n` fully active. In multi-select mode, pressing `n` dispatches `handleNewInCWD` → `createSessionInCWD` (model.go:3704-3713), which creates a session and quits the picker, silently discarding the entire marked set. `n` is not in the live-set, and `x` — which is likewise not strictly a row action (it is a page-toggle) — is nonetheless explicitly suppressed, establishing the intent that non-selection/mutating actions are locked out while marking. Leaving `n` live is a divergence from the mode's decided key coexistence and destroys in-progress selection state without warning.

**Solution**: Gate the `n` case in the multi-select rune switch the same way `k`/`r`/`x` are — `if m.multiSelectMode { return m, nil }` — so create-new-session cannot fire (and silently drop the selection) while in multi-select mode.

**Outcome**: `n` is suppressed in multi-select mode, consistent with the closed live-set and the sibling `k`/`r`/`x` suppression; a stray `n` no longer discards the marked set mid-selection.

**Do**:
1. In `internal/tui/model.go`'s multi-select rune switch (~3400-3401), add the `if m.multiSelectMode { return m, nil }` guard to the `n` case, mirroring the existing `k`/`r`/`x` guards at model.go:3383-3425.
2. Confirm no other entry point routes to `handleNewInCWD` while `multiSelectMode` is true.
3. Leave the non-multi-select `n` behaviour (`handleNewInCWD` → `createSessionInCWD`) unchanged.

**Acceptance Criteria**:
- Pressing `n` while in multi-select mode is a no-op: no session is created, the picker does not quit, and the marked set is preserved.
- `n` outside multi-select mode continues to create a session in the cwd and quit as before.
- The live-set (Space, /, s) and the suppressed set (k, x, r, and now n) match the spec's key-coexistence rule.

**Tests**:
- Unit (tui): enter multi-select mode, mark ≥1 session, send `n`; assert no create/quit command is dispatched and `selectedSessions` is unchanged.
- Regression: `n` outside multi-select still dispatches `createSessionInCWD` and quits.

## Task 3: Run pre-flight before the unsupported gate on the picker burst path
status: approved
severity: medium
sources: architecture

**Problem**: The spec frames spawn as "one service, two callers" running "the identical pre-flight → sequential spawn → per-window ack → self-attach-last flow." The CLI's `runSpawn` runs the pre-flight has-session gate FIRST (cmd/spawn.go:132) — ahead of the N≥2 unsupported gate (cmd/spawn.go:152) — with an explicit in-code rationale ("so a gone session aborts with the more-actionable gone-session message even on an unsupported terminal"). The picker inverts this: `decideBurst` short-circuits to the unsupported atomic no-op on `DetectUnsupported()` (internal/tui/burst_progress.go:410) BEFORE `dispatchBurst` launches the goroutine, and pre-flight only runs inside `burstRunner.run` (burst_progress.go:188-193), which is never reached on an unsupported terminal. On the same scenario (N≥2 marked, unsupported/undriven terminal, one marked session externally killed), the CLI reports "'s2' is gone — nothing opened" and prunes it; the picker re-asserts "unsupported terminal — … — nothing opened", never runs pre-flight, and neither surfaces nor prunes the gone session — diverging from the CLI AND from the picker's own pre-flight-abort prune-keeping-survivors contract (`handlePreflightAbort`). Pre-flight is the spec's primary Enter gate ("Before opening a single window, verify every selected session still exists"; "All-or-nothing applies at the pre-flight gate"), so the picker's ordering is the outlier. Impact is bounded (nothing opens either way) but it is a genuine cross-caller flow-order divergence plus a skipped selection-prune the spec mandates.

**Solution**: Run pre-flight before the unsupported gate on the picker path so both callers share the ordering. Have `decideBurst` (or a step just ahead of it) evaluate `spawn.PreflightMissing` over the marked set first and route a non-empty result to the pre-flight-abort arm (gone message + prune survivors) before the `DetectUnsupported()` no-op — mirroring `cmd/spawn.go`'s preflight-then-unsupported sequence.

**Outcome**: Both callers evaluate pre-flight before the unsupported gate; a gone session on an unsupported terminal surfaces the more-actionable gone message and is pruned from the selection on the picker path, matching the CLI and the picker's own `handlePreflightAbort` contract.

**Do**:
1. In `internal/tui/burst_progress.go`, before the `DetectUnsupported()` short-circuit in `decideBurst` (~399-418), evaluate `spawn.PreflightMissing` over the marked set.
2. If pre-flight finds missing sessions, route to the existing pre-flight-abort arm (`handlePreflightAbort` — gone message + prune survivors) and return, before reaching the unsupported no-op.
3. Only if pre-flight passes, apply the existing `DetectUnsupported()` atomic no-op path.
4. Ensure this ordering holds for both the already-resolved and deferred-detection entry points into `decideBurst`.

**Acceptance Criteria**:
- On an unsupported/undriven terminal with N≥2 marked and one marked session externally killed, the picker surfaces the gone-session message (not the unsupported banner) and prunes the gone session from the selection — matching `cmd/spawn.go`.
- When no session is gone, the unsupported atomic no-op still fires unchanged on an unsupported terminal.
- The supported-terminal path (pre-flight then sequential spawn) is behaviourally unchanged.
- Pre-flight is evaluated before the unsupported gate on both the CLI and picker paths.

**Tests**:
- Unit (tui): N≥2 marked, `DetectUnsupported()` true, one marked session missing; assert the pre-flight-abort arm fires (gone message + survivors retained) and the unsupported no-op does NOT.
- Unit (tui): N≥2 marked, `DetectUnsupported()` true, all sessions present; assert the unsupported atomic no-op fires as before.
- Regression: supported-terminal burst dispatch and existing unsupported-no-op tests pass unchanged.

## Task 4: Extract the shared partial-failure "leave-what-opened" message renderer into internal/spawn/message.go
status: approved
severity: medium
sources: architecture

**Problem**: Cycle 1 built `internal/spawn/message.go` as the shared cross-caller renderer so the CLI and picker "name sessions identically" — `GoneMessage` and `UnsupportedNoopMessage` both derive from `QuoteJoin`/`GoneVerb` there. The partial-failure ("others left open") line was left out and is hand-built separately in each caller with different wording: the CLI emits `spawn: failed to open window(s) for 's2' — others left open` (cmd/spawn.go:195) while the picker emits `'s2' failed to open — others left open` (internal/tui/burst_partial_failure.go:116). The spec's `portal spawn` CLI contract explicitly requires the partial-spawn-failure exit-1 line to be "the same one-line message the picker would show", so this is a broken cross-caller contract and an incomplete application of the message-parity abstraction cycle 1 established — the same missed-composition shape as the gone/unsupported renderers, one message short. A future copy edit to either sentence widens the drift silently.

**Solution**: Add a sibling renderer to `internal/spawn/message.go` — `PartialFailureMessage(failed []string) string` returning the bare body (no `spawn:` prefix, no ⚠ glyph), composed from `QuoteJoin` — and have both `cmd/spawn.go` and `internal/tui/burst_partial_failure.go` render through it, so the partial-failure line joins `GoneMessage`/`UnsupportedNoopMessage` in the single parity layer.

**Outcome**: The partial-failure line has one renderer; the CLI and picker cannot diverge; the spec's "same one-line message" contract is structurally enforced.

**Do**:
1. `internal/spawn/message.go`: add `PartialFailureMessage(failed []string) string` returning the bare body composed from `QuoteJoin` (fix the single canonical wording — e.g. `fmt.Sprintf("%s %s to open — others left open", QuoteJoin(failed), <failed-verb>)`, matching whichever phrasing the spec/design pins; no `spawn:` prefix, no glyph).
2. `cmd/spawn.go` (~line 195): replace the hand-built partial-failure string with the CLI's `spawn: ` prefix wrapping `spawn.PartialFailureMessage(failed)`.
3. `internal/tui/burst_partial_failure.go` (~line 116): replace the hand-built flash text with `spawn.PartialFailureMessage(failed)` (bare body; the notice band prepends ⚠ as usual).
4. Confirm the ⚠ glyph is added by the notice band (`statusGlyph`), not the returned body, and the CLI prefix is applied only at the CLI call site.

**Acceptance Criteria**:
- The partial-failure sentence appears exactly once, inside `internal/spawn/message.go`; neither `cmd/spawn.go` nor `internal/tui/burst_partial_failure.go` hand-builds it.
- The CLI and picker render the same one-line body for the same failed-session set (the CLI additionally carries its `spawn: ` prefix; the picker additionally carries the notice band's ⚠).
- The rendered message honours the spec's "same one-line message" contract for the failed-session naming.

**Tests**:
- Unit (spawn): `PartialFailureMessage` for one name and ≥2 names (`QuoteJoin` quoting + verb agreement), bare body with no prefix/glyph.
- Cross-caller parity: the CLI exit-1 line body and the picker flash body are identical for the same failed set (modulo the CLI prefix / notice-band glyph).
- Regression: existing CLI partial-failure and picker partial-failure-flash assertions, updated to the single canonical wording, pass.

## Task 5: Extract the shared burst test-model construction prefix helper
status: approved
severity: medium
sources: duplication

**Problem**: Four burst test-model constructors repeat the same ~10-line setup: build `[]tmux.Session` from a `names` slice via `sessions[i] = tmux.Session{Name: n, Windows: i + 1}`, construct `FakeAckChannel` + `FakeAdapter`, `NewModelWithSessions`, `wireBurstSeams(…, spawn.ResolutionNative, allPresent, ack)`, `resolveDetection(…, ghosttyIdentity())`, enter multi-select (`pressSession`/`pressM`), and `for i := range names { m = markRow(t, m, i) }`. The constructors are `burstPendingModel` (internal/tui/burst_input_lock_test.go:36-57), `realCancellableBurst` (burst_cancel_test.go:97-114), `setupConfirmingBurst` (burst_selfattach_test.go:37-57), and `newPendingBurstModel` (burst_partial_failure_test.go:50-70). Three share the full wire→resolve→enter→mark-all prefix and differ only in the tail (force `burstPending` / all-false Confirm / precondition check); `newPendingBurstModel` repeats the sessions-builder and mark-all loops too. The sibling-shared-helper convention is already established here (`wireBurstSeams`, `resolveDetection`, `markRow`, `allPresent`, `ghosttyIdentity`, `driveBurstToTerminal` live once in sibling files), so this prefix is the remaining un-shared copy-paste; four near-identical builders risk drifting (e.g. the `Windows` index convention) as the burst suite grows.

**Solution**: Extract a shared `sessionsFromNames(names) []tmux.Session` helper and a `markedSupportedBurstModel(t, names) (Model, *FakeAdapter, *FakeAckChannel)` helper (the common wire→resolve→enter→mark-all prefix) into one sibling burst test file, and have the four constructors call them then apply only their distinct tail.

**Outcome**: One canonical burst test-model prefix and sessions-builder; the four constructors keep only their distinct tail; the `Windows`-index convention and wiring cannot drift across the burst suite.

**Do**:
1. In one sibling `burst_*_test.go` (or a shared burst test-helper file), add `sessionsFromNames(names []string) []tmux.Session` reproducing the `Name: n, Windows: i + 1` loop.
2. Add `markedSupportedBurstModel(t *testing.T, names []string) (Model, *FakeAdapter, *FakeAckChannel)` performing the shared `FakeAckChannel`/`FakeAdapter` build, `NewModelWithSessions`, `wireBurstSeams(…, spawn.ResolutionNative, allPresent, ack)`, `resolveDetection(…, ghosttyIdentity())`, enter multi-select, and mark-all loop.
3. Rewrite `burstPendingModel`, `realCancellableBurst`, `setupConfirmingBurst`, `newPendingBurstModel` to call these helpers then apply only their distinct tail (force `burstPending` / all-false Confirm / precondition check / etc.).
4. Keep each constructor's distinct tail behaviour identical to today.

**Acceptance Criteria**:
- The sessions-builder loop and the wire→resolve→enter→mark-all prefix exist once as shared helpers; the four constructors no longer duplicate them.
- Each constructor's distinct tail is preserved and its tests behave identically to today.
- The `Windows`-index convention (`Windows: i + 1`) is defined in exactly one place.

**Tests**:
- Regression: all four burst test suites (input-lock, cancel, self-attach, partial-failure) pass unchanged against the refactored constructors.
- The shared helpers are exercised by all four constructors (no dead helper).

## Task 6: Promote the nanoid alphabet to a single shared constant referenced by spawn and session
status: approved
severity: low
sources: duplication

**Problem**: `internal/spawn/ackid.go:19` `spawnIDAlphabet` is a byte-for-byte copy of `internal/session/naming.go:13` `alphabet` (`abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789`). `ackid.go`'s own comment flags the relationship ("identical to the session package's nanoid alphabet"), and the correctness of the option-name-safe marker scheme depends on it (no ".", ":", "-", or space). `session.alphabet` is unexported, so the constant was re-declared rather than shared. Drift risk is real — a change to the session alphabet that added a "-" would silently break the unambiguous `<batch>-<token>` marker split — currently mitigated at runtime only by `isOptionSafeID`'s post-generation check. This is the same "shared concern left duplicated instead of extracted" pattern as the log-emission and partial-failure findings, at the constant level.

**Solution**: Promote the alphabet to a single exported constant (export it from `internal/session`, or lift it into an existing leaf both packages already import) and reference it from both `ackid.go` and `naming.go`, removing the silent cross-package literal copy.

**Outcome**: One alphabet constant; a change to it is a single edit that both the session nanoid and the option-name-safe ack-id observe; the silent cross-package literal copy is gone.

**Do**:
1. Choose the single home: export `session.alphabet` (e.g. `session.NanoIDAlphabet`) or lift it into an existing leaf both packages already import — verify the dependency direction so no import cycle is introduced (`internal/spawn` must not gain a cycle via `internal/session`).
2. Reference the shared constant from `internal/session/naming.go` and `internal/spawn/ackid.go`; delete the local copies.
3. Update `ackid.go`'s comment to reference the shared constant rather than "identical to the session package's".
4. Keep `isOptionSafeID`'s post-generation safety check unchanged (defence in depth).

**Acceptance Criteria**:
- The alphabet string literal appears exactly once; both `naming.go` and `ackid.go` reference the shared constant.
- No new import cycle is introduced (verify the chosen home's dependency direction; `go build ./...` green).
- `isOptionSafeID`'s post-generation check is retained.
- Session name generation and ack-id generation behave identically to today.

**Tests**:
- Unit: the shared constant equals the previous literal; ack-id generation still passes `isOptionSafeID`; session-name generation is unchanged.
- Build: `go build ./...` green (no import cycle).

## Task 7: Remove (or document) the unreachable OutcomeUnsupported Result taxonomy member
status: approved
severity: low
sources: architecture

**Problem**: "Unsupported" is a resolution-tier concept: the Resolver returns `(nil, ResolutionUnsupported)` when no driver matches, and `OpenWindow` is only ever called on a resolved (supported) adapter. No adapter constructs an `OutcomeUnsupported` Result — the ghostty/argv/script adapters return only `Success`/`SpawnFailed`/`PermissionRequired` — so both `OutcomeUnsupported` (internal/spawn/adapter.go:29-31) and the `Unsupported()` constructor (adapter.go:59-62) are referenced solely at their own declaration. Worse than merely dead: it is a latent trap. If a future driver author returns `Unsupported()` from `OpenWindow` (a natural reading of the "closed taxonomy"), the burster classifies it as `!OK()` → `AckFailed` (burst.go:164-168) → a failed window handled by leave-what-opened with a "failed to open" flash, and `FirstPermission` (classify.go:43-50) only recognises `OutcomePermissionRequired` — so it would NOT route to the atomic unsupported no-op the resolution tier owns. The taxonomy invites a value the orchestrator has no handler for at the `OpenWindow` layer.

**Solution**: Remove `OutcomeUnsupported` and the `Unsupported()` constructor from the Adapter Result taxonomy (leaving "unsupported" solely on Resolution, where it is decided and handled). If retained for spec-completeness, document at the declaration that `OpenWindow` must never return it and that "unsupported" is a resolution-tier outcome — so a future adapter author does not reach for it and silently mis-classify.

**Outcome**: The Adapter Result taxonomy no longer carries an unreachable member that would mis-classify if produced; "unsupported" lives solely on the Resolution tier where it is decided and handled (or, if kept, its `OpenWindow`-forbidden status is documented at the declaration).

**Do**:
1. Grep for `OutcomeUnsupported` and `Unsupported()` references (expect: only their declarations in `adapter.go`). Confirm no adapter or test relies on the constructor beyond existence.
2. Preferred: remove `OutcomeUnsupported` (adapter.go:29-31) and the `Unsupported()` constructor (adapter.go:59-62); confirm the ghostty/argv/script adapters and the burster still compile and classify unchanged.
3. If retained: add a declaration-site comment stating `OpenWindow` must never return `OutcomeUnsupported` and that "unsupported" is a resolution-tier outcome handled before `OpenWindow`.
4. Keep `ResolutionUnsupported` and the resolution-tier unsupported no-op path unchanged.

**Acceptance Criteria**:
- `OutcomeUnsupported` and `Unsupported()` are either removed, or retained with an explicit declaration-site comment forbidding their return from `OpenWindow`.
- The resolution-tier unsupported handling (`ResolutionUnsupported` → atomic no-op) is unchanged.
- The package builds and all spawn tests pass; no dangling reference to a removed symbol remains.

**Tests**:
- Build/compile: `go build ./...` and `go test ./internal/spawn/...` green after removal.
- If retained: no new test required (a comment needs no assertion); existing adapter/classify tests remain green.
