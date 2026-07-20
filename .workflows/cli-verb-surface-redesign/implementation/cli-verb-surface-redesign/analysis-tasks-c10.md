---
topic: cli-verb-surface-redesign
cycle: 10
total_proposed: 2
---
# Analysis Tasks: cli-verb-surface-redesign (Cycle 10)

## Task 1: Extract shared down-server result helper in doctor.go
status: pending
severity: low
sources: duplication

**Problem**: The three runtime health checks in `cmd/doctor.go` — `checkDaemonAlive` (483-487), `checkSaverUp` (511-515), and `checkHooksRegistered` (542-546) — each open with the identical down-server guard: a `const name` followed by `if !serverUp { return checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning} }`. This is a clean Rule-of-Three instance: the same "runtime not running" result shape is constructed verbatim in three sibling functions. The shared detail string is already single-sourced as the `doctorRuntimeNotRunning` constant, which signals the intent to consolidate the whole result rather than only its text. A future change to how a down server is reported (e.g. switching `checkFail` to a distinct status, or adjusting the marker) would have to be applied in three places in lockstep.

**Solution**: Extract a `runtimeDownResult(name string) checkResult` helper in `cmd/doctor.go` that returns `checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning}`, and call it from the `!serverUp` arm of all three checks. Purely a consolidation of existing code — no behaviour change.

**Outcome**: The down-server result shape is single-sourced; a future change to how a down server is reported becomes a one-line edit. Doctor's exit-code contract and rendered output are byte-identical to before.

**Do**:
- Add `runtimeDownResult(name string) checkResult` to `cmd/doctor.go` returning `checkResult{name: name, status: checkFail, detail: doctorRuntimeNotRunning}`.
- Replace the `!serverUp` return arm in `checkDaemonAlive`, `checkSaverUp`, and `checkHooksRegistered` with a call to `runtimeDownResult(name)`.
- Preserve each check's existing `const name` value verbatim so the produced result is unchanged.

**Acceptance Criteria**:
- All three checks return via `runtimeDownResult` on the `!serverUp` path.
- The produced `checkResult` (name/status/detail) is byte-identical to the current behaviour for each of the three checks.
- No behaviour change: doctor's per-line output and exit-code contract (0 iff all pass; down server → fail via daemon/saver/hooks) are unchanged.

**Tests**:
- Existing doctor tests asserting the down-server `checkFail` / `doctorRuntimeNotRunning` results for the daemon, saver, and hooks checks pass unchanged.
- Add or extend a focused unit test asserting `runtimeDownResult(name)` returns `checkFail` + `doctorRuntimeNotRunning` for each of the three check names.

## Task 2: Remove the fabricated-Domain latent trap in the degenerate single-surface burst
status: pending
severity: low
sources: architecture

**Problem**: For the degenerate single-surviving-surface case, `dispatchOpenBurst` converts a `spawn.Surface` back into a `resolver.QueryResult` via `surfaceToResult` (`cmd/open_burst.go:148`, `:154-163`) so it can reuse `openResolved`'s shared command-guard + ack-write + connector dispatch (`cmd/open.go:360-378`). `surfaceToResult` *fabricates* the domain: every mint surface is reconstructed as `PathResult{Domain: DomainPath}` regardless of whether the original hit was an alias or zoxide match, and every attach as `SessionResult{Domain: DomainSession}` (never `DomainGlob`). This is correct today only because `openResolved` reads the result's concrete TYPE (attach vs mint) but never its `.Domain` field, and the resolve-decision log line was already emitted upstream in `resolveOpenSurfaces` — so the fabricated `Domain` is dead data on this path. It is a latent trap, not a live bug: any future change that made `openResolved` consult `r.Domain` (to emit/re-emit a resolve line, drive a metric, or branch behaviour) would silently log/act on `DomainPath` for what was actually an alias or zoxide mint. Correctness rests on caller discipline (`openResolved` must keep ignoring `Domain`) — a non-self-contained assumption that composes badly across future edits. The backward conversion is also mild unnecessary indirection: the value already flowed `QueryResult → Surface` moments earlier.

**Solution**: Make the degenerate single-surface path self-contained rather than dependent on "openResolved never reads Domain." Preferred (cheapest): have `resolveOpenSurfaces` (or `dispatchOpenBurst`) retain the original `resolver.QueryResult` for the single-surviving-surface case and pass that real result to `openResolved`, so no domain is fabricated. Alternatively, carry the true `resolver.Domain` on `spawn.Surface` so `surfaceToResult` reconstructs accurate provenance. If the round-trip is kept as-is, at minimum add a focused comment/assert at `surfaceToResult` stating the fabricated `Domain` is intentionally never consumed and must not be read (documenting the invariant the correctness rests on).

**Outcome**: The degenerate single-surface path no longer depends on the non-self-contained "openResolved never reads Domain" assumption — the dormant wrong-value trap is removed, or (if the round-trip is retained) the invariant is explicitly documented so a future edit cannot silently trip it. No behavioural change today.

**Do**:
- Preferred: thread the original `resolver.QueryResult` through `resolveOpenSurfaces`/`dispatchOpenBurst` for the single-surviving-surface degenerate case and pass it to `openResolved`, instead of reconstructing via `surfaceToResult`.
- If threading the real result is impractical, either carry `resolver.Domain` on `spawn.Surface` and reconstruct accurate provenance in `surfaceToResult`, or add a focused comment/assert at `surfaceToResult` documenting that the fabricated `Domain` is intentionally never consumed and must not be read.
- Do not change observable behaviour: attach-vs-mint dispatch, the command guard, the ack write, connector dispatch, and the upstream resolve-decision log line must all stay identical.

**Acceptance Criteria**:
- The degenerate single-surface path no longer passes a fabricated `Domain` into `openResolved` (preferred), OR — if the round-trip is retained — the invariant is explicitly documented at `surfaceToResult`.
- attach-vs-mint dispatch, command-guard, ack-write, and connector-dispatch behaviour are unchanged.
- The upstream resolve-decision log line (emitted in `resolveOpenSurfaces`) is unchanged — no duplicate or incorrect resolve line is produced.

**Tests**:
- Existing single-target open-burst tests (attach and mint, including alias- and zoxide-origin mints) pass unchanged.
- If the real `QueryResult` is threaded through, add or extend a test asserting that an alias- or zoxide-origin single mint carries its true domain (not `DomainPath`) at the `openResolved` boundary.
