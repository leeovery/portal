TASK: The Shell-Quoting Package Holds the Whole Rule Again (lazy-resume-on-attach-4-13, tick-d425dd) — export the argv joiner on internal/shellquote, delete the two byte-identical local copies (cmd shellWords, spawn renderCommandString), re-point every caller, move the renderer's tests to the package suite, update the CLAUDE.md row.

ACCEPTANCE CRITERIA:
- `internal/shellquote` exports the joiner and no other package declares one; `shellWords` and `renderCommandString` are gone and every former caller goes through the export
- The rendering is unchanged byte-for-byte: each element single-quoted, space-joined, an embedded quote as close-escape-reopen, an empty element as `''`
- `internal/shellquote` still depends on the standard library alone — `internal/shellquote/leaf_guard_test.go` passes in both lanes
- The hydrate chain's `sh -c '<draw>; <recover>'` command line and Ghostty's embedded payload are byte-identical to what they are today
- `go test ./...` and `go test -tags integration -p 1 ./...` pass

STATUS: complete

SPEC CONTEXT: The lazy-resume parked chain composes `sh -c '<exe> state resume-draw …; <exe> state resume-recover …'` from argvs carrying the user-authored resume command; the spec requires that command to reach the panel as the single argument it left as, so the rendering of an argv into a shell command line must be the quoted form everywhere. This task is a consolidation: the rule already existed twice, byte-identically, and the spec's behaviour depends on it staying unchanged.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/shellquote/shellquote.go:1-6 (package doc widened to both halves), :20-31 (`Join(argv []string) string`, body identical to both deleted copies: `Single` per element, `strings.Join(words, " ")`)
  - cmd/state_hydrate.go:299-303 (`parkedResumeChain` renders draw and recover through `shellquote.Join`); :292 (the backstop's marker-clear argv, added by a later task, also goes through `Join`)
  - internal/spawn/configadapter.go:53, :85 (argv and script recipe adapters)
  - internal/spawn/ghostty.go:21-24 (`wrapWithShellFallback`, comment re-pointed to `shellquote.Join`), :31 (`ghosttyEmbed`)
  - CLAUDE.md shellquote row names `Join(argv)` beside `Single(s)`; the Multi-window spawn bullet's two `renderCommandString` mentions were re-pointed to `shellquote.Join` in the same commit
- Notes: `shellWords` (cmd/state_resume_chain.go) and `renderCommandString` (internal/spawn/recipe.go) are gone. A repo-wide search outside .workflows/.tick finds neither name in any source, test or doc file. Neither package lost its `strings` import wrongly: cmd/state_resume_chain.go still uses it at :124 and :133, internal/spawn/recipe.go at :25 and :44. No other package declares a quote-each-and-join helper. The two remaining `strings.Join` sites near `Single` are not joiners: internal/session/create.go:34 deliberately joins the user's command unquoted as a script body, and internal/tmux/period_session_target_realtmux_test.go:144 is another feature's test fixture that quotes only chosen elements. Byte-identity holds by reading because the exported body is character-for-character the body of both deleted copies (shown in commit ca139dc4a). The hydrate chain has since gained a trap prefix and a backstop suffix from later approved tasks, which is deliberate and outside this task's byte-identity claim.

TESTS:
- Status: Adequate
- Coverage: internal/shellquote/shellquote_test.go:35-82 `TestJoin` holds the three moved cases under their original subjects (the spawn argv, the spaced element that stays one word, and close-escape-reopen) plus the two new cases the task names (empty argv renders "", an empty element renders as `''`). internal/spawn/recipe_test.go lost `TestRenderCommandString` and nothing else: before and after, it has `TestValidateRecipe` and `TestValidRecipeForEntry`. The Ghostty suite's golden literals (internal/spawn/ghostty_command_test.go:37, :39, asserted in `TestGhosttyEmbedGoldenLiteral` at :279) pin the embedded payload as literal bytes, independent of the renderer, so a change to `Join`'s output would fail there. The adapter suites and cmd/state_hydrate_lazy_test.go were re-pointed to call `shellquote.Join`, and nothing else in them changed.
- Notes: None.

CODE QUALITY:
- Project conventions: Followed. The leaf stays stdlib-only: shellquote.go imports only `strings`, and it is the package's only non-test source. The CLAUDE.md map was updated to match the new surface.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good. The `Join` doc comment is accurate against the code (quoted through `Single`, space-separated, empty argv renders as "").
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` and `go test -tags integration -p 1 ./...` pass" — run both lanes. Reading finds no caller left on a deleted name and no import left unused, but only a compile-and-run settles this.
