---
status: complete
created: 2026-07-19
cycle: 2
phase: Traceability Review
topic: CLI Verb Surface Redesign
---

# Review Tracking: CLI Verb Surface Redesign - Traceability (Cycle 2)

Fresh, full bidirectional traceability pass over the whole plan against the
validated specification
(`.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md`),
the planning file, and all six phase task-detail files (36 tasks total: 5 / 7 / 8 / 8 / 3 / 5).
This was a from-scratch re-verification, not a delta check of cycle-1 fixes.

## Verdict

**Clean — no findings.** The plan is a complete and faithful translation of the
specification. Every spec decision, axiom, resolution rule, flag, burst mechanic,
maintenance verb, retirement, and surface-presentation item traces to at least one
task with matching acceptance criteria and implementation-ready depth; every task's
content traces back to a specific spec section. The single gap cycle 1 found
(daemon automatic stale-project pruning) is closed by the added Task
`cli-verb-surface-redesign-4-8`, which is present and correctly scoped.

## Direction 1 — Specification → Plan (completeness)

Every specification element was traced to a covering task. Coverage map:

- **Governing principle / Axiom 1 (absorb / net-N) / Axiom 2 (attach-vs-mint)** —
  Overview + Phase 3 (net-N); Tasks 1-1 (session-hit → attach), 1-2 (dir-hit → mint,
  no find-or-create).
- **Grammar & target resolution** — glob pre-check (1-3), precedence chain
  session→path→alias→zoxide (1-1, 1-2), user-visible session set only (1-1, 1-3, 2-1),
  bare-project-shorthand-does-not-reattach (1-2), total-miss hard-fail + removed TUI
  fallback + `nothing resolved for 'x' — try -f x` (1-2), `resolve` log component /
  INFO decision line / attrs `target`/`domain`/`resolved_path` / glob-and-pin gating /
  miss emission / one-line-per-guessing-chain-target (1-4, 3-3).
- **Flags & command passthrough** — `-s/-p/-z/-a` pins + pinned-domain hard-fail
  contract (2-1..2-4), `-z` explicit `ErrZoxideNotInstalled` (2-4), `-a` shadow bypass
  + key globs (2-3), `-f` sole non-composing flag + mutual exclusivity (1-5, 2-5),
  `-e`/`--` mint-scoped + both-spellings/empty-command usage errors (2-6), no-target
  command → Projects (mint-only) picker with `Pick a project to run <cmd>` banner +
  filtered-Projects variant (2-7), hidden `--ack` + best-effort marker write (3-1).
- **Multi-target burst mechanics** — target-set union (3-2), argv-order recovery from
  `os.Args` (`-s api` / `-s=api` / `--session=api`, `-e`/`--`/`--ack` exclusion, no
  dedup) (3-2), read-only resolve/classify + glob/alias-glob K-expansion + literal-dir
  reduction (3-3), atomic aggregated pre-flight abort reporting every miss + single-target
  `-f` carve-out (3-4), spawned-window `open`-grammar argv (`--session` / `--path
  <literal-dir> --ack`) + warm-latch `os.Executable()` + TMUX strip (3-5), trigger
  absorbs first / spawns N-1 / connects last / no current-session special-casing / no
  dedup (3-6), command rides mint windows only byte-identically + trigger local-mint +
  multi-target zero-mint usage error (3-7), leave-what-opened + per-window ~8s ack timeout
  + `portal.log` outcomes + permission-stop (3-8).
- **Tab completion** — all six spec slots: open bare positional / open -s / kill →
  session names (6-3); open -a → alias keys (6-4); open -p / open -z → shell delegation
  (6-4); `__complete` bootstrap-exemption hazard (6-3).
- **`attach` retired** — deleted outright, behaviours migrated to `open --session`,
  spawned-window exec target, command-agnostic abridged fast-path (5-1); retired-surface
  guard (5-3).
- **`kill`** — unchanged (single + exact); only spec-mandated addition is completion,
  covered by 6-3. No behavioural task needed — correct.
- **`uninstall`** — runtime-only teardown, touches no files, idempotent, leaves all
  sessions incl. `_portal-bootstrap`, byte-exact completion message, no `--yes`, deletes
  `state cleanup` (4-6).
- **`doctor`** — full 7-item check catalog: state-dir checks incl. sessions.json via
  ReadIndex (4-1), runtime tmux checks + distinct down-server report + per-event hook
  count (4-2), read-only stale-entry checks + down-server not-evaluable guard (4-3),
  host-terminal informational line replacing `spawn --detect` (4-4), `--fix` repairs +
  unconditional log-sweep side-action + re-diagnose (4-5); exit-code contract across
  4-1/4-2/4-4; `clean` + `state status` deletion + helper relocation + exempt-set drop
  (4-7); automatic daemon stale-project pruning (4-8, the cycle-1 fix).
- **`state` namespace fully hidden** — parent `Hidden:true`, six children stay
  argv-invocable, `state` prefix preserved for substring matchers (6-2).
- **Remaining verbs** — `hooks` → `hook` + permanent silent `hooks` cobra alias +
  skipTmuxCheck repoint (6-1); `list`/`alias`/`init`/`version`/`completion` unchanged
  (no task needed — correct).
- **Bootstrap exemption** — `doctor` (4-1), `uninstall` (4-6), `hook` rename retains
  exemption (6-1), `__complete` (6-3), `state` stays / `clean` leaves (6-2, 4-7).
- **Bare `portal`** — help/usage, not the picker; control-plane root guard (6-5).
- **Back-compat & deprecation** — no aliases for `attach`/`spawn` (5-1, 5-2, 5-3), the
  single `hooks` carve-out (6-1).
- **Deferred scope** — stay-put multi-open flag, multi-match zoxide, bulk-kill via
  picker, in-tmux burst error surface: correctly NOT built anywhere in the plan.

Depth was checked, not just presence: every task carries file paths, function/method
names, exact error/message strings, edge cases, and named tests — an implementer would
not need to return to the specification.

## Direction 2 — Plan → Specification (fidelity / anti-hallucination)

Every task's Problem / Solution / implementation detail / acceptance criteria / tests
traces to a specific spec section. Specific fidelity checks:

- **`internal/spawn` code extensions** (`Surface` type, `composeOpenArgv`,
  `SplitTriggerFirst`, `Burster.Run` `[]string → []Surface`) are consistent with the
  spec's scope statement, which places only internal **names** out of scope
  (`internal/spawn`, the `spawn` log component, `@portal-spawn-*` markers are "unaffected
  by the redesign" = not renamed/deleted). The spec's own burst mechanics
  (`--path <literal-dir>` mint windows) require these code changes; the tasks retain the
  package name, the `spawn` log component (`log.For("spawn")`), the markers, and
  `SplitNetN`. Not a hallucination.
- **Implementation constructs** (`checkNotEvaluable`, `checkInfo`, `Surface`,
  `openTarget`, `resolveOpenSurfaces`, `OpenBurstDeps`) each trace to a spec concept
  (down-server not-evaluable guard; host-terminal informational; attach/mint surfaces;
  argv ordering; read-only resolve; net-N dispatch). No invented behaviour.
- **Flagged planner decisions where the spec is silent** — miss-message wording
  (`No session found` / `No alias found` / `No zoxide match for`), empty `-f`/`-e` →
  usage error, absent-vs-corrupt `sessions.json` via `ReadIndex`, aggregated multi-target
  miss wording, pin-fault hard-vs-miss aggregation, single-target multi-match glob "first
  match" placeholder — all surfaced inline in the task detail and intentional per the
  planning context; not re-flagged.
- **Two user-resolved burst decisions** (multi-target unsupported-terminal block —
  Task 3-6; permission-wall still connects the trigger — Task 3-8) are recorded with
  their 2026-07-18 decision provenance and are internally consistent (unsupported = whole
  burst blocked, no half-connect; permission wall on a supported terminal ≠ that case).
  Not fidelity violations.
- **No untraceable content found** — no task requirement, edge case, acceptance
  criterion, or test lacks a corresponding specification section.

## Findings

None.
