TASK: resume-hooks-silently-lost-8-28 — "The Shell-Quoting Rule Has Three Separately-Maintained Homes" (phase 8, implementation-analysis consolidation; severity: duplication)

ACCEPTANCE CRITERIA:
- One declaration of the quoting rule; all three sites call it.
- No new import edge between `internal/restore`, `internal/session` and `internal/spawn`.
- The leaf's transitive deps are stdlib-only.
- Every composed command is byte-identical to today's.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 consolidation task, so its authority is its own body rather than the specification. The spec touches the subject only where it names the boundary the rule protects: `.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md:162` states that `buildHydrateCommand` interpolates the hook key "through `internal/shellquote`'s `Single`" and that the quoting is **retained** because the new token character set holds no shell metacharacters — a boundary that becomes strictly safer, not one that may be relaxed. Corrigendum at specification.md:570 records the extraction itself ("`shellQuoteSingle` → `internal/shellquote`'s `Single`, throughout … implemented byte-for-byte through the moved function"), so the spec body and the delivered code agree.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/shellquote/shellquote.go:15` — `Single(s string) string`, the sole declaration of the close-escape-reopen rule (`internal/shellquote/shellquote.go:16`). Package doc at `internal/shellquote/shellquote.go:1-5` states the leaf rationale.
  - `internal/restore/session.go:352` and `internal/restore/session.go:357` — `buildHydrateCommand` now calls `shellquote.Single` for `exe`/`fifo`/`file` and for the optional `--hook-key`; the package-local `shellQuoteSingle` is deleted (verified by `git show 4b3ee0b7 -- internal/restore/session.go`).
  - `internal/spawn/recipe.go:73` — `renderCommandString` quotes each element through the leaf; the package-local `shellQuote` is deleted.
  - `internal/session/create.go:33` and `internal/session/create.go:35` — `BuildShellCommand` calls the leaf rather than inlining `strings.ReplaceAll(joined, "'", "'\\''")`.
  - `internal/shellquote/leaf_guard_test.go:14-18` — the dependency guard, modelled on `internal/nanoid/leaf_guard_test.go:14-18` (same shape, same `Lanes()` loop, same `ForbiddingThirdParty()` empty-allowlist form).
  - `CLAUDE.md:80` — new architecture-table row for the leaf; `CLAUDE.md:218` re-points the spawn row's parenthetical from "which owns the nested `'\''` re-quoting" to "which quotes every element through `internal/shellquote`", so the doc no longer claims spawn owns the rule.
  - `internal/tui/pagepreview_surface_audit_test.go:206` — the new package added to the surface-audit allowlist, keeping that guard truthful rather than green-by-omission.
- Notes:
  - **AC1 verified by enumeration, not by inspection of the named sites alone.** A repo-wide grep for the escape idiom over non-test Go sources returns exactly one production declaration: `internal/shellquote/shellquote.go:16`. The other production hits are unrelated — `internal/tmuxout/strip.go:15` (rune comparison in a quote *stripper*), `internal/tmux/portal_saver.go:30` (a fixed literal with no interpolation), and `internal/spawn/message.go:11` (`QuoteJoin`, which renders user-facing copy such as "'a', 'b' are gone" and composes no shell string). `internal/spawn/ghostty_command_test.go:50-80` holds a hand-written *decoder* (`decodeRenderedArgv`), which is an independent inverse oracle rather than a restatement of the rule.
  - **AC2 verified by enumeration.** Grepping the three packages' non-test sources for imports of each other returns nothing: `internal/restore`, `internal/session` and `internal/spawn` each import `internal/shellquote` and none imports another of the three.
  - **AC4 verified against the pre-change assertions.** `restore` and `spawn` are pure renames of a byte-identical function body, so their output cannot move. `session.BuildShellCommand` was re-shaped (`fmt.Sprintf("%s -ic '%s; exec %s'", shell, escaped, shell)` → `fmt.Sprintf("%s -ic %s", shell, shellquote.Single(script))`), and the commit changed no test file: `git show 4b3ee0b7:internal/session/create_test.go` still asserts `"/bin/zsh -ic 'claude; exec /bin/zsh'"` and the escaped-quote case `"/bin/zsh -ic 'echo '\\''hello'\\''; exec /bin/zsh'"`, both of which the new form reproduces exactly. The two forms diverge only when `$SHELL` itself contains a single quote, where the old form emitted a broken command line (an unquoted `exec <shell>` inside the single-quoted script would terminate the quoting) and the new one escapes it — strictly better, and unobservable for any real shell path. Not a loss.
  - The current `internal/session/create.go:33-35` also quotes the shell *path* (`quotedShell`); that is a later task's change (`674048ce`, `Tresume-hooks-silently-lost-9-15`, confirmed by `git log -L 29,36:internal/session/create.go`) and sits outside this task's change-set.
  - Do-item 1's conditional clause ("move `renderCommandString`'s join with it **if** the join belongs beside the rule") was answered by leaving the join in `internal/spawn/recipe.go:70-76`. That is the right call, not an omission: the join has one consumer, so moving it would put a spawn-shaped helper in a leaf reached by two packages that do not need it.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/shellquote/shellquote_test.go` carries exactly the three cases the task's Tests section names, each as its own subtest with the prescribed prose: "it wraps a plain string in single quotes" (line 10), "it re-quotes an embedded single quote" (line 18), "it quotes an empty string" (line 26). Each asserts one concrete string; the empty case carries the reason in its failure message ("an empty argument must survive as one word").
  - `internal/shellquote/leaf_guard_test.go:14` pins the transitive dependency set across both lanes via `sourceguardtest.Lanes()`. The guard is not vacuous: `AssertDepsWithin` (`internal/sourceguardtest/assertdepswithin.go:24-49`) fatals on an empty set and on a set not holding the judged package itself, and `packageDeps` (`internal/sourceguardtest/packagedeps.go:158-172`) reads the judged package's own directory so the sources are recorded as cache inputs of the binary that judges them — so a new import cannot land behind a cached pass.
  - The refactor evidence the task asked for is present and unmodified: the commit touched no assertion file, and the three sites' byte-level argv assertions still stand — `internal/restore/session_build_hydrate_test.go:14-73` (including the embedded-quote hook key at line 73), `internal/spawn/recipe_test.go:169-193`, `internal/spawn/configadapter_argv_test.go` / `configadapter_script_test.go` / `ghostty_command_test.go` (all composing their `want` through `renderCommandString`), and `internal/session/create_test.go:15-70` plus `prepare_test.go:102` and `quickstart_test.go:294,319`.
- Notes: No over-testing. The leaf's own suite is three assertions with no setup and no mocking, and it does not duplicate the call-site suites — those assert composed argv, this asserts the rule. Would a break be caught? Yes: dropping the escape pass fails `shellquote_test.go:18` and, downstream, `session_build_hydrate_test.go:73`, `recipe_test.go:189` and `create_test.go:44`; adding a non-stdlib import fails `leaf_guard_test.go:14`.

CODE QUALITY:
- Project conventions: Followed. The leaf is the shape CLAUDE.md already prescribes for a rule that mutually non-importing packages all need (`internal/nanoid`, `internal/tmuxerr`, `internal/tmuxout`), the guard is a line-for-line sibling of `internal/nanoid/leaf_guard_test.go`, and the architecture table gained its row (`CLAUDE.md:80`) in the same commit rather than being left to drift. The package logs nothing, which is correct — a leaf reachable from `internal/session` (a package that must not take a logging edge) could not import `internal/log` anyway.
- SOLID principles: Good. One exported function with one responsibility; the join that is spawn-specific stayed in spawn.
- Complexity: Low. A single expression.
- Modern idioms: Yes. `strings.ReplaceAll` over the raw-string escape literal, which avoids the double-backslash form (`"'\\''"`) the old `internal/session/create.go` used and which is easy to misread.
- Readability: Good. The naming reads at the call site without stutter (`shellquote.Single(exe)`), and the doc comments state the *why* — why a backslash escape is not available inside single quotes, and why an empty string must still render as a quoted word — rather than restating the expression.
- Comment accuracy: Verified against the code. The package doc's claim that the leaf "depends on the standard library alone" is true (`strings` is its only import) and is enforced by `leaf_guard_test.go`. The `Single` doc's three claims — POSIX single quotes, close-escape-reopen for an embedded quote, an empty `s` rendering as an empty quoted word — each hold against line 16 and each has a test. `internal/restore/session.go:344-348`'s surviving comment ("Every interpolated value is single-quoted") still holds after the swap. `CLAUDE.md:218` was corrected in the same commit so it no longer attributes ownership of the rule to `renderCommandString`. No comment references a task id, phase or spec section.
- Security: Improved. Consolidating a shell-metacharacter rule that had drifted into three hand-maintained copies removes exactly the failure mode the task named — fixed once, still wrong twice. No new interpolation was introduced.
- Performance: N/A. One allocation per call, as before.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
