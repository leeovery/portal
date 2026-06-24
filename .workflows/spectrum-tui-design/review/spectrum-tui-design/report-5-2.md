TASK: spectrum-tui-design-5-2 — Progress channel + goroutine orchestrator wrapper (cold/TUI path)

ACCEPTANCE CRITERIA (from tick-933d99):
- On cold + TUI, Bubble Tea launches on the loading page before the orchestrator finishes; the orchestrator runs in a goroutine
- One tea.Msg is delivered per real bootstrap step in step order; serverStarted is carried over the channel
- A terminal complete event drives the transition to Sessions; Sessions enumeration is still gated on that event (TUI inert during loading)
- The channel is drained and closed on completion; no goroutine leak and no blocked receive after the program quits
- The warm/CLI synchronous path keeps serverStartedKey context delivery and the sync.Once memo unchanged (byte-for-byte parity)
- Non-visual plumbing — vhs-exempt; verification is behavioural

STATUS: Complete

SPEC CONTEXT:
§10.2 startup flip. Today the 11-step bootstrap runs synchronously in PersistentPreRunE before Bubble Tea launches, so the loading page renders over a frozen terminal (cosmetic 1.2s pad). The fix: on the cold+TUI path only, launch Bubble Tea on the loading page from frame one and run Orchestrator.Run concurrently in a goroutine, streaming live per-step progress over a channel that also carries serverStarted (replacing the context value + package-memo delivery used by the synchronous model). The warm/CLI synchronous path must stay byte-for-byte unchanged. The TUI must be inert during loading (no enumeration / no page nav until the terminal complete event) — this is the race-containment property. §10.5 is the fatal cold-boot surface (task 5-6) that this task leaves a carry-forward hook for.

IMPLEMENTATION:
- Status: Implemented (clean, idiomatic, correct)
- Location:
  - cmd/bootstrap/progress_emitter.go — context-carried ProgressEmitter seam (StepEvent, WithProgressEmitter, progressEmitterFromContext, ProgressEmitterFromContextForTest). Lowest-risk wiring choice (context, not an Orchestrator field) is documented in the file header.
  - cmd/bootstrap/bootstrap.go:281-286, :310,319,328,346,357,400,416,425,440,456,466 — emit resolved once; emitStep no-op when emitter nil; one emit per step at the same site as the "step complete" INFO log, in step order. Fatal steps emit nothing for the aborting step or after (fatalf returns before emitStep).
  - cmd/bootstrap_progress.go — bootstrapProgress event struct, buffered channel (size 64), bootstrapProgressPipe owning channel + goroutine, start() (wires emitter via WithProgressEmitter, runs runner.Run in a goroutine, sends terminal Done event, defer close), send() (ctx-guarded select), receiver() tea.Cmd (single blocking receive mapped to tui.BootstrapProgressMsg / BootstrapCompleteMsg / BootstrapFatalMsg / bootstrapChannelClosedMsg), ServerStarted/Warnings/Err post-program accessors.
  - cmd/open.go:445-452, :558-560, :596 — openTUI builds the pipe iff a deferred bootstrap is on the context, starts the goroutine, forces serverStarted=true (cold by construction), wires cfg.progressReceiver, launches tea.NewProgram.
  - cmd/root.go:170-183 — concurrent route returns before runBootstrap, so serverStartedKey + bootstrapOnce are never touched on the deferred route.
- Notes:
  - Emitter-wiring choice (context) satisfies the "keep synchronous path emitter nil" mandate exactly: Background context → nil emitter → no-op. Verified by TestRun_NoEmitterIsNoOp.
  - serverStarted is carried twice by design: on the terminal channel event (ServerStarted field, read by the model) AND on the pipe accessor (post-program drain seam). Production reads the model's value off BootstrapCompleteMsg; the accessor is consumed by 5-2 unit tests + the Part-D integration test. Not orphaned.
  - The §10.5 fatal carry-forward (FailedStep, Fatal on the EVENT, fatalMsgFromEvent) and §10.7 warnings carry-forward are wired here as value-copy-on-event so the receiver never reads the goroutine's struct fields mid-flight — the documented 5-2 race-avoidance contract. Correct.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/bootstrap/progress_emitter_test.go: emits one StepEvent per step in exact spec order with canonical index+name (TestRun_EmitsProgressEventPerStepInOrder); no-emitter synchronous route runs all 11 steps untouched (TestRun_NoEmitterIsNoOp); fatal step emits nothing for/after the abort (TestRun_FatalStepStopsEmitting).
  - cmd/bootstrap_progress_test.go: 11 progress msgs + terminal + close (EmitsPerStepThenTerminalThenCloses); exact 1..11 order (PreservesStepOrder); serverStarted carried (CarriesServerStartedOnTerminalEvent); fast M=0 boot still emits per-step + terminal (FastColdBoot_ZeroRestoreItems); channel closes on fatal + Err carried (ClosesChannelOnFatal); warnings ride the terminal event (CarriesWarningsOnTerminalEvent); post-close receive returns the sentinel WITHOUT blocking (ReceiverStopsReIssuingOnClose) — the no-leak / no-blocked-receive-after-Quit AC.
  - cmd/concurrent_coldboot_integration_test.go: real-tmux Part-D pins — step ordering + daemon singleton + serverStarted=true on terminal event + no leaked/zombie daemon (assertNoExtraDaemons); fast empty restore; @portal-restoring window set before restore; WARM-PATH PARITY (synchronous route, no deferred bootstrap, serverStarted=false, no loading page).
  - Warm-memo parity: cmd/bootstrap_orchestrator_test.go (TestPersistentPreRunE_OrchestratorMemoisedAcrossInvocations) + cmd/concurrent_bootstrap_route_test.go.
  - TUI-inert / gated-transition + enumeration-gating verified at the consumer in internal/tui (BootstrapProgressMsg arm never drives the transition; only terminal BootstrapCompleteMsg does).
- Notes:
  - drainPipe's per-receive goroutine + 2s deadline is the idiomatic substitute for the Bubble Tea runtime and correctly fails loud on a wedged (never-closing) channel — this is the goroutine-leak guard in unit form.
  - Not over-tested: each test pins one distinct property; the two "11 progress + terminal" tests differ in intent (count/close vs M=0 fast-boot edge) and are not redundant.
  - go.uber.org/goleak is not a project dependency, so its absence here is consistent with the codebase; leak-freedom is instead asserted behaviourally (ReceiverStopsReIssuingOnClose + the integration assertNoExtraDaemons).

CODE QUALITY:
- Project conventions: Followed. No t.Parallel in cmd tests (package-mutable bootstrapDeps). Small interfaces + context-carried seam match the DI pattern. Component logging untouched on both routes. Build + full go test ./... reported GREEN.
- SOLID principles: Good. The emitter seam is interface-segregated (ProgressEmitter func type), the orchestrator gains no field, the pipe has a single responsibility (channel+goroutine ownership), RestoreProgressSink kept off the Restorer interface so the synchronous contract is unchanged.
- Complexity: Low. start/send/receiver are each short and linear; the only branch is the ctx-guarded select and the terminal-event mapping.
- Modern idioms: Yes. Sole-sender-closes via defer close; ctx.Done() in the send select; value-struct events (no pointer sharing on the channel); typed sentinel for close detection.
- Readability: Good — arguably documentation-dense, but every comment is load-bearing (the carry-forward race contract, the lastStep+1 failed-step derivation, the buffer-bound rationale).
- Concurrency correctness (scrutinised per golang-concurrency): SOUND.
  * Channel ownership/close: the single goroutine in start() is the sole sender and closes via defer close(p.ch); the receiver never closes. Correct.
  * Goroutine exit: guaranteed — runner.Run returns, the ctx-guarded terminal send cannot block forever, then close fires. No fire-and-forget leak.
  * ctx.Done() in select: send() selects on p.ch<-ev and <-ctx.Done(), so an early-Quit (cancelled program ctx) drops the event instead of wedging on a full buffer — the documented >bufferSize restore-burst hazard (5-3) is pre-handled here.
  * No pointer sharing: bootstrapProgress is sent by value; the slice/interface fields (Warnings, Fatal) it carries are read only post-terminal-event, single-threaded.
  * Field-write race (p.serverStarted/warnings/err written by the goroutine, read by accessors): race-free by happens-before — the writes precede the terminal send/close that the reader synchronises on (tests/integration read accessors only after the channel has closed). The struct-field write is also redundantly mirrored onto the terminal EVENT (value copy) for the model's read path, so the model never touches the struct fields at all.
  * lastStep mutation lives entirely on the orchestrator goroutine (emitter runs synchronously inside Run; terminal read is post-Run on the same goroutine). No cross-goroutine access.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/bootstrap_progress.go:159-176 — the start() emitter closure receives ctx as a parameter but the comment refers to "carry-forward from 5-2"; the closure captures the outer ctx and also re-passes it into p.send(ctx, ...). The parameter and the captured value are the same ctx, so the explicit ctx arg on send() is technically redundant given send is a method on the same pipe. Consider whether send() should read a pipe-stored ctx instead of taking it as an arg, to remove the duplicate-ctx threading. Pure ergonomics — decide whether the explicit-arg form (clearer call-site intent) is preferred over the field form. No behavioural change.
