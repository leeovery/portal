AGENT: architecture
FINDINGS:
- FINDING: Open-target routing domain is a bare string with a non-exhaustive, default-less switch
  SEVERITY: medium
  FILES: cmd/open_targets.go:9-12, cmd/open_surfaces.go:56-97, cmd/open_burst.go:76-83
  DESCRIPTION: The multi-target pipeline routes on Target.Domain, a plain string
    ("bare"/"session"/"path"/"zoxide"/"alias") produced by the openTargetPins map
    and switched on as a control-flow discriminant in two places. The
    resolveOpenSurfaces switch (open_surfaces.go:57) has NO default arm: an
    unrecognised domain silently falls through — the target is neither turned into
    a surface nor collected as a miss, so it vanishes from the burst with no error
    and no log line. This is safe today only because openTargetPins emits exactly
    the five handled domains, but that coupling is convention, not type: adding a
    future value-taking pin requires edits in four coupled sites — the cobra flag,
    openTargetPins (guarded by TestOpenTargetPinsCoverValueTakingFlags), the
    single-target pinDispatch table, and this switch — and only the switch is
    unguarded. Forget it and burst targets in that domain silently disappear. The
    inconsistency is sharpened by the fact that the sibling attach/mint dichotomy
    right next door IS a proper typed enum (spawn.SurfaceKind), and globExpandableDomain
    (open_burst.go:76) does defend itself with a default:false — so the codebase
    already knows the pattern; resolveOpenSurfaces is the one seam that trusts the
    caller instead of being self-contained. This matches the code-quality
    anti-patterns "untyped parameters when concrete types are known at design time"
    and "correctness depends on caller discipline rather than being self-contained."
  RECOMMENDATION: Introduce a small typed domain constant set in cmd (mirroring
    spawn.SurfaceKind) that openTargetPins and both switches share, and give
    resolveOpenSurfaces a default arm that fails loudly (collect the value as a
    miss, or return an internal error) rather than dropping the target. This makes
    the "every routable domain is handled" invariant structural instead of
    convention-plus-a-partial-guard.

- FINDING: Host-terminal Detector seam is constructed three independent ways, so the shared-seam bundle's single-source guarantee doesn't cover it
  SEVERITY: low
  FILES: cmd/spawn_seams.go:34-42, cmd/spawn_seams.go:63-68, cmd/open_burst_run.go:88-89, cmd/open.go:888
  DESCRIPTION: productionSpawnSeams exists explicitly to be "the single source both
    [the open burst and the picker] read... the compiler cannot catch a seam that
    is added, swapped, or re-constructed on only one side" (spawn_seams.go:27-33).
    It carries a Detector field (spawn.NewDetector(client)). The picker honours the
    bundle (open.go:888 reads spawnSeams.Detector), but the open burst bypasses it:
    buildOpenBurstDeps defaults Detector via the parallel spawnDetector(cmd)
    (open_burst_run.go:89) — a second spawn.NewDetector construction — while pulling
    Resolve/Ack/Exe/Getenv/Logger from the same bundle. Since buildOpenBurstDeps
    always builds sharedSeams() anyway (Resolve is always defaulted in production),
    the bundle's Detector is constructed and then discarded on every open-burst run,
    and a third construction lives in doctor's resolveDoctorDeps. *spawn.Detector
    already satisfies the TerminalDetector interface, so there is no type barrier to
    reading the bundle's field. The drift risk is small (all three reduce to
    spawn.NewDetector over the same client), but the detector is precisely the seam
    the bundle was created to single-source, and it is the one shared seam the open
    burst does not read from it. (Doctor's independent construction is separately
    justified by its deferred terminals.json read and can stay.)
  RECOMMENDATION: Default the open burst's Detector from sharedSeams().Detector and
    retire spawnDetector, so productionSpawnSeams genuinely single-sources every
    shared seam for both burst callers and the bundle carries no field one consumer
    ignores.
SUMMARY: Architecture is strong and cohesive — the spawn burst core (Burster,
  classify, message/logemit renderers, Surface) is cleanly shared by both the open
  burst and the picker burst, boundaries are well-placed, the retired verbs are
  provably gone, and doctor/uninstall/completion are well-scoped. Two lower-severity
  seam issues: the open-target routing domain is a stringly-typed discriminant whose
  resolveOpenSurfaces switch can silently drop a target (no default arm), and the
  shared-seam bundle's single-source guarantee doesn't actually cover the Detector,
  which is constructed three ways.
