TASK: restore-host-terminal-windows-10-2 — Extract one shared production spawn-seam builder for the CLI and picker (tick-5ae99f, chore/refactor, Phase 10 analysis cycle)

ACCEPTANCE CRITERIA:
1. The seven shared production spawn seams (detector, resolve, ack channel, exe, getenv, exists, spawn logger) are constructed in exactly one helper; both buildSpawnDeps and openConfig read them from it.
2. CLI test injection via spawnDeps still overrides every shared field (shared builder consulted only for unset fields); the --detect dry-run's detector resolution is unchanged.
3. The CLI-only Connector and lazy NewBurster defaults, and the picker-only non-spawn config fields, are unchanged.
4. Wired production seams byte-for-byte equivalent to today on both paths; go build ./... and cmd + tui suites green.

STATUS: Complete

SPEC CONTEXT: This is a Phase 10 analysis-cycle chore (duplication, medium severity), not a spec-behaviour task. The underlying feature (§6/§6-3 host-terminal detection + N>=2 picker/CLI window burst) is unchanged; this task closes a copy-paste parallel: the identical set of production spawn seams was wired independently in cmd/spawn.go's buildSpawnDeps (CLI) and cmd/open.go's openTUI/tuiConfig population (picker). The stated risk is silent CLI<->picker divergence if a seam is added/swapped on only one side (SpawnDeps and tuiConfig are distinct struct shapes, so the compiler can't catch it). buildResolver()/buildSessionConnector() already demonstrate the shared-helper pattern in this package.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/spawn.go:268-370 (productionSpawnSeams struct + buildProductionSpawnSeams builder + rewired buildSpawnDeps), cmd/open.go:566-611 (openTUI builds the bundle once and reads shared fields into tuiConfig). Commit c4831700.
- Notes:
  - Criterion 1: buildProductionSpawnSeams(*tmux.Client) constructs all seven seams in one place (Detector=spawn.NewDetector(client), Resolve=buildResolver().Resolve, Ack=spawn.NewServerOptionAckChannel(client,client), Exe=os.Executable, Getenv=os.Getenv, Exists=client.HasSession, Logger=log.For("spawn")). The picker (open.go:569, :598-611) reads all seven from the bundle. The CLI reads six (Resolve/Ack/ExePath/Getenv/Exists/Logger) from the bundle and — as the tick's Do step 2 explicitly sanctions ("source only the other six shared seams from the struct") — keeps its Detector default routed through spawnDetector (the standalone --detect authority). spawnDetector and the bundle both call spawn.NewDetector(client), so the two Detector constructions are equivalent today (see NON-BLOCKING note).
  - Criterion 2: buildSpawnDeps copies *spawnDeps first (injected-field precedence wins), then defaults only nil fields; the shared bundle is lazily memoised (seamsBuilt flag) and consulted ONLY for genuinely-unset fields. The RunE --detect arm (spawn.go:86-96) still calls spawnDetector(cmd).Detect() directly and never reaches buildSpawnDeps — unchanged.
  - Criterion 3: Connector default (buildSessionConnector) and the lazy NewBurster closure remain CLI-only and untouched; picker-only tuiConfig fields (lister/killer/reader/etc.) untouched.
  - Criterion 4: On both paths the wired seams are behaviourally identical to the prior inline construction (same detector over the same client, same buildResolver().Resolve, same ServerOptionAckChannel(client,client), os.Executable/os.Getenv, client.HasSession, log.For("spawn")). Memoisation preserves the "at most once" contract: buildResolver / NewServerOptionAckChannel run no more often than the old inline defaults, and a fully-injected caller never resolves the tmux client (verified against the spawn pipeline suite, which fully-injects). No extra terminals.json load or client resolution introduced.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/spawn_seams_test.go TestBuildProductionSpawnSeams — proves the builder wires the expected seams: Exists is client.HasSession (asserted behaviourally via the recorded `has-session -t =mysession` commander call returning true), Ack is *spawn.ServerOptionAckChannel, Logger emits under component "spawn" (logtest.Sink OnlyRecord), and Detector/Resolve/Exe/Getenv are non-nil (Getenv proven == os.Getenv). PORTAL_TERMINALS_FILE is isolated to a temp path so buildResolver never reads the real terminals.json.
  - cmd/spawn_seams_test.go TestBuildSpawnDeps_PartialInjectionKeepsInjectedFillsRest — the core precedence assertion: a partially-injected spawnDeps (Resolve/Exists/Logger) keeps those injected values (each behaviourally distinguishable from its production default) while the unset fields (Ack/ExePath/Getenv) fill from the shared builder, and the CLI-only defaults (Detector/Connector/NewBurster) are still populated. Directly covers criteria 2 and 3.
  - Regression: existing spawn_test.go pipeline suite (fully-injected spawnPipelineDeps) and --detect suite exercise both call paths; both bypass the new client-resolution concern (full injection / --detect arm), so neither regresses. Would fail if the shared default filling or injected precedence broke.
- Notes: No DIRECT test pins that the picker path (openTUI/tuiConfig) reads its shared seams from buildProductionSpawnSeams — the picker side of criterion 1 rests on code review plus existing picker-burst integration coverage. The change there is a mechanical field substitution (spawn.NewDetector(client) -> spawnSeams.Detector, etc., where spawnSeams.Detector = spawn.NewDetector(client)), so the residual risk is low, but the tick's Tests guidance did suggest an explicit "both paths originate from the one builder" parity assertion (see NON-BLOCKING). Not over-tested: no redundant assertions, mocking is minimal (recordingCommander + logtest.Sink), tests assert behaviour not internals.

CODE QUALITY:
- Project conventions: Followed. Matches the package's existing shared-builder idiom (buildResolver/buildSessionConnector), the small-interface DI pattern, and the *Deps nil-default convention. No t.Parallel (correctly noted in the test header, since spawnDeps is package-level mutable state). Test isolation of PORTAL_TERMINALS_FILE is correct.
- SOLID principles: Good. Single construction site for the shared seams; the bundle struct is a focused value object.
- Complexity: Low. The lazy-memoised closure is a clean, well-commented idiom.
- Modern idioms: Yes.
- Readability: Good. Comments explain the six-vs-seven split, the memoisation rationale, and why the Detector stays routed through spawnDetector.
- Issues: One doc-comment went stale (Logger default mechanism) — see NON-BLOCKING.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/spawn.go:66-68 (and the related note at cmd/spawn.go:16-19) — The SpawnDeps.Logger field comment says "Defaults to the package-level spawnLogger", but buildSpawnDeps now defaults Logger from sharedSeams().Logger (buildProductionSpawnSeams' log.For("spawn")), not the spawnLogger package var. Behaviourally identical (same "spawn" component through the shared handler), but the mechanism reference is now imprecise; reword to "Defaults to the spawn-component logger (log.For(\"spawn\")) built by buildProductionSpawnSeams". Pure doc edit, zero logic risk.
- [idea] cmd/spawn.go:292-302 & :337-339 — The Detector seam is still constructed at two sites across the CLI+picker split: buildProductionSpawnSeams.Detector (read by the picker) and spawnDetector's spawn.NewDetector(tmuxClient(cmd)) (used by the CLI/--detect). Both call spawn.NewDetector(client), so they are equivalent today, but a future change to detector construction must touch both — the exact drift class this task targets, now closed for six of seven seams. Consider having buildProductionSpawnSeams.Detector delegate through spawnDetector (or a shared newDetector(client) helper) so the Detector is single-sourced too. The tick explicitly left this open ("either delegate ... or source only the other six"), so it is a genuine design choice, not a defect. (Also: on the CLI path buildProductionSpawnSeams constructs a Detector that buildSpawnDeps discards — a negligible cost, spawn.NewDetector is a cheap struct build.)
- [idea] cmd/spawn_seams_test.go — No test directly asserts that the picker path (openTUI/tuiConfig) sources its shared seams from buildProductionSpawnSeams; the tick's Tests guidance suggested a parity assertion that "both paths' shared seams originate from the one builder". Because openTUI launches a Bubble Tea program it is awkward to unit-test in isolation, so deciding whether/how to pin the picker-side parity is a judgment call. Current coverage (code review + existing picker-burst integration tests) is defensible.
