AGENT: standards
FINDINGS:
- FINDING: Abridged saver revive failure discards the underlying error with no portal.log breadcrumb
  SEVERITY: low
  FILES: cmd/abridged_saver.go:48-50
  DESCRIPTION: On the abridged path, when BootstrapPortalSaver fails to revive an
    absent `_portal-saver`, the returned error is dropped entirely
    (`if err := tmux.BootstrapPortalSaver(...); err != nil { bootstrapWarnings.Add(bootstrap.SaverDownWarning()) }`)
    and only the canned SaverDownWarning is surfaced. The helper takes no logger
    and emits no WARN. This is a spec-vs-convention divergence the spec did not
    explicitly resolve: the spec's "Where it lives (build shape)" and
    "Abridged EnsureSaver hard-failure" sections describe the failure as
    warning-sink-only and are silent on logging, so the implementation is
    literally spec-conformant — but the full-bootstrap sibling step logs the same
    class of failure with the actual cause (cmd/bootstrap/bootstrap.go:378-380,
    `o.Logger.Warn("step failed", "step", stepEnsureSaver, "error", err)`), and
    the project's logging discipline (CLAUDE.md: saver/daemon lifecycle "have
    closed event catalogs") treats every such failure as an observable WARN.
    Net effect: on a warm/abridged command an operator sees only the generic
    "Portal save daemon failed to start" message with zero detail in portal.log
    about *why* the revive failed — a diagnosability regression relative to every
    other bootstrap failure path, and relative to the full-bootstrap EnsureSaver
    it is derived from. Because it is a best-effort revive whose failure still
    reaches the user via the warning sink, impact is low.
  RECOMMENDATION: Give ensureSaverLiveness the bootstrap-component logger (e.g.
    `bootstrapLogger`) and, on the BootstrapPortalSaver error branch, emit one
    WARN carrying the underlying error before adding the SaverDownWarning —
    mirroring the full-bootstrap step-5 WARN so the abridged path preserves the
    same forensic breadcrumb. This keeps the spec's warning-sink funnel intact
    while restoring convention parity.
SUMMARY: The feature conforms closely to the specification — version-stamped latch semantics, set-point timing (after the last soft step, before the orchestration summary and the concurrent Done event), soft-vs-fatal latch gating, single-read three-way branch, abridged liveness-only EnsureSaver (no version gate), full removal of the hooks CleanStale step plus its seam/adapter (10-step orchestrator, loading bar denominator retuned to 10), and daemon-homed throttled hooks cleanup on the idle tick branch (lastCleanup anchored to daemon-start, pinned args, nil-store no-op, mass-deletion guard reuse) are all implemented as decided, with matching test coverage and a clean build. The only drift is a low-severity observability gap where the abridged saver revive error is swallowed unlogged, diverging from the full-bootstrap sibling and the project's WARN-the-cause convention.
