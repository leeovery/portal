TASK: cli-verb-surface-redesign-6-1 — Rename `hooks` → `hook` (canonical) + permanent silent `hooks` cobra alias + skipTmuxCheck repoint

ACCEPTANCE CRITERIA (plan row 6-1 + Phase 6 phase-level):
- `hooks` alias silent (no deprecation warning; cobra lists it only on the Aliases line)
- machine-generated `portal hooks set …` (external Claude SessionStart skill) still works via the alias
- `skipTmuxCheck` keys on canonical `c.Name()`="hook" so the `hooks` alias is bootstrap-exempt too
- set/rm/list reachable under both names
- update the `skipTmuxCheck` doc comment that names hooks/clean

STATUS: Complete

SPEC CONTEXT:
Spec §"Remaining Verbs — Keep As-Is, except `hooks` → `hook`" (lines 352-357): `hooks` → `hook` follows the singular-namespace-noun convention (`docker container`, `gh pr`); `hook` is canonical/documented, `hooks` retained as a cobra alias. §"Back-Compat & Deprecation" (line 390): the `hooks` alias is the ONE deliberate exception to the no-back-compat rule — permanent, silent, no deprecation timer — justified because `portal hooks set …` is machine-written by the user's external SessionStart skill (real operational cost to break, unlike author muscle memory for the removed attach/spawn). §skipTmuxCheck (line 367): the rename keeps the bootstrap exemption because skipTmuxCheck keys on `c.Name()` (cobra's canonical name), so the alias is covered. `clean` leaves the exempt set (deleted in Phase 4); `state` stays. Surface table (line 415): `portal hook {set,rm,list}` — renamed from `hooks` (`hooks` kept as a silent alias).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/hooks.go:79-83 — `hookCmd` declared with `Use: "hook"`, `Aliases: []string{"hooks"}`, `Short: "Manage resume hooks"`. No `Deprecated` field (a plain Aliases entry is silent by design).
  - cmd/hooks.go:74-78 — doc comment explains the carve-out and explicitly warns against `cmd.Deprecated` (which would print a notice).
  - cmd/hooks.go:188-191 — list/set/rm subcommands added under `hookCmd`; `hookCmd` added to `rootCmd`.
  - cmd/root.go:63 — `skipTmuxCheck["hook"] = true` (canonical name only; no `"hooks"` entry needed since the walk keys on `c.Name()`).
  - cmd/root.go:165-170 — `PersistentPreRunE` walks the parent chain testing `skipTmuxCheck[c.Name()]`; cobra resolves an alias invocation to the canonical command whose `Name()` is "hook", so the alias inherits the exemption.
  - cmd/root.go:21-33 — doc comment updated: names `hook` (not `hooks`), explicitly states the silent `hooks` alias is "covered for free" because skipTmuxCheck keys on canonical `c.Name()=="hook"`; no `clean` reference remains (removed in Phase 4).
  - internal/log/process_role.go:80-85 — `case "hook", "hooks":` maps both spellings to `roleHooksCLI`, so hook-mutation log lines carry a stable `process_role` regardless of the spelling used.
- Notes: Internal Go identifiers stay plural (`hooksListCmd`/`hooksSetCmd`/`hooksRmCmd`/`hooksDeps`/`HooksDeps`/`hooksFilePath`/`loadHookStore`). This is consistent with the codebase decision (per CLAUDE.md) that the `hooks.json` filename and the `hooks` log component keep the plural — only the user-facing verb went singular. No drift; no orphaned/dangling references to a removed `hooksCmd`.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/hooks_test.go:766-847 (TestHookCommandRename) directly targets the rename: canonical name == "hook" (767-771); "hooks" present in Aliases (773-783); alias resolves list/set/rm to subcommands parented by "hook" via rootCmd.Find (785-798); silent — no "deprecat"/"warning" text on stdout+stderr for `hooks list` (800-823); machine-generated `hooks set --on-resume` persists via the alias (825-846).
  - cmd/root_test.go:340-355 (orchestrator-not-called-for-skipTmuxCheck) proves BOTH the canonical `hook` rows (list/set/rm) and the `hooks` alias rows (list/set/rm) execute without invoking the 10-step bootstrap orchestrator — the direct proof that the exemption keys on the canonical name and the alias inherits it, and that set/rm/list are reachable and bootstrap-exempt under BOTH names.
  - cmd/hooks_test.go:13-760 — the pre-existing behavioural suites (TestHooksListCommand/TestHooksSetCommand/TestHooksRmCommand) exercise list/set/rm end-to-end through the `hooks` alias with real store side-effect assertions, so alias functional parity is fully covered.
- Notes: Would fail if the feature broke — dropping the alias fails the Find/silent/persist subtests and the alias rows in root_test.go; adding `Deprecated` fails the silent subtest; removing `"hook"` from skipTmuxCheck fails the canonical `hook` rows in root_test.go. Not over-mocked (KeyResolver seam + isolated PORTAL_HOOKS_FILE only).

CODE QUALITY:
- Project conventions: Followed. Uses the package-level `*Deps` seam pattern; no `t.Parallel()`; the doc comment carries the design rationale in-source (house style). Consistent with the CLAUDE.md note that the `hooks` filename/log-component stay plural while the verb goes singular.
- SOLID principles: Good — single-responsibility command wiring; the alias is one declarative field.
- Complexity: Low — a struct-field addition plus a map-key rename; no new control flow.
- Modern idioms: Yes — idiomatic cobra Aliases (not Deprecated); no custom back-compat plumbing.
- Readability: Good — the cmd/hooks.go:74-78 and cmd/root.go:30-33 comments make the silent-alias intent and the "keys-on-canonical-name" exemption self-documenting.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/hooks_test.go:825-846 — the "machine-generated hooks set still persists via the alias" subtest byte-duplicates TestHooksSetCommand's "sets hook for current pane" happy path (same resolver key, same argv, same assertion); it only adds value as a named guard for the SessionStart-skill acceptance criterion. Optional: fold its distinct intent into a comment on the existing set test, or keep it and drop the duplicate assertion detail — low priority, it documents an explicit acceptance line so retaining it is defensible.
