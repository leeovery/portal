---
topic: state-notify-cascade-on-binary-upgrade
cycle: 2
total_proposed: 4
---
# Analysis Tasks: state-notify-cascade-on-binary-upgrade (Cycle 2)

## Task 1: Extract a shared per-event eviction helper used by both convergeEvent and UnregisterPortalHooks
status: approved
severity: medium
sources: duplication, architecture

**Problem**: `convergeEvent` (`internal/tmux/hooks_register.go:193-240`) and `UnregisterPortalHooks` (`internal/tmux/hooks_unregister.go:87-115`) independently re-author the same per-event pipeline: read via `ShowGlobalHooksForEvent(event)` → on error emit the canonical `show-hooks failed` WARN (`error_class=unexpected`) and wrap `"show-hooks failed[ on %s]: %w"` → collapse the whole-table parse with `ParseShowHooks(raw)[event]` → filter to Portal-authored entries via `containsAny` → sort matching indices descending → loop `UnsetGlobalHookAt(event, idx)` best-effort. Four cycle-2 findings converge on this one pipeline: (a) duplication MEDIUM — two ~20-line eviction loops that must stay behaviourally identical (same reverse-order semantics, same best-effort-continue contract) yet drifted in sort idiom (`sort.Reverse(sort.IntSlice)` vs `sort.Slice` predicate) and in filter (hand-rolled loop vs `portalEntriesFor`); (b) duplication LOW — the `show-hooks failed` WARN message + `error_class="unexpected"` literal + wrap prefix is a single contract written verbatim in both callers; (c) architecture LOW — the `ParseShowHooks(raw)[event]` single-event collapse is re-implemented in three places with no single home for a future "parsed key != requested event" guard; (d) architecture LOW — the two halves source the WARN from different loggers (`convergeEvent` uses the injected `*slog.Logger`, `UnregisterPortalHooks` uses the package-level `bootstrapLogger`), so the teardown WARN is structurally untestable through the same recording-logger seam the registration WARN is pinned by. This is a copy-paste-drift surface, not a correctness defect — both paths are correct today.

**Solution**: Introduce two thin unexported helpers in the `tmux` package and route both production callers through them: (1) `parseEventEntries(raw, event string) []HookEntry` wrapping `ParseShowHooks(raw)[event]`, giving the single-event collapse exactly one home (and one future place to add a "parsed key != requested event" guard); (2) a shared eviction helper, e.g. `evictPortalEntries(c *Client, logger *slog.Logger, event string, entries []HookEntry) (evicted int, err error)`, owning the descending-index sort + per-index `UnsetGlobalHookAt` best-effort loop with the per-index `failed to evict portal hook` WARN. Optionally fold the read+WARN+wrap into a `readEventEntries(c, logger, event) ([]HookEntry, error)` helper that emits the canonical `show-hooks failed` WARN and returns the wrapped error, so the WARN message + `error_class` + wrap prefix have one production definition. Route the Portal-authored filter through the existing `portalEntriesFor`-style predicate so there is one definition of "Portal-authored entry on this event."

**Outcome**: One production definition each of: the per-event read+WARN+wrap, the Portal-authored filter, the descending reverse-unset loop, and the `ParseShowHooks(raw)[event]` collapse. `convergeEvent` keeps only its registration-specific logic (idempotent fast-path equality check + final `AppendGlobalHook`); `UnregisterPortalHooks` keeps only its aggregation. The teardown and registration WARN shapes can no longer drift independently.

**Do**:
1. Add `parseEventEntries(raw, event string) []HookEntry` to the `tmux` package returning `ParseShowHooks(raw)[event]`. Do NOT modify `ParseShowHooks` itself — the spec mandates zero changes to it (`internal/tmux/hooks_parse.go:42` stays byte-identical).
2. Add the shared eviction helper that takes the already-filtered Portal entries (or filters internally via a `portalEntriesFor`-style predicate), sorts indices descending, loops `UnsetGlobalHookAt`, emits the per-index `failed to evict portal hook` WARN (carrying only the `error` attr) on failure, continues, and returns the successfully-evicted count. Decide the error contract per caller: `convergeEvent`'s per-index failures are best-effort (WARN + continue, not counted); `UnregisterPortalHooks`'s per-index failures fold into its `errors.Join` aggregate naming `event[index]`. If a single helper cannot serve both error contracts cleanly, have it return the per-index errors (or a `[]error`) and let each caller apply its own folding — do NOT collapse the two distinct error-aggregation behaviours.
3. Optionally add `readEventEntries(c, logger, event)` (or `warnShowHooksFailure(logger, event, err)` returning the wrapped error) so the `show-hooks failed` WARN + wrap has one definition. Preserve the exact wrap-prefix difference if the callers intentionally differ: registration wraps `"show-hooks failed: %w"`, teardown wraps `"show-hooks failed on %s: %w"`. If unifying, pick the event-named form and update the register-side test expectation accordingly only if a test pins the bare-form string.
4. Route `convergeEvent` and `UnregisterPortalHooks` through the new helpers, deleting the inline duplicated blocks.
5. Resolve the WARN-sink asymmetry by ONE of: (a) the minimal option — add a one-line comment in `convergeEvent` cross-referencing `bootstrapLogger` (`internal/tmux/hooks_unregister.go:17`) as the deliberate teardown counterpart so future edits keep both WARN shapes in lockstep; or (b) the stronger option — give `UnregisterPortalHooks` an injected-logger inner variant (e.g. `unregisterPortalHooks(c *Client, logger *slog.Logger) error`) so the teardown WARN is exercisable through the same recording-logger seam, and retain the no-logger `UnregisterPortalHooks(c *Client) error` wrapper that calls it with `bootstrapLogger`. If a shared `readEventEntries` helper from step 3 already routes the teardown WARN through an injected logger, option (b) is largely already achieved — prefer it and add a teardown-WARN unit assertion.
6. **CONSTRAINTS — do not violate**: (i) `UnregisterPortalHooks` MUST keep its exported signature `func(*Client) error` — it is consumed as a function value by `cmd/state_cleanup.go`, so the no-logger wrapper must remain. (ii) Preserve the deliberate registration-vs-teardown fingerprint-set divergence: teardown's `portalCommandSubstrings` retains `portal state migrate-rename` (legacy-binary cleanup, `internal/tmux/hooks_unregister.go:19-36`) and the registration table never installs it — the shared filter helper must be parameterised by fingerprint set, not hard-coded to one. (iii) Do NOT change `ParseShowHooks`. (iv) Stay inside the closed log taxonomy: no new component or attr key (`bootstrap` component, `error`/`error_class` attrs only).

**Acceptance Criteria**:
- The `ParseShowHooks(raw)[event]` collapse appears in exactly one production location (`parseEventEntries`); both callers route through it.
- The descending-index `UnsetGlobalHookAt` best-effort loop has one production definition; both callers route through it.
- The `show-hooks failed` WARN (`error_class=unexpected`) + wrap shape has one production definition OR is provably kept in lockstep by an explicit cross-reference comment.
- `UnregisterPortalHooks` retains the exact exported signature `func(*Client) error`; `cmd/state_cleanup.go` still compiles against it as a function value with no change.
- `portal state migrate-rename` remains in teardown's fingerprint set and absent from registration's; the registration-vs-teardown fingerprint divergence is preserved.
- `ParseShowHooks` (`internal/tmux/hooks_parse.go`) is byte-for-byte unchanged.
- No new log component or attr key is introduced.
- `go build`, `go vet ./internal/tmux/...`, and `go test ./internal/tmux/...` pass; `go test ./cmd/...` passes (covers the `state_cleanup.go` function-value consumer).

**Tests**:
- All existing `internal/tmux` unit and real-tmux tests pass unchanged (the refactor is behaviour-preserving): no-growth-across-bootstraps, K-deep self-heal, teardown-at-depth on both blind events, churn-free index stability, the `assertShowHooksWarnShape` register-side WARN pin, and the per-event read-failure fold-and-continue tests.
- If option (b) is taken (injected-logger teardown variant): add a unit test asserting the teardown read-failure emits the canonical `show-hooks failed` WARN (`error_class=unexpected`) through the recording-logger seam — closing the gap `TestUnregisterPortalHooks_ReapsAtDepth...` notes it cannot currently assert.
- The per-event read-failure fold-and-continue semantics remain verified for BOTH paths (register folds into `errors.Join` and continues; teardown folds `show-hooks failed on %s` and continues) with the no-double-log invariant preserved.

## Task 2: Consolidate the test-side "read-per-event → ParseShowHooks → count-by-fingerprint" helper
status: approved
severity: medium
sources: duplication

**Problem**: The same test-helper body — `ShowGlobalHooksForEvent(event)` with `t.Fatalf` on error → `ParseShowHooks(raw)[event]` → iterate entries counting/matching `strings.Contains(e.Command, fingerprint)` — is independently re-authored roughly seven times across three test files: `countPortalEntriesForEvent` (`internal/tmux/hooks_register_realtmux_test.go:70-84`), `hasPortalEntry` (`:239-246`), `countSignalHydrateEntries` (`internal/tmux/hooks_migration_test.go:39-55`), and the inline loops in `TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed` and `..._DoesNotEvictHandAuthoredHooksLackingFingerprint` (`hooks_migration_test.go:106-122`, `:278-293`). `hooks_register_realtmux_test.go` and `hooks_migration_test.go` share the same external `tmux_test` package, so `countPortalEntriesForEvent` can already serve all of them with zero new abstraction — they were authored independently across task boundaries. The argv/parse-shape decoding is duplicated enough to drift if the per-event seam or fingerprint matching ever changes.

**Solution**: Within the `tmux_test` package, make one helper the single source of truth — keep `countPortalEntriesForEvent` (or add a small `portalEntryCommandsForEvent(t, client, event, fingerprint) []string` returning the matching command bodies, with `countPortalEntriesForEvent` becoming `len(...)` over it). Route `countSignalHydrateEntries` (which becomes a thin map-builder iterating `HydrationTriggerEvents` over the shared helper) and the two `hooks_migration_test.go` inline loops through it. Leave `hasPortalEntry` as a boolean convenience or express it as `> 0` over the shared count — author's judgment.

**Outcome**: One `tmux_test`-package definition of "read a single event, parse, count/list entries matching a fingerprint." `countSignalHydrateEntries` collapses to a map-builder; the two migration inline loops disappear. Coverage and assertions are unchanged.

**Do**:
1. Choose the canonical primitive: either keep `countPortalEntriesForEvent(t, client, event, fingerprint) int` or introduce `portalEntryCommandsForEvent(t, client, event, fingerprint) []string` and define `countPortalEntriesForEvent` as `len(portalEntryCommandsForEvent(...))`.
2. Rewrite `countSignalHydrateEntries` (`hooks_migration_test.go`) as a thin loop over `tmux.HydrationTriggerEvents` calling the canonical helper with the `"portal state signal-hydrate"` fingerprint, building the `map[string]int`.
3. Replace the inline ParseShowHooks-and-count loops in `TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed` and `..._DoesNotEvictHandAuthoredHooksLackingFingerprint` with calls to the canonical helper.
4. Preserve the load-bearing per-event-read rationale: the helper MUST read via `ShowGlobalHooksForEvent(event)`, never the no-arg global form (a no-arg read is itself blind to `pane-focus-out`/`window-layout-changed`, making the count assertions vacuous). Keep the explanatory comment on the canonical helper.
5. Leave `verifyHydrationHookEntries` in `cmd/bootstrap/reboot_roundtrip_test.go:1284-1308` AS-IS — it is in a different package and cannot consume an unexported `tmux_test` helper. (Out of scope unless a shared cross-package test-helper package is introduced, which it is not.)

**Acceptance Criteria**:
- Exactly one `tmux_test`-package helper implements the read-per-event → parse → fingerprint-match body; `countSignalHydrateEntries` and both migration inline loops route through it.
- The canonical helper reads exclusively via `ShowGlobalHooksForEvent`; no caller reverts to a no-arg global read.
- `verifyHydrationHookEntries` (different package) is unchanged.
- `go test ./internal/tmux/...` passes with identical coverage and assertions.

**Tests**:
- All existing `internal/tmux` migration and real-tmux tests pass unchanged; this is a test-helper refactor with no production or assertion-semantics change.
- The blind-spot guard (per-event read is the only non-blind oracle) still holds: the migration count assertions remain non-vacuous on `pane-focus-out`/`window-layout-changed`.

## Task 3: Reuse the existing set-hook argv extractors instead of inline mock.Calls scanning
status: approved
severity: low
sources: duplication

**Problem**: `hooks_register_test.go` defines `setHookCalls` (`internal/tmux/hooks_register_test.go:204-212`) and `hooks_unregister_test.go` defines `unsetHookCalls` (`hooks_unregister_test.go:56-64`), yet several tests in the same package re-walk `mock.Calls` inline with the literal `len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga"` guards (and the `-gu` equivalent) to recover append/unset ordering or to split the event-name prefix before `"["` (`hooks_register_test.go:348-360`, `:437-449`, `:474-484`; `hooks_register_warn_test.go:147-152`; `hooks_migration_test.go:387-397`). This is the same argv-shape decoding copied across multiple test bodies — short and assertion-local, but the index literals and `"set-hook"`/`"-ga"`/`"-gu"` strings repeat enough to drift if the mock call shape ever changes.

**Solution**: Where a test needs append/unset ordering relative to each other (the K-deep-stack-collapse and stale-notify-on-session-closed "append follows unset" assertions), add one small ordered accessor co-located with `setHookCalls`/`unsetHookCalls` — e.g. `setHookEvents(calls) [][2]string` returning ordered `[verb, target]`, or an `eventOfUnsetTarget` splitter that extracts the event-name prefix before `"["` — and reuse it across the inline sites, rather than re-encoding the argv guards per test. Keep the mock-argv contract (the index literals and verb/flag strings) in one place next to the existing extractors.

**Outcome**: The `set-hook -ga`/`-gu` argv-decoding literals live in one or two accessor functions co-located with the existing extractors; the inline `mock.Calls` walks are replaced by accessor calls. If the mock call shape changes, only the accessors need updating.

**Do**:
1. Add a small ordered accessor next to `setHookCalls`/`unsetHookCalls` covering the cases the inline loops handle: cross-verb ordering (append-vs-unset interleaving) and/or the event-name-prefix split before `"["`. Author's judgment on whether one combined accessor or two focused ones (`setHookEvents` + `eventOfUnsetTarget`) reads best.
2. Replace the inline `mock.Calls` scans at `hooks_register_test.go:348-360`, `:437-449`, `:474-484`; `hooks_register_warn_test.go:147-152`; `hooks_migration_test.go:387-397` with calls to the new accessor(s) and the existing `setHookCalls`/`unsetHookCalls`.
3. Keep all assertion semantics identical — only the extraction mechanism changes.

**Acceptance Criteria**:
- The `len(c) >= 4 && c[0] == "set-hook" && c[1] == "-ga"` (and `-gu`) argv guards no longer appear inline in test bodies; they live only in the extractor/accessor functions.
- The new accessor(s) are co-located with `setHookCalls`/`unsetHookCalls`.
- The ordering and event-prefix assertions (K-deep-stack-collapse, stale-notify-on-session-closed "append follows unset") are unchanged in meaning.
- `go test ./internal/tmux/...` passes.

**Tests**:
- All existing `internal/tmux` register/unregister/migration/warn tests pass unchanged; this is a test-helper refactor with no assertion-semantics change.

## Task 4: Rename test functions that reference deleted migration helpers
status: approved
severity: low
sources: standards

**Problem**: The spec's Migration-Helper Consolidation decided to DELETE `migrateHydrationHooks` and `migrateSessionClosedHook` and fold their behaviour into the single ensure-exactly-one `convergeEvent` path. The production deletion was done correctly (no production symbol or comment references them). But the test function names still encode the deleted helpers as their subject: `TestMigrateHydrationHooks_*` (8 functions in `internal/tmux/hooks_migration_test.go` at lines :73, :129, :169, :202, :254, :313, :365, :421, plus 1 in `hooks_register_warn_test.go:86`) and `TestConvergeSessionClosed_ShowHooksFailureWarnIsNormalized` (`hooks_register_warn_test.go:123`). There is no `migrateHydrationHooks`, `migrateSessionClosedHook`, or `convergeSessionClosed` symbol — the production function is `convergeEvent`. The bodies and doc comments are accurate (they correctly drive `RegisterPortalHooks` and explain the helpers were deleted); only the names are stale, mildly misleading a future reader who greps for the nonexistent helper. Not a behavioural or coverage gap. Per the project convention that comments/names must accurately describe the code they sit on.

**Solution**: Rename the affected test functions to describe the convergence behaviour under test rather than the deleted helper. Suggested renames: `TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed → TestRegisterPortalHooks_HydrationConvergesUnSeparatedToDashForm`; `TestConvergeSessionClosed_ShowHooksFailureWarnIsNormalized → TestRegisterPortalHooks_SessionClosedReadFailureEmitsCanonicalWarn`. Apply the same `TestMigrateHydrationHooks_* → TestRegisterPortalHooks_*` (behaviour-describing) pattern to the remaining 7. The accurate file-level and per-test doc comments stay.

**Outcome**: Every test function name describes an observable behaviour of the live `RegisterPortalHooks`/`convergeEvent` path; no test name references a symbol that no longer exists. Grepping for the deleted helpers returns only the accurate explanatory comments.

**Do**:
1. Rename the 9 `TestMigrateHydrationHooks_*` functions (8 in `hooks_migration_test.go`, 1 in `hooks_register_warn_test.go`) to `TestRegisterPortalHooks_*` names that describe the convergence behaviour each asserts. Preserve each test's specific intent in the new name (e.g. the un-separated-eviction one, the hand-authored-survival one).
2. Rename `TestConvergeSessionClosed_ShowHooksFailureWarnIsNormalized` to a `TestRegisterPortalHooks_SessionClosed...` name describing the canonical-WARN normalisation it pins.
3. Update any references to these renamed functions (e.g. `t.Run` subtests, cross-references in comments that name the test by symbol — but leave comments that describe the deleted *production* helpers, since those are accurate).
4. Do NOT alter any test body, assertion, or doc comment that accurately describes the deleted-helper history — only the function names change.

**Acceptance Criteria**:
- No test function name references `migrateHydrationHooks`, `migrateSessionClosedHook`, or `convergeSessionClosed` (none of which exist as symbols).
- Each renamed test name describes the `RegisterPortalHooks`/convergence behaviour it exercises.
- Test bodies, assertions, and the accurate doc comments are unchanged.
- `go test ./internal/tmux/...` passes (all renamed tests still run and pass).

**Tests**:
- The renamed tests execute and pass with identical assertions; `go test -run TestRegisterPortalHooks ./internal/tmux/...` discovers the renamed functions.
