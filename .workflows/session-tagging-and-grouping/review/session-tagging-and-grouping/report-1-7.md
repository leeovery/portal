TASK: session-tagging-and-grouping-1-7 — Lazy active-pane → git-root directory resolution

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: tmux method reads active pane current_path in one call; resolver returns canonical git-root; active-pane only (not all panes); killed-mid-resolve non-fatal; no readable current_path → ok=false; output canonicalised via CanonicalDirKey.

SPEC CONTEXT: spec § lazy stamp-on-render fallback — derive dir from active pane current_path → git-root, use for this render. Plan resolves ambiguity: ResolveGitRoot returns input unchanged for non-repo (no error), so "no derivable git-root" = "no readable current_path".

IMPLEMENTATION: Implemented.
- dirresolve.go:18-20 PaneCurrentPathReader single-method seam (structurally enforces active-pane-only); dirresolve.go:49-76 ResolveSessionDir; tmux.go:343-349 ActivePaneCurrentPath via display-message (single call, active pane), classifies no-such-session via wrapNoSuchSession. Production wired at open.go:511-514. TrimSpace guard before ResolveGitRoot avoids os.Stat(""). Canonicalised via CanonicalDirKey.

TESTS: Adequate. dirresolve_test.go + tmux_test.go:3194+ — happy path (real tempdir); active-pane-only/exactly-once; killed-mid-resolve (ok=false, runner not called); empty path; canonical key matches stored Project.Path; non-repo yields cwd. Compile-time seam assertion. Behaviour-focused.

CODE QUALITY: Conventions followed (typed sentinels, narrow seam, no t.Parallel); SOLID good (ISP exemplary); low complexity; idiomatic errors.Is/%w. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES:
- [quickfix] dirresolve_test.go:36-43 — fakeRunner.lastCmd/lastArg written but never asserted; dead test scaffolding (drop or assert git rev-parse invocation).
- [do-now] dirresolve.go:48 — tighten doc to point at the interface as the active-pane-only enforcement mechanism rather than prose claim. Trivial.
