TASK: Add tmux.HookKeyFormat tmux format string and verify against real tmux (session-rename-orphans-resume-hook-1-2 / tick-148daa)

ACCEPTANCE CRITERIA:
- tmux.HookKeyFormat equals #{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index} byte-for-byte; embedded literal @portal-id matches session.PortalIDOption.
- Real-tmux stamped: @portal-id=tok123 yields 'tok123:0.0'.
- Real-tmux un-stamped: yields '<sessionName>:0.0'.
- Real-tmux multi-window/multi-pane: each read yields same id prefix, distinct :w.p suffix.
- Real-tmux test carries NO build tag and skips cleanly via SkipIfNoTmux where tmux is absent.
- StructuralKeyFormat unchanged; go build succeeds; go test ./internal/tmux/... passes (skips real-tmux where absent).

STATUS: Complete

SPEC CONTEXT: The hook-key derivation must survive a session rename. The mutable session name cannot anchor a hook, so a session is stamped with a frozen @portal-id user-option (Task 1-3). HookKeyFormat is the tmux-resolved sibling of the pure-Go HookKey primitive: it pushes the "id-if-stamped, name-if-not" choice into tmux via a #{?cond,a,b} conditional so the two live-tmux key sites (registration in cmd/hooks.go, stale-cleanup enumeration — both Phase 2) share one canonical format with no Go-side "id absent" branch. Un-stamped sessions (legacy/no-migration) fall to #{session_name}, matching keys already on disk in hooks.json. The spec's Testing Requirements call for a real-tmux round-trip because the conditional's correctness (tmux treating unset/empty @portal-id as false; field resolution) is a live-server property no pure-Go test can prove.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go:849 (const HookKeyFormat); doc block 831-848; StructuralKeyFormat at 829 (unchanged).
- Notes:
  - Constant is byte-exact against the spec value (verified by direct string comparison): "#{?@portal-id,#{@portal-id},#{session_name}}:#{window_index}.#{pane_index}".
  - Embedded "@portal-id" matches session.PortalIDOption (internal/session/create.go:29 = "@portal-id"). Byte-identity is additionally guarded at runtime by TestHookKeyFormatContainsPortalIDLiteral (internal/tmux/hookkey_test.go:29), an un-gated static tripwire that runs without tmux — catches a one-char typo (@portal_id) everywhere the real-tmux guard would skip.
  - StructuralKeyFormat is unchanged: its last touch is a prior unrelated work unit (commit 9dd73f7a, bootstrap-cleanstale...); this task did not modify it.
  - Doc-comment carries the transferred "stable across releases — changing it silently invalidates every hooks.json entry" invariant and the "#{?cond,a,b} treats unset/empty @portal-id as false" note, exactly as the task's Do list specifies.
  - Note: HookKeyFormat is already consumed in production (ResolveHookKey at tmux.go:345 and ListAllPaneHookKeys at 902), so it is NOT orphaned code. The task description framed this task as "format string + guard only, no production caller yet," but sibling tasks 2-x that wire the readers are also complete (commit 83eb4f64). This is expected sequencing, not drift — the constant and its acceptance criteria are fully satisfied.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tmux/hookkey_format_realtmux_test.go — three real-tmux round-trip tests driving the exact production tmux.HookKeyFormat through display-message -p (readHookKey helper), on an isolated socket, no build tag, SkipIfNoTmux-gated:
    - TestHookKeyFormat_StampedSession: stamps @portal-id=tok123 via production SetSessionOption, asserts "tok123:0.0" (id branch).
    - TestHookKeyFormat_UnstampedSession: no stamp, asserts "<sessionName>:0.0" (session_name fallback branch).
    - TestHookKeyFormat_MultiWindowMultiPane: stamped tokMulti, splits pane + adds window, table-driven named subtests asserting tokMulti:0.0 / tokMulti:0.1 / tokMulti:1.0, plus a shared-prefix check and a distinctness (dup) check.
  - Static, tmux-less guard: TestHookKeyFormatContainsPortalIDLiteral (hookkey_test.go) covers byte-identity where the real-tmux tests skip.
- Notes:
  - All acceptance criteria map to an assertion. The three edge cases from the spec (un-stamped -> name branch; stamped -> id:w.p; multi-window/pane distinct suffixes under one id) are each covered by a dedicated test.
  - Tests drive the actual exported constant (tmux.HookKeyFormat), not a copy — so a drift in the constant fails the assertion. Correct closure of the seam.
  - -f /dev/null harness pins base-index/pane-base-index to 0, so the ":0.0"/":1.0" literals are justified (documented in-test at lines 22-23, 70-71).
  - No t.Parallel() (correct: real-tmux tests contend on the shared binary; also mandated by the task).
  - Not over-tested: the prefix-share and distinctness checks in the multi-pane case are non-redundant (they assert different properties than the exact-equality check). The pure-Go HookKey unit tests in hookkey_test.go cover the Go-side derivation separately and are not duplicated here — this file only tests the live-tmux resolution, which is the point.

CODE QUALITY:
- Project conventions: Followed. Black-box package tmux_test; mirrors portal_dir_roundtrip_realtmux_test.go as instructed; named subtests; no t.Parallel(); portalIDLiteral spelled as a literal to avoid the session->tmux import cycle (documented at hookkey_test.go:10-16).
- SOLID principles: N/A (constant + tests). Single, well-scoped responsibility.
- Complexity: Low. readHookKey is a 3-line helper; the multi-pane loop is a straightforward table.
- Modern idioms: Yes. strings.TrimRight for newline, map[string]struct{} for the seen-set, t.Helper() in readHookKey.
- Readability: Good. Doc blocks on the constant and the test file clearly state the invariant and why the seam needs a real-tmux guard.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
