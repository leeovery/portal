TASK: skip-bootstrap-when-warm-1-2 — Set the latch as the final action of a successful Orchestrator.Run

ACCEPTANCE CRITERIA:
- Orchestrator has a Latch LatchWriter (best-effort, nil-tolerant) field + a Version string field; *tmux.Client satisfies LatchWriter via its existing SetServerOption.
- A soft-warning-only Run reaches the stamp and calls SetServerOption("@portal-bootstrapped", version) exactly once, before the orchestration-complete summary.
- A Run that aborts at a fatal step returns before the stamp — writer never called.
- A stamp-write error is logged WARN under the bootstrap component and swallowed: Run returns (serverStarted, warnings, nil); warnings unchanged (no latch-write warning appended); no StepEvent emitted for the write.
- The stamp uses state.BootstrappedMarkerName and o.Version verbatim (parse-free).
- Production wiring (buildProductionOrchestrator) sets Latch: client and Version: version.
- On the concurrent path the latch is written before the terminal Done / BootstrapCompleteMsg (verified by in-Run-before-return ordering; no bootstrap_progress.go change).
- go build / go test ./cmd/... ./cmd/bootstrap/... pass; golangci-lint clean.

STATUS: Complete

SPEC CONTEXT: "Latch Set-Point & Timing" mandates the latch be set as the final action of a *successful* Orchestrator.Run — inside Run (uniform across sync + concurrent invocation modes), at the END (early-setting would let a concurrent command take the abridged path before Restore recreated sessions), gated on no FATAL error (soft warnings still latch — else one transient SaverDownWarning forces every command back to full bootstrap for the server lifetime), best-effort (write failure is a pure WARN log line, never fatal / never in warnings / never on the progress channel), and — on the concurrent path — written before the terminal completion event. Storage is the @portal-bootstrapped server option stamped with the bare cmd.version (parse-free equality downstream). Consumes task 1-1's state.BootstrappedMarkerName const.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/bootstrap/bootstrap.go:207-216 — LatchWriter seam (single-method SetServerOption(name, value string) error).
  - cmd/bootstrap/bootstrap.go:255-263 — Orchestrator.Latch + Orchestrator.Version fields, documented best-effort/nil-tolerant + ldflags-injected version.
  - cmd/bootstrap/bootstrap.go:485-501 — the latch write, placed after the last soft step (SweepOrphanFIFOs, emitStep(10)) and after all fatal steps (which return early via o.fatalf), before the orchestration-complete summary + return. Guarded by `if o.Latch != nil`, one-line comment explaining why no extra error gate is needed.
  - cmd/bootstrap/bootstrap.go:239-244, 293-300 — package/Orchestrator doc + Run godoc updated with the best-effort/WARN-swallow contract.
  - internal/state/markers.go:21-29 — state.BootstrappedMarkerName = "@portal-bootstrapped" (task 1-1 dependency present).
  - cmd/bootstrap_production.go:157-163 — buildProductionOrchestrator sets Latch: client and Version: version.
  - cmd/bootstrap_progress.go:178-199 — confirmed unchanged: the goroutine sends Done only after runner.Run returns, so the in-Run stamp precedes the terminal event on the concurrent path.
- Notes: The WARN body is `o.Logger.Warn("latch write failed for "+state.BootstrappedMarkerName, "error", err)` — the marker name is folded into the message rather than carried as a "marker" attr. This is a deliberate divergence from the literal DO text (`"marker", state.BootstrappedMarkerName`), applied by a later phase-7 task (commit 057719b3 "drop non-vocabulary marker attr from latch-write WARN") to honour the codebase's closed attr-key vocabulary (only "error" is permitted alongside the handler-injected baselines). This is a correctness fix, not drift, and the test asserts it. No CleanStale-ordering concern remains: totalSteps is already 10 (task 1-3 landed), so the write sits correctly at the emitStep(10) tail.

TESTS:
- Status: Adequate
- Coverage (cmd/bootstrap/latch_test.go, no t.Parallel(), recordingLatch double capturing (name,value) + primable err + optional ordering seq):
  - stampsLatchWithVersionAfterSoftWarning — EnsureSaverErr → SaverDownWarning; asserts exactly one SetServerOption(BootstrappedMarkerName, "v1.2.3") AND the SaverDownWarning survives in returned warnings. Covers the soft-warnings-still-latch rule + verbatim name/value.
  - doesNotStampLatchOnFatalAbort — SetErr aborts SetRestoring; asserts zero latch calls and a *FatalError return. Covers the fatal-abort-leaves-latch-unset edge.
  - swallowsLatchWriteFailureAsWarn — primed err; asserts Run returns (_, warnings, nil) with zero warnings appended, StepEvent count == step-complete count (write emits none — robust to totalSteps value), and a WARN under component=bootstrap whose message names the marker with NO "marker" attr. Covers the best-effort write-posture edge precisely.
  - stampsLatchBeforeOrchestrationComplete — shared seq proves the latch write precedes the "orchestration complete" INFO line — the ordering that guarantees the concurrent-path invariant.
  - Compile-time assertions: `var _ LatchWriter = (*recordingLatch)(nil)` (latch_test.go:47) and `var _ bootstrap.LatchWriter = (*tmux.Client)(nil)` (cmd/bootstrap_production_test.go:30).
  - Nil-tolerance of the guard is structurally exercised by every pre-existing bootstrap_test.go test: newOrchestrator (bootstrap_test.go:239-252) leaves Latch nil, so those runs hit the `o.Latch != nil` false branch.
- Notes: Not under-tested — all four required cases + both compile assertions present, every spec edge case (fatal-unset, soft-latch, write-swallow, ordering) covered. Not over-tested — each test targets a distinct behaviour with no redundant assertions; the event-count check is deliberately made robust to the sibling task 1-3 step-count change rather than hard-coding a total.

CODE QUALITY:
- Project conventions: Followed. Single-method seam (ISP), nil-tolerant best-effort posture matching the other soft steps, reuses the state.BootstrappedMarkerName const (no literal at call site), no t.Parallel() in tests, honours the closed slog attr-key vocabulary (the phase-7 marker-attr removal).
- SOLID principles: Good. LatchWriter is a focused 1-method interface satisfied implicitly by *tmux.Client; the orchestrator depends on the seam, not the concrete client.
- Complexity: Low. A single nil-guarded write with an inline error branch; no added control-flow depth.
- Modern idioms: Yes. Implicit interface satisfaction + context-threaded emitter unchanged.
- Readability: Good. The write site and both doc blocks explain the set-point, the no-extra-gate reasoning, the best-effort swallow, and the concurrent-path ordering guarantee.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (The WARN-message-vs-attr divergence from the DO text is an intended phase-7 correction, tested and vocabulary-compliant — no action.)
