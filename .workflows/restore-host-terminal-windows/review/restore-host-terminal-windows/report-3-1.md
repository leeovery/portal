TASK: 3.1 — Option-safe batch/token ids + `@portal-spawn` marker-name derivation (restore-host-terminal-windows-3-1)

ACCEPTANCE CRITERIA:
- NewSpawnID with a generator returning "b1abcd" yields ("b1abcd", nil); an erroring generator yields ("", err) wrapping the generator error with an empty id.
- NewSpawnID with a value containing "-", ".", ":", or a space returns a non-nil "not option-safe" error and an empty id.
- SpawnMarkerName("b1abcd","t2wxyz") == "@portal-spawn-b1abcd-t2wxyz".
- ParseSpawnMarkerName("@portal-spawn-b1abcd-t2wxyz") -> ("b1abcd","t2wxyz",true).
- ParseSpawnMarkerName("@portal-skeleton-foo") -> ok false; ParseSpawnMarkerName("@portal-spawn-onlyonepart") -> ok false.
- ParseSpawnAckFlag("b1abcd:t2wxyz") -> ("b1abcd","t2wxyz",true); "nocolon", ":t", "b:" all -> ok false.
- Two independent NewSpawnID calls with real session.NewNanoIDGenerator() produce two non-empty option-safe strings (no cached id reuse).

STATUS: Complete

SPEC CONTEXT:
Spec (Burst & Partial-Failure Contract -> Ack channel / Ack delivery & portal attach contract): the token-ack channel keys each spawned window's confirmation on a namespaced `@portal-spawn-<batch>-<token>` tmux server option, where <batch> and <token> are picker-generated option-name-safe nanoid-style ids, deliberately NOT the renameable session name (which can carry chars invalid in a tmux option name — a set-option failure would mean no marker -> false ack-timeout). The distinct `@portal-spawn-` prefix keeps the channel invisible to state.ListSkeletonMarkers (`@portal-skeleton-`) in both directions. This task builds only the id vocabulary + marker/flag formatters+parsers that every downstream site (ack channel 3.2, attach --spawn-ack 3.3, burst compose 3.5) must derive by one identical rule.

IMPLEMENTATION:
- Status: Implemented (with one beneficial, intentional deviation from the literal plan text)
- Location: internal/spawn/ackid.go:1-90
  - SpawnMarkerPrefix = "@portal-spawn-" (ackid.go:14) — distinct from state's @portal-skeleton-.
  - NewSpawnID (ackid.go:32-41) — calls gen(); wraps+propagates gen error to ("", err) via fmt.Errorf(...%w); on success defensively rejects non-option-safe (incl. empty) ids to ("", err). Matches AC1/AC2.
  - isOptionSafeID (ackid.go:45-49) — non-empty AND every rune in the charset.
  - SpawnMarkerName (ackid.go:54-56) / ParseSpawnMarkerName (ackid.go:62-72) — format + CutPrefix/Cut inverse, rejecting foreign prefix, missing delimiter, empty batch/token. Matches AC3/AC4/AC5.
  - FormatSpawnAckFlag (ackid.go:77-79) / ParseSpawnAckFlag (ackid.go:84-90) — colon form + Cut inverse, rejecting missing colon / empty part. Matches AC6.
- Notes: The plan's literal "Do" prescribed a LOCAL `const spawnIDAlphabet = "abc...789"` duplicating session's alphabet. The implementation instead reuses `session.NanoIDAlphabet` directly (ackid.go:47). This is a deliberate, coordinated improvement, not drift: session/naming.go:15-21 was updated so its doc-comment explicitly names the spawn package as a co-consumer of the single shared constant, and it realises the plan's own Context note ("reusing its alphabet ... keeps the id vocabulary in one place") more faithfully than a duplicated const would. Strictly better on DRY / single-source-of-truth and guards against silent charset divergence. No functional drift; all ACs still satisfied.

TESTS:
- Status: Adequate
- Location: internal/spawn/ackid_test.go:1-185
- Coverage:
  - AC1: TestNewSpawnID_GeneratesOptionSafeIDAndPropagatesError (ok id; erroring gen -> errors.Is(sentinel) + empty id). Good.
  - AC2: TestNewSpawnID_RejectsNonOptionSafeGeneratedID (has-hyphen/has.dot/has:colon/has space, plus the extra empty-id-from-generator case at :64 — a valuable superset of the AC).
  - AC3/AC4: TestSpawnMarkerName_FormatsAndRoundTrips (fixed string + round-trip).
  - AC5: TestParseSpawnMarkerName_RejectsForeignOrDelimiterless — foreign skeleton prefix, no delimiter, empty batch (leading `-`), empty token (trailing `-`), bare prefix, unrelated `@portal-restoring` (a superset of the AC's two cases; also asserts empty-string returns on reject).
  - AC6: TestFormatSpawnAckFlag_FormatsAndRoundTrips + TestParseSpawnAckFlag_RejectsMissingColonOrEmptyPart (nocolon/:t1/b1:/:/"" ).
  - AC7: TestNewSpawnID_IndependentRealGeneratorIDs — two real-generator calls, both asserted non-empty + option-safe.
  - Extra guard: TestIsOptionSafeID_GovernedBySharedNanoIDAlphabet pins the charset to exactly session.NanoIDAlphabet and rejects the load-bearing "-". Justified (guards the marker-scheme invariant against silent re-divergence); tests an unexported helper but for a load-bearing reason — acceptable, not over-tested.
- Notes: One real gap against AC7. The AC's operative clause is "independence: the function does not reuse a cached id," and the test's own comment claims "no cached id reuse," yet the test never asserts first != second. It only checks both are non-empty + option-safe — a broken impl that cached and returned the same id twice would still pass. Adding `if first == second` would actually verify the claim; collision with the real 6-char/62-symbol generator is ~1/62^6 (~1.8e-11), negligible. Not blocking (production ids come from independent crypto/rand reads; independence holds), but the test under-verifies the exact property it names. See NON-BLOCKING NOTES.
- No over-testing: assertions are distinct, table cases are non-redundant, no unnecessary mocking (a plain queued-closure fake generator).

CODE QUALITY:
- Project conventions: Followed. Small pure functions, no side effects, unit lane / package spawn (no tmux/daemon/subprocess — correct lane). Error wrapping via fmt.Errorf(...%w) matches golang-error-handling conventions; the plan-specified messages are used verbatim.
- SOLID principles: Good. Single-responsibility formatters/parsers; NewSpawnID takes an injected `gen func() (string, error)` seam (dependency inversion — production passes session.NewNanoIDGenerator(), tests pass fakes).
- Complexity: Low. Every function is trivial straight-line logic.
- Modern idioms: Yes — strings.CutPrefix, strings.Cut, strings.IndexFunc, strings.ContainsRune (current stdlib).
- Readability: Good. Doc-comments explain the load-bearing hyphen-free invariant and the defensive re-validation rationale.
- Security: The option-safety re-validation is itself the guard preventing a malformed id from producing a set-option-invalid / ambiguous marker name — correct and defensive. No injection/exposure concerns.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/ackid_test.go:148 (TestNewSpawnID_IndependentRealGeneratorIDs) — add an `if first == second { t.Errorf(...) }` assertion so the test actually verifies AC7's "does not reuse a cached id" (its own comment claims this but the current body would pass even if NewSpawnID returned a cached id twice). Real-generator collision probability is ~1/62^6, negligible.
