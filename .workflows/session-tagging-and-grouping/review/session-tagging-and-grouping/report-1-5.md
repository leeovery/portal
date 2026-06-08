TASK: session-tagging-and-grouping-1-5 — Stamp `@portal-dir` at session creation

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: stamp @portal-dir=<resolvedDir> at creation using PrepareSession git-root; survives rename; QuickStart exec-handoff path has no in-process stamp (covered by lazy fallback, set-option not injected into exec args); SetSessionOption failure non-fatal.

SPEC CONTEXT: specification.md:83-91 — stamp @portal-dir at creation (git-root from PrepareSession); survives rename + pane cd; un-stamped covered by lazy fallback (1-8/phase 5). Creation stamps raw resolved git-root (canonicalisation is render-time, task 1-4).

IMPLEMENTATION: Implemented.
- create.go:14 PortalDirOption const; create.go:96 best-effort `_ = sc.tmux.SetSessionOption(...)` after NewSession succeeds; create.go:49-53 TmuxClient gains SetSessionOption (ISP-clean).
- quickstart.go:42-62 deliberately does NOT stamp (syscall.Exec replaces process; documented; set-option not injected into exec args).
- Uses prepared.ResolvedDir (git-root), not re-derived. Rename survival is tmux runtime guarantee (left to integration round-trip test 6-4). Failure swallowed via `_ =` per session package having no log component.

TESTS: Adequate. create_test.go:431-529 — stamps with resolved git-root; SetSessionOption failure non-fatal; stamps prepared dir not input subdir; does NOT stamp when NewSession fails. quickstart_test.go:57-82 — exec path injects neither @portal-dir nor set-option. All edge cases; not over-tested.

CODE QUALITY: Conventions followed (narrow interface, sanctioned silent swallow, mock-injection DI); SOLID good (SRP — creation-stamp vs render-restamp separate files); low complexity; rationale comments prevent future "fix" of asymmetry. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
