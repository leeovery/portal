AGENT: duplication
FINDINGS:
- FINDING: Production spawn-seam defaults constructed twice (CLI vs picker)
  SEVERITY: medium
  FILES: cmd/spawn.go:261, cmd/spawn.go:278, cmd/spawn.go:284, cmd/spawn.go:286-290, cmd/spawn.go:293-294, cmd/spawn.go:305, cmd/open.go:592-604
  DESCRIPTION: The identical set of production spawn dependencies is wired from the
    shared *tmux.Client in two independent places — cmd/spawn.go's buildSpawnDeps (the
    CLI path) and cmd/open.go's openConfig population (the picker path). Both construct,
    line-for-line: Detector = spawn.NewDetector(client); Resolve = buildResolver().Resolve;
    Ack = spawn.NewServerOptionAckChannel(client, client); Exe = os.Executable;
    Getenv = os.Getenv; Exists = client.HasSession; Logger = log.For("spawn"). These are
    seven identical default constructions authored in two task-separated call sites. The
    cmd/open.go comment even states the picker seams "mirror the spawn CLI's SpawnDeps",
    acknowledging the parallel. This is precisely copy-paste across a task boundary (the CLI
    command task vs the picker-burst wiring task): if the ack-channel constructor changes, a
    new seam is added, or a default swaps (e.g. a different logger component), both sites must
    be edited in lockstep or the CLI and picker silently diverge in how they open windows —
    the exact drift this analysis targets. It is not caught by the compiler because SpawnDeps
    and openConfig are distinct struct shapes.
  RECOMMENDATION: Extract a single cmd-package helper that returns the shared production spawn
    seams from a *tmux.Client — e.g. a small struct { Detector, Resolve, Ack, Exe, Getenv,
    Exists, Logger } built once by a func like buildProductionSpawnSeams(client) — and have
    both buildSpawnDeps' nil-defaulting and cmd/open.go's openConfig population read from it.
    buildResolver() and buildSessionConnector() already demonstrate this shared-helper pattern
    in the same package; this closes the remaining un-shared subset so the CLI and picker
    provably wire the same adapters.

- FINDING: Repeated multi-select suppression guard across row-action dispatch arms
  SEVERITY: low
  FILES: internal/tui/model.go:3384, internal/tui/model.go:3392, internal/tui/model.go:3402, internal/tui/model.go:3425
  DESCRIPTION: The k (kill), r (rename), n (new-in-cwd), and x (page-toggle) dispatch arms in
    updateSessionList each open with the byte-identical guard `if m.multiSelectMode { return m,
    nil }` to no-op the row action while in multi-select mode. Four copies of the same
    two-line guard. It is minor (single-statement guards, not large blocks) and each arm
    carries a distinct explanatory comment about why that specific key is suppressed, plus the
    arms are deliberately kept present for keymap_dispatch_guard_test.go's default-mode parity
    probe — all legitimate reasons the current shape exists.
  RECOMMENDATION: Optional and low-value. If consolidated, a single predicate (e.g.
    `m.suppressedRowActionInMultiSelect()`) could front the four arms, but doing so would
    displace the per-key rationale comments that document the behaviour. Acceptable to leave as
    is; flagged only for completeness. Prefer leaving it unless a fifth suppressed row action
    is added, at which point the shared predicate earns its keep.
SUMMARY: Only one actionable duplication remains — the production spawn-seam defaults are
  wired identically but independently in the CLI (cmd/spawn.go) and the picker (cmd/open.go),
  a real cross-task copy-paste-drift risk worth consolidating behind a shared cmd helper. The
  spawn/burst core itself is already heavily de-duplicated by prior cycles (shared exec-boundary,
  message, classify, logemit, footer-fitter, and left-bar renderers), leaving only one low
  cosmetic repeat in the TUI key dispatch.
