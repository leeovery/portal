TASK: cli-verb-surface-redesign-3-7 — Command rides mint windows only, byte-identical (+ trigger local-mint parity + multi-target zero-mint usage error)

ACCEPTANCE CRITERIA (from the plan table):
- Command (-e/--) appended after `--ack` on MINT windows only; attach windows never carry it.
- Mixed set attaches bare + mints run the command.
- Byte-identical, no word-splitting (`-e "npm run dev"` is one unit) so local + spawned mints run identical commands.
- Trigger that is a command-carrying mint mints locally via CreateFromDir/QuickStart.
- Multi-target zero-mint + command (all-attach) → usage error.
- `-e`/`--` exclusivity + empty-command usage errors preserved from Phase 2.

STATUS: Complete

SPEC CONTEXT:
Spec §123-139 (Command passthrough — mint-scoped) and §196-206 (Burst exec-argv & mint responsibility, point 3 + "Command parity — no word-splitting"), plus §221 (zero-mint-command is the command's only error case). The command targets mint surfaces only because an existing session has no safe injection channel (send-keys corrupts a busy pane; respawn-pane -k destroys running work) — a safety floor, not a chosen restriction. The command must reach every mint surface *as authored*: a single `-e "npm run dev"` string stays one unit so the trigger's local mint and every spawned mint window run byte-identical commands. Zero mint targets + a command is a usage error (erroring beats silently dropping the command).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/command.go:47-66 (composeOpenArgv) — appends `--` + command… AFTER `--ack`, gated on `surface.Kind == SurfaceMint && len(command) > 0`. Attach surfaces and empty commands append nothing (no dangling bare `--`). command is appended verbatim via `append(argv, command...)` — no join/split/quote, so a single-element slice stays one argv element.
  - internal/spawn/burst.go:171 — Burster.Run threads the same `command` slice into composeOpenArgv for every external surface, so mint externals carry it and attach externals ignore it (per-surface, single source).
  - cmd/open_burst_run.go:150-152 (runOpenBurstWithDeps) — multi-target zero-mint guard: `if len(command) > 0 && !hasMintSurface(surfaces)` returns NewUsageError(commandAttachOnlyMessage) BEFORE detect/resolve/spawn/self-connect, so nothing opens.
  - cmd/open_burst_run.go:234-244 (hasMintSurface) — small helper scanning all surfaces (incl. the trigger).
  - cmd/open_burst_run.go:253-258 (connectTrigger) — a mint trigger routes to deps.LocalMint(cmd, dir, command); an attach trigger routes to deps.Connector.Connect (no command). LocalMint defaults to openPathFunc (open_burst_run.go:117-121).
  - cmd/open.go:582-610 (openPath → PathOpener.Open) → CreateFromDir(dir, command) / QuickStart.Run(path, command) (cmd/open.go:554-568) — the trigger's local mint feeds the command to the same creation path a spawned mint window's `open --path … -- <cmd>` re-enters, giving byte-identical parity.
  - cmd/open_burst.go:100 (commandAttachOnlyMessage) — the SOLE authoring site for the attach-command wording, shared by the single-target guard (openResolved, open.go:348-349) and the multi-target zero-mint guard, so the two arities cannot drift.
  - cmd/open.go:464-495 (parseCommandArgs) — Phase-2 `-e`/`--` mutual-exclusivity + empty-command usage errors, unchanged; runs in openCmd.RunE before dispatch so the multi-target path inherits them.
- Notes: Guard ordering is correct — the zero-mint refusal precedes SplitTriggerFirst/detect/resolve, so an all-attach+command set opens nothing. hasMintSurface includes the trigger, so a mint-trigger + all-attach-externals set correctly passes the guard and threads the command to the local mint only. No aliasing hazard: composeOpenArgv appends into a freshly-allocated literal argv, never mutating the caller's command slice. Byte-identical parity is structural: both the local mint (single-element []string{execFlag}) and the spawned mint (`-- "npm run dev"` re-parsed by cobra into the same single element) feed CreateFromDir/QuickStart the identical slice.

TESTS:
- Status: Adequate
- Coverage:
  - internal/spawn/command_test.go:150-163 — mint + command appends `-- claude --resume` after `--ack` (whole-argv equality).
  - internal/spawn/command_test.go:165-181 — attach surface never carries the command even when one is passed (+ belt-and-braces: no `--` on an attach argv).
  - internal/spawn/command_test.go:183-202 — empty command (nil AND {}) appends no bare `--`.
  - internal/spawn/command_test.go:204-228 — single-string command stays ONE argv element after `--` (no word-splitting), with an explicit post-`--` slice assertion.
  - cmd/open_burst_run_test.go:299-337 (TriggerMint_RoutesToLocalMint) — mint trigger self-connects via LocalMint with the command threaded verbatim (single unit); Connector not called; external attach still spawned.
  - cmd/open_burst_run_test.go:339-393 (Command_RidesMintExternalsOnly) — mixed external set: external mint window carries `-- claude`, external attach window does not, attach trigger connects bare.
  - cmd/open_burst_run_test.go:395-441 (AllAttachWithCommand_UsageError) — multi-target all-attach + command → *UsageError with the exact Task 2-6 message; NewBurster never built, no OpenWindow/Connect/LocalMint calls (nothing opens).
  - `-e`/`--` exclusivity + empty-command usage errors are preserved-from-Phase-2 (parseCommandArgs) and covered by the existing Phase-2 open command tests — correctly not re-duplicated here.
- Notes: Tests would fail if the feature broke (append-to-attach, word-split, missing zero-mint refusal, dropped local-mint command each map to a distinct failing assertion). Not over-tested: the composeOpenArgv subtests each pin a distinct behaviour (argv shape, strip, spacing, exe path, ack pair, command tail), and the burst-body tests use focused fakes rather than redundant happy-path variations. Byte-identical local-vs-spawned parity is proven across the two paths (LocalMint gets the verbatim slice; the external mint argv's post-`--` equals it) rather than by one combined test — acceptable and non-redundant.

CODE QUALITY:
- Project conventions: Followed. Small DI seams (LocalMint/Connector/NewBurster/Ack) with production defaults + t.Cleanup-free package-level override pattern; no t.Parallel; pure builder (composeOpenArgv) isolates OS specifics; single-sourced user-facing message.
- SOLID principles: Good. composeOpenArgv and hasMintSurface are single-responsibility; connectTrigger cleanly branches mint-vs-attach; the shared message constant keeps the single/multi guards from diverging (open/closed).
- Complexity: Low. Guard is a two-clause conditional; the argv builder is a linear append.
- Modern idioms: Yes (slices-based tests, variadic append, nil-tolerant command handling).
- Readability: Good. Doc comments on composeOpenArgv, the burst body, and connectTrigger explain the load-bearing ordering and the mint-only rationale precisely.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
