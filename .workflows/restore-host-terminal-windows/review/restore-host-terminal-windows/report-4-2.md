TASK: 4.2 — Recipe structural validation: exactly-one-of argv/script + {command}-placeholder presence (restore-host-terminal-windows-4-2 / tick-72bad1)

ACCEPTANCE CRITERIA:
- Both argv+script → invalid (zero RecipeKind + descriptive error); neither → invalid (zero RecipeKind + descriptive error).
- argv template with no {command} in any element → invalid; valid argv-only → RecipeArgv; valid script-only → RecipeScript.
- validRecipeForEntry emits exactly one spawn WARN naming the entry key (in detail) for a structurally-invalid recipe and returns ok=false.
- validRecipeForEntry returns ok=false with NO WARN when the entry declares no open capability (forward-compat).
- {command}-presence rule applies to argv only; a script recipe is never rejected for omitting {command} (delivered as $1).
- renderCommandString produces the single space-joined command string, identical in form to the native Ghostty embed.

STATUS: Complete

SPEC CONTEXT: Config Schema → Validation & error handling requires each terminals.json open recipe to carry exactly one of argv/script, and an argv template omitting {command} is an invalid entry — every rejection must emit a spawn-component WARN so a config typo is diagnosable rather than silently degrading to "unsupported". Recipe execution contract: script recipes receive {command} as $1 (positional), which is why the {command}-presence check is argv-only. Observability → Attr keys: the closed spawn attr set has no entry-key attr, so key+reason ride in the opaque `detail` attr.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/spawn/recipe.go
  - RecipeKind (int, RecipeArgv=iota+1, RecipeScript) with zero-value as explicit invalid sentinel (recipe.go:12-22).
  - validateRecipe (recipe.go:35-52): clean 4-arm switch — both → error, neither → error, hasArgv → placeholder check, default (script) → RecipeScript. Both/neither ordered before the argv/script arms so the default arm is reached only for the (!hasArgv && hasScript) case. Correct.
  - argvHasCommandPlaceholder (recipe.go:56-63): strings.Contains(el, "{command}") over elements.
  - validRecipeForEntry (recipe.go:77-87): Open==nil → (Recipe{},0,false) no WARN; validateRecipe err → single WARN `terminals.json entry rejected` with detail=fmt.Sprintf("%q: %v", key, err) then (Recipe{},0,false); else (*Open, kind, true).
  - renderCommandString (recipe.go:94-96): strings.Join(command, " ").
- Notes: All six acceptance criteria satisfied. The WARN routes through detectLogger (log.For("spawn"), detect.go:21) — verified to carry component=spawn. renderCommandString's space-join matches ghosttyEmbed's join (ghostty.go:32, strings.Join(command," ")); ghosttyEmbed additionally AppleScript-escapes for its embedding context, which is the recipe author's responsibility per spec — no drift. No drift from plan Do-section: error strings, signatures, and control flow match the plan verbatim.

TESTS:
- Status: Adequate
- Location: internal/spawn/recipe_test.go
- Coverage:
  - validateRecipe: both → err+kind0; neither → err+kind0; whitespace-only script → err+kind0 (extra branch proving TrimSpace); argv-without-{command} → err+kind0; valid argv → RecipeArgv; valid script → RecipeScript. Every error path asserts the zero RecipeKind.
  - validRecipeForEntry: structurally-invalid (both) → ok=false, kind=0, zero Recipe, exactly one WARN with component=spawn and detail containing the key; Open==nil → ok=false, zero WARNs; valid argv AND valid script → ok=true with correct kind/recipe and zero WARNs.
  - renderCommandString: exact space-joined string over a realistic composed attach argv.
- Notes: Tests verify behaviour (return values + WARN records), not implementation details. Each named test from the plan is present, plus two justified additions (whitespace-only script branch; valid-path validRecipeForEntry with WARN-count-zero assertion — the latter is explicitly called for in the plan Do-section). Would fail if the feature broke (e.g. dropping the both/neither check, mis-tiering the kind, or emitting the wrong WARN count). Not over-tested — no redundant assertions, minimal setup, WARN capture via the shared logtest.Sink helper. One gap noted below: the invalid-recipe WARN test asserts the key is in `detail` but not that the failure reason is (the WARN's diagnosability is the spec's stated purpose).

CODE QUALITY:
- Project conventions: Followed. Tolerant/pure-helper style, closed spawn attr set honoured (reason in opaque `detail`, no invented attr key), idiomatic errors.New static messages, white-box package spawn tests via logtest.Sink — all consistent with the golang-* skills and the terminalsconfig.go sibling.
- SOLID principles: Good. validateRecipe (pure rules), argvHasCommandPlaceholder (single predicate), validRecipeForEntry (extract+validate+warn), renderCommandString (rendering) each have one responsibility.
- Complexity: Low. Single flat switch; no nesting beyond the argv arm's guard.
- Modern idioms: Yes. Tagged-switch, iota-based kind enum with explicit invalid zero, strings.TrimSpace/Contains/Join.
- Readability: Good. Doc comments state the argv-only rationale and the two ok=false cases clearly.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/spawn/recipe_test.go:101-104 — the invalid-recipe test asserts detail contains the entry key but not the rejection reason; add an assertion that detail also contains the failure text (e.g. strings.Contains(detail, "both argv and script")) to pin the WARN's diagnosability, which is the spec's stated purpose for the breadcrumb. Passes as-is (detail = fmt.Sprintf("%q: %v", key, err)).
- [quickfix] internal/spawn/recipe.go:83 (and terminalsconfig.go:68,74) — the package's spawn-component logger is named detectLogger (declared detect.go:21) but is now used for non-detection WARNs in recipe.go and terminalsconfig.go; rename to spawnLogger for accuracy. Phase-1 origin and cross-file (detect.go, terminalsconfig.go, recipe.go, plus adapters/tests referencing it), so route through the pipeline rather than apply inline.
