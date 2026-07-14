TASK: restore-host-terminal-windows-2-6 — `portal spawn <sessions…>` pipeline: sequential spawn N−1 + self-attach Nth

ACCEPTANCE CRITERIA:
1. `portal spawn s1 s2 s3` (all success) → exactly two OpenWindow calls (s1 then s2, arg order); connector Connect called once with s3.
2. Each recorded OpenWindow argv is the composed env-self-sufficient attach command for that session.
3. `portal spawn s1` (N=1) → zero OpenWindow calls; self-attach s1 regardless of terminal.
4. Second window SpawnFailed → connector Connect never called; plain (non-UsageError) error naming the failed session (exit 1, stderr).
5. Inside-tmux vs outside-tmux self-attach uses SwitchConnector vs AttachConnector (buildSessionConnector branches on tmux.InsideTmux(); pipeline routes through injected Connector).
6. One INFO `spawn: opened N/N` summary with resolution/terminal/bundle_id/opened/total.
7. No unit test dials a real tmux server or runs real osascript.

STATUS: Complete

SPEC CONTEXT:
Spawn Architecture — one service, two callers: the picker/CLI always self-attaches to exactly one of the N and externally spawns the N−1 others (net-N windows, never N+1). Order is load-bearing: detect first, then resolve, spawn N−1, and only after all confirm exec-self into the Nth (a point of no return). N=1 is a plain single attach with no special-casing. Reporting: success self-execs away (no exit code); partial failure → exit 1 with a one-line stderr message, nothing self-execs. Count semantics: total = N (incl. trigger's self-attach target); opened = confirmed externals + trigger self-attach when it occurs.

IMPORTANT REVIEW CONTEXT — the codebase is at the FINAL post-plan state (all phases done), so task 2.6's Phase-2-only surface has been deliberately superseded by later in-plan tasks that build directly on it:
- The plan's `spawn.SpawnWindows` / `SpawnOutcome` were replaced by `spawn.Burster.Run` + `WindowResult` when Phase 3 added the token-ack layer. `grep` confirms zero orphaned `SpawnWindows`/`SpawnOutcome` remain — a clean supersession, not dead code.
- The composed argv now carries a trailing `--spawn-ack <batch>:<token>` (Phase 3), and the batch summary now carries `batch` + per-window `ack` attrs (Phase 3). Both are EXACTLY the "no ack/batch in Phase 2" items the 2.6 plan reserved for Phase 3, so the drift from 2.6's literal acceptance text (#2 bare argv, #6 no ack/batch) is intended plan evolution, not a defect. The self-attach gate, once "adapter success", is now "all tokens confirmed" — a strict strengthening of the same all-or-nothing invariant.
The core 2.6 behaviour (detect → resolve → SplitNetN → spawn N−1 sequentially → gate the single self-attach on all-success → N=1 direct self-attach → failure skips self-attach + exit 1 → inside/outside connector) is present and correct in the final code.

IMPLEMENTATION:
- Status: Implemented (evolved-as-planned by later in-plan tasks)
- Location:
  - cmd/spawn.go:116-218 (runSpawn) — the full pipeline: SplitNetN (125), preflight gate (136-139, Phase 3), N=1 direct self-attach (145-147), detect-then-resolve order (150-151), unsupported gate (156-159, Task 2.7), burst (161), Clean-before-handoff (169), all-confirmed gate + single self-attach (216-217).
  - cmd/spawn.go:36-69 (SpawnDeps) — Detector/Resolve/Connector/ExePath/Getenv seams from the plan, plus Exists/Ack/NewBurster/Logger added by later tasks. Production defaults wired via buildSpawnDeps (315-370) through the shared buildProductionSpawnSeams bundle (292-302) that the picker also reads (drift-proofing).
  - internal/spawn/burst.go:133-182 (Burster.Run) — resolve exe once up front (134-137), all ids up front (140-151), sequential per-window compose→open→await (154-180), no early-stop except permission (177-179).
  - internal/spawn/split.go:15-17 (SplitNetN), command.go:27-34 (composeAttachArgv), classify.go (PartitionResults/FirstPermission/Confirmed), preflight.go (PreflightMissing) — the shared pure chokepoints both CLI and picker derive from.
- Notes: The N−1/self-attach split, count semantics, and failure partition are all routed through single shared functions (SplitNetN, PartitionResults, LogBatchSummary) so the CLI and picker cannot diverge — this directly satisfies the plan's "faithful CLI test seam the picker reuses" intent. Detect is correctly skipped on the N=1 path (no adapter needed). Clean(batch) runs on every post-burst path and strictly before the exec handoff.

TESTS:
- Status: Adequate
- Coverage: The six plan-named tests map 1:1 to the CLI suite in cmd/spawn_test.go:
  - "self-attaches only after every external window's token is confirmed" (285) — AC#1: 2 OpenWindow calls, s1/s2 arg order, Connect once with s3.
  - "composes the env-self-sufficient attach command with the ack flag" (321) — AC#2: exact argv incl. --spawn-ack, via wantAttachArgv.
  - "self-attaches directly with zero spawns for N=1 regardless of terminal" (353) + "still self-attaches for N=1 on an unsupported terminal" (712) — AC#3.
  - "skips self-attach and cleans markers when a window is not confirmed" (436) + TestSpawnPartialFailure (946) — AC#4: no Connect, plain non-UsageError, no Result.Detail leak, failed session named.
  - "routes self-attach through the inside/outside-tmux connector" (475) — AC#5: buildSessionConnector type-branches on TMUX.
  - "emits a spawn: opened N/N summary…" (488) — AC#6: opened 3/3 with resolution/terminal/bundle_id/opened/total (+ Phase-3 batch/ack).
  - Burster.Run unit tests (internal/spawn/burst_test.go) cover exe-resolved-once, arg-order argv, per-window timer independence, no-early-stop-on-failure, permission early-stop, abort-before-any-window on exe/id failure, and progress/cancel — the engine behind the pipeline.
  - AC#7 structurally enforced: every seam injected; cmd TestMain poisons TMUX so a missed injection fails loudly rather than reaching the real server.
- Notes: Would fail if the feature broke (arg-order assertions, exact-argv equality, Connect-target equality, error-type/leak assertions). Not over-tested for this task — each case targets a distinct behaviour or edge (N=1, N=1-unsupported, timeout-vs-spawn-failed, permission, preflight-gone). The connector-routing test verifies the branch on buildSessionConnector directly rather than through the pipeline; acceptable because the pipeline's delegation to the injected Connector is already proven by the conn.calls assertions in the other cases.

CODE QUALITY:
- Project conventions: Followed. Injectable *Deps struct with production defaults, no t.Parallel(), package-bound `spawnLogger = log.For("spawn")`, closed attr vocabulary, shared spawn/tui chokepoints, fake adapter/connector/ack (no real tmux/osascript). Matches CLAUDE.md's DI/testing and logging conventions.
- SOLID principles: Good. Single-responsibility split across runSpawn (orchestration), Burster (external half), and pure helpers (split/classify/preflight/command). Seam interfaces are small (1 method). CLI/picker share behaviour through pure functions rather than duplication.
- Complexity: Acceptable. runSpawn is linear with clearly-commented branch order; the branching (preflight → N=1 → detect/resolve → unsupported → burst → partition) reads top-to-bottom with load-bearing ordering documented.
- Modern idioms: Yes (slices.Clone/Equal, context cancellation, functional seams).
- Readability: Good, though the in-source commentary is dense/verbose — consistent with the repo's house style.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (Task 2.6's Phase-2-literal argv/summary shape is intentionally superseded by in-plan Phase 3 additions — expected evolution, no action required. No concrete change surfaced that survives the action floor.)
