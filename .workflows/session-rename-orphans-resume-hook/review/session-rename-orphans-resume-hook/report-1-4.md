TASK: Stamp @portal-id in QuickStart.Run ExecArgs chain (internal ID session-rename-orphans-resume-hook-1-4, tick-959a17)

ACCEPTANCE CRITERIA:
- On success, ExecArgs contain the contiguous subsequence [set-option, -t, <name>, @portal-id, <token>] where <token> is the value the generator returns for the stamp call.
- The @portal-id set-option step is ordered AFTER the @portal-dir set-option step and BEFORE attach-session (stamped while detached).
- Token interpolated as a single literal argv element (not shell-escaped/quoted).
- Token-generation failure omits the step: ExecArgs contain NO @portal-id set-option step, Run returns no error, rest of chain (create -> @portal-dir -> attach) unchanged.
- No new-session -A introduced (detached-create + stamp-before-attach ordering preserved).
- go build succeeds; go test ./internal/session/... passes.

STATUS: Complete

SPEC CONTEXT:
Spec (Fix Overview: Stable Session Identity, line 40) mandates QuickStart.Run add an extra "; set-option -t <name> @portal-id <token>" step to the chained detached-create -> stamp -> attach ExecArgs, alongside the existing @portal-dir step, stamped while detached before attach-session blocks the chain. The token is generated in Go inside Run via the injected id generator BEFORE ExecArgs assembly (no error seam inside the argv chain); a generation failure omits the step (session still created, un-stamped -> name fallback), consistent with best-effort stamping. Generation is fire-and-forget (line 34) — no uniqueness check; that is precisely what lets QuickStart stamp inside an argv chain with no collision-retry seam. Acceptance Criterion 1 (line 162) and Testing Requirements "Creation & persistence" (lines 141-142) confirm both first-party creation paths stamp @portal-id, non-fatal on failure.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/session/quickstart.go:77-110
- Notes:
  - Token generated in Go BEFORE ExecArgs assembly: `idToken, idGenErr := qs.gen()` (line 87), placed after PrepareSession (which already consumed one qs.gen call for the name via GenerateSessionName in prepare.go:44) — this is correctly the SECOND qs.gen call, name-independent.
  - Step assembled conditionally (line 96): `if idGenErr == nil && idToken != ""` — matches the task's defensive "genErr == nil (and defensively token != "")" instruction exactly.
  - Ordering correct: @portal-dir step appended (lines 93-95), then @portal-id step (lines 97-99), then attach-session (lines 101-103). Slots between @portal-dir and attach.
  - Uses the PortalIDOption constant (create.go:29, from Task 1-3) — not a hardcoded literal — for the option name; idToken interpolated as a bare literal argv element (line 98), no shell-escaping (correct — opaque alphanumeric token cannot contain the ";" separator).
  - new-session step (line 89) uses "-d", never "-A". No -A introduced.
  - Generation error path: idGenErr != nil skips the step, Run still returns (nil error) — the error is never propagated (best-effort). Mirrors CreateFromDir (create.go:122-124).
  - Doc-comment (lines 42-76) updated: documented chain shape now includes the @portal-id step (line 51), states the stamp is best-effort/omitted-on-failure/stamped-before-attach (lines 62-70), and explains the second qs.gen call. Thorough and accurate.

TESTS:
- Status: Adequate
- Location: internal/session/quickstart_test.go (package session_test)
- Coverage:
  - "interpolates the @portal-id token as a literal set-option step in the exec chain" (112-133): asserts the [set-option, -t, <name>, @portal-id, abc123] contiguous subsequence via assertContainsSubseq. Covers AC1 + literal-interpolation.
  - "orders the @portal-id stamp before attach-session" (135-156): idIdx of @portal-id < attachIdx. Covers stamp-before-attach half of AC2.
  - "orders the @portal-id stamp after the @portal-dir stamp" (158-177): dirIdx < idIdx. Covers after-@portal-dir half of AC2.
  - "omits the @portal-id step when stamp-time token generation fails" (179-215): call-counting generator (name ok, stamp errors); asserts NO @portal-id subsequence, no Run error, AND full-chain equality via wantExecArgs(...,"") — so it verifies the rest of the chain (create -> @portal-dir -> attach) is byte-identical to today. Covers AC4 comprehensively.
  - "does not use new-session -A" (217-236): indexOf("-A") < 0. Covers AC5.
  - wantExecArgs helper (28-44) updated to interpolate the @portal-id step between @portal-dir and attach when token != ""; every full-chain reflect.DeepEqual case (happy path, shell-cmd, fish-shell, nil-cmd) now threads the token through, so the exact ordering and literal interpolation are re-asserted across the whole existing suite.
  - qs.gen-called-twice is verified indirectly-but-adequately: the omission test's call counter proves two calls (returns "abc123" on call 1 for the name suffix, errors on call 2 for the stamp), and the happy-path DeepEqual cases prove the stamp token equals the generator's return. There is no explicit "calls == 2" assertion in a success case, but the DeepEqual on the interpolated chain would fail if a second call did not occur (token would be empty and the step absent), so behavioural coverage is complete.
- Notes:
  - Not under-tested: every acceptance criterion has a dedicated or covering test; the failure/omission edge case is fully covered including the "chain otherwise unchanged" invariant.
  - Not over-tested: the three separate ordering/interpolation subtests (before-attach, after-dir, literal-subseq) target distinct properties rather than redundantly re-asserting one happy path; the full-chain DeepEqual cases are pre-existing and legitimately re-threaded. No excessive mocking (the generator is a plain closure). No implementation-detail coupling — all assertions are on the observable ExecArgs slice.
  - No t.Parallel() used, correct per the CLAUDE.md package-mutable-state rule (and here the subtests share no mutable state anyway).

CODE QUALITY:
- Project conventions: Followed. Uses PortalIDOption constant (no raw literal at call site); mirrors CreateFromDir's best-effort stamp shape; test naming is behavioural ("it ..." style); no t.Parallel(); mocks via closures/interfaces.
- SOLID principles: Good. Single responsibility preserved; the gen seam is injected (DI), stamp logic is a local conditional with no new coupling.
- Complexity: Low. One added local var + one guarded append; linear, no branching beyond the single best-effort guard.
- Modern idioms: Yes. Idiomatic Go multi-return error handling; conditional append is standard.
- Readability: Good. The added block is preceded by an accurate rationale comment (lines 83-86) explaining the pre-ExecArgs generation and the second-call semantics.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
