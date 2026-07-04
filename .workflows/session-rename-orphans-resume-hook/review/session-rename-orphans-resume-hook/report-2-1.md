TASK: Add tmux.ResolveHookKey client read using HookKeyFormat (session-rename-orphans-resume-hook-2-1 / tick-10d1b7)

ACCEPTANCE CRITERIA:
- ResolveHookKey exists and issues exactly one `display-message -p -t <paneID> <HookKeyFormat>` read (uses HookKeyFormat, not StructuralKeyFormat).
- Real-tmux stamped: @portal-id=tok123 -> "tok123:0.0" (base-index 0 under -f /dev/null harness).
- Real-tmux un-stamped: "<sessionName>:0.0".
- Read-failure: display-message read fails -> ("", err) wrapped (recoverable via errors.As/Is), NO synthesized key.
- No Go-side id-absent branch.
- ResolveStructuralKey, StructuralKeyFormat, ListAllPanes, PaneTarget unchanged.
- Real-tmux test carries NO build tag, skips cleanly via SkipIfNoTmux.
- go build succeeds; go test ./internal/tmux/... passes (skips real-tmux where absent).

STATUS: Complete

SPEC CONTEXT:
Spec § "Hook-Key Derivation → Stage 1 — Registration (cmd/hooks.go)" defines the Failure contract: ResolveHookKey is a single display-message read whose stamped-vs-unstamped conditional resolves inside tmux (no Go-side "id absent" branch). If the read itself fails, registration aborts with the error, exactly as ResolveStructuralKey does today — it must NOT synthesize a name-based key on failure, which would silently orphan a stamped session's hook. This task adds the client primitive only; the cmd/hooks.go switch onto it is Task 2-2. HookKeyFormat/HookKey are Phase 1 primitives (tmux.go:818-849).

IMPLEMENTATION:
- Status: Implemented (exact, matches acceptance criteria)
- Location: internal/tmux/tmux.go:331-350 (func ResolveHookKey), placed immediately after ResolveStructuralKey (tmux.go:320-329) as specified.
- Notes:
  - Mirrors ResolveStructuralKey byte-for-byte except the format constant and error string: `c.cmd.Run("display-message", "-p", "-t", paneID, HookKeyFormat)` — exactly one read, HookKeyFormat (not StructuralKeyFormat).
  - On error returns `("", fmt.Errorf("failed to resolve hook key for pane %q: %w", paneID, err))`; else `(output, nil)`. c.cmd.Run trims the trailing newline (verified against RealCommander/socketCommander.Run contract).
  - No Go-side id-absent branch — one read, one error path. Confirmed the stamped/unstamped conditional lives entirely in HookKeyFormat = "#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}" (tmux.go:849).
  - Doc-comment covers all required points: canonical live-read resolver, stamped -> <id>:w.p, un-stamped -> <name>:w.p, read failure aborts and MUST NEVER fall back to a name-based key, cross-references HookKey/HookKeyFormat.
  - Protected functions verified unchanged via `git show d81e9ea0` (the T2-1 impl commit): the tmux.go diff is a pure +21-line addition after line 328; ResolveStructuralKey, StructuralKeyFormat (tmux.go:829), ListAllPanes (tmux.go:869), PaneTarget (tmux.go:579) untouched.

TESTS:
- Status: Adequate
- Location: internal/tmux/resolve_hookkey_realtmux_test.go (package tmux_test, no build tag, SkipIfNoTmux-gated, no t.Parallel()).
- Coverage:
  - TestResolveHookKey_StampedSession: stamps @portal-id=tok123 via production SetSessionOption (portalIDLiteral, shared "@portal-id" const from hookkey_test.go:17, byte-identical to the literal in HookKeyFormat, guarded by TestHookKeyFormatContainsPortalIDLiteral), asserts "tok123:0.0" — proves the id branch is taken and the name is NOT used (rename-immune case).
  - TestResolveHookKey_UnstampedSession: no stamp, asserts "<sessionName>:0.0" — proves the #{session_name} fallback branch.
  - TestResolveHookKey_ReadFailureWrapsError: kills the isolated server (the reliable read-failure path on tmux 3.7, since display-message tolerates a bogus target with ":." exit 0 — sound, well-documented workaround), asserts err != nil, got == "" (no synthesized key), and errors.As recovers *tmux.CommandError (recoverable per the Failure contract). The socketCommander.Run wraps exec failures via tmux.WrapCommandError, so the *CommandError assertion is valid.
- Notes:
  - Each acceptance-criteria edge case (stamped, un-stamped, read-failure) has exactly one focused test. No redundant assertions, no over-mocking (real tmux on isolated socket). Would fail if the feature broke (wrong format constant, a name-based fallback on error, or an empty prefix would each trip an assertion). No under- or over-testing.

CODE QUALITY:
- Project conventions: Followed. Mirrors the established ResolveStructuralKey/ActivePaneCurrentPath client-method pattern; real-tmux round-trip guard matches the sibling hookkey_format_realtmux_test.go / list_all_pane_hookkeys_realtmux_test.go conventions (no build tag, SkipIfNoTmux, no t.Parallel). Error wrapped with %w, lowercase string, no trailing punctuation (golang-error-handling). errors.As used for typed chain inspection (golang-testing/error-handling).
- SOLID principles: Good — single-responsibility single-read primitive; the id-vs-name decision is delegated to tmux (the format string) rather than duplicated in Go, preserving one source of truth with HookKeyFormat.
- Complexity: Low (one read, one error branch).
- Modern idioms: Yes.
- Readability: Good — doc-comment is thorough and states the load-bearing "MUST NEVER synthesize" invariant.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
