AGENT: duplication
FINDINGS:
- FINDING: Abridged branch copy-pastes the synchronous branch's context-injection + CLI warning-drain tail
  SEVERITY: medium
  FILES: cmd/root.go:186-196, cmd/root.go:239-250
  DESCRIPTION: Task 2-3 added the latch-satisfied abridged branch by copying the tail of the
    existing synchronous full-bootstrap branch rather than sharing it. Both branches end with the
    same two units, near-verbatim:
      (a) CLI-path warning drain — `if !isTUIPath(cmd, args) { bootstrapWarnings.EmitTo(cmd.ErrOrStderr()) }`
          (abridged: lines 189-191; sync: lines 242-244), and
      (b) the context-injection triple — `ctx := context.WithValue(cmd.Context(), serverStartedKey, X);
          ctx = context.WithValue(ctx, tmuxClientKey, client); cmd.SetContext(ctx)`
          (abridged: lines 192-194 with serverStarted=false and an unconditional client set;
          sync: lines 246-250 with serverStarted=started and an `if client != nil` guard).
    The spec explicitly frames the abridged path as "reuses the SAME entry-path plumbing (warning sink +
    context injection) as the synchronous full path" — but the implementation duplicates that plumbing
    inline instead of factoring it. This is load-bearing control flow: the serverStartedKey/tmuxClientKey
    context keys feed downstream commands (openTUI's loading-page gate, tmuxClient(cmd)) and the
    warning-routing decides CLI stderr vs TUI notice-band. The two copies can silently drift (e.g. a future
    change to the context keys or the CLI-vs-TUI drain rule applied to one branch only), which is exactly
    the copy-paste-across-task-boundary risk the spec's "same plumbing" wording was meant to prevent.
  RECOMMENDATION: Extract the shared tail into two small package-cmd helpers reused by both branches —
    e.g. `injectBootstrapContext(cmd *cobra.Command, client *tmux.Client, serverStarted bool)` (folding the
    nil-client guard so both call sites are identical) and `drainCLIWarnings(cmd, args)` wrapping the
    `!isTUIPath → bootstrapWarnings.EmitTo` drain. The abridged branch then becomes
    `ensureSaverLiveness(...); drainCLIWarnings(cmd, args); injectBootstrapContext(cmd, client, false); return nil`,
    making "abridged reuses the sync plumbing" structurally true rather than a comment. No behaviour change.

- FINDING: Saver-revive-failure handling (WARN + SaverDownWarning) duplicated across the abridged helper and orchestrator step 5
  SEVERITY: low
  FILES: cmd/abridged_saver.go:55-58, cmd/bootstrap/bootstrap.go:378-382
  DESCRIPTION: Both the abridged liveness helper and full-bootstrap step 5 handle a failed saver
    ensure/revive with the same two-action shape: emit a bootstrap-component WARN carrying the underlying
    error, then surface a `bootstrap.SaverDownWarning()`. abridged_saver.go does
    `bootstrapLogger.Warn("abridged EnsureSaver: saver revive failed", "error", err);
    bootstrapWarnings.Add(bootstrap.SaverDownWarning())`; bootstrap.go step 5 does
    `warnings = append(warnings, SaverDownWarning()); o.Logger.Warn("step failed", "step", stepEnsureSaver, "error", err)`.
    ensureSaverLiveness's own godoc notes it is "mirroring the full-bootstrap step-5 'step failed' breadcrumb"
    — an acknowledged parallel written independently on the two paths. This is genuinely a deliberate design
    split (the spec routes the two paths through different warning sinks: the package-level bootstrapWarnings
    sink vs the orchestrator's returned warnings slice, later funnelled into the same sink), and each block is
    only ~3 lines, so full extraction is not clearly warranted (Rule of Three not met; proportionality). Flagged
    so the parallel is visible: the "on saver-down, WARN-then-SaverDownWarning" contract now lives in two
    places and could drift (e.g. a message-format or severity change applied to one only).
  RECOMMENDATION: Leave as-is unless a third saver-down site appears, given the intentional sink split and the
    small block size. If consolidated later, a shared `noteSaverDown(logger, sink-or-slice, cause)` helper in
    package cmd/bootstrap could pin the WARN-then-SaverDownWarning shape while letting each caller pass its own
    sink; do NOT merge the two error-handling paths themselves (the version-gate vs liveness-only split is
    load-bearing per spec).
SUMMARY: The feature is largely clean on duplication — executors reused shared primitives well
  (runHookStaleCleanup for the daemon's new hooks-cleanup home, SaverPanePIDOrAbsent for both probes, the
  RestoringChecker/TryGetServerOption seam for the new latch read, and the header-style consolidation for the
  loading screen). The one substantive item is the abridged branch in PersistentPreRunE copy-pasting the
  synchronous branch's context-injection + warning-drain tail (medium); a minor low-severity parallel exists in
  saver-revive-failure handling across the abridged helper and orchestrator step 5. Note: the setupAbridgedEnv /
  setupConcurrentColdBootEnv integration-harness mirroring and the clean.go pre-Load duplicate are both
  explicitly acknowledged in-source as deliberate two-instance copies (Rule of Three not reached) and are not
  flagged.
