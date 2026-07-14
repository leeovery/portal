AGENT: standards
FINDINGS: none
SUMMARY: Implementation conforms to specification and project conventions.

Reviewed against the full specification and project skills (golang-cli, code-quality,
CLAUDE.md conventions). Every major spec decision point was checked and found faithful:

- Spawn architecture: net-N split (spawn.SplitNetN single-source for CLI + picker),
  self-attach-LAST gated on all N-1 confirming, os.Executable() (not PATH lookup),
  env-self-sufficient argv (/usr/bin/env PATH=…), and the load-bearing TMUX/TMUX_PANE
  strip (composeAttachArgv -u TMUX -u TMUX_PANE) — all present and correct.
- CLI behaviour & exit codes: --detect dry-run, empty-invocation usage error,
  pre-flight-before-unsupported ordering; UsageError→exit 2, all else→exit 1 with the
  shared one-line message on stderr (main.classify) — matches spec Reporting & exit codes.
- Multi-select mode: m enter/toggle, Esc exit+clear, Enter N=0/N=1/N≥2 boundary,
  suppressed k/r/n/x, live s/m/Space/filter (below the SettingFilter guard), sticky
  selection keyed on session identity with de-dupe, filter inner sub-state, burst
  input-lock (only Ctrl-C/Esc live) — all conform.
- Burst & partial-failure: pre-flight all-or-nothing, leave-what-opened selection
  mutation (unmark confirmed, keep failed), explicit token ack (@portal-spawn-<batch>-
  <token> option-safe ids), per-window spawnAckTimeout (8s named constant), sequential
  spawn with permission-required as sole early-stop, cancellation returns to mode.
- Ack delivery: portal attach --spawn-ack validates fast (usage error), checks
  HasSession, writes marker last-before-exec, best-effort (execs even on write failure).
- Detection: inside/outside branch, NULL-filter to local clients, client_activity as
  local-only tiebreak, async once-cached lifecycle (detectDispatched latch, off
  first-paint), in-flight-at-Enter defer via pendingBurstEnter, transient→WARN vs clean
  NULL→INFO, DetectUnsupported resolution-based (not IsNull).
- Adapter/resolver: OpenWindow(command) single capability, closed Outcome taxonomy,
  precedence config→native→unsupported, NULL short-circuits config, config specificity
  tiers (bundle-id > named/alias > glob-by-literals) exactly per spec.
- Config schema: terminals.json tolerant decode with spawn WARNs, exactly-one argv/script,
  argv {command}-presence rule, script direct-exec + exec-bit gate, PORTAL_TERMINALS_FILE
  override registered in configFileComponents ("" component like prefs.json).
- Permissions quarantine: -1743/-1712 mapped only inside ghostty driver; general code
  switches on Outcome; config recipes never produce permission-required.
- Observability: new spawn component, closed attr keys only (batch/terminal/bundle_id/
  resolution/session/ack/opened/total/detail), count semantics (total=N, opened=confirmed
  +trigger-on-success), CLI/picker parity via shared spawn.Log* renderers.
- Notice-band precedence order matches spec (Opening → abort → multi-select → unsupported
  → signpost); the intentional two-row spawn-failure/permission co-render and the picker
  permission DEBUG asymmetry were confirmed as the pre-decided items and deliberately
  not re-flagged.
- Test-lane compliance: real-tmux/real-exec/real-Ghostty tests correctly carry
  //go:build integration or //go:build manual; unit-lane exec_boundary_test uses only
  hermetic sh/echo.
