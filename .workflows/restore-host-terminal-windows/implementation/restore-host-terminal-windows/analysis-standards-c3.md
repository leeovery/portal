AGENT: standards
FINDINGS:
- FINDING: Spawn-ack write-failure DEBUG uses a non-enumerated attr key and an out-of-catalog message under the closed `spawn` component
  SEVERITY: low
  FILES: cmd/attach.go:64-68
  DESCRIPTION: The best-effort spawn-ack marker-write failure logs
    `spawnLogger.Debug("spawn-ack marker write failed", "session", name, "batch", ackBatch, "error", err)`.
    The spec (§Observability → Attr keys) enumerates the closed `spawn` attr set as
    batch / terminal / bundle_id / resolution / session / ack / opened / total / detail, and
    designates `detail` as the opaque OS-specific payload attr; it also lists a closed event
    catalog (detection outcome / adapter resolution / per-window spawn+ack / permission-required /
    batch summary). This DEBUG line is not in that catalog and its `error` key is not in the
    spawn-specific enumeration. CLAUDE.md states "never invent at call-site" for the closed
    taxonomy. Mitigating context: `error` is an established cross-component attr used pervasively
    (internal/state/capture.go, commit.go, fifo_sweep.go, internal/log), so it is arguably drawn
    from the existing accepted vocabulary rather than invented — hence low impact.
  RECOMMENDATION: For strict conformance with the spec's spawn attr enumeration, carry the write
    failure via the spec-designated opaque `detail` attr (e.g. `"detail", err.Error()`) instead of
    `error`, matching how detect.go/logemit.go route opaque payloads. No behavioural change; keeps
    the `spawn` component within its enumerated attr set.

- FINDING: Permission-path per-window DEBUG asymmetry between the CLI and picker "one service" callers
  SEVERITY: low
  FILES: cmd/spawn.go:192-193, internal/tui/burst_observability.go:52-54, internal/tui/burst_partial_failure.go:43-44
  DESCRIPTION: On a permission-required burst-stop the CLI emits the per-window DEBUG loop
    (spawn.LogWindowResults) *and* the permission INFO (logSpawnPermission), whereas the picker's
    permission arm emits ONLY the permission INFO (emitPermission → spawn.LogPermission) and
    deliberately skips the per-window DEBUG. The spec frames spawn as "one service, two callers"
    with a shared closed emission vocabulary "so the two paths cannot drift"; the emission SHAPES
    are shared (internal/spawn/logemit.go), but the call pattern diverges, so a permission-stop
    batch yields per-window ack detail from the CLI but not from the picker. The divergence is
    documented in-source as intentional, and the spec does not hard-mandate per-window DEBUG on the
    permission path, so impact is low.
  RECOMMENDATION: Consider calling spawn.LogWindowResults from the picker's permission arm too (or,
    conversely, dropping it from the CLI's) so both callers surface identical per-window detail on a
    permission stop. If the asymmetry is genuinely intended, no code change is needed — the in-source
    note already records the decision.

- FINDING: `--spawn-ack` flag help text mislabels the marker delimiter
  SEVERITY: low
  FILES: cmd/attach.go:93
  DESCRIPTION: The flag help reads "internal: write the @portal-spawn-<batch>:<token> ack marker
    before attaching", using a colon between batch and token. The written server-option name is
    `@portal-spawn-<batch>-<token>` (hyphen — SpawnMarkerName in internal/spawn/ackid.go); the colon
    is only the flag-VALUE delimiter (FormatSpawnAckFlag → "<batch>:<token>"). The help text conflates
    the two delimiters. Cosmetic `--help` inaccuracy only; no functional impact.
  RECOMMENDATION: Reword to reflect the actual marker name, e.g. "internal: <batch>:<token> — write
    the @portal-spawn-<batch>-<token> ack marker before attaching".

SUMMARY: The implementation is highly faithful to the specification. Core spec-governed contracts
  are all correctly realised: net-N / N-1 split with self-attach-last, pre-flight all-or-nothing +
  leave-what-opened selection mutation, per-window token-ack with per-window timeout, permission
  burst-stop precedence, count semantics (total=N, opened=confirmed+trigger), resolver precedence
  (config→native→unsupported with NULL short-circuit and most-specific-wins), terminals.json tolerant
  validation with per-rejection WARNs, env self-sufficiency (PATH-only inject, TMUX/TMUX_PANE strip)
  uniform across native+config, @portal-spawn- marker namespace isolation from ListSkeletonMarkers,
  detection lifecycle (detect-once cached, off first-paint, transient→NULL+WARN, in-flight-at-Enter
  defer), multi-select mode keys/suppression/sticky selection keyed on session identity, notice-band
  and section-header precedence, exit-code classification (usage=2, all spawn failures=1), and the
  spec-sanctioned `spawn` log-component amendment. Only three low-severity items found — all confined
  to observability attr/catalog nits and one help-text label.
