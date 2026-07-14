AGENT: duplication
FINDINGS:
- FINDING: Spawn log-emission logic duplicated across the CLI and picker paths
  SEVERITY: high
  FILES: cmd/spawn.go:206-280 (tallyWindowResults, logSpawnGone, logSpawnUnsupported, logSpawnPermission, logSpawnSummary), internal/tui/burst_observability.go:32-102 (emitBurstSummary, emitPermission, emitUnsupportedNoop, emitPreflightAbort)
  DESCRIPTION: The two spawn surfaces each carry their own hand-written copy of the same
    `spawn`-component log emission — identical message strings and closed attr-key lists
    duplicated in full across the two files. Concretely the pairs are:
      * emitBurstSummary  ↔ tallyWindowResults + logSpawnSummary
        — both loop `Debug("external window", "session", …, "ack", …, "detail", …)`
          then `Info(fmt.Sprintf("opened %d/%d", …), "resolution", "terminal",
          "bundle_id", "opened", "total", "batch", …)`.
      * emitPermission    ↔ logSpawnPermission
        — both `Info("permission required — nothing self-attached", "resolution",
          "terminal", "bundle_id", "detail", …)`.
      * emitUnsupportedNoop ↔ logSpawnUnsupported
        — both `Info("unsupported terminal — nothing opened", "resolution",
          string(spawn.ResolutionUnsupported), "terminal", "bundle_id", …)`.
      * emitPreflightAbort ↔ logSpawnGone
        — both `Info(spawn.GoneMessage(gone))`.
    burst_observability.go's own header comment states the emit methods "MIRROR cmd/spawn.go's
    logSpawnSummary / tallyWindowResults / logSpawnUnsupported / logSpawnGone so the two
    emission paths stay byte-identical" — i.e. the duplication is acknowledged and kept in
    sync by hand. This matters because the `spawn` log component is a closed, spec-governed
    vocabulary (Observability & State Footprint §"Attr keys (closed set)"); an edit to a
    message string or attr key in one file silently drifts the taxonomy between the CLI
    (test seam) and the picker (the dominant production path). The codebase already extracted
    the *renderers* (spawn/message.go: GoneMessage/QuoteJoin/UnsupportedNoopMessage) and the
    *count-semantics* (spawn/classify.go: PartitionResults/FirstPermission) into internal/spawn
    precisely "so the two paths cannot drift" — the log emission is the one shared concern left
    duplicated instead of extracted. A latent drift already exists: emitBurstSummary hand-rolls
    the confirmed count inline (`if r.Confirmed() { opened++ }`, burst_observability.go:48) while
    tallyWindowResults derives it from the shared spawn.PartitionResults chokepoint — so the two
    "byte-identical" paths already compute `opened` by different mechanisms.
  RECOMMENDATION: Extract the four emission shapes into internal/spawn as shared helpers taking a
    *slog.Logger (e.g. spawn.LogBatchSummary, spawn.LogPermission, spawn.LogUnsupported,
    spawn.LogGone), mirroring the existing message.go / classify.go extraction pattern. Have
    both cmd/spawn.go and internal/tui/burst_observability.go call them, and route the summary's
    opened/failed count through spawn.PartitionResults inside the shared helper so the count
    chokepoint is honoured on both paths. This collapses the manual "keep byte-identical"
    burden to a single source of truth for the closed log vocabulary.

- FINDING: Burst test-model construction prefix duplicated across burst_*_test.go helpers
  SEVERITY: medium
  FILES: internal/tui/burst_input_lock_test.go:36-57 (burstPendingModel), internal/tui/burst_cancel_test.go:97-114 (realCancellableBurst), internal/tui/burst_selfattach_test.go:37-57 (setupConfirmingBurst), internal/tui/burst_partial_failure_test.go:50-70 (newPendingBurstModel)
  DESCRIPTION: Four burst test-model constructors repeat the same ~10-line setup block: build
    `[]tmux.Session` from a `names` slice via the identical loop
    `sessions[i] = tmux.Session{Name: n, Windows: i + 1}`, construct a FakeAckChannel +
    FakeAdapter, NewModelWithSessions, wireBurstSeams(…, spawn.ResolutionNative, allPresent, ack),
    resolveDetection(…, ghosttyIdentity()), enter multi-select (pressSession/pressM), and
    `for i := range names { m = markRow(t, m, i) }`. Three of the four (burstPendingModel,
    realCancellableBurst, setupConfirmingBurst) share the full wire+resolve+enter+mark-all
    prefix and differ only in the tail (force burstPending / all-false Confirm / precondition
    check). newPendingBurstModel repeats the sessions-builder loop and mark-all loop too. The
    sibling-shared-helper convention is already established here (wireBurstSeams, resolveDetection,
    markRow, allPresent, ghosttyIdentity, driveBurstToTerminal live once in sibling files), so
    this prefix is the remaining un-shared copy-paste; keeping four near-identical builders risks
    them drifting (e.g. the Windows index convention) as the burst suite grows.
  RECOMMENDATION: Extract a shared `sessionsFromNames(names) []tmux.Session` helper and a
    `markedSupportedBurstModel(t, names) (Model, *FakeAdapter, *FakeAckChannel)` helper (the
    common wire→resolve→enter→mark-all prefix) into one of the sibling burst test files, and have
    the four constructors call it then apply only their distinct tail — consistent with the
    existing shared-helper pattern.

- FINDING: spawnIDAlphabet duplicates session.alphabet byte-for-byte
  SEVERITY: low
  FILES: internal/spawn/ackid.go:19 (spawnIDAlphabet), internal/session/naming.go:13 (alphabet)
  DESCRIPTION: The option-name-safe ack-id charset is a byte-for-byte copy of the session
    package's nanoid alphabet ("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789").
    ackid.go's own comment flags the relationship ("identical to the session package's nanoid
    alphabet") and the correctness of the marker-name scheme depends on it (no ".", ":", "-",
    or space). session.alphabet is unexported, so the constant was re-declared rather than shared.
    Drift risk is real (a change to the session alphabet that added a "-" would silently break the
    unambiguous `<batch>-<token>` marker split) though currently mitigated at runtime by
    isOptionSafeID's post-generation check.
  RECOMMENDATION: Promote the alphabet to a single exported constant (e.g. export it from
    internal/session, or lift it into an existing leaf both packages already import) and reference
    it from both ackid.go and naming.go, removing the silent cross-package literal copy.
SUMMARY: The spawn feature's shared renderers and count-semantics were correctly extracted into
  internal/spawn, but the closed-vocabulary log emission was left duplicated between cmd/spawn.go
  and internal/tui/burst_observability.go with an explicit "keep byte-identical by hand" contract
  (high) — the strongest consolidation candidate. Secondary duplication: a repeated burst
  test-model construction prefix across four burst_*_test.go helpers (medium) and a byte-for-byte
  nanoid-alphabet constant copied between spawn and session (low).
