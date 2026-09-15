TASK: Declare the session-opening function's expansion once (tick-0d76af / open-with-forced-filter-9-4) — collapse the seven `portal open` literals in `cmd/init.go` onto a single `openFunctionExpansion` const, so what the emitted shell function runs and what its completion shim asks Portal to complete for are one string in the source.

ACCEPTANCE CRITERIA:
- `portal init bash`, `portal init zsh` and `portal init fish` each emit output byte-identical to today's, for the default `x` and for `--cmd p`.
- The literal `portal open` appears exactly once in `cmd/init.go` — in the const declaration — and all seven sites derive from it.
- The bash shim's `expansion=` value and its `COMP_WORDS=` prefix come from the same const, so its `COMP_LINE`/`COMP_POINT` rewrite cannot disagree with itself.
- No emitted text is added or removed, and no shell suite is edited to accommodate the change.

STATUS: complete

SPEC CONTEXT: The specification (`.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md:346`, `:357`) records why the shim exists at all: cobra's emitted script builds its completion request from the *typed* word (`requestComp="${words[0]} __complete ${args[*]}"`; zsh from `${words[1]}`), so Tab after `x` would run `portal open __complete …` — a real `open` invocation carrying `__complete` as a positional — and Portal's completer would never be consulted. The shim rewrites the word to the expansion before delegating to `__start_portal`/`_portal`. `xctl`, which expands to bare `portal`, needs no shim. That makes the expansion a value two roles must agree on; this task removes the possibility of them disagreeing by declaring it once.

IMPLEMENTATION:
- Status: Implemented (commit b98692f50, `cmd/init.go` only, +22/−16)
- Location:
  - `cmd/init.go:51-54` — the `openFunctionExpansion = "portal open"` declaration, placed immediately above the shim consts that consume it, as the task prescribed.
  - `cmd/init.go:66-67` — bash shim's two occurrences, both `%[1]s` (indexed, so one argument feeds both).
  - `cmd/init.go:76` — zsh shim's single `%s`.
  - Emission sites: `:85` (bash function body), `:102` (`Fprint` → `Fprintf` for the bash shim), `:118` (fish function body), `:138` (fish `-w '%s'`), `:151` (zsh function body), `:168` (`Fprint` → `Fprintf` for the zsh shim).
- Notes:
  - All four criteria hold on reading. `grep -c "portal open" cmd/init.go` → 1, at the const. Seven derived sites, enumerated above: three function bodies, fish's `-w` target, two bash-shim occurrences and one zsh-shim occurrence.
  - Byte-identity of the emitted text is settled by inspection rather than needing a run: every substitution is mechanical (`"%s() { portal open \"$@\"; }\n"` → `"%s() { %s \"$@\"; }\n"` with the const as the second argument, and so on), and neither raw-string shim contains a `%` beyond the verbs just introduced — I enumerated the `%` characters in both const bodies (`cmd/init.go:65-80`): exactly three, all of them the new verbs. So `Fprintf` renders them to the same bytes `Fprint` emitted, with no escaping hazard.
  - The bash shim uses explicit argument indexes (`%[1]s` in both places) with a single argument, which is what makes criterion 3 structural — the `expansion=` value and the `COMP_WORDS=` prefix cannot be given different values without editing the verbs themselves.
  - The sites naming the *binary* were correctly left alone: `%s() { portal "$@"; }` (`:88`, `:121`, `:154`), `complete -c %s -w portal` (`:141`), `compdef _portal %s` (`:174`) and the `__start_portal`/`_portal` function names all belong to the control verb's role, not the expansion's.
  - No other production source restates the expansion in a role that must stay in lockstep (`cmd/doctor.go:26` is user-facing prose, `internal/capture/fakes.go:126` is canned fixture text, `internal/spawn/adapter.go:7` is a comment forbidding the opposite).
  - The comment revision at `cmd/init.go:56-64` holds against the code and against the spec: the generated script does send its request through the typed word, so `__start_portal` registered directly on `x` would ask the expansion rather than `portal`; and the added "the shim is a format string, so any % added to it must be escaped" is now true of that const.

TESTS:
- Status: Adequate (unchanged, as the task required)
- Coverage: Every one of the seven derived sites has its *rendered* output pinned by an existing assertion, so the refactor is fully observed without a new test:
  - `cmd/init_test.go:147` — the bash shim body asserted verbatim, including `expansion="portal open"` and `COMP_WORDS=(portal open …)`.
  - `cmd/init_test.go:47` — the zsh shim body asserted verbatim.
  - Function bodies: `cmd/init_test.go:32` (zsh), `:132` (bash), `:232` (fish) for the default `x`, and `:186`, `:281`, `:369` for `--cmd p`.
  - Fish's registration: `cmd/init_test.go:248` and `:295` assert `complete -c x -w 'portal open'` / `complete -c p -w 'portal open'`.
  - `cmd/init_completion_shell_test.go:317` and `:403` drive the emitted scripts through real bash/zsh/fish over a recording `portal` stub and assert the request is `__complete|open|…`, so a shim that stopped rewriting correctly fails end-to-end rather than only at the string level.
- Notes: No test file was touched by the commit (`git show --stat b98692f50` → `cmd/init.go` alone), which is what criterion 4 asked for. A stray `%` added to either shim in future is caught twice over — `go vet`'s printf check sees a constant format passed to `Fprintf`, and the two verbatim shim-body assertions would see the `%!` corruption — so the absence of a dedicated guard test for the single-literal property is not a gap. Neither over- nor under-tested for an extraction of this kind.

CODE QUALITY:
- Project conventions: Followed. The const sits beside its consumers, the emitted-text contract stays in `cmd/init.go`, and nothing in the change reaches tmux, state or the log vocabulary.
- SOLID principles: Good — single source of truth for a value with two roles is precisely the point of the change.
- Complexity: Low. No new branches; two `Fprint` calls became `Fprintf`.
- Modern idioms: Yes. Indexed verbs (`%[1]s`) are the right tool for a value repeated within one format, and avoid the alternative of passing the same argument twice.
- Readability: Good. The const's doc comment states the invariant it exists to hold, and the shim comment now warns that the raw string is a format.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
