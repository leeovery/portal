AGENT: duplication
FINDINGS:
- FINDING: Doctor host-terminal detector+resolve wiring bypasses the shared productionSpawnSeams bundle
  SEVERITY: low
  FILES: cmd/doctor.go:142-145, cmd/spawn_seams.go:51-61
  DESCRIPTION: buildProductionSpawnSeams (cmd/spawn_seams.go) is the designated single
    construction site for the host-terminal Detector (spawn.NewDetector(client)) plus the
    config-aware Resolve (buildResolver().Resolve) seams. Its whole reason to exist is
    anti-drift: both burst callers — buildOpenBurstDeps (cmd/open_burst_run.go) and the
    picker's openTUI/tuiConfig (cmd/open.go) — read the pair from this one bundle so their
    production wiring "cannot silently diverge" (its own doc-comment). resolveDoctorDeps
    (cmd/doctor.go) is a THIRD consumer of the identical detector+resolve pair, but it
    re-constructs them inline (spawn.NewDetector(client) + a buildResolver().Resolve
    closure) instead of routing through the bundle. This is the exact Rule-of-Three
    copy-paste-drift category the bundle was created to close: three call sites need the
    same wiring, two share the source, one diverged. A future change to how the detector or
    resolver is built (a new NewDetector parameter, a different resolve construction) must
    now be applied in two places or the doctor host-terminal line silently drifts from the
    burst paths. The divergence is doubly notable because doctor's own comments assert it
    uses "the SAME Detect() seam the picker and the multi-target open burst use
    (cmd/spawn_seams.go)" and "the SAME config-aware resolver the burst uses
    (buildResolver().Resolve)" — while building them independently. The one intentional
    difference is laziness: doctor defers the terminals.json read behind a closure (so a
    NULL/remote identity never triggers it), whereas the bundle reads terminals.json eagerly
    at construction.
  RECOMMENDATION: Route doctor's Detector/Resolve through buildProductionSpawnSeams(client),
    reading only the two fields it needs (doctor already builds its own tmux.DefaultClient,
    which the bundle accepts). If the deferred terminals.json read is worth preserving,
    make the bundle's Resolve field itself lazy (a closure over buildResolver) so doctor can
    adopt it without an eager read — that keeps all three consumers on one source. Either way
    collapses the third construction back onto the single site. If the eager-read tradeoff is
    deemed unacceptable and not worth changing the bundle, replace doctor's "SAME seam"
    comments with an explicit note that the pair is intentionally re-built for laziness, so a
    reader isn't misled into thinking it flows from the shared bundle.
SUMMARY: The redesign is aggressively single-sourced — classify.go, message.go, logemit.go,
  expandSessionGlobAll, productionSpawnSeams, and shared miss/attach-only/gone strings all
  concentrate would-be-duplicated logic in one place — so genuine cross-file duplication is
  minimal. The single residual is doctor's host-terminal seam construction re-implementing
  the detector+resolve wiring that the designated productionSpawnSeams bundle centralizes for
  the two burst callers.
