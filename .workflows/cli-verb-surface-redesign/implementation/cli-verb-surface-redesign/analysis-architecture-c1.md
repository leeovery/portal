AGENT: architecture
FINDINGS:
- FINDING: Ordered-target argv scan maintains a second, unlinked source of truth for open's value-taking flags
  SEVERITY: medium
  FILES: cmd/open_targets.go:20-28, cmd/open_targets.go:56-83, cmd/open.go:982-990
  DESCRIPTION: The multi-target ordering recovery (orderedOpenTargets) is the seam that
    recovers true left-to-right target order from the raw argv because cobra collapses it.
    To do so it classifies each flag token against a hand-maintained static map
    (openTargetPins) that must exhaustively enumerate every value-taking open flag in both
    its short and long form. That map is structurally decoupled from the actual cobra flag
    definitions registered in openCmd's init(): orderedOpenTargets is handed only []string
    and never the *cobra.Command, so it cannot consult the real flag set. Its documented
    fallback for an unrecognised flag token is "skip WITHOUT consuming a following value"
    on the assumption that an absent flag is a boolean (open_targets.go:64-70, and the
    comment at lines 16-19 encodes exactly this assumption). A future value-taking flag
    added to openCmd but not mirrored into openTargetPins silently breaks that assumption:
    cobra parses `open --newflag val ~/code` correctly, but the raw scan treats --newflag
    as arity-zero and then classifies its VALUE ("val") as a bare positional target. That
    value is then run through the guessing chain (resolved / minted / attached) and can
    even flip a single-target invocation into a multi-target burst. Nothing — no
    compile-time link, no guard test — keeps the two representations of the flag set in
    sync. This is a latent misrouting seam created by the raw-argv-ordering mechanism the
    redesign introduced.
  RECOMMENDATION: Collapse the dual source of truth. Either thread the *cobra.Command into
    orderedOpenTargets and derive each token's arity from cmd.Flags().Lookup /
    ShorthandLookup (a pflag takes a value unless it is a bool / has a NoOptDefVal), so the
    scan and cobra classify by one flag set; or add a guard test that VisitAll's openCmd's
    flags and asserts every non-bool flag's -short and --long form is present in
    openTargetPins — the same descriptor↔dispatch drift guard already used for the TUI
    keymap (keymap_dispatch_guard_test.go).

- FINDING: internal/spawn burst library still documents the deleted `portal spawn` CLI as a live co-caller; two split helpers are now single-caller
  SEVERITY: low
  FILES: internal/spawn/split.go:3-37, internal/spawn/command.go:68-79, internal/tui/burst_progress.go:250,405,426,484-488, cmd/open_burst_run.go:194-213
  DESCRIPTION: internal/spawn was designed as a burst library shared by three consumers —
    the `portal spawn` CLI (cmd/spawn.go), the picker multi-select burst (internal/tui),
    and the new open multi-target burst (cmd/open_burst*). The redesign deleted cmd/spawn.go
    (correctly — `spawn` folded into `open`), but roughly fifteen doc comments across
    internal/spawn, internal/tui and cmd still cite runSpawn / cmd/spawn.go / buildSpawnDeps
    / SpawnDeps as the live drift-prevention anchor (e.g. burst_progress.go:250 "the single
    computation the CLI (runSpawn) also uses, so the two paths cannot drift"; split.go:7;
    open_burst_run.go:194 "DIVERGENCE FROM runSpawn (cmd/spawn.go)"). That invariant is now
    false and the anchors send a maintainer to a nonexistent file. The substantive effect,
    not just the stale text: spawn.SplitNetN (trailing-trigger) and spawn.AttachSurfaces now
    have exactly one production caller each — the picker — while spawn.SplitTriggerFirst
    (leading-trigger) has one caller, the open burst. So the "shared so it cannot drift"
    justification for the SplitNetN half has evaporated, leaving two near-mirror
    single-caller split helpers with opposite trigger conventions and no second caller to
    protect. The genuinely two-way-shared pieces (PartitionResults, and the classify /
    message / preflight / logemit renderers, all still used by BOTH surviving bursts) keep
    earning their library home; the concern is that the documentation now misdescribes the
    seam and hides which exports went single-caller.
  RECOMMENDATION: Re-point the stale anchors at the two surviving consumers (picker burst +
    open burst) so the "single source, cannot drift" claims stay truthful, and re-confirm
    that SplitNetN / AttachSurfaces still warrant a shared-library home now that only the
    picker uses them — either annotate the sole consumer or inline them into the picker.
    This also makes the deliberate leading-vs-trailing trigger split legible rather than
    justified by a deleted file.
SUMMARY: The redesign's resolve→surfaces→burst→connect flow composes cleanly and its DI
  seams (productionSpawnSeams single-source bundle, read-only resolveOpenSurfaces engine,
  doctor check catalog) are sound; the two concerns are a genuinely latent one — the
  ordered-target scan keeps a second, unlinked copy of open's flag set that will misroute a
  future value-taking flag's value into a target — and a lower one: the deleted `portal
  spawn` CLI is still cited across internal/spawn as a live drift-prevention co-caller,
  leaving two split helpers single-caller with an invalid shared-source rationale.
