---
topic: cli-verb-surface-redesign
cycle: 11
total_proposed: 1
---
# Analysis Tasks: cli-verb-surface-redesign (Cycle 11)

## Task 1: Name the host-terminal resolution seam type in internal/spawn
status: pending
severity: low
sources: architecture

**Problem**: The host-terminal identity to adapter resolution seam — `func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` — is the central contract that lets the picker, the multi-target open burst, and doctor share one resolver, yet it is spelled out inline at 8 distinct declaration sites across two packages (cmd, internal/tui) and never given a name. This contradicts the spawn package's own established convention: it already names the far-less-used single-return seam `ExecutableResolver func() (string, error)` (internal/spawn/command.go:6). The feature's most widely-consumed boundary contract is thus implicit and un-discoverable in the spawn API, with no compiler anchor — any future change to the signature (adding an error return, wrapping the pair in a struct) must be hand-edited at all 8 sites in lockstep. Verified sites:
- cmd/open.go:662 (openDeps.resolve field)
- cmd/doctor.go:129 (doctorDeps.Resolve field)
- cmd/doctor.go:404 (checkHostTerminal parameter)
- cmd/spawn_seams.go:36 (productionSpawnSeams.Resolve field)
- cmd/open_burst_run.go:33 (Resolve field)
- internal/tui/build.go:54 (Resolve field)
- internal/tui/spawn_detect.go:42 (WithResolve option parameter)
- internal/tui/model.go:465 (model.resolve field)

**Solution**: Declare a named type `AdapterResolver func(Identity) (Adapter, Resolution)` in internal/spawn alongside `ExecutableResolver`, then reference `spawn.AdapterResolver` at all 8 sites in place of the inline spelling. Pure naming extraction — no signature or behaviour change.

**Outcome**: The resolution seam is a first-class, named part of the spawn API — one definition referenced everywhere. A future signature change has a single compiler-anchored definition instead of 8 lockstep hand-edits, the boundary contract becomes discoverable in the spawn package next to its `ExecutableResolver` precedent, and the seam matches the package's own established named-func-type convention.

**Do**:
1. In internal/spawn (in command.go next to `ExecutableResolver`, or a suitable seams file), declare `type AdapterResolver func(Identity) (Adapter, Resolution)` with a doc comment describing it as the host-terminal identity→adapter resolution seam shared by the picker, the multi-target open burst, and doctor.
2. Replace the inline `func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` with `spawn.AdapterResolver` at each struct-field site: cmd/open.go:662, cmd/doctor.go:129, cmd/spawn_seams.go:36, cmd/open_burst_run.go:33, internal/tui/build.go:54, internal/tui/model.go:465.
3. Replace the inline type at each function/option parameter site: cmd/doctor.go:404 (checkHostTerminal's `resolve` parameter), internal/tui/spawn_detect.go:42 (WithResolve's `fn` parameter).
4. Within internal/spawn reference the type unqualified (`AdapterResolver`); in cmd and internal/tui reference it as `spawn.AdapterResolver`.
5. Do not alter the signature itself, any wiring, or any behaviour — this is a type-naming extraction only. Leave the separately-named `ExecutableResolver` seam untouched.

**Acceptance Criteria**:
- `type AdapterResolver func(Identity) (Adapter, Resolution)` is declared exactly once in internal/spawn with a doc comment.
- No inline `func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` spelling remains in cmd/ or internal/tui/ production code — a grep for the inline signature returns nothing; all 8 sites reference `spawn.AdapterResolver`.
- Wiring and behaviour are unchanged; the seam signature is byte-identical to before.
- `go build ./...` succeeds, `go test ./...` passes, and `golangci-lint run` is clean.

**Tests**:
- No new behavioural tests required — this is a pure refactor. The existing spawn, open-burst, doctor, and tui test suites must continue to pass unchanged, proving the seam's behaviour and wiring are preserved.
- Verify (grep or equivalent) that the inline signature no longer appears at any call site and that the named type is the single point of definition.
