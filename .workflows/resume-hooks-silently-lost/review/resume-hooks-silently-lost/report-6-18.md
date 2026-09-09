TASK: resume-hooks-silently-lost-6-18 — Split The Hooks Scaffolding Out Of internal/transienttest (tick-22a377)

ACCEPTANCE CRITERIA:
- `transienttest`'s package doc is one sentence naming one job.
- `internal/hookstest` holds the hooks staging/interrogation surface and nothing else.
- `transienttest` no longer imports `internal/logtest`.
- CLAUDE.md's row describes what each package actually holds.
- Both lanes green.

STATUS: complete

SPEC CONTEXT: This is a phase-6 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context). The body's subject is a test-helper packaging concern created by the
earlier phases: `internal/transienttest` — originally the shared `list-panes -a` failure-mode commander for the two
destructive integration suites — had accumulated the hook-key seed vocabulary, the `hooks.json` sidecar lock fixture
and a `logtest`-backed degraded-read assertion, giving it three subjects and consumers (unit-lane `internal/hooks` and
`cmd` suites) with no relationship to a transient `list-panes` failure. The repo's other helper packages
(`tmuxtest`, `portalbintest`, `sourceguardtest`, `logtest`, `themetest`, `spawntest`, `restoretest`) are each
single-purpose and named for their one job, which is what makes "where does this helper go?" answerable.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hookstest/doc.go`, `internal/hookstest/hooks.go`, `internal/hookstest/hooks_lock.go` (new package;
    `hooks.go`/`hooks_lock.go` moved verbatim from `internal/transienttest` bar the identifier prefix in diagnostics)
  - `internal/transienttest/doc.go:1-4` (doc reduced to the one-job sentence plus the standard test-only placement note)
  - `internal/transienttest/` now holds only `commander.go`, `socket.go`, `doc.go`
  - `internal/tui/pagepreview_surface_audit_test.go:193` (new-package allowlist entry, which that guard's own failure
    message instructs a contributor to add for an addition unrelated to the preview feature)
  - `CLAUDE.md:89` (helper-package row split into a `transienttest` clause and a `hookstest` clause)
  - Commit `36265b17`; 16 consumer test files re-pointed
- Notes:
  - Verified the split is clean: `git ls-tree 36265b17^ internal/transienttest` held
    `commander.go`/`doc.go`/`hooks.go`/`hooks_lock.go`/`hooks_test.go`/`socket.go`; after the commit the hooks trio
    (plus the moved test) is in `internal/hookstest` and nothing hooks-shaped remains behind.
  - Verified the move was mechanical: the commit's diff over `cmd/` and `internal/hooks/`, with the two package
    qualifiers filtered out, is empty — no behaviour rode along.
  - Consumer re-pointing is complete in both lanes. Every `transienttest.<Sym>` reference in the tree resolves to
    `Commander` / `FailureMode` / `PassThrough` / `FailExitNonZero` / `FailEmptyStdout` / `SocketCommander`, all of
    which the surviving package declares; every one of the 21 distinct `hookstest.<Sym>` names referenced across the
    tree is declared in `internal/hookstest`. All three files importing `transienttest` still use it (no unused
    import), and every file importing `hookstest` uses it — the four files that name `hookstest` without a dotted
    reference (`cmd/config_precedence_single_source_test.go:25`, `cmd/hookkey_vocabulary_test.go:4`,
    `internal/hooksweep/helpers_test.go:4`, `internal/xdg/leaf_guard_test.go:12`) name it in a comment or a scanned
    path string, not an import.
  - `cmd/config_precedence_single_source_test.go:25` pins `../internal/hookstest/hooks.go` / `ResolveHooksFilePathFromEnv`
    by path; both the file and the function exist at the new location (`internal/hookstest/hooks.go:53`), so that guard
    followed the move rather than silently scanning a gone file.
  - No import cycle: `internal/hooks`'s suites are `package hooks_test` and `internal/hookstest`'s own are
    `package hookstest_test`.
  - `internal/hookstest` carries no build tag and its only third-party dep is `golang.org/x/sys/unix`, already a direct
    module dependency used by production `internal/hooks/lock.go:10` — so the package serves both lanes as its
    cross-lane consumer set requires.
  - No repo guard enumerates the test-only helper packages, so the `tui` surface audit was the only allowlist needing
    the new name; I searched and found no other list to update.

TESTS:
- Status: Adequate
- Coverage: The task calls for no new case ("Both lanes passing with an unchanged verdict count is the test for a pure
  move"), and none was added. The single test file that lived in `transienttest` — `hooks_test.go`, whose whole subject
  was the hook-key seed vocabulary — moved with the vocabulary it covers, so the split lost no coverage:
  `Commander`/`SocketCommander` were never unit-tested before the move either (they are exercised through the two
  destructive integration suites), and their coverage is unchanged.
- Notes: I did not execute either lane (reading only, per this role). Judged by reading, nothing in the change-set can
  fail a compile or flip a verdict: the moved symbols are all declared at the new path, every importer's symbol set
  resolves, and no production file imports either helper package.

CODE QUALITY:
- Project conventions: Followed. The new package matches the house test-helper shape — a `<subject>test` name, a
  package doc naming its one subject, non-`_test.go` files so any package's tests can import it, and the explicit
  "production code must not" note. Multi-sentence docs are the sibling norm (`restoretest`, `sourceguardtest`,
  `harnesstest`), so `transienttest`'s restored doc — one job sentence plus the standard placement caveat — reads as
  the criterion intends.
- SOLID principles: Good. This is single-responsibility applied to a helper package: one subject per package, with the
  `logtest` edge travelling to the package whose assertions need it rather than staying on the commander's.
- Complexity: Low — a move plus a doc rewrite.
- Modern idioms: Yes (unchanged by the move).
- Readability: Good. The two package docs now each answer "what is this for?" in their first sentence.
- Issues: None found.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
