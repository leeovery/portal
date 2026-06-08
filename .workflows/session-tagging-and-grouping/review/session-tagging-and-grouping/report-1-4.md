TASK: session-tagging-and-grouping-1-4 — Canonical directory path key for dir→project lookup

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: provide canonical lookup-key matching stored Project.Path; normalise symlinks, trailing slash, ~ expansion. Edge: symlinked path, trailing slash, ~ home expansion, path not a known project, relative path.

SPEC CONTEXT: specification.md:111-113 — render-time lookup key must match stored Project.Path exactly; stamped value and fallback git-root reduce to same canonical form. Mismatch silently drops a session.

IMPLEMENTATION: Implemented at internal/project/pathkey.go:24-42 (CanonicalDirKey). Pipeline: resolver.ExpandTilde → filepath.Abs → filepath.EvalSymlinks → filepath.Clean, with two fallbacks. All five edge cases handled; "not a known project" is consumer concern (Index.Match), correctly out of scope. Build-time invariant honoured: dirresolve.go:70-75 stamps CanonicalDirKey(ResolveGitRoot(...)); Project.Path from same ResolveGitRoot. Genuinely consumed (index.go, tui/model.go, session/dirresolve.go).

TESTS: Adequate. pathkey_test.go covers all five edge cases plus TestCanonicalDirKey_MatchesResolveGitRoot (real git repo, asserts stored key == lookup key, exercises spec line 113). Behaviour-focused, refactor-resilient. Abs-failure branch unreachable, reasonably untested.

CODE QUALITY: Conventions followed (reuses ExpandTilde single source of truth); SOLID good (pure); low complexity; idiomatic. Doc comment explains why not reuse NormalisePath. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
