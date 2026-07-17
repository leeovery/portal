---
topic: ghostty-spawn-zero-windows
cycle: 1
total_proposed: 2
---
# Analysis Tasks: ghostty-spawn-zero-windows (Cycle 1)

## Task 1: Make burstPartialFailureFlash self-contained (remove the double partition / permission scan)
status: pending
severity: low
sources: duplication, architecture

**Problem**: In the picker partial-failure path, `handleBurstPartialFailure`
(`internal/tui/burst_partial_failure.go`) already computes
`confirmed, failed := spawn.PartitionResults(msg.Results)` (line 64) and
`spawn.FirstPermission(msg.Results)` (line 43), then hands only `failed` to
`burstPartialFailureFlash(msg.Results, failed)` (line 73). The helper re-runs BOTH
`spawn.FirstPermission(results)` (line 108) and `spawn.PartitionResults(results)` (line 120,
discarding the recomputed `failed`) to derive `othersOpened = len(confirmed) > 0`. The
helper's signature is therefore a hybrid: it receives one half of the partition (`failed`)
as a parameter while re-deriving the other half (`confirmed`) and the permission scan
internally from `results`. Fix 3 introduced the second `PartitionResults` call. Because
`PartitionResults`/`FirstPermission` are pure and cheap and the single unexported caller
passes a matching `failed`, this is not a correctness bug — but it is a redundant
recomputation of values the caller already owns, makes correctness depend on caller
discipline, and is the "compose, don't duplicate" smell both the duplication and
architecture agents flagged.

**Solution**: Make `burstPartialFailureFlash` self-contained by choosing a single
derivation source. Preferred: option (b) — give the helper only `results` and let it own
the single `FirstPermission(results)` + `PartitionResults(results)` derivation internally
(it already calls both), dropping the caller-passed `failed` coupling. Equally acceptable:
option (a) — pass the already-computed pair `burstPartialFailureFlash(results, confirmed,
failed)` reusing what `handleBurstPartialFailure` holds and remove the internal
recomputation. Whichever is chosen, keep the shared `spawn.PartialFailureMessage(failed,
othersOpened)` renderer as the single source (do not inline or duplicate its copy), and
leave the CLI path (`cmd/spawn.go`) untouched — it already computes the partition once and
reuses both halves, and byte-identical CLI/picker parity must hold.

**Outcome**: `burstPartialFailureFlash` computes `PartitionResults` and `FirstPermission`
at most once per partial-failure handling pass; its signature no longer mixes a passed-in
partition half with an internally re-derived half; and the `othersOpened` signal traces to
a single chokepoint. The rendered flash text is byte-identical to today for the
total-failure, genuine-partial, permission-wall, and degenerate-empty cases — no behaviour
change.

**Do**:
1. Choose the derivation shape. Recommended option (b): change the signature to
   `burstPartialFailureFlash(results []spawn.WindowResult) string` and have it own the
   single `FirstPermission(results)` + `PartitionResults(results)` derivation internally.
2. If option (b): update the caller at `burst_partial_failure.go:73` to
   `burstPartialFailureFlash(msg.Results)`; the caller keeps its own line-64
   `confirmed, failed` solely for `applyBurstSelectionMutation(confirmed)`.
   If option (a): change the call to pass the already-held `confirmed`/`failed`, and delete
   the internal `PartitionResults` (line 120) and the redundant `FirstPermission` re-scan.
3. Confirm by reading the final code that exactly one `spawn.PartitionResults` and one
   `spawn.FirstPermission` execute along the non-permission partial path per handling pass.
4. Keep `spawn.PartialFailureMessage(failed, len(confirmed) > 0)` as the single renderer;
   do not inline or duplicate its copy.
5. Do not modify `cmd/spawn.go`; preserve byte-identical CLI/picker parity.

**Acceptance Criteria**:
- `burstPartialFailureFlash`'s signature no longer receives one partition half while
  re-deriving the other; it is fully self-contained OR receives the complete already-computed
  pair.
- Along a single partial-failure handling pass, `spawn.PartitionResults` and
  `spawn.FirstPermission` each run at most once (no discarded recomputation).
- The rendered flash is byte-identical to current behaviour for: total-failure
  ("— nothing opened"), genuine-partial ("— others left open"), permission-wall (verbatim
  `Guidance`), and degenerate-empty ("" → no band).
- CLI/picker `PartialFailureMessage` parity is preserved (existing parity tests pass).

**Tests**:
- Existing `internal/tui` burst partial-failure tests pass unchanged (total-failure,
  genuine-partial, permission-wall, degenerate-empty, user-cancel-silent arms).
- Existing CLI↔picker byte-identical `PartialFailureMessage` parity tests pass.
- If the in-package helper signature changes, update its direct unit tests to the new call
  shape without altering any behavioural assertion.

## Task 2: Close the Fix 4 compile-guard's installed-but-not-running precondition gap
status: pending
severity: low
sources: standards

**Problem**: The `ghosttycompile` prevention guard
(`internal/spawn/ghostty_compile_ghosttycompile_test.go`) gates only on Ghostty being
installed (`t.Skip` otherwise, lines 63-65), but its recorded live-Mac confirmation
(comment lines 41-58) only observed Ghostty RUNNING. Spec §Fix 4 designated one explicit
"Assumption to confirm": whether osacompile terminology resolution *requires Ghostty to be
running*, since the installed-only gate does not cover the not-running case. The guard is a
standalone manual test (`go test -tags ghosttycompile`) that can be invoked with Ghostty
installed but closed. The recorded justification pivots to "the spawn feature is only ever
invoked from a running Ghostty," which conflates the *feature's* invocation with the *guard
test's* own invocation. If terminology resolution does require a running Ghostty, an
installed-but-closed invocation would produce exactly the "false failure unrelated to the
template" the spec warned against, and the installed-only gate adopts neither of the spec's
two prescribed remedies. Impact is bounded (manual guard, not production; compile-only sdef
resolution plausibly does not need the app running) — hence low severity.

**Solution**: Adopt one of the spec §Fix 4 remedies so the guard cannot emit a false
template-drift failure in the installed-but-not-running case, discharging the open nuance
with evidence rather than a feature-invocation analogy. Two acceptable paths: (a) verify
installed-but-not-running on a live Mac and, if osacompile still resolves the terminology
cleanly with Ghostty closed, record that observation explicitly in the comment; or (b)
defensively adopt a spec-prescribed precondition adjustment with no live-Mac dependency —
distinguish a not-running-caused terminology-resolution failure from a genuine template
drift and `t.Skip` (not `t.Fatalf`) the former, or gate the guard on Ghostty running.

**Outcome**: The guard either (a) documents recorded evidence that installed-but-not-running
resolves cleanly, or (b) structurally cannot report a false template-drift failure when
Ghostty is installed but not running — a resolution failure attributable to "app not
running" is skipped/precondition-gated while a genuine `-2741` drift still fails. The spec's
"Assumption to confirm" is discharged with evidence or a defensive precondition, not the
feature-invocation analogy at lines 52-58.

**Do**:
1. Pick a path. Path (a) requires a live Mac with Ghostty installed but fully quit; path (b)
   is a defensive code change requiring no live Mac.
2. If path (a): run
   `go test -tags ghosttycompile -run TestGhosttyOpenScript_CompilesAgainstInstalledDictionary ./internal/spawn/`
   with Ghostty fully quit, then replace the lines 52-58 "spawn feature is only ever invoked
   from a running Ghostty" justification with the recorded not-running observation (GOOS,
   Ghostty installed + NOT running, osacompile exit 0 for the corrected template).
3. If path (b): keep the installed gate but change the non-zero-exit handling in the
   `if err != nil` block so a terminology-resolution failure that could stem from
   Ghostty-not-running is not reported as template drift. Preferred defensive form: still
   `t.Fatalf` on the `-2741` template-drift signature, but `t.Skip` other
   terminology-resolution failures ("terminology could not be resolved — Ghostty may not be
   running"). Update the surrounding comment to state the adopted remedy and why.
4. Do not weaken drift detection: the pre-fix `make new surface configuration with
   properties {…}` template must still fail (the `-2741` discriminator), and the corrected
   `new window with configuration {…}` template must still pass.

**Acceptance Criteria**:
- The guard's rationale no longer rests solely on the "spawn feature is only ever invoked
  from a running Ghostty" analogy; it is discharged by recorded installed-but-not-running
  evidence, or by a defensive precondition that a not-running-caused resolution failure
  cannot be reported as template drift.
- Invoking the guard with Ghostty installed but not running cannot produce a false
  `t.Fatalf` template-drift failure (it passes on clean resolution, or `t.Skip`s / is
  precondition-gated).
- A genuine drift to the pre-fix `make new surface configuration` form still fails the guard
  (the `-2741` discriminator preserved).
- The corrected committed template still passes the guard.

**Tests**:
- The guard is the test:
  `go test -tags ghosttycompile -run TestGhosttyOpenScript_CompilesAgainstInstalledDictionary ./internal/spawn/`
  passes with the corrected committed template.
- (Path a, live-Mac manual) run the guard with Ghostty installed but fully quit and confirm
  no false failure; record the observation in-source.
- No default-lane (`go test ./...`) or integration-lane behaviour changes — the file
  compiles into neither lane.
