TASK: cli-verb-surface-redesign-14-3 (chore) — Route doctor's host-terminal Detector/Resolve through the shared spawn-seam bundle

ACCEPTANCE CRITERIA:
1. doctor's Detector and Resolve originate from buildProductionSpawnSeams; resolveDoctorDeps contains no independent spawn.NewDetector/buildResolver construction.
2. The doctor host-terminal report line is behaviourally unchanged (correct identity + resolution, informational-only, no exit-code impact).
3. (Option B only) The bundle performs no terminals.json read until Resolve is invoked.
4. go build -o portal ., go test ./..., and golangci-lint run are clean.

STATUS: Complete

SPEC CONTEXT:
buildProductionSpawnSeams (cmd/spawn_seams.go) is the single construction site for the shared host-terminal seams (Detector + config-aware Resolve) that the multi-target open burst and the in-process picker both read, so the two paths cannot silently diverge on how those seams are built (OpenBurstDeps and tuiConfig are distinct struct shapes the compiler can't cross-check). This chore removes doctor's hand-built THIRD copy (spawn.NewDetector(client) + a buildResolver().Resolve closure) that carried a self-admitted "must be kept in sync by hand" note — the exact drift obligation the bundle exists to abolish. The task permits either Option A (route through the bundle as-is, accepting one eager terminals.json read) or Option B (make the bundle lazy). doctor's host-terminal line is informational-only (checkInfo) and never drives the scriptable exit code.

IMPLEMENTATION:
- Status: Implemented (Option A taken)
- Location: cmd/doctor.go:156 (seams := buildProductionSpawnSeams(client)); cmd/doctor.go:167-168 (Detector: seams.Detector, Resolve: seams.Resolve); cmd/spawn_seams.go:44-61 (unchanged shared bundle, still eager buildResolver() at line 54).
- Notes:
  * Option A confirmed: resolveDoctorDeps calls buildProductionSpawnSeams(client) once and wires Detector/Resolve straight off the returned bundle. There is NO independent spawn.NewDetector or buildResolver call anywhere in resolveDoctorDeps (verified by reading + AST guard test). AC1 met.
  * The "must be kept in sync by hand" note is fully removed (grep for "kept in sync"/"keep in sync" across cmd/ returns nothing). The replacement comment (doctor.go:147-155) documents the deliberate acceptance of the bundle's single eager terminals.json read and justifies it: doctor is bootstrap-exempt, already reads hooks.json/projects.json/sessions.json, and buildResolver is read-only + fail-safe (missing/malformed → empty native-only config, never an error), so the informational line is behaviourally unchanged. AC-outcome (note removed) met.
  * AC2 met: checkHostTerminal (doctor.go:404-414) is behaviourally unchanged; the Detector/Resolve it consumes now originate from the bundle but produce the identical spawn.NewDetector(client) detector and buildResolver().Resolve resolver. Still checkInfo, still outside doctorUnhealthy's checkFail-only count → zero exit-code impact.
  * AC3 is Option-B-only and correctly N/A: Option A was chosen, so the bundle remains eager and no lazy-read behaviour was introduced. The task explicitly permits Option A ("Prefer routing doctor's Detector/Resolve through buildProductionSpawnSeams(client)"). No drift.
  * spawn import in doctor.go remains used (spawn.AdapterResolver at :129/:404, spawn.ResolutionUnsupported at :410) — no orphaned import from the removed construction. Build stays clean by reading.
  * Minor accepted trade (not a finding): buildProductionSpawnSeams also constructs Ack/Exe/Getenv/Exists/Logger that doctor discards; only buildResolver() does I/O, the rest are method/func values. This is inherent to the shared-bundle design the task endorses — narrowing it would reintroduce a second construction site, defeating the goal. Documented as acceptable; proposes no concrete change.

TESTS:
- Status: Adequate
- Coverage:
  * TestResolveDoctorDepsUsesSharedSpawnSeams (cmd/doctor_spawn_seams_guard_test.go) — the key regression guard for THIS task. AST-based, scoped to the resolveDoctorDeps body: errors if buildResolver() is called directly, errors if spawn.NewDetector is called directly, and requires buildProductionSpawnSeams to be called (sawBundle). Comments are ignored (parser flag 0), so it enforces the invariant structurally, exactly matching the "one construction site, not three" outcome. Would fail if the hand-built copy were reintroduced.
  * TestDoctorHostTerminalLine (doctor_test.go:733) — all three classifications (supported / recognised-but-undriven / NULL-remote) via injected seams; unchanged in intent.
  * TestDoctorHostTerminalNeverDrivesExit (doctor_test.go:798) — informational line can't push exit non-zero and can't rescue a real failure.
  * TestDoctorCheckOrder (doctor_test.go:850) — host terminal appended last.
  * runDoctor helper (doctor_test.go:146-163) correctly gained isolateTerminalsFile(t) (line 151) because the Execute path now reads terminals.json eagerly through the bundle — a REQUIRED isolation change honouring the "never touch the real system" invariant (PORTAL_TERMINALS_FILE pointed at a temp path). Necessary, not over-testing.
  * No contradictory Option-B lazy-read test exists (correct — such a test would fail under Option A). Confirmed by grep.
- Notes: Test balance is right. The AST guard is the correct instrument for a "single construction site" chore (a value-level test could not catch a re-added parallel copy). No redundant or implementation-detail-coupled assertions beyond what the invariant requires.

CODE QUALITY:
- Project conventions: Followed — matches the per-field nil-check *Deps merge idiom (commitNowDeps/bootstrapDeps), single tmux.DefaultClient() built once and shared across seams, heavy explanatory comments consistent with the codebase.
- SOLID principles: Good — this is a DRY win; the third duplicate construction site is eliminated and all three consumers (doctor, open burst, picker) now share one builder.
- Complexity: Low — resolveDoctorDeps is unchanged in structure; two hand-built lines replaced by two bundle field reads.
- Modern idioms: Yes.
- Readability: Good — the replacement comment clearly states the Option-A judgment and its safety rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
