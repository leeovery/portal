TASK: restore-host-terminal-windows-6-10 — Spawn batch-summary observability from the chokepoint (picker/TUI parallel to the CLI spawn emission)

ACCEPTANCE CRITERIA:
- Full success emits one INFO `spawn: opened N/N` with batch/terminal/bundle_id/resolution/opened/total, total==N (external + trigger), opened==N (trigger self-attach counted).
- Partial/permission failure emits `opened <k>/N` where k counts only confirmed external windows — the skipped trigger self-attach NOT counted.
- Per external window, one DEBUG record carries session + ack (+ opaque detail); Result.Detail never appears in the user-facing flash.
- Unsupported N≥2 no-op emits resolution=unsupported with terminal/bundle_id and no per-window records.
- No emitted record carries an attr key outside the closed set batch/terminal/bundle_id/resolution/session/ack/opened/total/detail.
- total includes the trigger self-attach target (= N) on every (summary) path.

STATUS: Complete

SPEC CONTEXT:
Spec §Observability & State Footprint → Observability (`spawn` log component) / Attr keys (closed set) / Count semantics. The spawn flow gets its own closed `spawn` log component; emission shape matches bootstrap/restore/daemon (one INFO cycle-summary `spawn: opened 11/14` + DEBUG per-window). Closed attr keys: batch/terminal/bundle_id/resolution/session/ack/opened/total/detail; driver OS-specific string rides up as opaque `detail`, never parsed (driver-quarantine). Count semantics: total = N (all sessions incl. trigger self-attach target); opened = each acked spawn plus the trigger self-attach when it occurs (full success = opened N/N; failure path skips + does not count the trigger). This task is the picker's parallel emission; the CLI already emits from cmd/spawn.go.

IMPLEMENTATION:
- Status: Implemented (matches acceptance criteria; no drift)
- Location:
  - internal/tui/burst_observability.go:27 — WithSpawnLogger option; :41 emitBurstSummary (total := len(m.burstExternal)+1, delegates to spawn.LogBatchSummary); :52 emitPermission; :60 emitUnsupportedNoop; :68 emitPreflightAbort.
  - internal/tui/model.go:496 — spawnLogger *slog.Logger field; :2523 full-success arm calls emitBurstSummary(..., triggerAttached=true) BEFORE resetBurstState()/tea.Quit.
  - internal/tui/burst_partial_failure.go:43-46 — permission → emitPermission (no opened/total), else → emitBurstSummary(..., triggerAttached=false).
  - internal/tui/burst_preflight_abort.go:41 — emitPreflightAbort(msg.Gone).
  - internal/tui/burst_progress.go:434,484 — unsupported no-op → emitUnsupportedNoop.
  - internal/spawn/logemit.go — single source of the closed emission shapes (LogBatchSummary/LogWindowResults/LogPermission/LogUnsupported/LogGone); opened derived from spawn.PartitionResults + (triggerAttached?1:0); all nil-logger tolerant via log.OrDiscard.
  - internal/tui/build.go:69,219 — Deps.SpawnLogger + unconditional WithSpawnLogger wiring.
  - cmd/open.go:611 — production wires spawnLogger = spawnSeams.Logger = log.For("spawn") (cmd/spawn.go:300 buildProductionSpawnSeams).
- Notes: Count semantics exactly correct. total is computed from m.burstExternal (not len(results)), so it stays N even on a pre-spawn-abort/cancel that left fewer results — and emitBurstSummary is called strictly before resetBurstState() on both summary arms, so burstExternal is still populated. Emission shapes live once in internal/spawn (DRY across CLI + picker), preventing drift; a byte-for-byte cross-caller parity is pinned (TestEmitPermission_ParityWithCLI ↔ cmd TestLogSpawnPermission_ParityBody share the same golden). Nil-tolerance is honoured (harness leaves nil → discard). The deliberate picker/CLI asymmetry (picker permission path emits ONLY the permission INFO, no per-window DEBUG) is implemented by routing the permission arm to emitPermission rather than emitBurstSummary.

TESTS:
- Status: Adequate
- Coverage: internal/tui/burst_observability_test.go covers all six acceptance criteria plus edge cases:
  - Full success opened 3/3, trigger counted, closed attrs + resolution/terminal/bundle_id/batch (TestBurstObservability_FullSuccessOpenedNofN).
  - Partial opened 1/3, k counts confirmed externals only, total N (TestBurstObservability_PartialFailureOpenedKofN).
  - DEBUG per external window with session/ack/opaque detail, including a spawn-failed window's detail riding as `detail` (TestBurstObservability_DebugPerExternalWindow).
  - Unsupported no-op resolution=unsupported, no opened/total, zero DEBUG records (TestBurstObservability_UnsupportedNoopNoPerWindow).
  - Pre-flight abort names gone, zero DEBUG records (TestBurstObservability_PreflightAbortNamesGone).
  - Closed-attr-key discipline across all four paths driven into one sink (TestBurstObservability_OnlyClosedSpawnAttrKeys).
  - Permission event vs generic summary branch + CLI parity golden (PermissionRequiredEmitsPermissionEvent, PartialFailureNoPermissionEmitsSummary, EmitPermission_ParityWithCLI).
  - total==N on both summary paths (TestBurstObservability_TotalIncludesTriggerOnEveryPath).
  Tests correctly drive the real completion handlers (injectComplete / pressEnter / spawnAbortMsg Update) rather than calling the emit wrappers directly, so they verify emission from the chokepoint, not just the wrapper. Preconditions assert the observable behaviour (tea.Quit self-attach on full success; no quit on partial), which would fail if the count semantics or the branch selection broke.
- Notes: Slight, justified overlap — TotalIncludesTriggerOnEveryPath re-asserts total==3 already covered by the Full/Partial tests, but it isolates the "total on every path" AC into one focused check (acceptable, not bloat). One small gap: the documented picker/CLI asymmetry (permission path emits NO per-window DEBUG) is not pinned by an explicit DEBUG-count==0 assertion in TestBurstObservability_PermissionRequiredEmitsPermissionEvent (the unsupported and preflight tests do assert len(debugs)==0; the permission test does not). Not under-tested overall.

CODE QUALITY:
- Project conventions: Followed. Component logger bound via log.For("spawn"); only closed attr keys emitted; small nil-tolerant *slog.Logger seam injected via Deps + With* option (Portal DI style); logtest.Sink capture seam used in tests; no t.Parallel (consistent with the tui surface).
- SOLID principles: Good. emitBurstSummary/emitPermission/emitUnsupportedNoop/emitPreflightAbort are thin single-responsibility wrappers; emission shapes centralised in internal/spawn/logemit.go (one source, both callers delegate — no drift possible).
- Complexity: Low. Each wrapper is 1-2 lines; opened/total arithmetic lives once in the shared helper.
- Modern idioms: Yes. Value receivers for the read-only emit methods; PartitionResults chokepoint reused for the count; fmt.Sprintf for the summary message.
- Readability: Good. Extensive, accurate doc comments explain the count semantics, the burst-external total derivation, and the permission asymmetry rationale.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/burst_observability_test.go:350 (TestBurstObservability_PermissionRequiredEmitsPermissionEvent) — add an explicit `recordsByLevel(sink.Records(), slog.LevelDebug)` length==0 assertion to pin the documented picker/CLI asymmetry (permission arm emits ONLY the permission INFO, no per-window DEBUG lines); currently the test asserts the INFO shape and the absence of an `opened` summary but never that zero DEBUG records were emitted, so a regression re-introducing per-window DEBUG on the permission path would pass.
