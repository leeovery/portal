---
phase: 1
phase_name: Enforce exact-match session targeting at the tmux chokepoint
total: 3
---

## kill-rename-prefix-collision-1-1 | approved

### Task 1: Introduce exactTarget primitive and fix KillSession to exact-match

**Problem**: `Client.KillSession(name)` in `internal/tmux/tmux.go` builds its argv as `kill-session -t <name>` with a bare `-t` target. tmux's default target resolution is prefix-match, so when the named session is not a live exact match but another live session has that name as a prefix (target `foo` with a live `foo-2`), tmux silently resolves `-t foo` to `foo-2` and kills the wrong session. The kill path is the dangerous one — destructive, no undo, silent on a wrong-session kill — and there is no centralising helper enforcing the `=` exact-match prefix for session targets, so the prefix is hand-applied per call site and any caller can silently opt out.

**Solution**: Introduce a package-level `exactTarget(session string) string` helper in `internal/tmux` (the session-level sibling of the existing pane-level `PaneTargetExact`) that returns `"=" + session`, then route `KillSession`'s `-t` target through it so the argv becomes `kill-session -t =<name>`. The fix lives entirely at the Client-method chokepoint — `KillSession` is the single argv-construction point, so fixing the argv inside it covers every caller uniformly with no caller-side change.

**Outcome**: `exactTarget("foo")` returns `"=foo"`; `KillSession("my-session")` issues `kill-session -t =my-session`; a live prefix-colliding session (`foo-2`) is never killed when the target (`foo`) is not a live exact match; `go build` and `go test ./internal/tmux/...` are green.

**Do**:
- In `internal/tmux/tmux.go`, add the helper beside `PaneTargetExact` (~line 546):
  ```go
  // exactTarget formats a session target with tmux's "=" exact-match prefix
  // (e.g. "=my-session"). It is the session-level sibling of PaneTargetExact
  // (pane-level) and the canonical way to build an exact-match `-t` target for
  // a session name — no inline "="+name session target should remain in this
  // package.
  //
  // The "=" prefix forces tmux's exact-match target resolution rather than the
  // default prefix match. Without it, a target "foo" coexisting with a live
  // "foo-2" would silently prefix-match "foo-2", operating on the wrong session.
  func exactTarget(session string) string { return "=" + session }
  ```
- Change `KillSession` (~line 352) so its `-t` argument is `exactTarget(name)` instead of bare `name`. Keep the error-wrapping and signature unchanged.
- Add a rationale godoc block to `KillSession` mirroring the already-fixed sites (e.g. `HasSession`/`SwitchClient` godoc): explain that the `=` prefix forces exact-match resolution so a destructive kill never silently prefix-matches a colliding session, and that the fix is at the chokepoint so every caller (including the internal `_portal-saver` callers in `cmd/state_cleanup.go` and `internal/tmux/portal_saver.go`) is covered without a caller-side change.
- In `internal/tmux/tmux_test.go`, update `TestKillSession` (~line 723): the happy-path subtest's `wantArgs` must change from `"kill-session -t my-session"` to `"kill-session -t =my-session"`. The error-path subtest is unaffected.
- Add the focused `exactTarget` unit test in a same-package internal test file. `exactTarget` is unexported and `tmux_test.go` is `package tmux_test` (external — it calls `tmux.NewClient`), so it cannot reach `exactTarget` directly. The package already uses `package tmux` internal test files (e.g. `option_discriminator_internal_test.go`, `export_test.go`), so follow that convention: create `internal/tmux/exact_target_internal_test.go` with `package tmux` and a `TestExactTarget` asserting `if got := exactTarget("foo"); got != "=foo" { t.Errorf("exactTarget(\"foo\") = %q, want \"=foo\"", got) }`.
- Add a prefix-collision regression test for `KillSession` in `tmux_test.go` mirroring `TestHasSessionUsesExactMatchPrefix` (~line 443): use a `MockCommander{RunFunc: ...}` that simulates tmux's exact-match semantics. The simulated live server holds only `foo-2`. When `KillSession("foo")` is called, the `RunFunc` inspects the `-t` argument: `=foo` returns a non-zero error (`fmt.Errorf("can't find session: foo")`) because `foo` is dead; bare `foo` (the regression case) would prefix-match `foo-2` and return success — assert that the bare form is never reached and that `KillSession("foo")` returns an error (proving the colliding `foo-2` was not killed). Assert the recorded call's `-t` argument begins with `"="` (`strings.HasPrefix(got[2], "=")`).

**Acceptance Criteria**:
- [ ] `exactTarget(session string) string` exists in `internal/tmux` (sibling to `PaneTargetExact`) and returns `"=" + session`.
- [ ] A direct unit assertion proves `exactTarget("foo") == "=foo"`.
- [ ] `KillSession("my-session")` issues exactly `kill-session -t =my-session` (verified by the updated `TestKillSession` happy-path assertion).
- [ ] `KillSession` carries a rationale godoc block mirroring the already-fixed sites.
- [ ] A new prefix-collision regression test proves a live colliding session (`foo-2`) is never killed when `foo` is not a live exact match (the `=foo` exact-match arm returns absent; the bare-`foo` prefix-match arm is never reached).
- [ ] `go build -o portal .` and `go test ./internal/tmux/...` are green.

**Tests**:
- `"it builds an exact-match session target"` — `exactTarget("foo") == "=foo"` (direct, internal-package assertion).
- `"it kills the session with an exact-match target"` — `TestKillSession` happy path expects `kill-session -t =my-session`.
- `"it never kills a prefix-colliding session when the target is not a live exact match"` — regression test: simulated server has only `foo-2`; `KillSession("foo")` resolves `=foo` to absent (error) and never reaches the bare-`foo` prefix-match arm, so `foo-2` is not killed.
- `"it returns an error when the tmux command fails"` — existing `TestKillSession` error-path subtest stays green (unchanged).

**Edge Cases**:
- Live colliding `foo-2` must never be killed when `foo` is absent — the regression test is the guard. The bare-`foo` arm in the `RunFunc` must assert-fail (or be structured so reaching it produces a test failure) if `KillSession` drops the prefix.
- The internal `_portal-saver` `KillSession` callers (`cmd/state_cleanup.go`, `internal/tmux/portal_saver.go`) gain the `=` prefix harmlessly: `_portal-saver` is a fixed literal name with no possible prefix collision, so no caller-side change is needed and their behaviour is unaffected.
- `tmux_test.go` is `package tmux_test` (external) and cannot reach the unexported `exactTarget`, so the focused unit assertion lives in the `package tmux` internal test file `exact_target_internal_test.go` (per the Do step). The regression tests, which need `MockCommander`, stay in the external `tmux_test.go` and drive the exported `KillSession`.

**Context**:
> From the specification (§ Required Behaviour & The Fix > 1. Introduce the `exactTarget` session-level primitive):
> ```go
> func exactTarget(session string) string { return "=" + session }
> ```
> This is the session-level sibling of the existing `PaneTargetExact` (pane-level). Together they become the two canonical ways to build an exact-match `-t` target — no inline `"="+name` for a session name left anywhere in `internal/tmux`.
>
> From § 2. Fix the two destructive callers: `KillSession`: `kill-session -t exactTarget(name)`, with a rationale godoc block mirroring the already-fixed sites. The fix lives entirely at the Client-method chokepoint — no caller-side change anywhere, including the internal `_portal-saver` `KillSession` callers, which gain the `=` prefix harmlessly (fixed literal name, no possible prefix collision).
>
> From § Testing Requirements: update `TestKillSession` → expect `kill-session -t =my-session`; add a prefix-collision regression test mirroring `TestHasSessionUsesExactMatchPrefix` (simulate tmux's exact-match semantics via `MockCommander.RunFunc` so a dropped-`=` regression fails loudly); add a focused unit test `exactTarget("foo") == "=foo"`.
>
> Existing pattern to mirror — `TestHasSessionUsesExactMatchPrefix` in `internal/tmux/tmux_test.go` (~line 443) — drives a `MockCommander{RunFunc:...}` whose live server holds only `foo-2`: `=foo` returns `fmt.Errorf("can't find session: foo")`, `=foo-2` returns success, and the bare `foo` case returns success only if the prefix was dropped (the regression). `MockCommander` records every invocation in `Calls [][]string`.
>
> Project constraint: tests must NOT use `t.Parallel()`.

**Spec Reference**: `.workflows/kill-rename-prefix-collision/specification/kill-rename-prefix-collision/specification.md` (§ Required Behaviour & The Fix; § Testing Requirements & Acceptance Criteria)

## kill-rename-prefix-collision-1-2 | approved

### Task 2: Fix RenameSession to exact-match target with bare newName

**Problem**: `Client.RenameSession(oldName, newName)` in `internal/tmux/tmux.go` builds its argv as `rename-session -t <oldName> <newName>` with a bare `-t` target. Because tmux defaults to prefix-match, when `oldName` is not a live exact match but a live session has it as a prefix (target `foo` with a live `foo-2`), tmux silently renames the wrong session (`foo-2`). The rename path is less severe than kill (recoverable) but still incorrect, and it carries the same live-collision exposure since session names are `{project}-{nanoid}` and freely renamed by the user.

**Solution**: Route `RenameSession`'s `-t` target through the `exactTarget` helper (introduced in Task 1) so the argv becomes `rename-session -t =<oldName> <newName>`. The `=` prefix goes on the target only; `newName` is the literal positional new-name argument and must stay bare. The fix is at the Client-method chokepoint — `RenameSession` is the single argv-construction point, so no caller-side change is needed.

**Outcome**: `RenameSession("old-name", "new-name")` issues `rename-session -t =old-name new-name` with `new-name` bare; a live prefix-colliding session (`foo-2`) is never renamed when the target (`foo`) is not a live exact match; `go build` and `go test ./internal/tmux/...` are green.

**Do**:
- In `internal/tmux/tmux.go`, change `RenameSession` (~line 361) so its `-t` argument is `exactTarget(oldName)` instead of bare `oldName`. Leave `newName` exactly as it is — bare, no prefix. Keep the error-wrapping and signature unchanged.
- Add a rationale godoc block to `RenameSession` mirroring the already-fixed sites: explain that the `=` prefix forces exact-match resolution on the target so a rename never silently prefix-matches a colliding session, AND explicitly call out the implementer trap — the prefix goes on the **target only**; `newName` must stay bare because prefixing it would literally name the session `=...`.
- In `internal/tmux/tmux_test.go`, update `TestRenameSession` (~line 939): the happy-path subtest's `wantArgs` must change from `"rename-session -t old-name new-name"` to `"rename-session -t =old-name new-name"`. The error-path subtest is unaffected.
- Add a prefix-collision regression test for `RenameSession` in `tmux_test.go` mirroring `TestHasSessionUsesExactMatchPrefix` (~line 443): use a `MockCommander{RunFunc: ...}` that simulates tmux's exact-match semantics. The simulated live server holds only `foo-2`. When `RenameSession("foo", "bar")` is called, the `RunFunc` inspects the `-t` argument: `=foo` returns a non-zero error (`fmt.Errorf("can't find session: foo")`) because `foo` is dead; bare `foo` (the regression case) would prefix-match `foo-2` and return success — assert that `RenameSession("foo", "bar")` returns an error (proving the colliding `foo-2` was not renamed) and that the bare-`foo` arm is never reached.
- In the regression test, also assert `newName` stayed bare: inspect the recorded call's argv and confirm the positional new-name argument is exactly `"bar"` (no `=` prefix) — i.e. `got` equals `["rename-session", "-t", "=foo", "bar"]`, with the prefix on the target slot only.

**Acceptance Criteria**:
- [ ] `RenameSession("old-name", "new-name")` issues exactly `rename-session -t =old-name new-name` (verified by the updated `TestRenameSession` happy-path assertion).
- [ ] The `=` prefix appears on the `-t` target only; `newName` remains bare (no `=` on the positional new-name argument) — verified by argv inspection.
- [ ] `RenameSession` carries a rationale godoc block mirroring the already-fixed sites, explicitly noting the target-only / bare-`newName` trap.
- [ ] A new prefix-collision regression test proves a live colliding session (`foo-2`) is never renamed when `foo` is not a live exact match.
- [ ] `go build -o portal .` and `go test ./internal/tmux/...` are green.

**Tests**:
- `"it renames the session with an exact-match target and a bare new name"` — `TestRenameSession` happy path expects `rename-session -t =old-name new-name`.
- `"it never renames a prefix-colliding session when the target is not a live exact match"` — regression test: simulated server has only `foo-2`; `RenameSession("foo", "bar")` resolves `=foo` to absent (error) and never reaches the bare-`foo` prefix-match arm.
- `"it keeps the new name bare"` — argv inspection confirms the new-name slot is `"bar"`, not `"=bar"` (prefix on target only).
- `"it returns an error when the tmux command fails"` — existing `TestRenameSession` error-path subtest stays green (unchanged).

**Edge Cases**:
- The one implementer trap (per spec): in `RenameSession`, the `=` prefix goes on the **target only**. `newName` must stay bare — prefixing it would corrupt the new session name (the session would literally be named `=...`). The "keeps the new name bare" test is the guard against this.
- Live colliding `foo-2` must never be renamed when `foo` is absent — the regression test guards this. The bare-`foo` arm in the `RunFunc` must produce a test failure if reached.
- Depends on `exactTarget` from Task 1; if implemented as a separate commit, ensure Task 1 lands first.

**Context**:
> From the specification (§ 2. Fix the two destructive callers): `RenameSession`: `rename-session -t exactTarget(oldName) <newName>`, with a rationale godoc block mirroring the already-fixed sites.
>
> **Edge case (the one implementer trap):** in `RenameSession`, the prefix goes on the **target only**. `newName` is the literal positional new-name argument and **must stay bare** — prefixing it would corrupt the new session name (the session would literally be named `=...`).
>
> From § Testing Requirements: update `TestRenameSession` → expect `rename-session -t =old-name new-name` (prefix on target only; `new-name` stays bare); add a prefix-collision regression test mirroring `TestHasSessionUsesExactMatchPrefix` (simulate tmux's exact-match semantics via `MockCommander.RunFunc`).
>
> Existing pattern to mirror — `TestHasSessionUsesExactMatchPrefix` (~line 443) drives a `MockCommander{RunFunc:...}` whose live server holds only `foo-2`. `MockCommander` records every invocation in `Calls [][]string`, so the regression test can inspect both the target slot (`=foo`) and the new-name slot (`bar`).
>
> Project constraint: tests must NOT use `t.Parallel()`.

**Spec Reference**: `.workflows/kill-rename-prefix-collision/specification/kill-rename-prefix-collision/specification.md` (§ Required Behaviour & The Fix > 2. Fix the two destructive callers; § Testing Requirements & Acceptance Criteria)

## kill-rename-prefix-collision-1-3 | approved

### Task 3: Migrate five behaviour-neutral session-target sites onto exactTarget

**Problem**: Five existing sites in `internal/tmux` already carry the `=` exact-match prefix for a **session** target as an inline `"="+name` string: `HasSession`, `HasSessionProbe`, `SwitchClient` (in `tmux.go`), and `saverPanePID`, `SaverPaneID` (in `saver_pane_pid.go`). Leaving them as inline strings after Tasks 1-2 introduce the `exactTarget` helper would produce a mixed end-state (helper for the two destructive callers, inline `"="+name` for the rest) — itself a new inconsistency and exactly the drift surface that let the original bug exist. The spec's chosen approach is a uniform migration so the codebase reads "as if the gap was never there."

**Solution**: Replace the inline `"="+name` session-target construction at all five sites with a call to the `exactTarget` helper, producing byte-identical argv. This is a pure readability/anti-drift refactor: the argv each site emits does not change, so the existing tests stay green, and that green state is the proof the migration is behaviour-neutral. No new tests are required for the migrated sites — the existing pins (`TestSwitchClient`, `TestHasSessionProbe`, `TestHasSessionUsesExactMatchPrefix`, and the saver-pane tests) already assert the `=` argv.

**Outcome**: All five migrated sites build their `-t` session target via `exactTarget(...)`; the emitted argv is byte-identical to before; no inline `"="+name` session-target strings remain anywhere in the `internal/tmux` package; all existing `internal/tmux` tests stay green; `go build` and `go test ./...` are green.

**Do**:
- In `internal/tmux/tmux.go`, replace `"="+name` with `exactTarget(name)` at:
  - `HasSession` (~line 136): `c.cmd.Run("has-session", "-t", exactTarget(name))`.
  - `HasSessionProbe` (~line 166): `c.cmd.Run("has-session", "-t", exactTarget(name))`.
  - `SwitchClient` (~line 378): `c.cmd.Run("switch-client", "-t", exactTarget(name))`.
- In `internal/tmux/saver_pane_pid.go`, replace `"="+sessionName` with `exactTarget(sessionName)` at:
  - `saverPanePID` (~line 49): `c.cmd.Run("list-panes", "-t", exactTarget(sessionName), "-F", "#{pane_pid}")`.
  - `SaverPaneID` (~line 84): `c.cmd.Run("list-panes", "-t", exactTarget(sessionName), "-F", "#{pane_id}")`.
- Do NOT touch `SelectWindow`'s inline `"=" + bareTarget` (~line 936 in `tmux.go`): that is a window-level target, explicitly left to implementer discretion by the spec and NOT part of this task. Leave it exactly as-is.
- Do NOT touch any out-of-scope bare session-target reads/sets (`ActivePaneCurrentPath`, `SetSessionOption`, `ListPanesInSession`, `ShowEnvironment`, `SetSessionEnvironment`), the `PaneTarget` hooks.json key formatter, the `display-message -t <paneID>` pane-ID read, caller-supplied pane/window writers, or pane/window sites already on `PaneTargetExact`. None of these are session-target inline `"="+name` strings.
- Run the existing `internal/tmux` test suite and confirm it stays green with no test changes (the green state proves byte-identical argv). Verify with a grep that zero inline `"="+name` / `"=" + sessionName` session-target strings remain in `tmux.go` and `saver_pane_pid.go` (only the window-level `"=" + bareTarget` in `SelectWindow` may remain, and only at implementer discretion).

**Acceptance Criteria**:
- [ ] `HasSession`, `HasSessionProbe`, `SwitchClient` (in `tmux.go`) and `saverPanePID`, `SaverPaneID` (in `saver_pane_pid.go`) all build their `-t` session target via `exactTarget(...)`.
- [ ] The argv emitted by each migrated site is byte-identical to before (no test assertions change; existing tests stay green).
- [ ] No inline `"="+name` session-target strings remain in `internal/tmux/tmux.go` or `internal/tmux/saver_pane_pid.go` (the window-level `SelectWindow` `"=" + bareTarget` is the only `=`-construction that may remain, by spec).
- [ ] Out-of-scope sites (bare session reads/sets, `PaneTarget`, `display-message -t <paneID>`, pane/window writers, sites already on `PaneTargetExact`) are untouched.
- [ ] `go build -o portal .` and `go test ./...` are green.

**Tests**:
- `"it keeps HasSession argv exact-match"` — existing `TestHasSessionUsesExactMatchPrefix` stays green with `has-session -t =foo` (no change).
- `"it keeps HasSessionProbe argv exact-match"` — existing `TestHasSessionProbe` stays green with `has-session -t =my-session` (no change).
- `"it keeps SwitchClient argv exact-match"` — existing `TestSwitchClient` stays green with `switch-client -t =my-session` (no change).
- `"it keeps the saver-pane list-panes argv exact-match"` — existing `saverPanePID` / `SaverPaneID` tests stay green with `list-panes -t =_portal-saver ...` (no change).

**Edge Cases**:
- The migration must be behaviour-neutral: argv must stay byte-identical. The proof is the existing green tests — if any migrated site's test goes red, the refactor changed argv and is wrong.
- The two `saver_pane_pid.go` sites target the fixed `_portal-saver` name (no live-collision exposure), but they carry the same inline drift surface, so they migrate for consistency. Their behaviour does not change.
- Boundary: `SelectWindow`'s window-level `"=" + bareTarget` and `PaneTargetExact`'s pane-level prefix are intentionally NOT migrated — `exactTarget` is for session targets only. Do not absorb the window/pane sites into this task.
- Depends on `exactTarget` from Task 1.

**Context**:
> From the specification (§ Migration Scope & Out of Scope > Sites to migrate onto `exactTarget` (behaviour-neutral)): these already carry the `=` prefix as an inline `"="+name` string for a **session** target. Migrate them onto `exactTarget` so the pattern is uniform across `internal/tmux` — a pure readability/anti-drift refactor producing **identical argv**: `HasSession`, `HasSessionProbe`, `SwitchClient` (`tmux.go`); `saverPanePID`, `SaverPaneID` (`saver_pane_pid.go`).
>
> The two `saver_pane_pid.go` sites target the fixed `_portal-saver` name (no collision exposure), but they carry the same inline `"="+session` drift surface, so they migrate for consistency. Their existing tests stay green (argv unchanged); that green state *is* the proof the migration is behaviour-neutral.
>
> From § Pane/window-level sites — leave as-is: `SelectPane`, `ResizePaneZoom`, and `SelectWindow` already centralise the prefix via `PaneTargetExact` (pane-level) or build it at the window-target level. They stay on that path. Any tidy-up of `SelectWindow`'s inline `"=" + bareTarget` is an implementation-detail call left to the implementer — not required by this fix.
>
> From § Explicitly out of scope (do NOT touch): `PaneTarget` (no-prefix hooks.json key formatter — changing it silently invalidates hook entries); bare `-t <session>` reads/option-and-env sets (`ActivePaneCurrentPath`, `SetSessionOption`, `ListPanesInSession`, `ShowEnvironment`, `SetSessionEnvironment`); caller-supplied pane/window writers (`SendKeys`, `RespawnPane`, `CapturePane`, `NewWindow`, `SplitWindow`, `SelectLayout`); `display-message -t <paneID>` (categorically immune — must stay bare); `internal/session/quickstart.go` bare targets.
>
> From § Testing Requirements: the migrated sites (`HasSession`, `HasSessionProbe`, `SwitchClient`) keep their existing tests **green** with unchanged argv — that green state is the proof the migration is behaviour-neutral.
>
> Verified inline-string inventory (`internal/tmux`): `tmux.go:136` (HasSession), `tmux.go:166` (HasSessionProbe), `tmux.go:378` (SwitchClient), `tmux.go:936` (SelectWindow — window-level, NOT migrated), `saver_pane_pid.go:49` (saverPanePID), `saver_pane_pid.go:84` (SaverPaneID).
>
> Project constraint: tests must NOT use `t.Parallel()`.

**Spec Reference**: `.workflows/kill-rename-prefix-collision/specification/kill-rename-prefix-collision/specification.md` (§ Migration Scope & Out of Scope; § Testing Requirements & Acceptance Criteria)
