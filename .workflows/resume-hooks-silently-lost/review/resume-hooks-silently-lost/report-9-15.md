TASK: resume-hooks-silently-lost-9-15 — session.BuildShellCommand Leaves The Leading Shell Word Unquoted (tick-a31eaf)

ACCEPTANCE CRITERIA:
- `BuildShellCommand([...], "/My Apps/zsh")` renders the shell as one quoted word, and the composition run through a shell invokes `/My Apps/zsh` rather than `/My`.
- A shell path with no metacharacters still composes a working command; only its quoting changes.
- A single quote inside either the shell path or the command is re-quoted so the outer quoting survives.
- An empty command still returns the empty string with no quoting applied.
- The `exec $SHELL` tail refers to the same shell the leading word names.

STATUS: complete

SPEC CONTEXT: This is a phase-9 implementation-analysis task, so its authority is its own body rather than the `resume-hooks-silently-lost` specification (which governs hook-key durability, not session creation). The historical v1 spec illustrates the composition as the literal `$SHELL -ic '<cmd>; exec $SHELL'` (`.workflows/v1/specification/portal/specification.md:714`), and `CLAUDE.md:80` uses the same shorthand when naming `internal/shellquote`'s consumers. Both are descriptions of the *site*, and the task deliberately changes its quoting — the divergence is authorised by the task body and is an improvement, not a loss. `internal/shellquote` is the single declaration of the POSIX single-quoting rule (`internal/shellquote/shellquote.go:15`), and this task only routes an extra value through it.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/session/create.go:29-36` — `quotedShell := shellquote.Single(shell)` (`:33`) is computed once and used both as the leading word and inside the script's `exec` tail (`:34`), with the whole script quoted again at `:35`. `internal/shellquote` is untouched (commit 674048ce changes only the four `internal/session` files).
- Notes: Both reachable callers inherit the fix through the single composition point — `internal/session/prepare.go:46` (`PrepareSession`) feeds both `SessionCreator.CreateFromDir` (`create.go:74`) and `QuickStart.Run` (`quickstart.go:54`), so the `tmux new-session` argv on both the inside-tmux and outside-tmux paths carries the quoted form. The nesting is correct: an inner `exec '/bin/zsh'` is re-escaped by the outer `shellquote.Single(script)` to `exec '\''/bin/zsh'\''`, which the shell that runs the `-ic` script reads back as one word. The empty-command early return (`:30-32`) is before any quoting, so `""` is still returned verbatim. No other production site composes a shell string around `$SHELL`: `cmd/state_hydrate.go:153-167` execs the shell as an argv (`ExecShell(shell, []string{shell})`) and needs no quoting, and `internal/restore/session.go:352-357` / `internal/spawn/recipe.go:73` already route every element through the leaf.

TESTS:
- Status: Adequate
- Coverage: `internal/session/create_test.go`. The four named tests from the plan all exist: `:75` (space in the shell path), `:81` (single quote in the shell path), `:87` (metacharacter-free path), `:59` (empty command, alongside the pre-existing nil case at `:53`). The three behavioural sub-tests stage a real executable stand-in (`stageFakeShell`, `:99`) and hand the composed string to a real `/bin/sh -c` (`assertComposedRunsShell`, `:124`), asserting `shell=<full path>\nflag=-ic\ncmd-ran\nreexec=<full path>` — which observes the leading word (AC1/AC2), the re-quoting (AC3) and the `exec` tail naming the same shell (AC5) through actual word-splitting rather than through a string comparison. Each would fail against the pre-change code: an unquoted `/My Apps/zsh` splits and `run.Run()` errors; an unquoted `it's-zsh` breaks the script's own outer quoting.
- Notes: The byte-identity expectations the behaviour change invalidated were all updated — `create_test.go:26,32,38,44,50,436,478,499`, `prepare_test.go:102`, `quickstart_test.go:294,319` — and they are the only assertions in the tree derived from `BuildShellCommand`. The three remaining occurrences of the old unquoted string (`cmd/open_test.go:1309`, `cmd/open_test.go:2779`, `internal/tmux/tmux_test.go:529`) are opaque fixture strings fed to a mock `QuickStarter`/`Commander` whose subject is argv pass-through and verbatim log rendering; nothing there derives from or asserts the composition, so they neither break nor mislead. Not over-tested: the table keeps its existing cases and the fixture-driven sub-tests add exactly the four the plan named. Lane placement is correct — the suite spawns `/bin/sh` and writes only under `t.TempDir()`; it builds no portal binary, spawns no daemon and touches no tmux server, so the unit lane is right.

CODE QUALITY:
- Project conventions: Followed. The quoting rule stays declared once in the `internal/shellquote` leaf and is reached rather than restated; no logging, no new dependency, no seam added.
- SOLID principles: Good — single composition point, unchanged signature, callers untouched.
- Complexity: Low (one extra local; the function is still four statements).
- Modern idioms: Yes.
- Readability: Good. Naming `quotedShell` at the point of quoting makes the two use sites obviously the same value, which is what makes AC5 readable off the source.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
