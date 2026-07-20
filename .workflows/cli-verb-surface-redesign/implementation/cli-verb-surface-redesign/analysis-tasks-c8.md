---
topic: cli-verb-surface-redesign
cycle: 8
total_proposed: 4
---
# Analysis Tasks: CLI Verb Surface Redesign (Cycle 8)

## Task 1: Introduce a typed Domain across the cmd↔resolver routing boundary
status: pending
severity: medium
sources: architecture

**Problem**: The open resolution/routing pipeline discriminates its target domains with bare strings across two overlapping runtime vocabularies that must stay hand-aligned: `cmd.Target.Domain` ("bare"/"session"/"path"/"zoxide"/"alias", produced by `openTargetPins` values in `cmd/open_targets.go:33-41`) and the resolver result `Domain` fields ("session"/"path"/"alias"/"zoxide"/"glob"/"miss", on `PathResult`/`SessionResult` in `internal/resolver/query.go:45-71`). The concrete set is fully known at design time, yet it is routed by string-literal switches — `globExpandableDomain(domain string)` switches on "bare"/"session"/"alias" (`cmd/open_burst.go:76-83`), `resolveOpenSurfaces` switches on `t.Domain` literals (`cmd/open_surfaces.go:56-97`). Alignment is enforced only by convention plus guard tests (`open_targets_guard_test`, `open_pin_source_guard_test`) — at test time, not compile time. A typo in one switch arm, or a domain added to one vocabulary but not another, is a silent misroute unless a guard test happens to cover it. This is the "runtime string checks where the signature should be a specific type" anti-pattern in code-quality.md. (The third vocabulary — cobra flag names in `openTargetPins` keys — legitimately stays string-typed; cobra flag names are strings.)

**Solution**: Introduce a single typed `Domain` constant set (e.g. `type Domain string` with named constants, or an int enum with a `String()`) defined in `internal/resolver` (which `cmd` already imports). Have `cmd.Target.Domain` and the resolver `QueryResult` `Domain` fields both key off the typed values, and convert the routing switches to switch on the typed constants. Preserve the spec-coupled `resolve` log-attr value (session/path/alias/zoxide/miss) by producing it from the enum's `String()`, so the log line is byte-identical. This collapses the two runtime vocabularies into one and removes the drift class the guard tests currently paper over.

**Outcome**: `Target.Domain`, the resolver result `Domain`, and the routing switches all reference one typed constant set; the switches become exhaustive-checkable; the `resolve` decision log line's `domain` attr values are unchanged.

**Do**:
1. Define `type Domain` with the full constant set (bare, session, path, alias, zoxide, glob, miss) in `internal/resolver`, with a `String()` that returns the exact spec log-attr strings.
2. Change `resolver.PathResult.Domain` / `resolver.SessionResult.Domain` (and any construction sites in `query.go`, e.g. `&SessionResult{Domain: "session"}` at line 139, `Domain: "glob"` in `expandSessionGlobAll`) to the typed constants.
3. Change `cmd.Target.Domain` to the typed `resolver.Domain` and update `openTargetPins` values (`cmd/open_targets.go:33-41`) plus the "bare" assignment (`cmd/open_targets.go:69`) to the typed constants.
4. Convert `globExpandableDomain` (`cmd/open_burst.go`) and `resolveOpenSurfaces` (`cmd/open_surfaces.go`) switches to switch on the typed constants.
5. Where the `resolve` log line is emitted (`cmd/open.go`), source the `domain` attr value from the enum's `String()` so the emitted string is identical.
6. Update or simplify `open_targets_guard_test.go` / `open_pin_source_guard_test.go` to reflect the typed values (keep the flag-set-drift guard, which still covers cobra flag names).

**Acceptance Criteria**:
- `cmd.Target.Domain` and the resolver `QueryResult` `Domain` fields are the typed `Domain`, not `string`.
- `globExpandableDomain` and `resolveOpenSurfaces` switch on typed constants, not string literals.
- The `resolve` component log line's `domain` attr values are unchanged (session/path/alias/zoxide/miss).
- Glob routing and every domain-pin route behave identically to before.
- `go build -o portal .`, `go test ./...`, and `golangci-lint run` are clean.

**Tests**:
- Existing `open_targets_guard_test` / `open_pin_source_guard_test` pass (updated to typed values where needed).
- The `resolve` decision-line test still asserts identical `domain` attr values for a session hit, path/alias/zoxide hit, and a miss.
- A routing test confirming `globExpandableDomain` classifies bare/session/alias identically to the prior string switch and that a glob target still fans out via the burst.

## Task 2: Extract a shared exact-session-match helper and unify its error handling in the resolver
status: pending
severity: low
sources: architecture

**Problem**: The exact-session match (`ListSessionNames` + `slices.Contains`) is authored three separate times in `internal/resolver/query.go` — in `Resolve`, `ResolveSessionPin`, and `ResolveSessionPinAll` — and with subtly inconsistent error handling: `Resolve` gates on `err == nil && slices.Contains(...)` (`query.go:138`) while the pin variants ignore the lister error entirely (`names, _`). Three copies of the same match rule can silently diverge, and the lister-error handling already has. This is a parallel-implementation-that-must-stay-in-sync shape.

**Solution**: Extract one unexported helper on `QueryResolver` (e.g. `matchExactSession(query string) (matched bool, err error)` or a predicate returning just the boolean under a single decided error policy) that owns the `ListSessionNames` fetch, the `slices.Contains` membership test, and one consistent lister-error policy (treat a lister error as "no match", matching today's `Resolve` behaviour). Replace the three inline match sites with calls to it. Scope this task to the shared match helper and its error-handling unification only — the broader `*All`-vs-single-pin surface symmetry restructure the finding also discusses is out of scope here.

**Outcome**: The exact-session match rule and its lister-error policy live in exactly one place; the three former sites cannot diverge, and their error handling is consistent.

**Do**:
1. Add an unexported helper on `QueryResolver` that calls `sessions.ListSessionNames()`, applies the single lister-error policy (error → no match), and returns whether `query` is an exact member.
2. Replace the inline match in `Resolve` (`query.go:138`), `ResolveSessionPin`, and `ResolveSessionPinAll` with calls to the helper.
3. Keep the `SessionResult.Domain` value assigned at each call site (some sites yield "session", glob paths yield "glob") — only the match/error logic is centralized, not the result construction.

**Acceptance Criteria**:
- One helper owns the `ListSessionNames` + `slices.Contains` + lister-error policy; no site re-implements the match.
- `Resolve`, `ResolveSessionPin`, and `ResolveSessionPinAll` all route through the helper.
- The `err == nil` happy path behaves identically to today; the lister-error path is now consistent across all three (no match, no panic).
- `go build -o portal .`, `go test ./...`, and `golangci-lint run` are clean.

**Tests**:
- Resolver tests asserting an exact-session hit resolves correctly via `Resolve`, `ResolveSessionPin`, and `ResolveSessionPinAll`.
- A test exercising a lister-error from `ListSessionNames` across all three entry points, pinning the unified "no match, no error escalation" policy.

## Task 3: Route doctor's host-terminal Detector/Resolve through the shared spawn-seam bundle
status: pending
severity: low
sources: architecture

**Problem**: `resolveDoctorDeps` (`cmd/doctor.go:131-158`) hand-rebuilds `spawn.NewDetector(client)` and a `buildResolver().Resolve` closure for the doctor host-terminal line, deliberately NOT using `buildProductionSpawnSeams` (`cmd/spawn_seams.go:44-61`). That bundle exists specifically so the open-burst and picker paths cannot silently diverge on how the Detector + Resolve seams are constructed — its own comment states the compiler cannot catch a seam added/swapped on only one side, so the bundle is "the single source both read." doctor's in-source comment concedes the by-hand copy "must be kept in sync by hand," reintroducing exactly the drift obligation the bundle abolished and becoming a third construction site. The stated justification (a lazy `terminals.json` read) saves at most one file read on a NULL-identity run of a bootstrap-exempt command that already reads hooks.json/projects.json/sessions.json.

**Solution**: Remove the hand-built third copy. Prefer routing doctor's `Detector`/`Resolve` through `buildProductionSpawnSeams(client)` (accepting one eager `terminals.json` read on this already file-heavy, bootstrap-exempt command). If preserving the deferred `terminals.json` read is judged worthwhile, instead make the bundle itself lazy (defer `buildResolver()` inside the `Resolve` closure) so all three consumers share one construction site without the eager read. Either way, one construction site — not three.

**Outcome**: doctor's `Detector` and `Resolve` come from the shared bundle; there is no independent `spawn.NewDetector`/`buildResolver` construction in `resolveDoctorDeps`, and the "must be kept in sync by hand" note is removed.

**Do** (option A — route through the bundle):
1. In `resolveDoctorDeps`, build `seams := buildProductionSpawnSeams(client)` and set `deps.Detector = seams.Detector`, `deps.Resolve = seams.Resolve`.
2. Delete the by-hand `spawn.NewDetector(client)` + `buildResolver().Resolve` closure and the accompanying "kept in sync by hand" comment block.

**Do** (option B — make the bundle lazy, if the deferred read must be preserved):
1. Change `buildProductionSpawnSeams` so its `Resolve` field is `func(id spawn.Identity) (spawn.Adapter, spawn.Resolution) { return buildResolver().Resolve(id) }` (no eager `terminals.json` read at construction).
2. Have `resolveDoctorDeps` consume the bundle's `Detector`/`Resolve` as in option A.
3. Confirm the open-burst and picker callers still behave identically with the now-lazy Resolve.

**Acceptance Criteria**:
- doctor's `Detector` and `Resolve` originate from `buildProductionSpawnSeams`; `resolveDoctorDeps` contains no independent `spawn.NewDetector`/`buildResolver` construction.
- The doctor host-terminal report line is behaviourally unchanged (correct identity + resolution, informational-only, no exit-code impact).
- (Option B) The bundle performs no `terminals.json` read until `Resolve` is invoked.
- `go build -o portal .`, `go test ./...`, and `golangci-lint run` are clean.

**Tests**:
- Existing doctor host-terminal line tests pass unchanged.
- (Option B) A test asserting the bundle does not read `terminals.json` until `Resolve` is called (or that a doctor run with an overridden Resolve seam performs no `terminals.json` read).

## Task 4: Add an explicit sentinel at iota 0 for the doctor checkStatus enum
status: pending
severity: low
sources: standards

**Problem**: The `checkStatus` enum (`cmd/doctor.go:38-50`) declares `checkPass checkStatus = iota` — the healthy outcome sits at the zero value. The golang-naming skill explicitly forbids mapping iota 0 to a real state ("Always place an explicit Unknown/Invalid sentinel at iota position 0"). For a health diagnostic this is the most dangerous default: a zero-value `checkResult{}` silently classifies as pass, and `doctorUnhealthy` (which drives the scriptable exit code, spec § Exit-code contract) counts only `checkFail`, so a forgotten status assignment would mask a failure as green. No active bug exists today — every `checkResult` in `cmd/doctor.go` is constructed with an explicit status — so this is a defensive/convention fix, not a live defect.

**Solution**: Insert an explicit `checkUnknown` sentinel at iota 0 and shift `checkPass`/`checkFail`/`checkInfo`/`checkNotEvaluable` up by one, so an uninitialised `checkResult` can never read as a passing health check. `checkMarker`/`doctorUnhealthy` already have default arms, so no new call-site handling is required — but verify the sentinel does not render as a pass marker and does not count toward a healthy exit.

**Outcome**: A zero-value `checkResult{}` reads as `checkUnknown`, never as pass; the exit-code contract cannot be silently satisfied by a forgotten status assignment.

**Do**:
1. Add `checkUnknown checkStatus = iota` as the first constant in the `const` block (`cmd/doctor.go:38`), moving `checkPass`/`checkFail`/`checkInfo`/`checkNotEvaluable` after it (each shifts up by one).
2. Confirm `checkMarker` and `doctorUnhealthy` handle `checkUnknown` via their existing default arms such that it renders without a pass marker and does not drive a healthy exit.

**Acceptance Criteria**:
- `checkUnknown` is the iota-0 value; `checkPass` and the other three statuses are shifted up.
- A zero-value `checkResult{}` does not render as a pass marker and does not contribute to a healthy (`doctorUnhealthy == false`) result.
- The four real statuses render and drive the exit code exactly as before.
- `go build -o portal .`, `go test ./...`, and `golangci-lint run` are clean.

**Tests**:
- A test asserting a zero-value `checkResult{}` is not classified as pass by `checkMarker` and does not count as healthy.
- Existing doctor exit-code / marker tests remain green with the shifted enum values.
