TASK: restore-host-terminal-windows-6-3 — N≥2 burst dispatch + async spawn tea.Cmd + streaming message protocol

ACCEPTANCE CRITERIA:
1. N≥2 Enter on a resolved-supported terminal enters burst-pending and dispatches an async burst calling the fake adapter's OpenWindow once per external session in list order, never for the trigger.
2. External set = marked set minus trigger (net-N): N marked → N-1 OpenWindow calls; BurstTotal()==N.
3. A multi-tag By-Tag session marked once appears once in the open order (de-duped at its first list position).
4. A cursor-but-unmarked row is never opened.
5. Goroutine streams a spawnProgressMsg per window + one terminal spawnCompleteMsg; receiver re-issued on progress, stops on terminal.
6. N≥2 Enter while detection in-flight defers until terminalDetectedMsg, then branches (supported → burst; NULL → 6-9).
7. N=0 and N=1 Enter (Task 5.7) remain unchanged (no burst).
8. Picker's default Resolve seam is the config-aware spawn.NewResolver(terminals.json).Resolve — identity matching a config entry resolves to the config adapter + ResolutionConfig, identical to portal spawn.

STATUS: Complete

SPEC CONTEXT: Spec Burst & Partial-Failure Contract (In-picker execution model: async non-blocking tea.Cmd streaming progress + per-window acks), Spawn Architecture (N vs N-1 split; order is load-bearing — self-attach LAST to exactly one of N), Trigger-Context Matrix & Open Order (open in list order, selection is a set; highlighted-but-unmarked never opened; trigger unspecified/implementation-convenience), Terminal Identity & Detection (in-flight identity is awaited, never treated as unsupported for being unresolved). This task establishes only the dispatch machinery, streaming protocol, and burst-pending entry; completion behaviours are 6-4/6-6/6-7/6-8/6-9/6-10.

IMPLEMENTATION:
- Status: Implemented (with deliberate, documented evolutions from the original task text — see Notes)
- Location:
  - internal/tui/burst_progress.go:30-513 — burstProgress event, burstProgressPipe (channel + goroutine + send + receiver), message shapes (spawnProgressMsg/spawnCompleteMsg/spawnAbortMsg/burstChannelClosedMsg), burstRunner.run (pre-flight → Burster.Run → cleanBatch → terminal event), orderedMarkedSessions, beginBurst, decideBurst, dispatchBurst, seam options.
  - internal/tui/model.go:2459-2539 — terminalDetectedMsg arm resolving a deferred Enter; spawnProgressMsg/spawnCompleteMsg/spawnAbortMsg/burstChannelClosedMsg Update arms.
  - internal/tui/model.go:3560-3573 — handleMultiSelectEnter N=0/N=1/N≥2 boundary; N≥2 → beginBurst.
  - internal/tui/model.go:505-526 — burst lifecycle model fields + pendingBurstEnter.
  - internal/spawn/burst.go:133-215 — additive Run(ctx, external, progress) with per-window progress(i+1,len) + ctx checks between windows and inside awaitToken poll.
  - internal/spawn/split.go:15-17 — SplitNetN single computation (shared by CLI + picker).
  - cmd/spawn.go:161 — CLI call site updated to Run(context.Background(), external, nil) (behaviour-preserving).
  - cmd/spawn.go:292-302 — buildProductionSpawnSeams: single shared bundle (Detector/Resolve/Ack/Exe/Getenv/Exists) the CLI and picker both read.
  - cmd/open.go:426-431, 598-608 — picker seams wired from spawnSeams; resolve reused as the single injection site.
  - internal/tui/build.go:61-64, 213-216 — Deps + With* nil-tolerant wiring.
- Notes:
  - Net-N split, list order, de-dup, cursor-exclusion, defer-while-in-flight, config-aware resolve reuse are all correctly implemented and match the ACs.
  - orderedMarkedSessions walks sessionList.Items() (full backing set, not VisibleItems), so a marked session is never dropped from the open set by an applied filter — correct per "Enter opens the marked set only".
  - Goroutine isolation is clean: burstRunner captures all inputs by value; the goroutine communicates with Update only over the channel — no shared mutable model state, no data race (aligns with golang-concurrency).
  - Deliberate evolution vs the original task "Do" (documented, sound, not drift): (a) the deferred branch stashes NO ordered/trigger/external snapshot — it re-derives the marked set live from selectedSessions at resolution time (§7-5), honouring a mark toggle during the input-lock-free defer window; (b) the planned burstBatch/burstResults/burstIdentity/burstResolution model fields were intentionally NOT added — the resolved outcome travels on spawnCompleteMsg (no stale model state); (c) dispatchBurst reads the detection-time cached adapter (m.detectAdapter) rather than re-resolving, closing a documented config-script TOCTOU nil-adapter panic. All three are improvements with clear in-source rationale.
  - The reviewed codebase is the fully-implemented feature, so the spawnCompleteMsg/spawnAbortMsg arms carry the full 6-4/6-6/6-7/6-8/6-10 behaviour rather than 6-3's "minimal record-only" stub. The 6-3-owned dispatch machinery, streaming protocol, and burst-pending entry are all present and correct.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/burst_dispatch_test.go: OpensExternalInListOrder (AC1/AC2 — pending, BurstTotal==3, trigger=charlie, external=[alpha,bravo], 2 OpenWindow calls in order, never charlie, burstTotal stays N at terminal event, ack cleaned once, pending cleared on terminal); MultiTagDedup (AC3); CursorUnmarkedNeverOpened (AC4); StreamsProgressThenComplete (AC5 — [progress,progress,complete], receiver re-issued on progress, tea.Quit on terminal); DefersWhileDetectionInFlight + DetectionNeverDispatched_DefersThenResolves (AC6, incl. the dispatch-detection-so-defer-can-resolve edge); SplitDerivesFromSplitNetN (net-N drift guard); ConfigResolveUsesConfigAdapter (AC8).
  - internal/tui/multi_select_enter_test.go: N0, N1, N1IgnoresCursor, N2DetectionUnwired (AC7 — N=0/N=1 unchanged; N≥2 unwired defers, nothing opens).
  - internal/spawn/burst_test.go:422/434/455 — Run progress semantics (1/3,2/3,3/3), nil-progress CLI parity, ctx-cancel-between-windows; all CLI call sites updated to Run(context.Background(),…,nil).
- Notes:
  - Tests verify observable behaviour (open order via composed attach argv, call counts, message sequence, pending/selected state) rather than implementation internals — appropriate.
  - No over-testing: each test maps to a distinct AC/edge; helpers (sessionsFromNames, wireBurstSeams, markedSupportedBurstModel, driveBurstToTerminal, drainBatchToModel) are well-factored and shared, no redundant assertions.
  - AC8's default-wiring "config-aware resolver, degrading to empty config on error" is covered at the shared builder level (cmd buildProductionSpawnSeams/buildResolver) rather than re-asserted in tui — testing it again in tui would be over-testing; the picker only reuses spawnSeams.Resolve, so no gap.
  - Tests would fail if the feature broke (order swapped, trigger opened, dedup lost, defer regressed, denominator overwritten by N-1, config adapter bypassed).

CODE QUALITY:
- Project conventions: Followed. Small nil-tolerant DI seams via Deps + With* options; test accessors mirror existing convention; streaming machinery is a faithful clone of cmd/bootstrap_progress.go; net-N split routed through the single spawn.SplitNetN chokepoint shared with the CLI; production seams bundled once in buildProductionSpawnSeams so CLI/picker cannot drift. No t.Parallel (correct for the cmd/tui mutable-state convention).
- SOLID principles: Good. Clear single responsibilities (pipe transport vs burstRunner orchestration vs decideBurst branching vs dispatchBurst wiring); dependencies inverted behind seams.
- Complexity: Low/Acceptable. decideBurst and dispatchBurst are linear with well-labelled branches; each guard is documented with its rationale.
- Modern idioms: Yes (slices.Clone, context.WithCancel, range-over-int in tests).
- Readability: Good. Exceptionally thorough doc comments explaining the naked-terminal-vs-ctx-guarded-progress send split, the cached-adapter TOCTOU rationale, and the re-derive-live defer semantics.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (Considered and dropped: a symmetric nil-guard for m.ackChannel in dispatchBurst mirroring the belt-and-braces nil-adapter guard — dropped because a nil ack cannot arise in production, where adapter and ack are wired together from one bundle, so it would guard an impossible state; and the 64-slot buffer's naked terminal send under a >64 marked set with an abnormally-discarded receiver — dropped as an already-documented, accepted tradeoff for an unrealistic input.)
