TASK: Embed exit-status + trimmed stderr in the three production exec.Cmd boundary sites (portal-observability-layer-4-1)

ACCEPTANCE CRITERIA:
- Non-zero ps/pgrep/git exit → error string contains binary path, argv, underlying exit-status error, trimmed stderr.
- defaultIdentifyPS still returns captured stdout alongside error; IdentifyDaemon still classifies pid-not-found (non-zero + empty stdout) as IdentifyDead, non-zero-with-stdout as transient.
- PgrepPortalDaemons still returns (nil,nil) on pgrep status-1 empty stdout; only non-status-1/OS-layer failure returns stderr-enriched wrapped error.
- ResolveGitRoot still returns (dir, nil) when git rev-parse fails; only RealCommandRunner.Run enriched.
- PATH-lookup failure (*exec.Error) wraps cleanly with empty stderr, recoverable via errors.As.
- No _, _ = cmd.Run() or cmd.Output()-without-stderr-capture remains; errors.As against *exec.ExitError still traverses.

STATUS: Complete

SPEC CONTEXT:
Spec § Diagnostic context preservation at boundaries (731-806) Boundary class 1. Capture stderr, embed exit status + trimmed stderr. Shared helper internal/log.CombinedOutputWithContext permitted at 3+ sites. defaultIdentifyPS named gap-closure site.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/log/exec_context.go:33-47 (CombinedOutputWithContext: bytes.Buffer to cmd.Stderr, cmd.Output(), returns stdout both paths, wraps path+argv+%w+trimmed stderr, empty-stderr omits clause, stdlib-only no internal/state import). Site 1 daemon_identity.go:66-76; Site 2 pgrep.go:62-103 (status-1-no-matches branch FIRST/unchanged, only fallthrough gains stderr); Site 3 gitroot.go:28-34 (ResolveGitRoot swallow untouched).
- Notes: Preferred helper path (3-site threshold). No hard-coded exit codes (signal-killed via *exec.ExitError String(), *exec.Error wraps empty stderr). No log emission added (error wrapping only).

TESTS:
- Status: Adequate
- Coverage: helper exec_context_test.go (non-zero embeds argv+stderr+recoverable ExitError; stdout on error path; happy; empty-stderr clean; PATH-lookup *exec.Error clean + errors.As); daemon_identity_test.go (IdentifyDead empty-stdout incl real-ps; transient non-empty stdout; preserves underlying via errors.Is); pgrep_test.go ((nil,nil) status-1; status-2 stderr-enriched + ExitError recovery; real-pgrep no-match); resolver tests (Run enriched, ResolveGitRoot dir-unchanged).
- Notes: 1:1 with named tests. t.Cleanup restore, no t.Parallel. Behaviour-focused. Helper+site overlap intentional (distinct contracts).

CODE QUALITY:
- Project conventions: Followed (seam DI; stdlib-only leaf helper respecting cycle guard).
- SOLID: Good — single-responsibility helper, DI via *exec.Cmd.
- Complexity: Low.
- Modern idioms: Yes (%w, errors.As, bytes.Buffer, TrimSpace).
- Readability: Good — "Boundary class 1" comments; helper doc documents stdout-on-error contract + cycle guard.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tmux/tmux.go:82 RealCommander is a separate cmd.Output() site governed by Boundary class 2 (out of scope here, covered by 4-2); relies on *exec.ExitError.Stderr auto-population.
