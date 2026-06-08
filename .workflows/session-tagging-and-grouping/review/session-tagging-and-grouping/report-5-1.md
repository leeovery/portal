TASK: session-tagging-and-grouping-5-1 — Wire lazy stamp-on-render fallback into the render path

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA: post-reboot/first-ship/QuickStart sessions appear under project/tags on first grouped render not Unknown/Untagged; derived dir stamped after first render; empty current_path/unresolvable → Unknown/Untagged; builders contain no resolution logic; open.go injects production stamper+runner.

SPEC CONTEXT: spec § lazy stamp-on-render fallback + AC9. Previously dead code (ResolveAndStampDir helpers existed/unit-tested but no production caller) — this analysis-cycle task wires it in.

IMPLEMENTATION: Implemented.
- model.go:1020-1033 resolveSessionDirs (nil-seam guard, maps via session.ResolveAndStampDir, overwrite-on-success, value-copy so m.sessions never mutated); :1047-1057 wired only on ByProject/ByTag arms (Flat/signpost bypass); :608-620 WithDirResolver option; :237-249 dirStamper/dirRunner fields. open.go:391/513-514 production wiring (client + RealCommandRunner, gated non-nil). Builders pure (consume resolved Session.Dir). Reuses 1-7/1-8 helper.

TESTS: Adequate. rebuild_dir_resolution_test.go — empty-Dir resolves under project (By Project) / tags (By Tag); stamped once after render; unresolvable→Unknown; no m.sessions mutation; nil seam no panic. Gate test (Flat/signpost zero reads). fakeDirRunner exercises real CanonicalDirKey. All 6 edge cases.

CODE QUALITY: Conventions followed (small-interface DI + functional option, value-copy, no new logging); SOLID good (resolveSessionDirs single responsibility, builders pure); low complexity; idiomatic. Documented semantics. No issues.

BLOCKING ISSUES: None.
NON-BLOCKING NOTES: None.
