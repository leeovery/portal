---
status: complete
created: 2026-07-12
cycle: 5
phase: Traceability Review
topic: Restore Host Terminal Windows
---

# Review Tracking: Restore Host Terminal Windows - Traceability

Cycle-5 final-convergence pass. Full fresh bidirectional trace of the specification against the 6-phase / 45-task plan (`planning.md` + `phase-1..6-tasks.md`). No memory or summary shortcuts — the specification was re-read in full and every task file read end-to-end.

**Result: CLEAN. No findings.** The plan is a faithful, complete translation of the specification in both directions.

## Direction 1 — Specification → Plan (completeness)

Every specification section has task coverage at implementer-grade depth:

- **Overview / Foundational shape** (multi-select `m`→mark→Enter, net-N-never-N+1, Ghostty-first + config escape hatch, bundle-id detection, no dup-guard) → Phases 5 (mode) + 6 (burst), Phase 2 (Ghostty), Phase 4 (config), Phase 1 (detection).
- **Spawn Architecture** (one-service/two-callers, CLI behaviour + exit codes, N vs N−1 split, load-bearing order, `os.Executable()` composition, env-self-sufficient PATH-inject + `TMUX`/`TMUX_PANE` strip, `--spawn-ack` in composed argv) → 1-6, 2-3, 2-6, 2-7, 3-3, 3-5.
- **Multi-Select Mode** (trigger/marking, N=0/N=1 boundary, key coexistence, sticky selection, filter-as-inner-sub-state, mode affordance/banner/`●`/footer, per-session session-identity granularity) → 5-1..5-7, 6-2, 6-5.
- **Burst & Partial-Failure Contract** (pre-flight all-or-nothing, spawn-then-self-attach-LAST gated on all-confirm, explicit token-ack channel, per-window timeout `spawnAckTimeout` ~8s, cleanup, leave-what-opened, async non-blocking `tea.Cmd`, `Opening n/N…`, input-lock, cancellation post-state, sequential spawn) → 3-1..3-7, 6-3, 6-4, 6-5, 6-6, 6-8.
- **Trigger-Context Matrix & Open Order** (in/out tmux reuse, already-attached-elsewhere, includes-self, vanished→pre-flight, Enter-opens-marked-set-only, list-order/set, trigger unspecified, focus to OS) → 2-3, 2-6, 3-4, 5-7, 6-3, 6-4, 6-7.
- **Terminal Identity & Detection** (standalone op, outside/inside detection model + host-local principle, bundle-id family match, user-facing both-forms display, layered config keys, no-headless→NULL fold, detect-once-cached lifecycle off-first-paint, error-vs-clean-NULL, in-flight-at-Enter, unsupported banner + Enter) → 1-1..1-6, 2-2, 2-7, 4-3, 6-1, 6-2, 6-9.
- **Adapter Contract & Extensibility** (detection separate from adapter, config→native→unsupported precedence, generic `OpenWindow(command)`, two implementations, single open-capability) → 2-1, 2-2, 2-4, 2-5, 4-4, 4-5, 4-6.
- **Config Schema (`terminals.json`)** (location/format, structure, argv/script recipe, `{command}` placeholder, execution contract, tolerant validation + WARNs, within-config most-specific precedence) → 4-1..4-6.
- **Permissions & Error Quarantine (TCC)** (typed-result architectural boundary, self-exempt/no-first-run-gate, defensive net `-1743`/`-1712`→permission-required + guidance + burst-stop) → 2-1, 3-7.
- **Observability & State Footprint** (near-zero state, reads `terminals.json`, writes only transient `@portal-spawn-*`, no `sessions.json`/daemon/prefs/restore touch; `spawn` component + closed attr keys + count semantics) → 1-5, 2-6, 3-2, 3-5, 4-6, 6-10.
- **Concurrency & Post-Reboot Safety** (latch removes race, burst gated to post-hydration, abridged attaches don't perturb capture) → context in 1-6, 6-1, 6-3.
- **Testing Strategy & DI Seams** (Adapter fake seam, detection small seams, driver split, mode/keymap state machine, irreducible manual/integration residue) → 2-1, 1-2..1-4, 2-4/2-5, 5-1..5-7, 5-8, 6-11.
- **Design References** (three delivered frames, violet/amber/red no-new-tokens, clean selected-only no dim `○`, visual-gate process) → 5-2, 5-3, 5-4, 5-8, 6-2, 6-7, 6-11.
- **Dependencies, Deferred Scope & Build-Time Residuals** → satisfied dependency noted; build-time residuals (iTerm2/Terminal self-scripting, `ps -o comm=` single-macOS confirm, Ghostty preview-API pin) carried in 1-3/2-4/2-5.

All deferred scope is correctly excluded from the plan (group-select; remember-grouping + Spaces placement; window-arrangement/focus; host-window introspection/window-vs-tab; additional adapter capabilities `introspect`/`place`; truly-headless `portal spawn` + `--terminal`; defensive `@portal-spawn-*` sweep; parallel spawn; detect-and-wait one-bootstrap-cap). The daemon-readable ack-channel forward-compat is recorded as not-built (3-2).

## Direction 2 — Plan → Specification (fidelity, anti-hallucination)

Every task's Problem/Solution/acceptance/tests/edge-cases traces to a cited spec section (each task carries a Context block + Spec Reference). No hallucinated behaviour, edge case, or acceptance criterion was found. The handful of implementation choices the spec does not pin are each explicitly flagged as latitude rather than presented as spec requirements, and each is grounded in a genuine spec need:

- Passthrough identity for a local unknown bundle id (1-1) — required by the spec's copy-paste-the-shown-bundle-id config affordance and the `.app`/raw-bundle-id/`*`-glob config keys.
- Friendly-name derivation algorithm (1-1), `__CFBundleIdentifier` plausibility rule (1-3), per-client transient-walk-error policy (1-4), the Ghostty family glob shape (2-2), the permission-guidance deep-link string (3-7), and the `Opening n/N…` denominator=N choice (6-5) — all flagged as "not pinned by the spec" and consistent with it.
- The picker's `msg.Err != nil` pre-spawn-error flash (6-6) is the faithful picker analogue of the CLI's `return err` on the same `Burster.Run` error path (3-5), surfaced as a flash instead of an exit — completing an error path the spec's own architecture creates.

## Prior-cycle fixes re-verified as holding

- **Copy fidelity (c4 finding).** The count-aware `goneVerb(n)` (`"is"`/`"are"`) is present and consistent: 3-4 renders `spawn: 's2' is gone — nothing opened` / `… 's2', 's4' are gone …`; 6-7 renders `⚠ 'fab-flowx-explore' is gone — nothing opened` (byte-matching the twice-stated design copy) / `⚠ 's2', 's4' are gone — nothing opened`. Both match their own Outcome/Acceptance Criteria.
- **Helper ownership.** `quoteJoin` and `goneVerb` are defined once in `internal/spawn/message.go` by Task 3.4 (its first consumer). Tasks 3.6 and 6.6 reuse `quoteJoin`, and Task 6.7 reuses both — each with an explicit "do NOT re-declare" note — so CLI and picker stay in lockstep with no duplicate-declaration hazard.
- **Config-resolver parity.** The picker reuses the config-aware `spawn.NewResolver(terminals.json).Resolve` (single injection site 6-1; reused by 6-3), identical to the CLI's Task 4.6 resolver — not the zero-config `spawn.ResolveAdapter`.
- **Resolution-based `DetectUnsupported()`.** The unsupported test keys on `detectResolution == ResolutionUnsupported` (true for NULL remote/mosh AND a non-NULL recognised-but-undriven identity like Apple Terminal), not `IsNull()`, consistently across 6-1/6-2/6-3/6-9 — mirroring CLI 2-7. Copy branches on `IsNull()`; the gate branches on resolution.
- **`Burster.Run` ctx/progress migration** (6-3) with the `cmd/spawn.go` + `internal/spawn/burst_test.go` call-site updates to `Run(context.Background(), external, nil)`; **`AckChannelFull`** seam declared in 3-2; **count semantics** `total=N` / `opened` incl. trigger-on-success across 2-6/3-5/6-10.

## Findings

None. No `update-task` / `add-task` / `remove-task` / `add-phase` / `remove-phase` changes are required. The plan is approved-clean against the specification on this final convergence pass.
