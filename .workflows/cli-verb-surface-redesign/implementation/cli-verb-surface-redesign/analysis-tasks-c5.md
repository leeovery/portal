---
topic: cli-verb-surface-redesign
cycle: 5
total_proposed: 1
---
# Analysis Tasks: CLI Verb Surface Redesign (Cycle 5)

## Task 1: Correct doctor's host-terminal seam provenance comments (they claim shared-bundle single-sourcing that does not exist)
status: pending
severity: low
sources: duplication

**Problem**: `resolveDoctorDeps` (cmd/doctor.go:142-145) independently constructs the host-terminal `Detector` (`spawn.NewDetector(client)`) and `Resolve` (`buildResolver().Resolve` closure) pair inline, rather than routing through `buildProductionSpawnSeams` (cmd/spawn_seams.go:51-61) — the designated single construction site whose stated purpose is that the open-burst (`buildOpenBurstDeps`) and picker (`openTUI`/`tuiConfig`) paths "cannot drift." Doctor is thus a third consumer of the identical detector+resolve wiring that bypasses the anti-drift bundle. Compounding the divergence, doctor's own comments assert shared-bundle provenance that does not exist: the `DoctorDeps.Detector` field comment (cmd/doctor.go:99-100) says it is "the SAME Detect() seam the picker and the multi-target open burst use (cmd/spawn_seams.go)", the `Resolve` field comment (cmd/doctor.go:104-106) says "the SAME config-aware resolver the burst uses", and `checkHostTerminal` (cmd/doctor.go:362-364) says the line is "computed from the SAME Detect()+Resolve seams the picker and the multi-target open burst use." A maintainer who later changes how `buildProductionSpawnSeams` builds the pair (a new `NewDetector` parameter, a different resolve construction) would reasonably believe the change propagates to doctor when it does not — silently drifting doctor's host-terminal line from the burst paths.

**Solution**: Correct doctor's comments to state the truth: the detector+resolve pair is INDEPENDENTLY re-constructed in `resolveDoctorDeps` from the same primitives (`spawn.NewDetector` + `buildResolver`), deliberately NOT routed through the shared `buildProductionSpawnSeams` bundle — because doctor defers the terminals.json read behind its `Resolve` closure (a NULL/remote identity never triggers it) whereas the bundle reads terminals.json eagerly at construction. Note in-source that, because the construction is independent, a change to the bundle's detector/resolve wiring must be mirrored here by hand. This is a comment-only change; the wiring is left exactly as-is. (Optional, executor's discretion only — NOT required: the drift could instead be closed by routing doctor through `buildProductionSpawnSeams(client)` and reading its `Detector`/`Resolve` fields. That path either introduces an eager terminals.json read on the doctor path — churning doctor's deliberate laziness — or requires making the bundle's `Resolve` field lazy, rippling through both burst callers. Both touch working, tested wiring out of proportion to a low-severity concern in a converging loop; prefer the comment-accuracy fix.)

**Outcome**: The comments accurately describe the wiring — a reader understands doctor re-builds the detector+resolve pair independently (for the deliberate deferred terminals.json read) rather than sharing the `buildProductionSpawnSeams` bundle, and knows the two must be kept in sync manually. No behavior change; the misleading "flows from the shared bundle" impression is removed.

**Do**:
1. Edit the `DoctorDeps.Detector` field doc-comment (cmd/doctor.go ~98-103): remove the "SAME Detect() seam the picker and the multi-target open burst use (cmd/spawn_seams.go)" phrasing. Replace with a note that production wires `spawn.NewDetector` over the doctor tmux client, constructed independently in `resolveDoctorDeps` — NOT via the shared `buildProductionSpawnSeams` bundle.
2. Edit the `DoctorDeps.Resolve` field doc-comment (cmd/doctor.go ~104-108): remove "SAME config-aware resolver the burst uses". Replace with a note that `buildResolver().Resolve` is wrapped in a closure so terminals.json is read lazily (only when a non-NULL identity computes the line), and that this deliberate laziness is precisely why doctor does not adopt the eager `buildProductionSpawnSeams` bundle.
3. Edit the `checkHostTerminal` doc-comment (cmd/doctor.go ~362-364): reword "computed from the SAME Detect()+Resolve seams the picker and the multi-target open burst use" so it no longer implies shared-bundle single-sourcing — e.g. "computed from the same detection recipe (`spawn.NewDetector` + `buildResolver().Resolve`) the picker and open burst use, re-constructed independently in `resolveDoctorDeps`."
4. Confirm the inline comment at `resolveDoctorDeps` (cmd/doctor.go 136-141) documents the deferred-read rationale; augment it with an explicit "not routed through `buildProductionSpawnSeams` — kept in sync by hand" note if that is not already conveyed.

**Acceptance Criteria**:
- No comment in cmd/doctor.go asserts doctor uses "the SAME" seam/resolver as the picker/burst in a way that implies it reads from the shared `buildProductionSpawnSeams` bundle.
- The comments explicitly state the detector+resolve pair is independently constructed in `resolveDoctorDeps`, and name the deliberate deferred terminals.json read as the reason doctor does not use the eager bundle.
- No behavioral or code change to `resolveDoctorDeps`' construction — the `Detector`/`Resolve` wiring is byte-for-byte unchanged; this is a comment-only edit.
- `go build ./...` succeeds and `golangci-lint run` is clean on cmd/doctor.go.

**Tests**:
- No new tests required (comment-only change). The existing doctor host-terminal suite exercising `DoctorDeps.Detector`/`Resolve` and `checkHostTerminal` (cmd/doctor_test.go) must continue to pass unchanged, confirming the wiring is untouched.
