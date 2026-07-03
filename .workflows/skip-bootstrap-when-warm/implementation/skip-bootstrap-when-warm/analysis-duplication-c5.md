AGENT: duplication
FINDINGS: none
SUMMARY: No genuinely-new, actionable duplication detected in cycle 5. The feature's
  production code reuses shared primitives cleanly: the daemon's new hooks-cleanup home
  (maybeRunHookCleanup) and `portal clean`'s cleanStaleHooks both drive the single
  runHookStaleCleanup helper with pinned args; the abridged ensureSaverLiveness and the
  daemon's defaultSaverMembershipProbe both compose the shared tmux.SaverPanePIDOrAbsent
  primitive (for genuinely different purposes — liveness-revive vs self-supervision, not
  parallel copies); the new @portal-bootstrapped latch read reuses the
  RestoringChecker/TryGetServerOption seam alongside @portal-restoring; the latch write
  reuses the LatchWriter seam (*tmux.Client.SetServerOption); and loading_progress.go keeps
  the 10-step mapping a single-source stepLabelTable/totalBootstrapSteps. The new/changed
  test files are well-factored around shared fixtures (makeDeps, daemonFakeCommander,
  hookCleanupDeps, newTempHooksStore/keysOf, the satisfiedLatch*/saverAbsentReviveFails
  commander fixtures, the collapsed table-driven not-satisfied latch test, and the
  consolidated cleanstale transient-listpanes scaffolding). markers.go's
  BootstrappedLatchSatisfied vs IsRestoringSet share the TryGet-then-found shape but are a
  deliberate, in-source-documented design split (error-swallow+value-equality vs
  error-propagate+presence) at only two instances — below Rule of Three and load-bearing, not
  a consolidation candidate. Prior-cycle items are not re-reported: the PersistentPreRunE
  abridged/sync context-injection + warning-drain epilogue two-site parallel (cmd/root.go) is
  formally closed as acceptable below-Rule-of-Three; the abridged_saver.go vs orchestrator
  step-5 saver-down WARN+SaverDownWarning parallel, the setupAbridgedEnv ≡
  setupConcurrentColdBootEnv env-builder copy, and the openTUIFunc override-capture test idiom
  are each documented in-source as deliberate two-instance copies. No new extraction candidate
  crossed the proportionality/Rule-of-Three bar (state.Dir()/EnsureDir one-liners and the
  slog discard-logger one-liner are sub-threshold and out of feature scope).
