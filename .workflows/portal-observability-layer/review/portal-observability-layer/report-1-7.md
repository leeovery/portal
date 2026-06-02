TASK: Adopt the main exit shape: single os.Exit, panic recovery, Close on non-panic path (portal-observability-layer-1-7)

ACCEPTANCE CRITERIA:
- Clean Execute() → code=0, log.Close(0) once, os.Exit(0).
- Execute error: UsageError→2; ordinary→1; bootstrap.FatalError→1 no duplicated stderr; IsSilentExitError→1 nothing printed.
- Panic recovered → code=2, panicked=true, Close NOT called, os.Exit(2).
- log.Init called before cmd.Execute().
- os.Exit appears exactly once in main (final statement).
- Existing stderr output for those paths unchanged.

STATUS: Complete

SPEC CONTEXT:
Spec § Defensive invariants → main exit shape (518-561). main owns the single os.Exit; run Execute inside recover-closure; Close gated behind !panicked so exactly one terminal marker fires (exit on return, panic on recovered panic). Bare os.Exit prohibited outside main.

IMPLEMENTATION:
- Status: Implemented (matured beyond Phase-1 snapshot — Phase-2 panic emission now present at main.go:61, correct convergence)
- Location: main.go:25-110 (main, run, classify); cmd.Version() cmd/version.go:17; log.ResolveProcessRole; log.Close init.go:151; cmd.IsSilentExitError state_commit_now.go:31.
- Notes: Exit shape matches spec template. Init resolves stateDir/processRole/version before run(); run() wraps executeFunc() in recover-closure (code=2/panicked=true); Close gated behind !panicked; os.Exit single final statement. classify() ports original ordering verbatim (FatalError→1 no stderr, IsSilentExitError suppression, UsageError→2, else 1). Grep confirms no production os.Exit outside main.go:44 except daemon self-eject + hydrate exec-failure, both via cmd-package osExit seam (left untouched per task).

TESTS:
- Status: Adequate
- Coverage: main_test.go covers all six cases via run() helper + executeFunc/errOut seams (clean→0 no stderr; ordinary→1 "boom"; UsageError→2; FatalError→1 empty stderr; IsSilentExitError→1 empty; panic→2 panicked=true). main_panic_test.go adds marker assertions (exactly one ERROR process: panic reason="kaboom"; Close skipped on panic; table-driven mutual-exclusivity proving exactly one terminal marker per run).
- Notes: Behaviour-focused (codes, stderr bytes, marker counts/levels/attrs). Mutual-exclusivity test guards the "exactly one terminal marker" invariant. Panic-path Close-skip asserted via mainEmitClose model (avoids real os.Exit) — sound, well-commented.

CODE QUALITY:
- Project conventions: Followed (package-level mutable-seam DI mirroring cmd *Deps; t.Cleanup restore; cmd.Version() accessor).
- SOLID: Good — run() owns no control flow; main owns single os.Exit; classify() pure error→code mapper.
- Complexity: Low.
- Modern idioms: Yes — errors.As typed discrimination, errors.Is-backed IsSilentExitError, io.Writer stderr seam.
- Readability: Good — comments explain defer-skip rationale, single-marker invariant, classification ordering.
- Issues: None. `_, _ = fmt.Fprintln(errOut, err)` deliberate ignore justified in-comment.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] classify() prints to stderr before returning the code, mixing a side effect into an otherwise-pure mapper. Faithfully preserves original ordering (brief said port verbatim); if a future task wants classify() pure, the stderr write could move to run()/main.
