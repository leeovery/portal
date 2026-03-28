TASK: Remove Duplicate AllPaneLister Interface From cmd/clean.go

ACCEPTANCE CRITERIA:
- No AllPaneLister interface definition exists in cmd/clean.go
- CleanDeps.AllPaneLister field uses hooks.AllPaneLister type
- No circular import introduced

STATUS: Complete

SPEC CONTEXT: The specification describes stale hook cleanup in the clean command, which requires listing all live tmux panes via an AllPaneLister interface. The interface was originally defined in both cmd/clean.go and internal/hooks/executor.go. This task consolidates to a single definition in the hooks package.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/cmd/clean.go:15 (field type), :24 (return type), :7 (import)
- Notes: No AllPaneLister interface definition exists in cmd/clean.go. The CleanDeps struct field at line 15 uses `hooks.AllPaneLister`. The buildCleanPaneLister function at line 24 returns `hooks.AllPaneLister`. The canonical interface definition lives at /Users/leeovery/Code/portal/internal/hooks/executor.go:27-29. No circular import: cmd imports internal/hooks, which does not import cmd.

TESTS:
- Status: Adequate
- Coverage: The existing clean command tests at /Users/leeovery/Code/portal/cmd/clean_test.go cover hook cleanup scenarios (stale hook removal, no tmux server, missing hooks file, all panes live, combined project+hook cleanup). The mockCleanPaneLister at line 460 satisfies hooks.AllPaneLister via structural typing. Tests exercise the DI path through CleanDeps.
- Notes: This was a refactoring task (type consolidation). The acceptance criteria specifies "all existing tests pass" which is a pass/fail check. No new test behavior was needed since this is a type alias change with identical method signatures.

CODE QUALITY:
- Project conventions: Followed. Uses the established DI pattern (package-level *Deps struct with test injection). Small interface usage aligns with the codebase convention documented in CLAUDE.md.
- SOLID principles: Good. DRY improved by removing the duplicate interface. Dependency inversion maintained via the hooks.AllPaneLister interface.
- Complexity: Low. Straightforward type reference change.
- Modern idioms: Yes. Uses Go structural typing correctly.
- Readability: Good. The import makes it clear where the interface is defined.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
