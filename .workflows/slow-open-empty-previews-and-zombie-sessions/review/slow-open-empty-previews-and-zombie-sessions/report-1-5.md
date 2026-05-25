TASK: 1-5 — Document test-isolation contract in CLAUDE.md

ACCEPTANCE CRITERIA:
- CLAUDE.md contains new subsection under "DI / testing pattern"
- Searchable via grep -i "test isolation" and "XDG_CONFIG_HOME"
- Names helper with full signature
- Cites reporter's bug as canonical example
- Notes backstop is defence-in-depth, NOT substitute
- Notes no lint or CI enforcement
- Section length ≤ 15 lines

STATUS: Complete

SPEC CONTEXT: Spec § Component G item 3 — contributor-facing documentation of test-isolation rule. Item 4: no lint enforcement, contributor-discipline + structural `*testing.T` parameter.

IMPLEMENTATION:
- Status: Implemented
- Location: `CLAUDE.md` lines 69-71
- Notes:
  - New `#### Test isolation for daemon-spawning tests` subsection appended after "DI / testing pattern"
  - Single dense ~3-line paragraph (well under 15-line cap)
  - Helper named with full signature `portaltest.IsolateStateForTest(t *testing.T) (env []string, stateDir string)` — reflects Phase 9 task 9-5 rename, consistent with `internal/portaltest/isolated_env.go`
  - Incident cited as canonical example
  - Backstop framed as "defence-in-depth, not a substitute"
  - No-lint statement plus structural `*testing.T` rationale present

TESTS:
- Status: Adequate (documentation task)
- Coverage: `grep -i "test isolation"` → line 69 heading; `grep -i "XDG_CONFIG_HOME"` → line 71

CODE QUALITY:
- Project conventions: Followed; dense reference style matches CLAUDE.md
- Readability: Good; coherent single paragraph linking rule, reasoning, helper, backstop, enforcement strategy
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Doc uses Phase 9 renamed helper `IsolateStateForTest` rather than spec-original `NewIsolatedStateEnv`. Current text accurate to current code
- [idea] Paragraph hard-codes `internal/portaltest/isolated_env.go` path — minor maintenance touchpoint if package ever moves
