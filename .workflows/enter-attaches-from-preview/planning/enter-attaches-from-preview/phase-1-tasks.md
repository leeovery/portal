---
phase: 1
phase_name: Enter binding with pre-select and attach
total: 8
---

## enter-attaches-from-preview-1-1 | approved

### Task 1-1: Add SelectWindow method to tmux.Client

**Problem**: `tmux.Client` exposes `SelectPane(session, window, pane int)` but has no peer for selecting a window without also pinning a pane. The four-call attach pipeline (spec § Pre-select + attach sequence) requires `tmux select-window -t <session>:<window_index>` as a discrete step before `select-pane`, and there is no current entry point for that call. Without a dedicated wrapper, the pipeline would either inline `c.cmd.Run("select-window", ...)` (bypassing the Client abstraction) or fold window selection into `SelectPane` (collapsing the two failure surfaces the spec keeps separate).

**Solution**: Add `SelectWindow(session string, window int) error` to `internal/tmux/tmux.go`, mirroring the shape of the existing `SelectPane` helper — build the `<session>:<window>` target, dispatch via the `Commander.Run` seam, and wrap any non-nil error with the conventional `"failed to select-window <target>: %w"` prefix so the caller sees a `*CommandError` (via `errors.As`) carrying tmux's stderr.

**Outcome**: Callers can issue `client.SelectWindow("work", 2)` and receive a wrapped error on non-zero exit (window absent) or `nil` on success. The error preserves the underlying `*exec.ExitError` so downstream discriminator code can type-assert the exit-vs-OS-error distinction.

**Do**:
- Open `internal/tmux/tmux.go`. Add `SelectWindow` next to `SelectPane` (currently around line 772), keeping the surface alphabetised within its neighbourhood and the godoc style consistent with `SelectPane`.
- Build the target as `fmt.Sprintf("%s:%d", session, window)`. Do NOT prepend the `=` exact-match prefix here — exact-match wrapping is the responsibility of task 1-2, applied uniformly across HasSession / SelectWindow / SelectPane / SwitchClient / AttachConnector. Keeping prefix policy out of this task isolates the surface change from the cross-cutting prefix decision.
- Dispatch `c.cmd.Run("select-window", "-t", target)`. Wrap non-nil errors as `fmt.Errorf("failed to select-window %s: %w", target, err)`.
- Add a godoc comment explaining: best-effort caller semantics live with the caller, not here — `SelectWindow` itself just returns the wrapped error; callers (the attach pipeline in task 1-4) decide whether to swallow or escalate.
- Add a test `TestSelectWindow` in `internal/tmux/tmux_test.go` with the same shape as `TestSelectPane` (around line 2078): success path (zero exit, nil error) and failure path (non-zero exit, wrapped error containing the target string).

**Acceptance Criteria**:
- [ ] `Client.SelectWindow(session, window)` issues `tmux select-window -t <session>:<window>` exactly once via the injected `Commander`.
- [ ] On `Commander.Run` returning nil, `SelectWindow` returns nil.
- [ ] On `Commander.Run` returning a non-nil error, `SelectWindow` returns an error whose `Error()` contains `"failed to select-window"` and the target string, and which unwraps to the original error (verifiable via `errors.As(err, &cmdErr)` against `*tmux.CommandError`).
- [ ] No exact-match prefix is added by this method (target is `<session>:<window>`, not `=<session>:<window>`).

**Tests**:
- `"it issues select-window with the composed target"` — mock Commander captures args, asserts `["select-window", "-t", "work:2"]`.
- `"it returns nil on zero exit"` — mock Commander returns `("", nil)`.
- `"it wraps non-zero exit as CommandError"` — mock Commander returns a `*CommandError`, assertion via `errors.As` and substring check on `Error()`.

**Edge Cases**:
- Window no longer exists: surfaces as a wrapped non-zero exit error. The method itself does not interpret this — it returns the wrapped error and lets the caller (task 1-4) log-and-swallow per spec § Pre-select + attach sequence > step 2.
- Session no longer exists: same shape as window-absent — wrapped non-zero exit error. The proactive `has-session` probe in task 1-3 / 1-4 catches this earlier; `SelectWindow` does not need a separate code path.

**Context**:
> Spec § Pre-select + attach sequence > step 2: "`tmux select-window -t <session>:<window_index>`. Best-effort. Uses the window index preview captured at open and walked with `]`/`[`. Zero exit: proceed. Non-zero exit (window no longer exists): log and swallow."
>
> The spec keeps select-window failure separate from select-pane failure (spec § Pre-select + attach sequence > step 3) so each can be logged independently. Folding them into one helper would lose that granularity.

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Pre-select + attach sequence

---

## enter-attaches-from-preview-1-2 | approved

### Task 1-2: Apply exact-match `=` target prefix uniformly across HasSession, SelectWindow, SelectPane, SwitchClient, and AttachConnector

**Problem**: Tmux's default `-t <session>` resolution matches by prefix: a killed session `"foo"` coexisting with a live `"foo-2"` would silently bind `has-session -t foo` to `"foo-2"`, causing the bail path to be missed and the connector to attach to or auto-create the wrong session. Spec § Pre-select + attach sequence > Exact-match target syntax mandates the `=` exact-match prefix uniformly across the four-call sequence; any single call that drops it re-introduces the prefix-collision hazard. The current `Client.HasSession`, `Client.SelectPane`, `Client.SwitchClient`, the about-to-be-added `Client.SelectWindow`, and the `AttachConnector.Connect` helper in `cmd/open.go` all pass session names verbatim with no `=` prefix.

**Solution**: Update the five call sites to wrap the session name as `"=" + session` (or apply the `=` to the composed target where window/pane suffixes are present) before passing it to `-t`. The change is purely additive at the tmux argv layer — Go-side method signatures and existing callers remain unchanged. Existing non-Enter code paths (server bootstrap, session creation, kill/rename, switch-client from Sessions-page Enter) inherit the prefix automatically and benefit from the same prefix-collision protection.

**Outcome**: Every `-t` flag against a user session uses tmux's exact-match prefix. A killed session named `foo` with a coexisting live `foo-2` no longer silently resolves to `foo-2` for `has-session`, `select-window`, `select-pane`, `switch-client`, or `attach-session`.

**Do**:
- Edit `internal/tmux/tmux.go`:
  - `HasSession(name)` (around line 119): change `c.cmd.Run("has-session", "-t", name)` to `c.cmd.Run("has-session", "-t", "="+name)`.
  - `SelectPane(session, window, pane)` (around line 776): change the target composition to use the `=` prefix on the session segment. Reuse or extend the `PaneTarget` helper (referenced at line 777) so the prefix lives in one place — preferred shape is to update `PaneTarget` itself to emit `=<session>:<window>.<pane>` since every caller of `PaneTarget` issues a `-t` flag against a user session. Audit callers of `PaneTarget` (`ResizePaneZoom` at line 791, restore code, daemon code) and confirm no caller passes a non-session string; if any do, introduce a sibling helper instead.
  - `SwitchClient(name)` (around line 289): change `c.cmd.Run("switch-client", "-t", name)` to `c.cmd.Run("switch-client", "-t", "="+name)`.
  - `SelectWindow(session, window)` (added in task 1-1): emit `=<session>:<window>` as the target.
- Edit `cmd/open.go` `AttachConnector.Connect` (around line 70): change `syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", name}, ...)` to `syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-A", "-t", "="+name}, ...)`. Note the `-A` flag is also added per spec § Pre-select + attach sequence > step 4 ("`tmux attach-session -A -t '=<session>'`") — re-confirm against existing call sites that `-A` is not already present elsewhere; if it is, keep its existing position.
- Update existing tests in `internal/tmux/tmux_test.go` (`TestHasSession`, `TestSwitchClient`, `TestSelectPane`) to assert on `=<session>` / `=<session>:<window>.<pane>` targets rather than bare-name targets. Add a regression test `TestHasSessionUsesExactMatchPrefix` (named explicitly so the intent is searchable in future audits) that asserts the prefix is present.
- Update `cmd/attach_test.go` / `cmd/open_test.go` if any test asserts the exact argv passed to `attach-session` — switch the assertion to expect `-A -t =<name>`.
- Search for any other `Run("...","-t", name)` patterns against user session names in `internal/tmux/` and apply the prefix uniformly. Notably exclude internal-name targets like `_portal-saver` / `_portal-bootstrap` if a different naming policy applies — but in practice the `=` prefix is safe for those too (exact match works for any name), so prefer uniform application unless a test surfaces a regression.

**Acceptance Criteria**:
- [ ] `HasSession("foo")` invokes tmux with args `["has-session", "-t", "=foo"]`.
- [ ] `SelectWindow("foo", 2)` invokes tmux with args `["select-window", "-t", "=foo:2"]`.
- [ ] `SelectPane("foo", 2, 3)` invokes tmux with args `["select-pane", "-t", "=foo:2.3"]`.
- [ ] `SwitchClient("foo")` invokes tmux with args `["switch-client", "-t", "=foo"]`.
- [ ] `AttachConnector.Connect("foo")` exec's tmux with argv `["tmux", "attach-session", "-A", "-t", "=foo"]`.
- [ ] Existing non-Enter callers of these methods (bootstrap, session creation, sessions-page Enter, kill/rename) compile and pass their existing tests against the updated argv expectations.
- [ ] Regression test `TestHasSessionUsesExactMatchPrefix` documents the prefix-collision rationale in its godoc comment.

**Tests**:
- `"HasSession passes the exact-match prefix to tmux"`
- `"SelectWindow composes session:window with exact-match prefix"`
- `"SelectPane composes session:window.pane with exact-match prefix"`
- `"SwitchClient passes the exact-match prefix to tmux"`
- `"AttachConnector exec'd argv includes -A and =name"`
- `"prefix-collision regression: HasSession on killed `foo` does not match live `foo-2`"` — drive a fake commander that returns success only when the literal `=foo` target is passed; assert that bare-name resolution would have falsely matched.

**Edge Cases**:
- Session names containing `=`: tmux treats `=` only as a target-prefix indicator; an in-name `=` becomes part of the literal name. Result: `="foo=bar"` correctly matches a session literally named `foo=bar`. No special handling required.
- Session names containing `:`: tmux already uses `:` as the session/window separator. Existing portal session-name generator (`{project}-{nanoid}`) does not produce names with `:`, so no new collision is introduced.
- Existing non-Enter callers: bootstrap's `_portal-saver` / `_portal-bootstrap` calls inherit the prefix; tests must be updated to expect the new argv shape. Re-run the full test suite after the change, not just tmux/-targeted tests.

**Context**:
> Spec § Pre-select + attach sequence > Exact-match target syntax: "`has-session` and all subsequent `-t <session>` calls (`select-window`, `select-pane`, `attach-session`, `switch-client`) MUST use tmux's exact-match prefix `=` — i.e. `-t '=<session>'` rather than `-t <session>`. Without this, tmux's default target resolution matches by prefix: a killed session 'foo' coexisting with a live 'foo-2' would have `has-session -t foo` return zero (matching 'foo-2'), causing the bail path to be missed and the connector to attach to or auto-create the wrong session."
>
> Spec § Pre-select + attach sequence > step 4: connector argv is `tmux attach-session -A -t '=<session>'` outside tmux and `tmux switch-client -t '=<session>'` inside tmux.

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Pre-select + attach sequence > Exact-match target syntax, § Pre-select + attach sequence > step 4

---

## enter-attaches-from-preview-1-3 | approved

### Task 1-3: Add ExitError discriminator for has-session probe

**Problem**: Spec § Pre-select + attach sequence > step 1 mandates a discriminator: `*exec.ExitError` (non-zero tmux exit — session genuinely absent — bail) MUST be distinguished from non-`ExitError` OS-layer errors (missing binary, exec lookup failure — treat as "session present" and proceed). The current `Client.HasSession` collapses both into a single boolean (returns `false` on any error), losing the distinction the bail path depends on. Without the discriminator, an OS-layer hiccup in `has-session` would falsely trigger the externally-killed-session bail UX while the session is in fact still present.

**Solution**: Add a new `Client.HasSessionProbe(name string) (present bool, err error)` method that returns the structured result. `present == true` means tmux returned zero (session exists). `present == false, err == nil` would never be returned — instead, a non-zero tmux exit yields `(false, *CommandError-wrapping-*exec.ExitError)`, and an OS-layer error yields `(true, *CommandError-wrapping-non-ExitError)` so callers default to "proceed" on OS-layer faults per spec. The boolean conveys the bail decision; callers can still log the err. The existing `HasSession(name) bool` stays in place unchanged for boolean-only callers (bootstrap, portal-saver checks).

**Outcome**: The attach pipeline (task 1-4) calls `HasSessionProbe`, branches on `(present, err)`, and treats only the `present == false && errors.As(err, &exitErr)` shape as the bail signal. OS-layer errors (lookpath failures, exec faults) are reported as `present == true` so the pipeline proceeds and the connector itself surfaces any genuine tmux unavailability.

**Do**:
- Edit `internal/tmux/tmux.go`. Add `HasSessionProbe(name string) (bool, error)` next to `HasSession` (around line 119).
- Implementation:
  - Run `c.cmd.Run("has-session", "-t", "="+name)` (the `=` prefix is added by task 1-2; this task can either land before task 1-2 with the bare name and be updated by 1-2's edit, or land after 1-2 and use the prefix directly — coordinate ordering at implementation time).
  - On `nil` error: return `(true, nil)` (session present).
  - On non-nil error: extract `*CommandError` via `errors.As`. Then test whether the underlying error unwraps to `*exec.ExitError` via `errors.As(cmdErr.Err, &exitErr)` (or equivalently `errors.As(err, &exitErr)` since `*CommandError` `Unwrap`s through). If it does (`*exec.ExitError`), return `(false, err)` — genuine non-zero exit, bail signal. If it does NOT (OS-layer failure: missing binary, fork failure), return `(true, err)` — proceed, the connector will surface the real problem.
- Document the contract in the godoc: explain the three observable shapes — `(true, nil)` = present, `(false, err)` = absent (caller bails), `(true, err)` = OS-layer fault (caller proceeds, err logged as warning at most).
- Leave `HasSession(name) bool` untouched. Existing callers (bootstrap step 4 portal-saver checks, etc.) continue to use the boolean form.
- Add tests in `internal/tmux/tmux_test.go`:
  - `TestHasSessionProbe_Present`: commander returns `("", nil)` → `(true, nil)`.
  - `TestHasSessionProbe_Absent`: commander returns a `*CommandError` whose underlying `Err` is a synthetic `*exec.ExitError` → `(false, err)` and `errors.As(err, &exitErr)` succeeds.
  - `TestHasSessionProbe_OSError`: commander returns a `*CommandError` whose underlying `Err` is a non-ExitError (e.g. `errors.New("exec: not found")` wrapped) → `(true, err)` and `errors.As(err, &exitErr)` fails.

**Acceptance Criteria**:
- [ ] `HasSessionProbe` returns `(true, nil)` on zero exit.
- [ ] `HasSessionProbe` returns `(false, non-nil-err)` when the underlying error unwraps to `*exec.ExitError`; the returned err is the original wrapped error (preserves `*CommandError` shape for stderr inspection).
- [ ] `HasSessionProbe` returns `(true, non-nil-err)` when the underlying error does NOT unwrap to `*exec.ExitError`.
- [ ] Existing `HasSession(name) bool` signature, behaviour, and callers are unchanged. Existing `TestHasSession` tests still pass.

**Tests**:
- `"HasSessionProbe returns (true, nil) when tmux exits zero"`
- `"HasSessionProbe returns (false, err) when tmux exits non-zero"` — assert `errors.As(err, &exitErr)` succeeds.
- `"HasSessionProbe returns (true, err) on OS-layer failure"` — assert `errors.As(err, &exitErr)` fails; this is the "proceed despite error" branch.
- `"HasSession bool form is unaffected"` — existing TestHasSession assertions still pass.

**Edge Cases**:
- `*exec.ExitError` with stderr present (tmux complaining about session not found): the probe still returns `(false, err)`. The caller does not need to inspect the stderr text — exit-vs-OS-error is the only discriminator.
- Commander returns a non-`*CommandError` error type (test mocks that don't wrap): the discriminator should still work — `errors.As` walks any unwrap chain. If the test commander returns a bare `*exec.ExitError` directly, the function should still treat it as the bail signal.
- Commander returns nil error but non-empty stderr (theoretically impossible from the existing `runCommand` shape since it only returns errors when `cmd.Output()` fails, but defensive coding): treat as `(true, nil)`. tmux exit zero is the only signal that matters.

**Context**:
> Spec § Pre-select + attach sequence > step 1: "OS-layer error (missing binary, exec failure) — distinct from a non-zero exit: treat as 'session present' and proceed to step 2. An OS-layer error is not a tmux-state signal; the connector will fail in the same shape it would have without the check, and `EnsureServer` already validates tmux is invocable in bootstrap."
>
> "**Discriminator contract**: build phase MUST distinguish `*exec.ExitError` (non-zero exit — bail) from non-`ExitError` errors (OS-layer failure — proceed). The discriminator mechanism is a build decision (e.g. extend `Commander` return shape, type-assert at call site); the spec-level contract is that the two cases are not collapsed into a single 'any error' branch."
>
> The chosen mechanism for this work unit is a sibling method (`HasSessionProbe`) that returns structured `(present, err)`. The existing boolean `HasSession` stays as a stable convenience for callers that only need yes/no.

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Pre-select + attach sequence > step 1

---

## enter-attaches-from-preview-1-4 | approved

### Task 1-4: Build four-call attach pipeline tea.Cmd factory with bail message and WARN-swallow logger

**Problem**: The four-call attach sequence (`has-session` → `select-window` → `select-pane` → connector) defined in spec § Pre-select + attach sequence is a multi-step pipeline with distinct success/failure semantics per step, exact-ordering constraints, and two terminal outcomes (connector handoff vs externally-killed bail). Authoring it inline in `previewModel.Update` (task 1-6) would mix tmux orchestration with TUI message routing and make the pipeline impossible to unit-test against fake commanders. A standalone factory function returning a `tea.Cmd` keeps the orchestration focused and lets task 1-6 simply dispatch the cmd.

**Solution**: Create a new file `internal/tui/preview_attach.go` (peer of `pagepreview.go`) containing:
- A small `previewAttachPipeline` struct holding the dependencies the pipeline needs: a `HasSessionProbe`-shaped function, `SelectWindow`-shaped function, `SelectPane`-shaped function, a `SessionConnector` (to be defined as a TUI-package interface mirroring `cmd/open.go`'s `SessionConnector`), and a `*state.Logger` (or a small logger interface) for WARN-swallow logging.
- A `previewAttachBailMsg{ Session string }` message type (returned when has-session indicates absence; consumed by the top-level `Model.Update` in task 1-7).
- A `previewAttachErrorMsg{ Err error }` message type for connector errors (preserves existing tea.Quit-on-error semantics; consumed at the top level).
- A factory function `(p *previewAttachPipeline) Run(session string, window, pane int) tea.Cmd` returning a single `tea.Cmd` that performs the four-call sequence in order and returns the appropriate terminal message.

**Outcome**: The pipeline is exercisable in isolation by injecting fake commanders and a captured-output logger, with assertions on call ordering, branch selection (bail vs proceed), and log emission. Task 1-6 wires it into `previewModel.Update` by dispatching `pipeline.Run(session, window, pane)`. Task 1-7 handles the resulting `previewAttachBailMsg`.

**Do**:
- Create `internal/tui/preview_attach.go`.
- Define a TUI-local `SessionConnector` interface with `Connect(name string) error` (or import the `cmd/open.go` interface — but since `internal/tui` cannot import `cmd`, the cleaner approach is a local interface that production code in `cmd/open.go` will implement via a thin adapter wired in task 1-5).
- Define interface(s) for the tmux dependencies. Suggested shape:
  ```go
  type previewAttachTmux interface {
      HasSessionProbe(name string) (bool, error)
      SelectWindow(session string, window int) error
      SelectPane(session string, window, pane int) error
  }
  ```
  In production this is satisfied by `*tmux.Client`. In tests it is satisfied by a fake.
- Define the message types `previewAttachBailMsg{ Session string }` and `previewAttachErrorMsg{ Err error }`. Document in the godoc that `previewAttachBailMsg` carries the captured session name so the bail handler (task 1-7, and Phase 2's flash) can render the exact `<name>` in the user-facing message.
- Define `previewAttachPipeline` struct with fields for the tmux interface, connector, and logger.
- Implement `Run(session string, window, pane int) tea.Cmd`. The returned cmd:
  1. Calls `p.tmux.HasSessionProbe(session)`. If `present == false` (i.e. `errors.As(err, &exitErr)` is true via the discriminator from task 1-3), return `previewAttachBailMsg{ Session: session }` and STOP — do not run select-window, select-pane, or connector. If `present == true` (whether err is nil or an OS-layer error), proceed to step 2; if err is non-nil, log it at WARN through the structured logger before proceeding.
  2. Calls `p.tmux.SelectWindow(session, window)`. On non-nil error, log at WARN through `state.Logger` with a greppable component string — use `state.ComponentTUI` if it exists, otherwise add a new constant `ComponentPreview = "preview"` to `internal/state/logger.go` (small dependency; the spec § Pre-select + attach sequence > step 2 calls this out as a build decision: "build phase picks the exact component string (e.g. ComponentPreview or ComponentTUI)"). Do NOT abort the pipeline. Do NOT return an error message. Proceed to step 3.
  3. Calls `p.tmux.SelectPane(session, window, pane)`. Same WARN-swallow shape as step 2. Proceed to step 4 regardless.
  4. Calls `p.connector.Connect(session)`. The connector outcome:
     - **Outside-tmux `AttachConnector`**: `Connect` is `syscall.Exec` — it does not return on success. A returned error is genuine. Return `previewAttachErrorMsg{ Err: err }` so the top-level handler can surface it (or fold into the existing tea.Quit-on-error path used by `processTUIResult`).
     - **Inside-tmux `SwitchConnector`**: `Connect` returns nil on success and a wrapped error on failure. Return `previewAttachErrorMsg{ Err: nil }` on success (the TUI should quit so `processTUIResult` runs the connector again — or, simpler, set the model's `selected` field via a dedicated success message that quits the TUI; align with how the Sessions-page Enter currently terminates the TUI). Re-check the existing flow in `cmd/open.go` and `internal/tui/model.go` to mirror the established post-connect message shape; if a `SessionSelectedMsg` or equivalent already exists, reuse it.
- Add a logger constant if needed: `internal/state/logger.go` line 30-37, add `ComponentPreview = "preview"` (preferred per the spec's "ComponentPreview or ComponentTUI" suggestion — `preview` is more specific and greppable).
- Create `internal/tui/preview_attach_test.go` with exhaustive tests against fake commander/connector/logger:
  - All four calls fire in order on the success path.
  - has-session bail: only step 1 runs; pipeline returns `previewAttachBailMsg{Session: "foo"}`; no select-window, no select-pane, no connector.
  - has-session OS-layer error: pipeline proceeds through all four steps; the OS-layer err is logged at WARN.
  - select-window error: logged at WARN with `ComponentPreview`; pipeline proceeds to select-pane and connector.
  - select-pane error: logged at WARN with `ComponentPreview`; pipeline proceeds to connector.
  - Both selects error: both logged; connector still fires.
  - Connector error: returned as `previewAttachErrorMsg{Err: err}`.
  - Inside-tmux vs outside-tmux: parameterise the connector fake to verify the pipeline is connector-agnostic.

**Acceptance Criteria**:
- [ ] `previewAttachPipeline.Run` returns a non-nil `tea.Cmd`.
- [ ] On the success path, the cmd invokes HasSessionProbe, SelectWindow, SelectPane, then connector.Connect — in that exact order — exactly once each.
- [ ] On `(present=false, *exec.ExitError)` from HasSessionProbe, the cmd returns `previewAttachBailMsg{Session: <name>}` and DOES NOT invoke SelectWindow, SelectPane, or connector.
- [ ] On `(present=true, OS-layer-err)` from HasSessionProbe, the cmd logs at WARN with `ComponentPreview` and proceeds.
- [ ] SelectWindow non-zero exit logs at WARN with `ComponentPreview` and pipeline proceeds.
- [ ] SelectPane non-zero exit logs at WARN with `ComponentPreview` and pipeline proceeds.
- [ ] Connector error is returned as `previewAttachErrorMsg{Err: err}`.
- [ ] No call passes a `nil`-receiver session/window/pane combo through silently — empty session bails out before any tmux call (defensive guard).
- [ ] The pipeline performs NO structural enumeration on Enter — no `list-panes`, no `list-windows`, no `list-sessions`, no `display-message -p`, and no other tmux call shape beyond the four spec-pinned commands (`has-session`, `select-window`, `select-pane`, and the connector's `attach-session` / `switch-client`). Verified by asserting the fake commander's recorded call list contains exactly those argv prefixes and no others.

**Tests**:
- `"pipeline runs has-session, select-window, select-pane, connector in order on success"`
- `"pipeline returns previewAttachBailMsg when has-session reports absent"` — and verifies the session name is preserved in the message.
- `"pipeline does not invoke selects or connector after a bail signal"`
- `"pipeline proceeds and logs on has-session OS-layer error"`
- `"pipeline logs WARN with ComponentPreview when select-window fails"`
- `"pipeline logs WARN with ComponentPreview when select-pane fails"`
- `"pipeline returns connector error as previewAttachErrorMsg"`
- `"pipeline forwards connector choice (Attach vs Switch) without orchestration changes"` — runs the same fixture with two connector implementations.
- `"pipeline does not invoke list-panes, list-windows, or any other enumeration on the success path"` — fake commander records every argv; assert the recorded set is exactly `{has-session, select-window, select-pane, attach-session-or-switch-client}` with no `list-*` or `display-message` calls.
- `"pipeline does not invoke list-panes, list-windows, or any other enumeration on the bail path"` — bail path runs only `has-session`; assert no enumeration calls follow.

**Edge Cases**:
- Logger is nil (test harness without a logger): the pipeline must not panic. Use the `state.Logger` "nil-receiver is a no-op" contract (see `internal/state/logger.go` line 58: "A nil *Logger is a valid no-op: all methods bail early").
- has-session returns `(false, nil)` (logically impossible per task 1-3's contract but defensive): treat as bail (present=false dominates).
- Connector handoff via syscall.Exec: the outside-tmux connector never returns on success. The cmd must therefore allow the goroutine to be terminated by exec without leaking goroutine state — there is nothing to clean up after a successful exec.
- The pipeline runs synchronously inside the tea.Cmd goroutine; tmux calls are blocking. This is acceptable — sub-millisecond locally per spec § Pre-select + attach sequence > step 1.

**Context**:
> Spec § Pre-select + attach sequence > Transition mechanics: "The pre-select and attach sequence is issued **as one logical unit from preview's `Update`**. No intermediate render. No round-trip to the Sessions page. ... Implementation shape (e.g. `tea.Sequence` vs a single combined connector wrapper) is a build-phase detail. The spec-level constraint is: the four-call sequence (`has-session` → `select-window` → `select-pane` → connector) must complete in order, with selects completing before the connector hands off the terminal."
>
> Spec § Pre-select + attach sequence > step 2/3 log shape: "swallowed failures log at WARN through the existing structured logger (`internal/state`), consistent with how bootstrap logs similar best-effort failures. Build phase picks the exact component string (e.g. `ComponentPreview` or `ComponentTUI`); the spec-level contract is WARN-level + structured-logger + greppable component, not silent."
>
> Spec § Pre-select + attach sequence > Hook firing: "The pre-select sequence does not trigger any tmux hook events ... This feature does not change hook semantics." — no hook orchestration logic in the pipeline.

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Pre-select + attach sequence

---

## enter-attaches-from-preview-1-5 | approved

### Task 1-5: Wire attach pipeline seam into tuiConfig, tui.Model, and openTUI production construction

**Problem**: Task 1-4 produces `previewAttachPipeline`, but the TUI model has no field holding it and no way for `previewModel.Update` (task 1-6) to dispatch it. The wiring must follow the established TUI dependency-injection pattern: the pipeline is a constructor-injected seam on `tui.Model`, exposed via a `tui.Option` (mirroring `WithEnumerator` / `WithScrollbackReader`), wired through `cmd/open.go`'s `tuiConfig` and `buildTUIModel`, and constructed in production from a `*tmux.Client` + the right `SessionConnector` (Attach vs Switch).

**Solution**: Add the pipeline as a `tui.Model` field with a `WithPreviewAttachPipeline` option, extend `tuiConfig` and `buildTUIModel` in `cmd/open.go` to thread it through, and construct the production pipeline in `openTUI` using `tmux.DefaultClient()` (or the existing `tmuxClient(cmd)` helper) plus the connector returned by `buildSessionConnector(client)`. Tests inject a fake pipeline via the same option; the pre-existing `Deps` struct pattern is not needed here because the pipeline itself is already a small interface.

**Outcome**: Production `openTUI` constructs a real pipeline backed by `*tmux.Client` (HasSessionProbe / SelectWindow / SelectPane), the appropriate `SessionConnector`, and the production logger; the pipeline is passed into `tui.Model` via `WithPreviewAttachPipeline`. Tests construct a `tui.Model` with a fake pipeline and assert dispatch behaviour from `previewModel.Update`.

**Do**:
- Edit `internal/tui/model.go`:
  - Add a field `previewAttachPipeline *previewAttachPipeline` (or use a small interface type if the model file should not directly depend on the concrete struct — preferred: define `type PreviewAttacher interface { Run(session string, window, pane int) tea.Cmd }` so the seam is interface-typed and cleanly mockable).
  - Add a `tui.Option`: `WithPreviewAttachPipeline(p PreviewAttacher) Option { return func(m *Model) { m.previewAttachPipeline = p } }` — placed adjacent to `WithEnumerator` / `WithScrollbackReader` (around line 455-470).
  - Update the `previewModel` constructor invocation in the Space-handler (around line 1282 of `internal/tui/model.go`) to pass the pipeline through to `previewModel` so its `Update` (task 1-6) can dispatch it. The cleanest shape is to add a `PreviewAttacher` field on `previewModel` itself, set at construction time. Update `NewPreviewModel`'s signature accordingly: `NewPreviewModel(session string, enumerator TmuxEnumerator, reader ScrollbackReader, attacher PreviewAttacher, width, height int) (previewModel, bool)`. Update existing `NewPreviewModel` callers in tests (`internal/tui/pagepreview_*.go` test files) to pass a nil or fake attacher.
- Edit `cmd/open.go`:
  - Add field `previewAttacher tui.PreviewAttacher` (or the concrete pipeline type if exported) to `tuiConfig` (around line 283).
  - In `buildTUIModel`, append `tui.WithPreviewAttachPipeline(cfg.previewAttacher)` to the options list when non-nil.
  - In `openTUI` (around line 380-395), construct the pipeline:
    ```go
    connector := buildSessionConnector(client)
    attacher := tui.NewPreviewAttachPipeline(client, connector, productionLogger)
    cfg.previewAttacher = attacher
    ```
    Note: `buildSessionConnector` is currently called AFTER the TUI runs (around line 429). Move or duplicate the call to obtain the connector before building the cfg. Re-examine the flow: today's `processTUIResult(model, connector)` re-invokes the connector after the user selects from the TUI. With this feature, the connector is also invoked from inside the TUI (via the pipeline) for preview-Enter. The post-TUI `processTUIResult` flow remains for Sessions-page Enter (which does not go through the pipeline). The connector instance can be shared between both paths — `&AttachConnector{}` is stateless and `&SwitchConnector{client}` is safe to share.
  - Define `tui.NewPreviewAttachPipeline(...)` as the exported constructor in `internal/tui/preview_attach.go` (added in task 1-4 but kept unexported; export the constructor here). The pipeline struct can stay unexported; only the constructor and the message types need to be exported.
  - Resolve the production logger: state has a package-level logger pattern — see `internal/state/logger.go` `OpenLogger`. Either use the same logger instance the daemon uses (resolved via `state.Dir() + "/portal.log"`) or open a fresh appendable logger. Match whatever pattern existing TUI logging code uses; if no TUI logger exists today, open one in `openTUI` after `state.Dir()` resolves (around line 376) and inject. If logger-opening can fail, swallow the error and pass nil — the pipeline tolerates a nil logger per task 1-4.
- Update test scaffolding:
  - Add a `fakePreviewAttacher` struct to whichever test helper file is appropriate (`internal/tui/model_test.go` or a new `internal/tui/preview_attach_helpers_test.go`) recording calls with `(session, window, pane)`.
  - Update existing `NewPreviewModel` callers in test files to pass a nil attacher where the test does not exercise Enter — verify each call site still compiles.
  - Add a smoke test in `cmd/open_test.go` or equivalent that asserts the production `openTUI` path constructs a non-nil attacher (use `bootstrapDeps`-style injection if needed).

**Acceptance Criteria**:
- [ ] `tui.Model` exposes a `WithPreviewAttachPipeline(PreviewAttacher) Option` constructor option.
- [ ] `tuiConfig` carries a `previewAttacher tui.PreviewAttacher` field.
- [ ] `buildTUIModel` passes the attacher through when non-nil.
- [ ] `openTUI` production path constructs a real pipeline backed by the correct `*tmux.Client` and `SessionConnector` (Attach outside tmux, Switch inside tmux) and injects it.
- [ ] Tests can inject a fake `PreviewAttacher` via `tui.WithPreviewAttachPipeline(fake)` and verify it was dispatched.
- [ ] All existing `NewPreviewModel` callers compile against the updated signature; existing tests still pass.

**Tests**:
- `"openTUI constructs preview attach pipeline backed by AttachConnector outside tmux"` — assert via injected fakes.
- `"openTUI constructs preview attach pipeline backed by SwitchConnector inside tmux"` — set `tmux.InsideTmux()` fake, assert connector type.
- `"WithPreviewAttachPipeline wires the attacher onto Model"` — direct unit test on Model construction.
- `"NewPreviewModel propagates attacher onto previewModel"` — construct a previewModel with a fake attacher, assert the field is populated.

**Edge Cases**:
- Production logger fails to open: `openTUI` should swallow and pass nil — the pipeline's nil-logger no-op contract (task 1-4) prevents a nil-pointer crash.
- Inside-tmux detection (`tmux.InsideTmux()`) is determined at TUI startup; if the user somehow transitions to outside-tmux mid-session (impossible in practice), the pipeline keeps the originally-resolved connector. No re-detection mid-flight.
- `processTUIResult` post-TUI flow: must continue to work for Sessions-page Enter (today's path). The pipeline-driven preview-Enter path either causes a process exec (outside tmux, `processTUIResult` never reached) or sets the model `selected` (inside tmux, `processTUIResult` runs the connector again — this is benign duplication if the connector is idempotent for `switch-client`; verify and adjust if a redundant switch-client causes test failures).

**Context**:
> Spec § Pre-select + attach sequence > step 4: "The existing connector path runs, unchanged: Outside tmux: AttachConnector ... Inside tmux: SwitchConnector ..." — this task wires the pipeline to use the existing `SessionConnector` interface from `cmd/open.go`, preserving connector branch semantics.
>
> The TUI option pattern is established in `internal/tui/model.go` lines 383-470. New options must follow the same shape: a free function `WithFoo(...)` returning an `Option` that mutates the Model.

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Pre-select + attach sequence > step 4

---

## enter-attaches-from-preview-1-6 | approved

### Task 1-6: Intercept tea.KeyEnter in previewModel.Update and dispatch pipeline with raw indices

**Problem**: `previewModel.Update` (`internal/tui/pagepreview.go` line 257-317) currently has no `tea.KeyEnter` case. `Enter` falls through to the embedded `bubbles/viewport`, where it is a no-op for scrolling — meaning the user must press `Esc` and then `Enter` on the Sessions list to attach. Spec § Enter binding behaviour mandates that preview's `Update` intercept `Enter`, dispatch the four-call pipeline against the captured-then-walked `(window, pane)` raw indices, and NOT forward the key to the viewport.

**Solution**: Add a `tea.KeyEnter` case to `previewModel.Update` that calls `m.currentRawIndices()` (existing helper at line 118-121) for the raw `windowIndex`/`paneIndex`, dispatches the previously-injected `PreviewAttacher.Run(m.session, windowIndex, paneIndex)`, and returns the resulting `tea.Cmd`. The case returns from `Update` immediately so the key event does not propagate to the viewport.

**Outcome**: Pressing `Enter` on the preview page returns a `tea.Cmd` from the pipeline (task 1-4) that performs the four-call sequence. When the user has navigated with `]`/`[`/`Tab`, the raw indices reflect the navigation; when they have not navigated, the raw indices reflect the captured-at-open (window, pane). `Enter` is fully intercepted — `bubbles/viewport.Update` is not called for the Enter event.

**Do**:
- Edit `internal/tui/pagepreview.go` `Update` method (line 257-317).
- Add a new case inside the `tea.KeyMsg` switch, peer to `tea.KeyEsc` / `tea.KeyHome` / `tea.KeyEnd` / `tea.KeyTab`:
  ```go
  case tea.KeyEnter:
      if m.attacher == nil {
          return m, nil
      }
      windowIndex, paneIndex := m.currentRawIndices()
      return m, m.attacher.Run(m.session, windowIndex, paneIndex)
  ```
  Note the field name `m.attacher` matches the field added to `previewModel` in task 1-5.
- Document the case with a godoc comment explaining the spec rationale: Enter is intercepted (not forwarded to viewport) so future bubbles/viewport semantics for Enter cannot leak into preview; raw indices are used (not slice positions) per spec § Captured coordinate values.
- Defensive guard for nil attacher: if `m.attacher` is nil (test harness without injection), return `(m, nil)` instead of panicking. This preserves the test ergonomics where many existing preview tests do not exercise the attach path.
- Add a unit test in `internal/tui/pagepreview_enter_test.go` (new file):
  - `"Enter dispatches the attach pipeline with captured-at-open indices when user has not navigated"` — construct previewModel with a fake attacher, send `tea.KeyEnter`, assert the attacher was called with the (windowIndex, paneIndex) of the first window/pane.
  - `"Enter dispatches the attach pipeline with walked indices after Tab"` — drive Tab once, then Enter, assert the pane index changed.
  - `"Enter dispatches with raw tmux indices on non-contiguous index sessions"` — construct an enumerator returning `WindowGroup` slices with `WindowIndex: 5, PaneIndices: [2, 4]`. After zero navigation, Enter must fire with (5, 2). After Tab, Enter must fire with (5, 4). After `]` cycling to a second window with `WindowIndex: 7, PaneIndices: [3]`, Enter must fire with (7, 3) — proving slice-position math is not being used.
  - `"Enter is not forwarded to the embedded viewport"` — wrap the viewport with a recording shim or assert that no viewport mutation occurs (e.g. capture viewport YOffset before and after, must be equal).
  - `"Enter is a no-op when attacher is nil"` — defensive test.
- Verify the existing `pagepreview_audit_test.go` and `pagepreview_chrome_*` tests still pass — none should reference Enter behaviour, so updating them should not be needed. If `pagepreview_surface_audit_test.go` enumerates the owned key surface, update its expected list to include `KeyEnter`.

**Acceptance Criteria**:
- [ ] `previewModel.Update` returns `(m, attacher.Run(session, w, p))` for `tea.KeyEnter` with raw indices from `currentRawIndices()`.
- [ ] The Enter case returns BEFORE the `viewport.Update` delegation at the bottom of the function — i.e. viewport is not updated for the Enter event.
- [ ] When the user has not navigated, raw indices match the captured-at-open `(WindowIndex, PaneIndices[0])` of the first group.
- [ ] When the user has navigated via `]`/`[`/`Tab`, raw indices reflect the walked focus.
- [ ] On a session with non-contiguous `window_index` (e.g. 0, 2, 5) or non-zero `pane-base-index`, the dispatched indices are the raw tmux values, not slice positions.
- [ ] When `m.attacher` is nil, Enter is a silent no-op (returns `(m, nil)`).
- [ ] Enter dispatches the attach pipeline unconditionally regardless of viewport content state — real-bytes, `(nil, nil)` placeholder, and OS-level read error all produce identical dispatch behaviour. No confirmation prompt, no viewport-state guard.

**Tests**:
- `"Enter dispatches with captured-at-open raw indices when user has not navigated"`
- `"Enter dispatches with walked indices after Tab"`
- `"Enter dispatches with walked indices after ]"`
- `"Enter dispatches with raw tmux indices on non-contiguous-index session"`
- `"Enter is intercepted and not forwarded to viewport"`
- `"Enter is a no-op when attacher is nil"`
- `"Enter dispatches the pipeline when viewport rendered real bytes"` — construct previewModel with a ScrollbackReader returning real bytes, send Enter, assert attacher was called.
- `"Enter dispatches the pipeline when viewport rendered the (no saved content) placeholder"` — reader returns `(nil, nil)`, send Enter, assert attacher was called.
- `"Enter dispatches the pipeline when viewport rendered an OS read error"` — reader returns `(nil, errors.New("EIO"))`, send Enter, assert attacher was called.

**Edge Cases**:
- Non-contiguous `window_index` (e.g. after a window kill): captured `WindowIndex` from `WindowGroup` is preserved. The test fixture must use a non-trivial `WindowIndex` (e.g. 5) to prove slice-position is NOT used.
- Non-zero `pane-base-index` (tmux setting): same — `PaneIndices[]` carries the raw values, the test must use 1-based or higher pane indices.
- Degenerate session (one window, one pane): Tab/]/[ are silent no-ops per existing preview behaviour. Enter still dispatches with the captured `(WindowIndex, PaneIndices[0])`.
- Empty preview (theoretically impossible since `NewPreviewModel` returns `(zero, false)` on empty enumeration, blocking the Space handler from setting `pagePreview`): defensive — `currentRawIndices()` would index an empty slice. Since Space-to-preview is gated on `NewPreviewModel` returning ok=true, this case is not reachable in production. No additional guard needed.
- Future bubbles/viewport binding `Enter`: the intercept ensures preview owns Enter regardless of viewport keymap evolution. Test "Enter is intercepted" captures this — drift would make the test fail.

**Context**:
> Spec § Enter binding behaviour: "Preview's `Update` handler gains a `tea.KeyEnter` case. When the user presses `Enter` while preview is the active page, preview commits an attach to the previewed session, applying any `(window, pane)` focus the user navigated to inside preview before handing off to the existing connector path. `Enter` is **intercepted** by preview's `Update` handler and is NOT forwarded to the embedded viewport."
>
> Spec § Enter binding behaviour > What Enter commits to: "Window: the window index the user navigated to via `]`/`[`, defaulting to the captured window if the user did not navigate. Pane: the pane index the user navigated to via `Tab`, defaulting to the captured pane if the user did not navigate."
>
> Spec § Pre-select + attach sequence > Captured coordinate values — raw tmux indices, not slice positions: "The captured `(window, pane)` values passed to `select-window -t <session>:<window_index>` and `select-pane -t <session>:<window_index>.<pane_index>` MUST be raw tmux `window_index` and `pane_index` values ... not 0-based slice positions in the captured enumeration. ... The existing `WindowGroup` enumeration shape ... already preserves raw `WindowIndex` and `PaneIndices[]int`. Preview's existing `currentRawIndices()` helper (`internal/tui/pagepreview.go`) already distinguishes raw indices from slice cursors; the pre-select sequence must use the raw values from that helper (or equivalent)."

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Enter binding behaviour, § Pre-select + attach sequence > Captured coordinate values

---

## enter-attaches-from-preview-1-7 | approved

### Task 1-7: Handle previewAttachBailMsg in top-level Model.Update with placeholder bail (transition + refresh, no flash)

**Problem**: When the pipeline (task 1-4) emits `previewAttachBailMsg`, no top-level handler consumes it — the message would be dropped and the user would be stranded on the preview page with no feedback. Spec § Session-killed-externally bail path mandates the bail dispatch a refresh-and-bail action that transitions `pagePreview → pageSessions` and triggers the existing dismiss-time sessions-list refresh. The full inline-flash UX lands in Phase 2; this task lands the **placeholder bail** that handles the transition + refresh half so Phase 1 is shippable in its own right (the user lands on a refreshed Sessions list with the killed session removed, but with no flash text yet).

**Solution**: Add a `case previewAttachBailMsg` to the top-level message switch in `Model.Update` (`internal/tui/model.go` around line 877, where `previewDismissedMsg` is handled). The handler mirrors the `previewDismissedMsg` shape: capture the session name, transition `activePage` to `PageSessions`, zero out `m.preview`, and dispatch `m.refreshSessionsAfterPreviewCmd(preserveName)`. Phase 2 will extend this handler with flash emission.

**Outcome**: When the pipeline reports the previewed session is gone, the TUI flips back to the Sessions page and the existing live re-fetch removes the killed session from the list. The user lands on a clean Sessions list with no error message (Phase 2 adds the flash). Existing `Esc` dismiss path is untouched — it still emits `previewDismissedMsg` which still triggers the same refresh.

**Do**:
- Edit `internal/tui/model.go` `Update` method, in the cross-view message switch (line 810-912).
- Add a new case adjacent to `previewDismissedMsg`:
  ```go
  case previewAttachBailMsg:
      // Placeholder bail handler. Phase 2 extends this to emit the inline
      // flash ("session \"<name>\" no longer exists"). For Phase 1, the
      // bail performs the transition + refresh half — the user lands on
      // a refreshed Sessions list with the killed session absent, but
      // without a flash explanation.
      preserveName := msg.Session
      m.activePage = PageSessions
      m.preview = previewModel{}
      return m, m.refreshSessionsAfterPreviewCmd(preserveName)
  ```
- Note the use of `msg.Session` (carried on the message from task 1-4) rather than `m.preview.session`. Reading from `msg` directly is more robust against future preview-state mutations between dispatch and handler.
- Verify the existing `previewDismissedMsg` handler (line 877-898) is **unchanged** — the bail handler must be a peer, not a replacement. Tests for Esc dismiss must still pass.
- Verify the existing `previewSessionsRefreshedMsg` handler (line 899-911) handles refresh results correctly in both contexts (Esc dismiss AND bail dismiss). Since both paths dispatch the same `refreshSessionsAfterPreviewCmd`, the resulting `previewSessionsRefreshedMsg` handler does not need to discriminate the source.
- Add a `previewAttachErrorMsg` handler too if task 1-4 emits one — minimal handling: log the error and quit (mirrors today's tea.Quit-on-fatal pattern). Keep this case small; connector errors are rare and the existing `processTUIResult` flow already surfaces them post-TUI for the Sessions-page Enter path.
- Add tests in `internal/tui/model_test.go` (or a new `internal/tui/preview_attach_bail_test.go`):
  - `"previewAttachBailMsg transitions to PageSessions"`
  - `"previewAttachBailMsg dispatches the sessions-refresh cmd"` — execute the returned cmd against a fake lister, assert it produces a `previewSessionsRefreshedMsg`.
  - `"previewAttachBailMsg zeros out preview state"` — model.preview is the zero value after.
  - `"previewAttachBailMsg preserves session name into refresh"` — fake lister returns session list excluding the bailed name; reanchor lands on a clamped neighbour.
  - `"existing Esc dismiss path still triggers refresh unchanged"` — regression.
  - `"refresh cmd error after bail is tolerated silently"` — same swallow-on-error contract as `previewDismissedMsg` path.

**Acceptance Criteria**:
- [ ] `Model.Update` has a `case previewAttachBailMsg` that flips `activePage` to `PageSessions`, zeros `m.preview`, and returns the refresh cmd.
- [ ] The bail handler reads the session name from `msg.Session` (not `m.preview.session`).
- [ ] Existing `previewDismissedMsg` handler is unchanged (byte-equivalent to pre-task code apart from formatting).
- [ ] Existing `previewSessionsRefreshedMsg` handler handles refresh results from both Esc and bail paths without discrimination.
- [ ] Bail handler does NOT emit any flash — Phase 2 owns flash emission.
- [ ] Refresh cmd is non-nil when a session lister is wired; nil when not (consistent with `refreshSessionsAfterPreviewCmd` line 677-690).

**Tests**:
- `"previewAttachBailMsg flips activePage to PageSessions"`
- `"previewAttachBailMsg dispatches refreshSessionsAfterPreviewCmd"`
- `"previewAttachBailMsg zeros m.preview"`
- `"previewAttachBailMsg preserves session name into refresh message"`
- `"Esc dismiss path is unchanged after adding bail handler"` — regression on `previewDismissedMsg`.
- `"refresh-after-bail tolerates lister error silently"`

**Edge Cases**:
- Bail arrives while preview was mid-transition (e.g. user pressed Enter then Esc rapidly): `previewDismissedMsg` and `previewAttachBailMsg` may both reach the queue. Both flip to PageSessions and dispatch a refresh; the second handler's `m.preview = previewModel{}` is a no-op (already zero). Two refresh cmds may run concurrently; the second `previewSessionsRefreshedMsg` overwrites the first via `applySessions`. Acceptable — both refreshes converge on the live state.
- Bail arrives while session lister is nil (test harness): `refreshSessionsAfterPreviewCmd` returns nil; the bail still flips to PageSessions and zeros preview. No crash.
- Bail message session name is empty (defensive — should not happen since pipeline only emits with a real name): the refresh dispatches with empty preserveName, `reanchorSessionCursor` returns early on empty name (line 700-703). Acceptable.

**Context**:
> Spec § Session-killed-externally bail path > Behaviour: "On non-zero `has-session` exit, preview dispatches a refresh-and-bail message that: 1. Transitions `pagePreview → pageSessions` — the same page-state transition that `Esc` performs today. 2. Triggers the existing sessions-list refresh on that transition. ... 3. Emits an inline flash message — one ephemeral line pinned above the Sessions list."
>
> Phase 1 task table notes Phase 1 lands the "placeholder bail (transition + refresh, no flash)". Phase 2 owns flash emission, replacement semantics, tick auto-clear, and keystroke clear. The handler shape in this task must therefore be extensible — Phase 2 adds flash state mutation and a tick cmd to the same case body.
>
> Existing dismiss handler at line 877-898 is the structural template — bail is a peer with the same shape, differing only in the source message type and the use of `msg.Session` instead of `m.preview.session` (defensive against preview-state drift).

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Session-killed-externally bail path

---

## enter-attaches-from-preview-1-8 | approved

### Task 1-8: Update preview chromeLine to include `enter attach` token between `tab next pane` and `esc back`

**Problem**: The preview chrome line (`internal/tui/pagepreview.go` `chromeLine` method, line 163-173) currently advertises `] next win · [ prev win · tab next pane · esc back` — there is no `enter attach` discoverability hint. Users cannot tell from the UI that Enter is now bound. Spec § Discoverability mandates the exact new token placement and wording — between `tab next pane` and `esc back`.

**Solution**: Update the format string in `chromeLine` to insert `· enter attach` between `tab next pane` and `esc back`. The chrome wording is unconditional — it does not branch on viewport content state (real bytes / `(no saved content)` / `(unable to read scrollback)`) per spec § Token wording is unconditional.

**Outcome**: The chrome line reads exactly:
```
Window {w} of {wN} · Pane {p} of {pN} · win: {name}    ] next win · [ prev win · tab next pane · enter attach · esc back
```
across all viewport content states.

**Do**:
- Edit `internal/tui/pagepreview.go` `chromeLine` method (line 163-173).
- Update the format string from:
  ```
  "Window %d of %d · Pane %d of %d · win: %s    ] next win · [ prev win · tab next pane · esc back"
  ```
  to:
  ```
  "Window %d of %d · Pane %d of %d · win: %s    ] next win · [ prev win · tab next pane · enter attach · esc back"
  ```
- Verify the existing `pagepreview_chrome_test.go` (chrome rendering tests) — update the expected strings in any assertion that pins the full chrome line. Add a new test `"chromeLine includes enter attach token between tab and esc"` if not already covered.
- Verify `pagepreview_chrome_stability_test.go` — if it tests chrome stability across cycles, update its golden expectations to include the new token.
- Add (or extend) a test asserting wording is unconditional across viewport states:
  - `"chromeLine wording is identical when scrollback returns real bytes"`
  - `"chromeLine wording is identical when scrollback returns (nil, nil) placeholder"`
  - `"chromeLine wording is identical when scrollback returns OS error"`
  All three must produce byte-identical chrome strings (the placeholder/error live inside the viewport, not the chrome).
- Verify the Sessions-page help bar (`internal/tui/model.go` View / list help) is **unchanged**. Add a regression assertion if convenient — the help bar should not gain `enter attach` since Sessions-page Enter has its own pre-existing semantic (attach the highlighted session).

**Acceptance Criteria**:
- [ ] `chromeLine()` returns a string containing `· enter attach · esc back` (with the `enter attach` token between `tab next pane` and `esc back`).
- [ ] The `enter attach` token sits in the exact position specified — between `tab next pane` and `esc back`, not elsewhere.
- [ ] The chrome string is byte-identical regardless of viewport content state (real bytes / placeholder / error).
- [ ] The Sessions-page help bar / chrome is unaffected — no `enter attach` token added there.
- [ ] All existing chrome tests updated to the new expected string and pass.

**Tests**:
- `"chromeLine includes enter attach token between tab and esc"` — substring match on `· tab next pane · enter attach · esc back`.
- `"chromeLine token order matches spec"` — full string equality against the spec-pinned format.
- `"chromeLine wording unaffected by viewport bytes content"` — render with bytes=non-empty, assert chrome equals expected.
- `"chromeLine wording unaffected by (nil,nil) placeholder"` — render with reader returning `(nil, nil)`, assert chrome equals expected.
- `"chromeLine wording unaffected by OS error"` — render with reader returning `(nil, errors.New("..."))`, assert chrome equals expected.
- `"Sessions page help bar does not include enter attach token"` — render Sessions page View(), assert no `enter attach` substring.

**Edge Cases**:
- Degenerate session (one window, one pane): `]` / `[` / `Tab` are silent no-ops, but the chrome still advertises them per existing behaviour. The new `enter attach` token similarly always renders — degenerate sessions still have a meaningful Enter (attach to the only window/pane). No conditional rendering.
- Very narrow terminal: chrome may visually wrap. The format string itself is unaffected; rendering is the embedding TUI's responsibility. Tests assert on the format-string output, not on rendered terminal output.
- Window name containing special characters (pipes, escapes): handled by the existing `windowName` placeholder — this task does not change name-rendering semantics.

**Context**:
> Spec § Discoverability > New chrome line:
> ```
> Window {w} of {wN} · Pane {p} of {pN} · win: {name}    ] next win · [ prev win · tab next pane · enter attach · esc back
> ```
> "The `enter attach` token sits between `tab next pane` and `esc back`. Exact token placement and wording is fixed by this spec."
>
> Spec § Discoverability > Token wording is unconditional: "The `enter attach` token reads identically regardless of viewport content state (real bytes, '(no saved content)' placeholder, or OS read error). Enter's semantics are identical in all three cases — it attaches to the session, not to the scrollback — so the chrome wording does not branch on viewport state."
>
> Spec § Discoverability > Sessions-page help bar: "The Sessions-page help bar is **unaffected**. It already advertises `Enter` for Sessions-page attach; the preview chrome's new `enter attach` token does not propagate to or duplicate that bar."

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Discoverability
