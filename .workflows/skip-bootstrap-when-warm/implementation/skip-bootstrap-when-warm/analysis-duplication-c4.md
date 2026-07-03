AGENT: duplication
FINDINGS:
- FINDING: PersistentPreRunE abridged branch still open-codes the synchronous branch's context-injection + CLI warning-drain epilogue (persistent carryover, cycles 1-3 unaddressed)
  SEVERITY: low
  FILES: cmd/root.go:186-196, cmd/root.go:239-250
  DESCRIPTION: The latch-satisfied abridged branch and the synchronous full-bootstrap tail each re-inline the
    same two-part entry-path epilogue instead of sharing it:
      (a) CLI-path warning drain — `if !isTUIPath(cmd, args) { bootstrapWarnings.EmitTo(cmd.ErrOrStderr()) }`
          (abridged: 189-191; sync: 242-244), and
      (b) the context-injection triple — `ctx := context.WithValue(cmd.Context(), serverStartedKey, X);
          ctx = context.WithValue(ctx, tmuxClientKey, client); cmd.SetContext(ctx)`
          (abridged: 192-194 with serverStarted=false + unconditional client set; sync: 246-250 with
          serverStarted=`started` + an `if client != nil` guard).
    The spec explicitly frames the abridged path as reusing "the SAME entry-path plumbing (warning sink +
    context injection) as the synchronous full path," yet the implementation duplicates that plumbing inline
    in both places. This is load-bearing control flow: serverStartedKey/tmuxClientKey feed downstream
    consumers (openTUI's loading-page gate, tmuxClient(cmd)) and the drain gate decides CLI stderr vs TUI
    notice-band. The two copies must be kept in hand-lockstep; a future change to the context keys or the
    CLI-vs-TUI drain rule applied to one branch and not the other would silently diverge the abridged and
    synchronous entry paths. This exact item was raised in cycles 1, 2, and 3 (c3 at medium); phase 6 shipped
    only the comment fix (T6-1, stale step-11 references) and did not touch it, so it persists verbatim.
  RECOMMENDATION: Either action it or formally accept/close it — it should not drift through further cycles
    unresolved. If actioning: extract two small package-cmd helpers reused by both branches —
    `injectBootstrapContext(cmd *cobra.Command, client *tmux.Client, serverStarted bool)` (folding the
    nil-client guard so both call sites are identical) and `drainCLIWarnings(cmd, args)` wrapping the
    `!isTUIPath → bootstrapWarnings.EmitTo` drain — so the abridged branch becomes
    `ensureSaverLiveness(...); drainCLIWarnings(cmd, args); injectBootstrapContext(cmd, client, false); return nil`,
    making "abridged reuses the sync plumbing" structurally true rather than a prose promise. No behaviour
    change. Note the countervailing consideration honestly: this is only TWO instances (~5 lines each),
    below the project's Rule of Three (code-quality.md: "Avoid premature abstraction for code used once or
    twice"), so formally accepting it as-is is also defensible — the value is that it sits on the critical
    bootstrap path where the spec promised a shared helper. Whichever way, close the loop.
SUMMARY: The feature is largely clean on duplication — executors reused shared primitives well (the daemon's
  new hooks-cleanup home reuses runHookStaleCleanup verbatim with pinned args; both saver paths reuse
  SaverPanePIDOrAbsent; the latch read reuses the RestoringChecker/TryGetServerOption seam alongside
  @portal-restoring; the loading-progress mapping stays a single-source stepLabelTable; the new daemon
  hooks-cleanup tests share makeDeps/hookCleanupDeps/newTempHooksStore fixtures). The single genuinely-open
  production item is the PersistentPreRunE context-injection + warning-drain epilogue duplicated across the
  abridged and synchronous branches (low; a spec-promised shared helper that was never created, unaddressed
  since cycle 1). The historically-flagged low items — setupAbridgedEnv≡setupConcurrentColdBootEnv, the
  saver-down WARN+SaverDownWarning parallel across abridged_saver.go and orchestrator step 5, and the
  openTUIFunc override-and-capture closure across the routing tests — are each documented in-source as
  deliberate below-Rule-of-Three two-instance copies (or, for openTUIFunc, a package-wide pre-existing test
  idiom); they are not re-flagged as fresh findings.
