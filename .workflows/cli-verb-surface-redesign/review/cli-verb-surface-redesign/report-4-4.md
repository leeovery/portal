TASK: cli-verb-surface-redesign-4-4 — Host-terminal informational line (`Detect()` + resolver)

ACCEPTANCE CRITERIA (Phase 4 task row + phase-level ACs):
- NULL/remote/mosh identity → "unsupported (remote session)"
- recognised-but-undriven terminal → "unsupported"
- transient detect failure folds to informational
- supported = resolver `Resolution != Unsupported`
- the line is outside the pass/fail set and never drives the exit code
- reuses the same `Detect()`/`Resolve` seams the burst uses
- (Phase AC) `doctor` prints a host-terminal line by calling the same `Detect()` the picker uses, replacing the retired `spawn --detect`; host-terminal check is informational only and never drives the exit code.

STATUS: Complete

SPEC CONTEXT:
spec §"Exit-code contract" (line 319): the host-terminal check is informational only, OUTSIDE the pass/fail set — reported honestly but never makes doctor / doctor --fix non-zero; only daemon/hooks/saver/state-dir/sessions.json/stale-entries drive the exit code. spec §"Host-terminal detection folded in (`--detect` retired)" (line 323): `spawn --detect` folds into doctor; the picker keeps calling Detect() in-process, doctor calls the same function and prints e.g. `host terminal: Ghostty (supported)` / `unsupported (remote session)`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/doctor.go:393-403 (checkHostTerminal — the classifier)
  - cmd/doctor.go:363-370 (appended last in runDoctorDiagnosis, only when both seams non-nil)
  - cmd/doctor.go:98-116, 143-157 (DoctorDeps.Detector/Resolve fields + resolveDoctorDeps production wiring: spawn.NewDetector(client) + a closure over buildResolver().Resolve)
  - cmd/doctor.go:660-673 (checkMarker: checkInfo → blank marker), 677-684 (doctorUnhealthy counts only checkFail)
  - cmd/spawn_seams.go:23-25 (TerminalDetector seam), 80-86 (buildResolver — shared by doctor and burst)
  - internal/spawn/identity.go:24-26 (IsNull), internal/spawn/resolver.go:81-97 (Resolve; NULL short-circuit → Unsupported before config tier)
  - internal/spawn/detect.go:87-101 (Detect folds transient failure to NULL Identity{})
- Notes: Classification is exactly correct against the spec:
  - id.IsNull() → checkInfo "unsupported (remote session)" (short-circuits BEFORE Resolve, so even a `*` catch-all cannot reclassify a remote client — verified by the resolver's own NULL short-circuit AND the doctor test that passes ResolutionNative with a NULL identity).
  - non-null + Resolution == ResolutionUnsupported → "<Name> (unsupported)".
  - non-null + Resolution != Unsupported → "<Name> (supported)". "supported" is defined precisely as `resolution != spawn.ResolutionUnsupported`, matching the AC.
  - "transient detect failure folds to informational": Detect() (internal/spawn/detect.go:91-93) folds any transient error to Identity{} (NULL), which the doctor line maps to the informational "unsupported (remote session)". The fold lives in spawn (single owner) and is covered by internal/spawn/detect_test.go:104,137.
  - "reuses the same Detect()/Resolve seams the burst uses": the burst (cmd/open_burst_run.go:158-159) calls the identical `deps.Detector.Detect()` + `deps.Resolve(id)` seam types (TerminalDetector + func(Identity)(Adapter,Resolution)); production wires both from the same primitives — spawn.NewDetector(client) and buildResolver().Resolve. Doctor RECONSTRUCTS these by hand rather than via the shared buildProductionSpawnSeams bundle; this divergence is deliberate (deferred/lazy terminals.json read) and accurately documented after task 11-1's comment correction (cmd/doctor.go:98-115, 143-157). Same primitive seams — different construction site, by design. No drift.
- Drift: None. Report placement (appended last, after the pass/fail catalog) and marker (blank) match "informational, outside the pass/fail set". Rendered line `  host terminal: Ghostty (supported)` matches the spec copy.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/doctor_test.go:625-685 TestDoctorHostTerminalLine — all three classifications: driven→"Ghostty (supported)" (status checkInfo), NULL→"unsupported (remote session)" (with Resolve deliberately returning Native to prove the short-circuit ignores it), recognised-but-undriven→"SomeTerm (unsupported)".
  - cmd/doctor_test.go:690-737 TestDoctorHostTerminalNeverDrivesExit — both directions of the exit-code invariant: an unsupported host with an otherwise-healthy runtime stays healthy (doctorUnhealthy=false), and a supported host does NOT rescue a genuine daemon failure (doctorUnhealthy=true).
  - cmd/doctor_test.go:742-758 TestDoctorCheckOrder — pins the host-terminal line as the last, appended check.
  - The transient→NULL fold is owned and tested in internal/spawn/detect_test.go:104,137 (returns Identity{} + WARN); doctor's NULL→informational handling is tested here — the composition is fully backed by layering, not a gap.
  - Seam sharing/wiring covered by cmd/spawn_seams_test.go (buildResolver / production seams) and cmd/open_burst_run_test.go:142 (burst uses the same fakeTerminalDetector seam).
- Notes: Not under-tested — every AC classification plus both exit-code directions are asserted against exact detail strings. Not over-tested — no redundant assertions; each subtest pins one classification. Tests inject the Detect()/Resolve seams (behavior), not implementation internals. Would fail if the classifier or the informational-only contract broke.

CODE QUALITY:
- Project conventions: Followed. Uses the codebase's package-level *Deps DI seam with per-field nil-fallthrough (resolveDoctorDeps mirrors commitNowDeps/bootstrapDeps). Small 1-method TerminalDetector interface. No t.Parallel (respects cmd package mutable-state rule).
- SOLID principles: Good. checkHostTerminal is a single-responsibility pure classifier over injected seams; Resolve/Detect are interface-segregated seams; the NULL short-circuit responsibility sits in spawn.Resolver, not duplicated.
- Complexity: Low. Two-branch classifier; cyclomatic complexity trivial.
- Modern idioms: Yes. Idiomatic Go, fmt.Sprintf for detail, closure to defer terminals.json read.
- Readability: Good. Doc comments on checkHostTerminal enumerate the three classifications and explain the NULL short-circuit and informational-only rationale; the deliberate-independence-from-the-bundle comment is accurate post-11-1.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
