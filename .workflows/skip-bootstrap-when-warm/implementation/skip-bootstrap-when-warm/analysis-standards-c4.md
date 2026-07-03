AGENT: standards
FINDINGS:
- FINDING: Latch-write-failure WARN introduces a "marker" log attr not in the closed vocabulary
  SEVERITY: low
  FILES: cmd/bootstrap/bootstrap.go:499
  DESCRIPTION: The best-effort latch-write failure line
    `o.Logger.Warn("latch write failed", "marker", state.BootstrappedMarkerName, "error", err)`
    introduces `"marker"` as a slog attr key. A whole-tree search confirms `"marker"`
    appears as a log attr nowhere else in production code — it is used only on this
    single line. CLAUDE.md's logging contract states the attr-key vocabulary is
    "closed" and that "New components/attrs require amending the spec — never invent
    at call-site." The spec (§ Insertion point in Run / § Write posture) mandates only
    "a pure log line (WARN under the bootstrap component)" and does not authorise a new
    attr key. The value is also redundant: `state.BootstrappedMarkerName` is the
    compile-time constant "@portal-bootstrapped", and the message "latch write failed"
    plus the existing `"error"` attr already fully identify the event. There is no
    enforcement guard, so this passes green; it is a pure convention nit on a
    rarely-hit path, hence low severity.
  RECOMMENDATION: Drop the `"marker"` attr (the message already names the latch), or
    reuse an already-established key rather than minting a new one at the call-site.
    If a marker/option identifier attr is genuinely wanted as a reusable key, add it to
    the logging spec's closed vocabulary first per CLAUDE.md.
SUMMARY: The implementation conforms strongly to the specification: the version-stamped
  @portal-bootstrapped latch (set/read/semantics), the single-read three-way branch in
  PersistentPreRunE, the liveness-only abridged EnsureSaver, the 11->10 orchestrator
  reduction with CleanStale fully removed (step, seam, and adapter), the latch set-point
  (after last soft step, before the completion summary/Done, gated on no fatal error and
  best-effort), and the daemon-owned throttled hooks cleanup on the idle branch (10s
  cadence, lastCleanup=start-time, nil-store no-op, reset-after-run) all match the spec.
  The loading-progress denominator and step-label table correctly retune to 10, and the
  Run-before-Done ordering that underpins "latch present <=> full bootstrap completed"
  holds. The only drift is one low-severity convention nit: a newly-minted "marker" log
  attr on the latch-write-failure WARN line.
