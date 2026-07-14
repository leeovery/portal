TASK: restore-host-terminal-windows-4-4 — Config argv recipe adapter (argvRecipeAdapter + shared recipe-execution primitives)

ACCEPTANCE CRITERIA:
- {command} substituted into each template element containing it as ONE literal string (never shell-split); element count fixed; a standalone-{command} element becomes exactly the composed command string.
- Template elements not containing {command} pass through byte-for-byte verbatim.
- OpenWindow runs exactly the substituted final argv through the runner (asserted via the fake runner's recorded argv).
- Clean recipe exit -> OutcomeSuccess; non-zero exit -> OutcomeSpawnFailed with opaque output in Detail; execution error -> OutcomeSpawnFailed.
- mapRecipeResult NEVER returns OutcomePermissionRequired (config recipes never trigger the permission path).
- Integration-tagged test exercises a real argv exec and confirms success / non-zero-exit mappings; the unit lane runs no real recipe.

STATUS: Complete

SPEC CONTEXT:
Spec (Config Schema -> Recipe execution contract, lines 351-374; Spawn Architecture -> env self-sufficiency, line 94; Permissions & Error Quarantine, line 374/417/420) fixes the contract this task implements: {command} is substituted as a single already-resolved command string dropped literally into the recipe (line 372) — the argv-array form exists precisely to sidestep shell-quoting (line 348). Portal builds {command} as an env-self-sufficient argv, so the adapter runs it verbatim, never shell syntax (line 94). A non-zero recipe exit maps to spawn-failed; permission-required is native-adapter-only and config recipes never produce it (line 374). renderCommandString is the SAME space-join the native Ghostty embed uses (lines 89-96).

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/configadapter.go — substituteCommand (47-53), mapRecipeResult (65-70), recipeFailureDetail (75-77), recipeRunner seam (21-23), execRecipeRunner (28-39), argvRecipeAdapter + OpenWindow (84-99). Shared plumbing: internal/spawn/exec_boundary.go runArgvCombined (21-33) / execFailureDetail (56-68); renderCommandString in recipe.go (94-96). Resolver wiring (RecipeArgv -> &argvRecipeAdapter{recipe.Argv, r.runner}) correctly lives in resolver.go:116-117 (Task 4.6, out of scope here).
- Notes: substituteCommand returns a NEW slice via make([]string, len(template)) + strings.ReplaceAll per element — element count fixed, template never mutated, non-token elements byte-for-byte unchanged. mapRecipeResult has exactly two arms (clean -> Success(TrimSpace(out)); else -> SpawnFailed) with NO permission branch, so OutcomePermissionRequired is structurally unreachable. The recipeRunner seam is kept genuinely separate from Phase 2's osascriptRunner (distinct interface + distinct execRecipeRunner type); only the terminal-agnostic runArgvCombined plumbing is shared, and both ghostty.go:69-74 and configadapter.go:32-39 doc-comment-reconcile this. This honours the task's "do NOT refactor/churn the Phase 2 osascript seam" directive while satisfying DRY — a clean consolidation, not drift. No permission/AppleEvent code path leaks into the config path. No scope creep in the argv path (the scriptRecipeAdapter in the same file is Task 4.5).

TESTS:
- Status: Adequate
- Coverage: internal/spawn/configadapter_argv_test.go — TestSubstituteCommand covers (a) {command} embedded inside a larger AppleScript-string element with a multi-space composed command, asserting element count fixed and neighbours verbatim; (b) a standalone {command} element becoming exactly the whole command string as one element; (c) no-mutation of the input template. TestArgvRecipeAdapterOpenWindow asserts the runner receives exactly the substituted final argv (fake records argv, slices.Equal), and maps clean->Success / non-zero->SpawnFailed(with opaque body in Detail) / execution-error->SpawnFailed (never panic). TestMapRecipeResult pins the pure mapping including a 6-case table proving OutcomePermissionRequired is never produced even for output embedding -1743/-1712, plus an assertion that Guidance stays empty. Integration test configadapter_argv_integration_test.go (//go:build integration) drives the real execRecipeRunner against /usr/bin/true (Success) and /usr/bin/false (SpawnFailed) — the real-exec inch off the unit lane, no tmux/daemon/built-binary.
- Notes: Every acceptance criterion has a direct assertion. The spacedCommand() fixture deliberately produces a multi-space {command} so a hypothetical shell-split would balloon the count and fail loudly. Not over-tested: the adapter-level and pure-function-level mapping tests overlap only on clean/non-zero but test distinct layers (wiring vs pure mapping edge cases), which is legitimate layering. Minor gap: no test covers a template with {command} in TWO different elements (or twice in one element); the loop + ReplaceAll make this trivially correct and the doc comment claims "every template element that carries the token", but the "every element" plurality is not exercised.

CODE QUALITY:
- Project conventions: Followed. 1-method DI seam (recipeRunner) with compile-time `var _` assertions; pure functions isolated from the exec boundary; white-box unit tests on the unit lane and the real-exec test correctly fenced behind //go:build integration per the lane rule. No t.Parallel(). Uses log.CombinedOutputWithContext (the stderr-preserving boundary helper) as mandated.
- SOLID principles: Good. Clean interface segregation (single Run method), dependency inversion via the seam, single-responsibility split across substituteCommand / mapRecipeResult / runner.
- Complexity: Low. All functions are short and branch-light.
- Modern idioms: Yes. strings.ReplaceAll, make-with-len, slices.Equal in tests, errors.As at the boundary.
- Readability: Good. Doc comments are precise and explain the load-bearing decisions (fixed element count, no permission branch, separate-seam rationale).
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/spawn/configadapter_argv_test.go:36 — add a TestSubstituteCommand case with {command} in two distinct template elements (e.g. []string{"wrap", "{command}", "--label", "{command}"}) to exercise the "every element that carries the token" plurality the doc comment (configadapter.go:44) claims; current cases only cover a single token-bearing element.
