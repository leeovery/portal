TASK: Emit hook-lookup DEBUG breadcrumb and terminal hydrate: exec INFO in execShellOrHookAndExit (portal-observability-layer-6-1)

ACCEPTANCE CRITERIA:
- nil HookStore → DEBUG hook lookup result=miss (no error attr) then INFO exec target=<shell> args=<shell> hook_present=false, bare shell.
- LookupOnResume error → DEBUG result=error WITH error attr (wrapped, not .Error()), retains WARN, then bare-shell exec INFO.
- miss → DEBUG result=miss then bare-shell exec INFO.
- hit → DEBUG result=hit then INFO exec target=/bin/sh args="sh -c <command>; exec <shell>" hook_present=true.
- hydrate: exec uses target attr (NOT path), parallel to process: exec.
- args verbatim incl embedded quotes.
- exec INFO immediately before cfg.ExecShell, no intervening statement, every path.
- No exec path reaches ExecShell without first emitting the exec INFO.

STATUS: Complete

SPEC CONTEXT:
Spec § Hook-firing observability limit (924-982). Rule 1: DEBUG hook lookup, result ∈ {hit|miss|error} (+ error attr on error per 783). Rule 2: terminal INFO exec target+args+hook_present, parallel to process: exec. Hydrate attr group result/hook_present/bytes; target distinct from reserved path.

IMPLEMENTATION:
- Status: Implemented (no drift)
- Location: cmd/state_hydrate.go:290-325 execShellOrHookAndExit (nil-store DEBUG result=miss :295; lookup-error DEBUG result=error+error :301 + retained WARN :302; miss DEBUG :308; hit DEBUG :312; hit terminal INFO exec immediately before ExecShell("/bin/sh",args) :323-324); :252-262 execShellAndExit (bare-shell terminal INFO target=<shell> args=<shell> hook_present=false :260 before ExecShell :261; all three bare-shell branches funnel here).
- Notes: error attr passes wrapped err directly (:301). target key, never path. args via strings.Join (parallel to process: exec). hit chained = command + "; exec " + shell verbatim. Each branch one DEBUG + funnel guarantees one INFO; no exec path bypasses. Logger *slog.Logger via log.For("hydrate").

TESTS:
- Status: Adequate
- Location: cmd/state_hydrate_exec_log_test.go
- Coverage: NilHookStore_MissThenBareShellExec (no error attr); LookupError (result=error + error attr + retained WARN count==1 + bare exec, dir-as-hooks.json forces real error); UnregisteredPaneKey miss; RegisteredHook_HitThenHookChainExec (target=/bin/sh, joined args, hook_present=true); HitRendersArgsVerbatimIncludingEmbeddedQuotes; ExecInfoUsesTargetAttrNotPath; ExecInfoEmittedImmediatelyBeforeExecShell (table, snapshots sink.Body() inside ExecShell stub).
- Notes: execLogLine matches LEVEL+msg prefix, asserts exactly-one. Behavioural ("immediately before exec" via snapshot, not source inspection). Not over-tested.

CODE QUALITY:
- Project conventions: Followed (*slog.Logger via log.For; no t.Parallel; logtest.Sink; matches process: exec sibling).
- SOLID: Good — execShellAndExit single bare-shell funnel keeps "every exec path emits INFO" in one place; resolveShell shared.
- Complexity: Low.
- Modern idioms: Yes (slog pairs, wrapped error direct, strings.Join).
- Readability: Good — both INFO sites document unbuffered-writer/no-intervening-statement guarantee + process: exec parallel.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] execShellAndExit constructs strings.Join([]string{shell}," ") = just shell; harmless, deliberate for symmetry with hit-branch; a plain shell would be marginally clearer.
- [idea] hydrate: exec emits on every restored pane at INFO (lookup DEBUG filtered); per spec, but a large reboot produces N exec INFOs; awareness for log-volume tuning, no action.
