---
status: complete
created: 2026-07-12
cycle: 6
phase: Traceability Review
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Traceability

## Result

**CLEAN** — no findings. Cycle 6 is a full fresh two-directional pass over the
entire specification, the planning file, and all 45 authored task files
(phase-1-tasks.md … phase-6-tasks.md). The plan is a faithful, complete
translation of the specification. Nothing in the plan is untraceable to the
spec, and every spec element has adequately-detailed plan coverage.

## Direction 1 — Specification → Plan (completeness)

Every spec section maps to one or more tasks with matching acceptance criteria
and enough detail that an implementer need not return to the spec:

- **Spawn Architecture** (one service / two callers; N vs N−1 split; order
  load-bearing: detect → spawn N−1 → self-attach-last; command composition via
  `os.Executable()`; env-self-sufficient argv `/usr/bin/env … PATH=<picker PATH>`
  with load-bearing `TMUX`/`TMUX_PANE` strip; uniform mechanism for native +
  config recipes) → 2-3, 2-6, 3-5, 6-3, 6-4.
- **`portal spawn` CLI + Reporting/exit codes** (self-exec on success → no exit
  code; pre-flight abort / partial failure / N≥2-unsupported → exit 1 on stderr;
  permission-required → exit 1; usage error → exit 2; `--detect` dry-run) →
  1-6, 2-6, 2-7, 3-3, 3-4, 3-6, 3-7.
- **Multi-Select Mode** (m enter/toggle real mode with zero selected; M retired;
  Enter commit / Esc exit; N=0/N=1 boundary; live Space/`/`/s vs suppressed
  k/x/r; filter inner-sub-state owns Enter/Esc; sticky selection incl. Space-
  preview prune; violet banner + `●` markers + footer copy; session-identity
  selection set) → 5-1…5-8.
- **Burst & Partial-Failure Contract** (pre-flight all-or-nothing; self-attach
  gated on all-N−1-confirming; `@portal-spawn-<batch>-<token>` token-ack channel;
  option-name-safe ids; per-window `spawnAckTimeout` ~8s; leave-what-opened;
  permission burst-stop; `--spawn-ack` best-effort write point; async
  non-blocking `tea.Cmd`; `Opening n/N…`; input-lock; cancellation post-state) →
  3-1…3-7, 6-3…6-8.
- **Terminal Identity & Detection** (outside-tmux env fast-path + process-tree
  walk; inside-tmux list-clients NULL-filter + local-only activity tiebreak;
  host-local principle; bundle-id family match; user-facing both name+id;
  detect-once-cached off-first-paint; error-vs-clean-NULL; in-flight-at-Enter
  awaited; unsupported banner + N=1-works/N≥2-blocked asymmetry) → 1-1…1-5,
  6-1, 6-2, 6-9.
- **Adapter Contract & Extensibility** (generic `OpenWindow(command)`, session-
  aware variant rejected; typed `Result` taxonomy with opaque detail/guidance;
  resolver config→native→unsupported; capability extensibility) → 2-1, 2-2, 4-6.
- **Config Schema (`terminals.json`)** (tolerant-decode store; exactly-one-of
  argv/script + `{command}` presence; within-config most-specific precedence;
  argv/script recipe adapters incl. `~` expansion + `$1` + exec-bit gate;
  wired ahead of native; read-only, no sessions.json/daemon interaction) →
  4-1…4-6.
- **Permissions & Error Quarantine (TCC)** (driver-only OS specifics; typed
  `permission-required`/`unsupported`/`spawn-failed`; self-exempt no first-run
  gate; defensive net burst-stop with guidance-once) → 2-1, 2-5, 3-7, 6-6.
- **Observability & State Footprint** (`spawn` component + closed attr set;
  one INFO cycle-summary + DEBUG per-window; count semantics total=N incl.
  trigger, opened counts trigger only on full success; near-zero persistent
  state) → 1-5, 2-6, 3-5, 6-10; 3-2/4-6 (no sessions.json/daemon/prefs touch).
- **Trigger-Context Matrix & Open Order** (in/out tmux reuse; already-attached-
  elsewhere; includes-self; vanished→pre-flight; Enter opens marked set only;
  open in list order, trigger unspecified) → 2-6, 3-4, 6-3, 6-4, 6-7.
- **Concurrency & Post-Reboot Safety** (latch dependency; burst gated to after
  hydration via BootstrapCompleteMsg; abridged attaches don't perturb capture) →
  6-1 dispatch gating; 2-3 own-binary latch parity.
- **Design References / Visual gates** (three delivered frames + `Opening n/N…`
  residual; violet/amber/red, no new tokens; selected-only clean, no dim `○`;
  reference-first move to testdata/vhs/reference; dark-only) → 5-8, 6-11.

## Direction 2 — Plan → Specification (anti-hallucination)

Every task's Problem, Solution, Do steps, acceptance criteria, tests, and edge
cases trace to a cited spec section (each task carries a Spec Reference and
inline spec quotes). Spot checks confirmed no invented requirements:

- Implementation-only decisions the spec does not pin are **explicitly flagged
  as such** in-task rather than presented as spec requirements — e.g. friendly-
  name-derivation algorithm (1-1), `GHOSTTY_*` key set + "malformed" plausibility
  rule (1-3), ack poll interval ~75ms (3-5), exact CLI/flash copy wording
  (1-6, 2-7, 6-9), pre-flight-before-detect ordering (3-4).
- Design-vs-spec reconciliations are anchored to authority: banner/flash
  section-header placement governed by the delivered golden frames (5-3, 6-2,
  6-7 placement notes); env fast-path-first ordering justified against the
  spec's "or" (1-3); list-order trigger split against the spec's "trigger
  becomes: unspecified" (2-6, 6-3).
- The picker's `Burster.Run` pre-spawn-error flash (6-6) is the necessary picker
  analogue of the CLI's `return err` on the same os.Executable/ack-id error
  paths (2-3, 3-1) — completeness handling of an error the spec's own design
  produces, not invented behaviour.
- Build-time residuals (iTerm2/Terminal.app TCC check; macOS-version walk
  confirmation; Ghostty preview-API pin/watch; `spawnAckTimeout` build
  confirmation) are carried as noted manual confirmations in task context, not
  spuriously promoted to tasks.

## Prior-cycle convergence verification

The cycle-3/4/5 `QuoteJoin` / `GoneVerb` shared-helper thread is fully resolved
and internally consistent:

- **Defined once**, exported, in `internal/spawn/message.go` by Task 3-4 (its
  first consumer), with an explicit rationale that unexported forms are
  unreachable from `cmd`/`tui` and will not compile.
- **Consumed without re-declaration** as `spawn.QuoteJoin` / `spawn.GoneVerb` by
  Tasks 3-6 (CLI leave-what-opened message), 6-6 (picker partial-failure flash),
  and 6-7 (picker pre-flight-abort banner) — each carrying an explicit
  "do NOT re-declare" guard against a duplicate-declaration compile error.
- Singular/plural behaviour (`GoneVerb` → `is`/`are`) byte-matches the delivered
  design copy `⚠ '<session>' is gone — nothing opened` (6-7) and the CLI
  `spawn: 's2' is gone — nothing opened` (3-4).

## Findings

None.
