TASK: 1-3 — Add session.PortalIDOption constant and stamp @portal-id in CreateFromDir (tick-c705f2)

ACCEPTANCE CRITERIA:
- session.PortalIDOption == "@portal-id", exported/importable by all stamp sites (byte-identical to the literal in tmux.HookKeyFormat).
- On success, CreateFromDir calls SetSessionOption(sessionName, "@portal-id", <token>) with token from sc.gen's stamp call, targeting the created session.
- @portal-id stamp emitted alongside (not instead of) @portal-dir — both SetSessionOption calls made on success.
- Token-generation error swallowed: generator error at stamp -> no @portal-id call, still returns name, no error (un-stamped).
- SetSessionOption error swallowed: stamp error -> still returns name, no error (not aborted).
- Best-effort/non-fatal: no path makes @portal-id stamping fail creation.
- go build succeeds; go test ./internal/session/... passes.

STATUS: Complete

SPEC CONTEXT:
Fix Overview: Stable Session Identity (@portal-id). A rename-immune session user-option stamped at creation, carried on the session object (not its name), mirroring @portal-dir. Spec explicitly scopes CreateFromDir to a best-effort SetSessionOption(name, PortalIDOption, <token>) immediately after NewSession, token via sc.gen (same generator as names), generated fire-and-forget with NO uniqueness check; both a token-generation error and a SetSessionOption error are swallowed (no log component), leaving the session un-stamped → name fallback, never aborting creation. Acceptance Criterion 1 (Stamping) covers this task's half. Token width is an implementation detail (spec says NewNanoIDGenerator scheme, "widened if warranted") — task DECISION reuses sc.gen at 6 chars. Capture/restore/persistence and the rename-gap/legacy integration coverage belong to later phases and are correctly out of scope here.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/session/create.go:16-29 — PortalIDOption const with a thorough doc-comment covering immutability, rename-immune identity, resume-hook keying, the divergence from @portal-dir's lazy re-derivation (Phase 3 re-stamp note), and the byte-identity requirement against tmux.HookKeyFormat.
  - internal/session/create.go:105-124 — updated @portal-dir/@portal-id stamp block; @portal-dir stamp at :120; @portal-id at :122-124 (`if token, genErr := sc.gen(); genErr == nil { _ = sc.tmux.SetSessionOption(prepared.SessionName, PortalIDOption, token) }`).
- Notes:
  - Byte-identity verified: tmux.go:849 HookKeyFormat embeds "@portal-id"; PortalIDOption = "@portal-id". A static tripwire (internal/tmux/hookkey_test.go) already guards drift across both literals.
  - Ordering assumption behind the call-counting test holds: GenerateSessionName (naming.go:32-48) calls gen() exactly once on the no-collision happy path, so PrepareSession consumes call #1 (name) and CreateFromDir consumes call #2 (stamp). Correct.
  - Both a generation error and a stamp error are swallowed; NewSession failure early-returns before either stamp (create.go:101-103), so neither runs — matches spec.
  - Token-width decision (reuse sc.gen at suffixLen=6, no second generator, no suffixLen change) matches the task DECISION and the spec's "implementation detail" latitude. The accepted fire-and-forget collision residual is documented inline (create.go:117-119).

TESTS:
- Status: Adequate
- Coverage (internal/session/create_test.go): the mock was upgraded from a single recorded setOption call to setOptionCalls []setOptionCall with a setOptionCallFor(name) lookup (:124-174), and the pre-existing @portal-dir assertions were migrated to read from the slice (:465-475, :518-527, :546-548). All five task-mandated tests are present and correctly named:
  - "stamps @portal-id with a fresh token after creating a session" (:551) — asserts session target + value == token "abc123".
  - "stamps both @portal-dir and @portal-id on a successful create" (:577) — asserts both calls made.
  - "returns the session name when the @portal-id stamp SetSessionOption fails" (:598) — SetSessionOption errors, still returns name, no error.
  - "creates the session un-stamped when stamp-time token generation fails" (:621) — call-counting generator (name succeeds, stamp fails); asserts no @portal-id call AND correct returned name.
  - "does not stamp @portal-id when NewSession fails" (:654) — NewSession errors; asserts no @portal-id call.
  - Package session_test, no t.Parallel() — conforms to the CLAUDE.md / spec convention.
- Notes:
  - The stamp-value assertion checks the exact token ("abc123") and the session target — verifies behavior, not implementation; would fail if the wrong value/target or a missing call regressed.
  - Not over-tested: each test isolates one contract (token stamped / both stamped / stamp-error swallowed / gen-error swallowed+skipped / NewSession-guard). Minor benign redundancy — "does not stamp @portal-id when NewSession fails" (:654) overlaps the pre-existing "does not stamp at creation when NewSession fails" (:530, which asserts len(setOptionCalls)==0); the id-specific variant is explicitly required by the task test list, so this is acceptable, not a finding.
  - The "returns the session name when the @portal-id stamp SetSessionOption fails" test relies on the shared setOptionErr (which also fails the @portal-dir stamp), so it does not isolate the @portal-id call as the failing one. Acceptable — the contract under test is "any stamp failure is non-fatal and name still returns," which it verifies; the dedicated no-@portal-id-call-on-gen-failure case covers the id-specific skip path.

CODE QUALITY:
- Project conventions: Followed. Const named MixedCaps (PortalIDOption), placed beside PortalDirOption. Test mock/naming mirror the existing @portal-dir pattern. No t.Parallel(). Doc-comments idiomatic.
- SOLID principles: Good. Single responsibility preserved; stamp uses the existing injected TmuxClient + IDGenerator seams (no new dependency).
- Complexity: Low. One additional guarded if-block; no new branches beyond the genErr gate.
- Modern idioms: Yes. `if token, genErr := sc.gen(); genErr == nil { ... }` scopes the token tightly; `for range maxRetries` style already in package.
- Readability: Good. The stamp block carries a clear best-effort rationale, the @portal-dir vs @portal-id divergence, and the fire-and-forget collision residual.
- Error handling: The `_ = sc.tmux.SetSessionOption(...)` swallow appears to conflict with the golang-error-handling "never discard with _" rule, but it is a documented, spec-mandated best-effort exception (mirrors the existing @portal-dir stamp and QuickStart), with an explanatory comment block — the skill's own "to ignore a rule, add a comment" carve-out applies. Not a violation.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
