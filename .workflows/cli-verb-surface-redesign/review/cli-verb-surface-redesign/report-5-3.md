TASK: cli-verb-surface-redesign-5-3 — Retired-surface & reachability guard

ACCEPTANCE CRITERIA:
- rootCmd exposes no attach/spawn child and no cobra alias resolving to either.
- Neither verb appears in --help, generated completion, or bare `portal` help.
- No deprecation warning and no back-compat alias for either (deliberate no-back-compat posture; the hooks→hook alias carve-out is Phase 6, out of scope).
- Reachability of migrated behaviours: exact/no-guess attach via `open --session <name>`; spawned-window exec target `portal open --session <name> --ack <batch>:<token>`; multi-window via multi-target `open`.
- state-hide / hook rename / tab-completion additions are Phase 6 (this guard asserts only the two verbs' absence, not the finalized completion surface).
- x/xctl shell functions (map to `portal open`) untouched and still work.

STATUS: Complete

SPEC CONTEXT:
Spec §"attach — Retired" and §"Back-Compat & Deprecation Story": attach and spawn are deleted outright — not aliased, not deprecated-with-warning. attach's two jobs are absorbed by `open` (`open --session <name>` for exact/no-guess attach; `portal open --session <name> --ack <batch>:<token>` as the spawned-window exec target). spawn is covered by the multi-target burst. The single deliberate carve-out (hooks→hook silent alias) is explicitly NOT applicable to attach/spawn and is Phase 6. §"Command Surface Summary" lists attach/spawn under Removed. §"Bare portal" confirms bare `portal` prints help/usage (same Available-Commands block as --help).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/retired_surface_test.go (the guard, the primary deliverable); cmd/root.go (rootCmd definition + registration surface); cmd/open.go:999,1003-1004 (the --session / hidden --ack reachability seams); cmd/open.go:177 (Args: cobra.ArbitraryArgs).
- Notes:
  * No attach/spawn command is registered on rootCmd — `grep attachCmd|spawnCmd cmd/` returns nothing; the only rootCmd.AddCommand calls are alias, hook, init, kill, list, doctor, open, state, uninstall, version (cmd/*.go). The deletion itself is Tasks 5-1/5-2; 5-3 is the guard proving it.
  * The 5-3 deliverable is a test file; cmd/root.go is unmodified by this task (it is the home of rootCmd, not an edit site). That matches the task's guard-only nature.

TESTS:
- Status: Adequate
- Coverage (cmd/retired_surface_test.go):
  * TestRetiredSurface_NoChildNamedAttachOrSpawn — walks rootCmd.Commands() (surfaces hidden children too), asserts no child named attach/spawn. Covers AC "no attach/spawn child".
  * TestRetiredSurface_NoAliasResolvesToRetiredVerbs — (a) walks the WHOLE command tree (allCommandsInTree) asserting no command carries attach/spawn as a cobra Alias; (b) asserts rootCmd.Find([verb]) falls through to rootCmd with the token surviving in `rest`, which distinguishes "deleted" from "kept behind a silent alias". Covers AC "no alias resolving to either" and, transitively, "no deprecation-with-warning shim".
  * TestRetiredSurface_AbsentFromHelp — parses rootCmd.UsageString() via the shared availableCommandNames helper (state_test.go), matching on the leading command word so a prose mention cannot false-positive. UsageString() is exactly what bare `portal` and `portal --help` print, so this covers both the "--help" and "bare portal help" ACs.
  * TestRetiredSurface_AbsentFromCompletion — GenBashCompletion (v1) and asserts the subcommand-offering form `commands+=("attach")`/`commands+=("spawn")` is absent. Narrow, keyed on the generator's emitted form rather than a loose substring — correctly scoped away from the Phase-6 completion candidate surface.
  * TestRetiredSurface_AbsorbedBehavioursReachableViaOpen — asserts --session exists, --ack exists AND is Hidden, and openCmd.Args admits >=2 positionals. Covers the three reachability ACs (exact attach / spawned-window receipt / multi-window burst). Verified against cmd/open.go: --session registered (line 999), --ack registered + MarkHidden (1003-1004), Args=cobra.ArbitraryArgs (line 177) so Args(cmd, {"a","b"}) returns nil.
  * x/xctl untouched: not duplicated here — the file header references TestInitZsh/TestInitBash/TestInitFish ("outputs x function routing to portal open") which genuinely exist and assert `x() { portal open "$@" }` (cmd/init_test.go:16,113,212). Sound reuse, no coverage gap.
- Notes:
  * Not under-tested: every AC in the task row has a corresponding assertion, and the scope boundary (Phase-6 items excluded) is enforced both in code comments and by the narrowness of the completion check.
  * Not over-tested: no redundant re-assertion of the burst's runtime behaviour (correctly deferred to Phase 3); the multi-target check asserts admittance-of-2-positionals only, explicitly not validator identity.
  * "No deprecation warning" has no dedicated assertion, but it is airtight-by-construction: a deprecation warning in cobra rides a Deprecated field on a command, and no attach/spawn command exists to carry one — the absence-of-command + no-alias tests transitively guarantee it. No gap.

CODE QUALITY:
- Project conventions: Followed — no t.Parallel (cmd-package convention, called out in the header), mutates no package-level state, reuses the shared availableCommandNames helper (DRY) rather than re-implementing help parsing.
- SOLID principles: Good — allCommandsInTree is a small, single-purpose recursion; each test targets one surface.
- Complexity: Low.
- Modern idioms: Yes (strings.SplitSeq in the reused helper; table-free explicit sub-assertions are clear here).
- Readability: Good — the header block documents intent, scope boundary, and the deliberate no-alias posture; the Find fall-through comment explains the deleted-vs-aliased distinction precisely.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] cmd/retired_surface_test.go:123-135 (TestRetiredSurface_AbsentFromCompletion) — the guard is coupled to the legacy GenBashCompletion (v1) `commands+=("name")` emission form. It is effectively belt-and-suspenders over the child-registration test (completion is derived from the command tree, so a re-added verb would already fail TestRetiredSurface_NoChildNamedAttachOrSpawn). If completion ever migrates to GenBashCompletionV2 the substring form changes and this check becomes a permanent no-op pass. Decide whether to harden it (also assert absence against GenBashCompletionV2, or derive the expected token from the command tree) or accept it as intentionally narrow given the redundancy. Low consequence.
