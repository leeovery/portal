TASK: restore-host-terminal-windows-8-6 — Promote the nanoid alphabet to a single shared constant referenced by spawn and session

ACCEPTANCE CRITERIA:
- The alphabet string literal appears exactly once; both naming.go and ackid.go reference the shared constant.
- No new import cycle is introduced (verify dependency direction; go build ./... green).
- isOptionSafeID's post-generation check is retained.
- Session name generation and ack-id generation behave identically to today.

STATUS: Complete

SPEC CONTEXT: This is a Phase 8 analysis-cycle (duplication) cleanup. spawn/ackid.go previously carried spawnIDAlphabet as a byte-for-byte copy of session's unexported alphabet. Correctness of the option-name-safe "<batch>-<token>" marker split depends on the charset excluding "-" (and ".", ":", space). The task extracts the literal to one exported home (session.NanoIDAlphabet) so a change is a single edit both consumers observe, eliminating silent cross-package drift risk.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/session/naming.go:15-21 — new exported const NanoIDAlphabet with a load-bearing doc comment (explains the "-" absence keeps the <batch>-<token> split unambiguous).
  - internal/session/naming.go:66 — NewNanoIDGenerator now indexes NanoIDAlphabet (was the local alphabet).
  - internal/spawn/ackid.go:16-20 — comment rewritten to reference "the single shared session.NanoIDAlphabet" (Do step 3 satisfied; old "identical to the session package's" wording gone).
  - internal/spawn/ackid.go:45-49 — isOptionSafeID now checks membership against session.NanoIDAlphabet.
- Notes:
  - Literal appears exactly once in production code (grep: only internal/session/naming.go:21). The second occurrence is naming_test.go:17 as an independent golden `want` — deliberate test hygiene, not a source-of-truth duplicate; the "appears exactly once" criterion targets production and is met.
  - Old spawnIDAlphabet identifier fully removed (grep: zero residue).
  - Dependency direction verified: internal/session imports only crypto/rand, fmt, strings (no internal imports) → it is a leaf; internal/spawn imports internal/session (ackid.go:7, burst.go:7). spawn → session is one-directional; session does NOT import spawn (the naming.go:17 hit is a comment reference, not an import). No import cycle introduced.

TESTS:
- Status: Adequate
- Coverage:
  - internal/session/naming_test.go:12-26 (TestNanoIDAlphabet_MatchesExpectedCharset) — pins NanoIDAlphabet against an independent literal and asserts absence of '.', ':', '-', ' '. Directly covers "shared constant equals the previous literal" + the load-bearing exclusions.
  - internal/spawn/ackid_test.go:173-185 (TestIsOptionSafeID_GovernedBySharedNanoIDAlphabet) — asserts the whole shared alphabet is option-safe and that appending '-' is rejected; pins the ack-id vocabulary to the single shared constant and guards re-divergence.
  - internal/spawn/ackid_test.go:150-171 — real NewNanoIDGenerator-driven NewSpawnID still yields non-empty, option-safe ids (charset checked against session.NanoIDAlphabet).
  - internal/session/naming_test.go:71-185 (TestGenerateSessionName) — unchanged; confirms session-name generation behaviour is identical.
- Notes:
  - Not under-tested: every acceptance clause has a corresponding assertion (constant equals literal, ack-id passes isOptionSafeID, session-name generation unchanged).
  - Not over-tested: assertions are focused; the naming_test.go golden literal is a deliberate independent copy (referencing the constant would make the equality test tautological) — correct, not redundant.

CODE QUALITY:
- Project conventions: Followed. Exported constant with a clear doc comment; leaf-package direction (session stays import-free of spawn) respects the codebase's import-cycle discipline.
- SOLID principles: Good. Single source of truth for the charset; consumers depend on one constant.
- Complexity: Low. Pure extraction; no control-flow change.
- Modern idioms: Yes. `strings.IndexFunc` / `strings.ContainsRune` membership check retained.
- Readability: Good. Doc comment explicitly documents why "-" must be absent, making the cross-package coupling self-evident.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
