TASK: restore-host-terminal-windows-1-6 — portal spawn command — --detect dry-run + usage-error gate

ACCEPTANCE CRITERIA:
- `portal spawn --detect` with a fake detector returning Identity{Name:"Ghostty", BundleID:"com.mitchellh.ghostty"} prints a line containing both `Ghostty` and `com.mitchellh.ghostty` to stdout and returns nil (exit 0).
- `portal spawn --detect` with a fake detector returning Identity{} (NULL) prints a line containing `no host-local terminal` and returns nil.
- `portal spawn` (no args, no --detect) returns an error that errors.As matches *cmd.UsageError (exit 2).
- `portal spawn --bogus` returns an error that errors.As matches *cmd.UsageError (exit 2), via SetFlagErrorFunc.
- The command Executes with bootstrapDeps injected and never dials a real tmux server.

STATUS: Complete

SPEC CONTEXT:
Spec §Spawn Architecture → `portal spawn` CLI behaviour: `--detect` is a dry-run that prints the detected terminal identity (friendly name + bundle id) and opens nothing; no session args + no --detect is a usage error. §Reporting & exit codes: usage error (no sessions, no --detect; unknown flag) → exit 2. §Terminal Identity & Detection → User-facing display: both fields shown, design copy `Apple Terminal · com.apple.Terminal`; NULL (remote/mosh / no host-local client) → honest "no host-local terminal" no-op. Spec explicitly states the exact CLI wording is NOT pinned beyond containing both fields (resolved) / naming the no-host-local outcome (NULL). §Concurrency & Post-Reboot Safety: a direct `portal spawn` CLI runs its own bootstrap synchronously first (so spawn is intentionally not in skipTmuxCheck).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/spawn.go — TerminalDetector interface (24-26); SpawnDeps + var spawnDeps (30-69); spawnCmd Use/SilenceUsage/SilenceErrors + RunE (71-104); --detect branch NULL/friendly print (86-96); empty-args usage gate (98-100); spawnDetector resolution authority (261-266); init() --detect flag + SetFlagErrorFunc→NewUsageError + AddCommand (390-396). UsageError classify→exit 2 at main.go:105-108. spawn absent from root.go skipTmuxCheck (38-46), as required.
- Notes: The file has since grown to the full Phase 2/3 burst pipeline (runSpawn), which is expected task evolution — the Phase-1 placeholder session-args path noted in the task Do was correctly replaced by runSpawn in later phases, not drift. The task Do specified the --detect branch resolve its detector via `buildSpawnDeps(cmd).Detector`; the implementation instead factors detector resolution into a dedicated `spawnDetector(cmd)` and calls it directly (documented at 256-266). This is a deliberate, cleaner deviation: it avoids constructing the full deps (Connector/Ack/etc.) — which would resolve the tmux client — for a dry-run, while buildSpawnDeps defaults its Detector through the same spawnDetector, so resolution is byte-for-byte identical. No behavioural drift. --detect output correctly routed to cmd.OutOrStdout(); separator is U+00B7 middot echoing the design banner.

TESTS:
- Status: Adequate
- Coverage: cmd/spawn_test.go TestSpawnCommand (31-119) has exactly the four Phase-1 subtests mapping 1:1 to the four ACs: resolved --detect (friendly name + bundle id substrings), NULL --detect ("no host-local terminal"), empty-invocation UsageError, unknown-flag UsageError. AC5 satisfied via bootstrapDeps = nopRunner short-circuit + injected fake TerminalDetector (no tmux dial); spawnDeps restored via t.Cleanup; no t.Parallel. errors.As used for the UsageError assertions. All four edge cases from the task (resolved / NULL / no-args / unknown-flag) covered.
- Notes: Not over-tested — each subtest covers a distinct AC with no redundancy. Would fail if the feature broke (wrong flag wiring → no UsageError; missing detect branch → substrings absent). The one gap: the resolved-terminal test asserts only that "Ghostty" and "com.mitchellh.ghostty" appear as substrings, not the exact `"Ghostty · com.mitchellh.ghostty"` middot-separated line — so a regression that changed the design separator (e.g. to a comma) would go uncaught. Spec deliberately leaves wording unpinned beyond "contains both fields", so this is acceptable-as-designed; a one-line hardening is offered below.

CODE QUALITY:
- Project conventions: Followed. Component logger bound once (spawnLogger = log.For("spawn")); small 1-method seam interface (TerminalDetector); package-level *Deps injection with nil-means-production; SilenceUsage/SilenceErrors + main-owned classify; SetFlagErrorFunc bridge to the existing UsageError→exit-2 path. cmd-test discipline (bootstrapDeps injection under TMUX poison) observed.
- SOLID principles: Good. spawnDetector as a single detector-resolution authority is clean SRP; DI via seams.
- Complexity: Low. The Phase-1 RunE is a flat detect / empty-args / delegate branch.
- Modern idioms: Yes. Fprintln/Fprintf write errors propagated rather than swallowed.
- Readability: Good — thorough intent-revealing comments throughout.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/spawn_test.go:56-61 — the resolved --detect subtest asserts the two fields only as separate substrings; add `strings.Contains(out, "Ghostty · com.mitchellh.ghostty")` to lock the exact U+00B7 middot separator (the deliberate design-copy echo of the banner's `Apple Terminal · com.apple.Terminal`), so a separator regression is caught. Safe test-assertion addition; spec leaves wording unpinned, so this is hardening, not a required fix.
