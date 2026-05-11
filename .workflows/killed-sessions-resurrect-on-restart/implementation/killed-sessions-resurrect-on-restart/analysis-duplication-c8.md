# Duplication Findings — killed-sessions-resurrect-on-restart (cycle 8)

```
AGENT: duplication
FINDINGS:
- FINDING: runHydrate now contains three near-identical handler-and-exec recovery blocks after T2-3 added the timeout fall-through site; the two file-missing sites have been a byte-identical 5-line copy since built-in-session-resurrection T4-2, and T2-3 grew the pattern to three sites without unification
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/state_hydrate.go:103-117, /Users/leeovery/Code/portal/cmd/state_hydrate.go:140-149, /Users/leeovery/Code/portal/cmd/state_hydrate.go:159-167
  DESCRIPTION: runHydrate has three terminal recovery branches that all share the identical "invoke handler → propagate handler error → exec shell-or-hook → return nil" shape:

    Timeout branch (lines 103-117, T2-3 / killed-sessions-resurrect-on-restart):
        if cfg.HandleTimeout != nil {
            if err := cfg.HandleTimeout(cfg); err != nil { return err }
            time.Sleep(hydrateSettleSleep)
            execShellOrHookAndExit(cfg)
            return nil
        }

    File-missing on os.Open (lines 140-149, built-in-session-resurrection T4-2):
        if cfg.HandleFileMissing != nil {
            if hErr := cfg.HandleFileMissing(cfg, hydrateFileMissingContext{Cause: err}); hErr != nil { return hErr }
            execShellOrHookAndExit(cfg)
            return nil
        }

    File-missing mid-Copy (lines 159-167, built-in-session-resurrection T4-2):
        if cfg.HandleFileMissing != nil {
            if hErr := cfg.HandleFileMissing(cfg, hydrateFileMissingContext{Cause: err}); hErr != nil { return hErr }
            execShellOrHookAndExit(cfg)
            return nil
        }

  The two file-missing sites are byte-identical. The timeout site is structurally near-identical: only the handler signature and an inserted `time.Sleep` differ. The pattern was 2-of-a-kind before this work unit; T2-3 added the third occurrence.

  Counter-argument worth recording: runHydrate's body is sequentially ordered around the canonical helper protocol. Inlining each recovery branch at the point where the failure surfaces keeps the linear-read happy-path crisp. The timeout branch should stay open-coded — its inserted `time.Sleep(hydrateSettleSleep)` makes it structurally distinct enough that forcing a shared helper would require a boolean settle parameter (anti-pattern per code-quality.md "Boolean parameters") or a closure-typed sleep dependency.

  RECOMMENDATION: Extract `handleFileMissingAndExec(cfg hydrateConfig, cause error) error` owning the "nil-handler fallback → handler call → terminal exec → return nil" shape. Migrate the two file-missing call sites into single-line dispatches. Leave the timeout branch open-coded. Net deletion ~10 lines; future-edit blast radius for the file-missing exec contract drops from 2 sites to 1.

SUMMARY: Cycle 7's two cleanups landed cleanly. One new low-severity duplication candidate remains at cycle 8 scope: T2-3 added a third near-identical "if cfg.HandleX != nil { call; execShellOrHookAndExit; return nil }" block to runHydrate, perpetuating a pre-existing byte-identical 2-site file-missing copy. The two file-missing sites are extractable into a single helper; the timeout site stays open-coded because of its inserted settle-sleep. Mechanical extract-and-reuse cleanup; does not block correctness. The cycle 7 "bootstrapEagerHydrateScenario" candidate remains explicitly discarded and is not re-raised. Pre-existing inline OpenLogger preambles in cmd/state_hydrate_test.go (7 sites), internal/restore/session_test.go (4 sites), and internal/restore/integration_full_test.go (1 site) are deliberately not flagged: 11 of those 12 sites pre-date this work unit.
```
