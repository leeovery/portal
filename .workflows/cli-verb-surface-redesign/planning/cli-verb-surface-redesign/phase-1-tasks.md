---
phase: 1
phase_name: "`open` single-target resolution grammar"
total: 5
---

## cli-verb-surface-redesign-1-1 | approved

### Task 1.1: Exact session-name match → attach outcome

**Problem**: Today a bare positional to `portal open` runs only the directory chain (path → alias → zoxide → TUI fallback). It never checks whether the argument is the name of a running session, so `open api-x7Kd9a` cannot attach an existing session. Axiom 2 (attach-vs-mint) requires a session-domain hit to **attach**, and the session-domain must be checked *first* in the precedence chain.

**Solution**: Add a session-domain exact-name check at the front of `open`'s single-target resolution. Match the raw target against the **user-visible** session set (the leading-underscore-filtered `ListSessions` view, via `tmux.Client.ListSessionNames`), and on a hit connect this terminal to that session through the existing inside/outside connector. On no match, fall through unchanged to the existing directory chain.

**Outcome**: `portal open <exact-session-name>` attaches the running session (switch-client inside tmux, exec `tmux attach-session` outside); an internal `_portal-*` session is never matchable; a target that names no session falls through to the directory chain exactly as today.

**Do**:
- In `internal/resolver/query.go`, make the resolver the single resolution engine that classifies domain. Introduce a session-domain dependency and result type:
  - Add a `SessionLister` interface to the resolver package: `ListSessionNames() ([]string, error)` — returns the user-visible (filtered) set. `*tmux.Client` already satisfies this (its `ListSessionNames` delegates to `ListSessions`, which drops every `_`-prefixed name — see `internal/tmux/tmux.go:243-257`).
  - Add `SessionResult{ Name string; Domain string }` implementing `queryResult()`. `Domain` is `"session"` for an exact-name hit (Task 1.3 will also produce `SessionResult` for glob hits with `Domain = "glob"`).
  - Add `SessionLister` as a constructor parameter to `NewQueryResolver` (now `NewQueryResolver(sessions, aliases, zoxide, dirValidator)`), storing it on `QueryResolver`. Update every existing call site: `cmd/open.go` `buildQueryResolver`, and the ~8 constructions in `internal/resolver/query_test.go`.
  - In `QueryResolver.Resolve`, **before** the existing `IsPathArgument` branch, fetch the session set once (`qr.sessions.ListSessionNames()`; a nil/empty list or error collapses to "no sessions" — the tmux client already returns `([]string{}, nil)` when no server is running) and check `slices.Contains(names, query)`. On a hit, return `&SessionResult{Name: query, Domain: "session"}`. On no match, continue to the current path/alias/zoxide logic unchanged.
- In `cmd/open.go`:
  - Add a package-level seam mirroring `openTUIFunc`/`openPathFunc`:
    ```go
    var openSessionFunc = openSession
    func openSession(cmd *cobra.Command, name string) error {
        return buildSessionConnector(tmuxClient(cmd)).Connect(name)
    }
    ```
    `buildSessionConnector` already returns `*SwitchConnector` inside tmux and `*AttachConnector` outside (`cmd/open.go:120-125`).
  - In `openCmd.RunE`, add a `case *resolver.SessionResult:` arm to the result switch that returns `openSessionFunc(cmd, r.Name)`.
  - Extend `OpenDeps` with a `SessionLister resolver.SessionLister` field and thread it through `buildQueryResolver`: when `openDeps != nil` use `openDeps.SessionLister`; otherwise use `tmuxClient(cmd)` (so `buildQueryResolver` must receive the `cmd` or the client — pass the client in).
- Leave the directory chain, `FallbackResult`, and command threading untouched in this task (Task 1.2 removes `FallbackResult`; Task 1.3 adds the glob pre-check; Task 1.4 adds logging).

**Acceptance Criteria**:
- [ ] `open <name>` where `<name>` is exactly a user-visible session name returns a `SessionResult` and routes to `openSessionFunc` (attach), never to `openPath`/mint.
- [ ] The exact-name check is performed against `ListSessionNames` (the filtered view), so `open _portal-saver` / `open _portal-bootstrap` never attach — they fall through to the directory chain / miss as if the session did not exist.
- [ ] An empty session set (no server, or `ListSessionNames` returns `[]`) yields no session match and falls through to the directory chain without error.
- [ ] A target that matches no session name falls through to the existing path → alias → zoxide resolution unchanged.
- [ ] Inside tmux the attach uses `switch-client`; outside tmux it uses exec `tmux attach-session -t =<name>` — delegated to the already-tested `buildSessionConnector`.
- [ ] All existing `resolver.NewQueryResolver` call sites compile against the new 4-arg constructor.

**Tests**:
- `internal/resolver/query_test.go`: `"it returns SessionResult for an exact user-visible session-name hit"` — inject a fake `SessionLister` returning `["api-x7Kd9a","web-3fJk"]`, resolve `"api-x7Kd9a"`, assert `*SessionResult{Name:"api-x7Kd9a", Domain:"session"}`.
- `"it never matches an underscore-prefixed name because the lister view is already filtered"` — fake lister returns only the filtered set (no `_portal-saver`); resolve `"_portal-saver"` and assert it does NOT return a `SessionResult` (falls through — with no alias/zoxide match it reaches the existing fallthrough).
- `"it falls through to the directory chain when no session name matches"` — lister returns `["web-1"]`, resolve `"api"` with an alias mapping `api -> <existing dir>`, assert `*PathResult` (mint), proving the session check did not shadow the chain.
- `"it treats an empty session set as no match"` — lister returns `[]`, resolve any non-session query, assert the directory-chain outcome (no panic, no session result).
- `cmd/open_test.go`: `"it routes an exact session-name hit to the session connector"` — set `openDeps.SessionLister` to a fake returning `["dev"]`, override `openSessionFunc` to capture the name, Execute `open dev`, assert the captured name is `"dev"` and `openPathFunc`/`openTUIFunc` were not called.
- `cmd/open_test.go`: `"it delegates inside/outside connector selection to buildSessionConnector"` — reuse the existing `TestBuildSessionConnector` coverage; add a focused assertion that `openSession` connects via the connector `buildSessionConnector` returns (mock the client so no real tmux is touched).

**Edge Cases**:
- Internal `_`-prefixed sessions are unmatchable because the check uses the filtered `ListSessionNames` view, not `tmux HasSession` (which would see `_portal-saver`). This is the load-bearing distinction — do NOT use `HasSession` for session-domain resolution.
- Empty session set (cold server / no sessions): `ListSessionNames` returns `([]string{}, nil)`; no match; fall through.
- A `ListSessionNames` error is already collapsed to an empty slice by the tmux client, so the resolver treats it as "no sessions" rather than surfacing an error.

**Context**:
> Spec § `portal open` — Grammar & Target Resolution: "the precedence chain, first match wins: exact session name → path → alias → zoxide query" and "exact session name → attach existing session". Spec § "Session set — user-visible only": all session-domain resolution "matches only against the user-visible session set (the same leading-underscore-filtered `ListSessions` view used by the picker and tab completion)… a name or glob that would resolve only to a filtered internal session is treated as a miss."
>
> Deferred/ambiguity note: the spec formalises "a command targeting an attach (existing-session) target is rejected" in **Phase 2**, not here. Phase 1 wires the session-attach outcome only; the interaction between a present `-e`/`--` command and an attach target is out of scope for this task. Keep the session-attach branch minimal — connect via `openSessionFunc(cmd, r.Name)` and do not thread the command into it. The explicit "command + attach target ⇒ usage error" contract lands in Phase 2.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Grammar & Target Resolution (Target resolution precedence; Session set — user-visible only).

---

## cli-verb-surface-redesign-1-2 | approved

### Task 1.2: Directory chain (path → alias → zoxide) mint outcomes + total-miss hard-fail

**Problem**: The old terminal step of `open`'s resolution chain is an implicit TUI-picker-with-filter fallback (`FallbackResult` → `openTUI`). The redesign removes that implicit fallback: a directory-domain hit must always **mint** a fresh session (Axiom 2, no find-or-create), and a target that resolves to nothing must **hard-fail** with a message pointing at the `-f` escape hatch — not silently pop the picker.

**Solution**: Replace `FallbackResult` with a `MissResult` that `cmd/open.go` turns into a hard-fail error carrying the exact escape-hatch message. Confirm the path/alias/zoxide arms continue to mint (they already do, via `openPath` → `QuickStart`/`CreateFromDir`, which always create a new `{project}-{nanoid}` session), and confirm the existing command→mint threading is preserved.

**Outcome**: `open <path|alias|zoxide-query>` mints a brand-new session at the resolved directory (even when a session for that project already exists); a bare project name like `open api` mints and never reattaches an `api-*` session; a total miss prints `nothing resolved for '<target>' — try -f <target>` and exits non-zero (no picker).

**Do**:
- In `internal/resolver/query.go`:
  - Delete `FallbackResult` and its `queryResult()` method. Add `MissResult{ Target string }` implementing `queryResult()`; it carries the raw input for the caller's error/log.
  - Change the final `return &FallbackResult{Query: query}` (the "zoxide not installed or no match" tail of `Resolve`) to `return &MissResult{Target: query}, nil`.
  - Leave the path/alias/zoxide arms as-is: `IsPathArgument` → `ResolvePath` → `PathResult` (mint); alias hit → `validatedPath` → `PathResult` (mint); zoxide hit → `validatedPath` → `PathResult` (mint). Add a `Domain` field to `PathResult` (`"path"`/`"alias"`/`"zoxide"`) set at each arm — Task 1.4 reads it for the log line; Task 1.1 already added `Domain` handling to the result taxonomy.
  - Keep the `DirNotFoundError` behaviour: an alias/zoxide key that resolves to a non-existent directory still returns `(nil, &DirNotFoundError{...})` — that is a hard error, distinct from a miss.
- In `cmd/open.go` `openCmd.RunE`:
  - Remove the `case *resolver.FallbackResult:` arm (which called `openTUIFunc`). Add `case *resolver.MissResult:` returning a plain (non-usage) error with the exact wording:
    ```go
    return fmt.Errorf("nothing resolved for '%s' — try -f %s", r.Target, r.Target)
    ```
    A plain error maps to exit code 1 (see `main.classify`); it is NOT a `UsageError` (miss is a runtime resolution failure, not misuse). Keep the `*resolver.PathResult` arm routing to `openPathFunc(cmd, r.Path, command)` unchanged so the command still threads into the mint.
- Update the now-obsolete fallback tests in `cmd/open_test.go` (e.g. `TestOpenCommand_FallbackToTUI_SkipsSecondWait` and any `FallbackResult` assertions in `internal/resolver/query_test.go`) to assert the hard-fail miss error / `MissResult` instead of a picker launch.

**Acceptance Criteria**:
- [ ] A path/alias/zoxide hit returns `PathResult` and mints a fresh session via `openPath` — never reuses/attaches an existing session for the same project.
- [ ] `open api` (bare project name, no literal session named `api`) resolves through alias/zoxide to a directory and mints; it never attaches an existing `api-*` session.
- [ ] A total miss returns exactly `nothing resolved for '<target>' — try -f <target>` (single quotes around the target, ` — ` em-dash separator, raw target after `-f`) and exits non-zero (code 1); the TUI picker is never launched on a miss.
- [ ] An alias (or zoxide) that resolves to a non-existent directory returns the `Directory not found: <dir>` error — distinct from the miss message.
- [ ] zoxide-not-installed or zoxide-no-match falls through silently to `MissResult` (no error surfaced from the zoxide arm itself), preserving today's "continue to next domain" behaviour for the bare-target chain.
- [ ] The `-e`/`--` command continues to thread into the minted session (`openPath(cmd, path, command)`), unchanged from today.

**Tests**:
- `internal/resolver/query_test.go`: `"it returns MissResult when nothing resolves"` — no alias, zoxide returns `ErrNoMatch`, dir invalid; assert `*MissResult{Target:"blog"}`.
- `"it returns PathResult with domain=alias for an alias hit"` / `"…domain=zoxide for a zoxide hit"` / `"…domain=path for a path argument"` — assert the `Domain` field is set per arm.
- `"it returns DirNotFoundError for an alias to a non-existent directory"` — alias maps to a dir the validator rejects; assert `*DirNotFoundError`, not `MissResult`.
- `cmd/open_test.go`: `"it hard-fails with the escape-hatch message on a total miss"` — inject deps so nothing resolves, Execute `open blog`, assert `err.Error() == "nothing resolved for 'blog' — try -f blog"` and that `openTUIFunc` was NOT called.
- `cmd/open_test.go`: `"open api mints and never attaches an existing api- session"` — `SessionLister` returns `["api-x7Kd9a"]` (so the exact-name check misses), alias/zoxide map `api -> <existing dir>`, override `openPathFunc` to capture, Execute `open api`, assert `openPathFunc` was called with the resolved dir and `openSessionFunc` was NOT.
- `cmd/open_test.go`: `"a command threads into a minted directory target"` — Execute `open <alias> -e claude`, assert the captured command reaches `openPathFunc`.

**Edge Cases**:
- Bare project name (`api`) mints and never reattaches: the exact-name check (Task 1.1) fails because sessions are named `{project}-{nanoid}`, so `api` falls through to the directory chain and mints — the accepted "bare shorthand does not reattach" consequence.
- Alias/zoxide → non-existent dir: `DirNotFoundError` (hard error, exit 1), NOT a miss and NOT a mint.
- zoxide not installed / no match: the zoxide arm returns an error that the chain swallows (`err == nil` guard in `Resolve`), so resolution continues to `MissResult` — the bare-target chain "falls through silently", unlike the pinned `-z` which will error explicitly (Phase 2).

**Context**:
> Spec § Axiom 2 — attach-vs-mint dichotomy: "Directory targets always create a fresh `{project}-{nanoid}` session even when sessions already exist for that project… There is no find-or-create." Spec § Bare project shorthand does not reattach: `open api` "falls through to zoxide/path and mints a new session, even while an `api-*` session runs." Spec § Miss handling: "A target that resolves to nothing is a hard failure… Today's terminal step of the resolution chain — a TUI-picker-with-filter fallback — is removed," with the message `nothing resolved for 'blog' — try -f blog`. Spec § Domain-pinning flags note: "the bare-target chain treats any zoxide error as 'continue to next domain' (falls through silently)."
>
> The mint outcomes are already the behaviour of today's `openPath` (`QuickStart` runs `new-session -d` with a guaranteed-fresh `GenerateSessionName`), so this task's net-new work is removing the implicit fallback and adding the hard-fail message — the command→mint threading is preserved, not reworked. The attach+command formalization (rejecting a command on an attach target) is Phase 2.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § The two axioms (Axiom 2); § Bare project shorthand does not reattach; § Miss handling — total miss is a hard fail.

---

## cli-verb-surface-redesign-1-3 | approved

### Task 1.3: Glob pre-check → session-domain expansion + zero-match hard-fail

**Problem**: A bare target containing glob metacharacters must be treated as **session-domain by construction** — expanded against the live session names and never run through the path/alias/zoxide chain — with zero matches a hard fail. Without a glob pre-check, `open 'api-*'` would be sent to the directory chain and mishandled, and a directory path whose name contains glob metacharacters (`~/tmp/foo[1]`) would be silently mis-resolved instead of correctly hard-failing as an unreachable bare positional.

**Solution**: Add a glob-metacharacter predicate and a session-glob expansion step at the very front of `Resolve` (before the exact-name check and before `IsPathArgument`). A target containing `*`, `?`, or `[` is matched against the user-visible session set via `filepath.Match`; matches produce a session-domain attach outcome, zero matches produce `MissResult`.

**Outcome**: `open 'api-*'` matches the user-visible sessions and attaches (single-target: the sole/first match); a glob matching only internal `_`-prefixed sessions or nothing hard-fails; `open 'foo[1]'` (a path whose name looks like a glob) hard-fails as unreachable; the glob path never consults aliases/zoxide even when a same-named alias or directory exists.

**Do**:
- In `internal/resolver` (new `glob.go`, or alongside `path.go`):
  - Add `HasGlobMeta(s string) bool` — reports whether `s` contains any of `*`, `?`, `[`. Exported so `cmd/open.go` can gate the resolve-log line on it in Task 1.4.
  - Add a session-glob expansion helper, e.g. `MatchSessions(pattern string, names []string) []string`, using `path/filepath.Match(pattern, name)`. A `filepath.Match` error (malformed pattern, e.g. unclosed `[`) is treated as "no match for that name" — so a malformed glob yields zero matches (hard fail), never a panic.
- In `QueryResolver.Resolve`, add step 0 **before** the exact-name check (Task 1.1) and the `IsPathArgument` branch:
  ```go
  if HasGlobMeta(query) {
      names, _ := qr.sessions.ListSessionNames() // filtered, user-visible set
      matches := MatchSessions(query, names)
      if len(matches) == 0 {
          return &MissResult{Target: query}, nil
      }
      return &SessionResult{Name: matches[0], Domain: "glob"}, nil
  }
  ```
  This is unconditional — it runs even for `~`/`/`-containing targets, so a path with glob metacharacters is captured here (session-domain) and cannot reach `ResolvePath`.
- Do NOT build the multi-match burst here. At single-target arity a glob that matches multiple sessions resolves to the first match (tmux's reported order via `ListSessionNames`); the per-match window fan-out is Phase 3.

**Acceptance Criteria**:
- [ ] A target containing `*`, `?`, or `[` is routed to session-glob expansion and never to path/alias/zoxide — even when a same-named alias key or directory exists.
- [ ] Expansion matches only the user-visible (filtered) session set; a glob that would match only `_portal-*` sessions counts as zero matches (they are absent from `ListSessionNames`).
- [ ] Zero matches returns `MissResult` (hard fail via the Task 1.2 message).
- [ ] A single/multi match returns `SessionResult{Domain:"glob"}` (single-target: the first match) → attach.
- [ ] `open 'foo[1]'` — a path whose name contains glob metacharacters — is captured by the glob pre-check, matches zero sessions, and hard-fails (it is unreachable as a bare positional; `-p` will reach it in Phase 2).
- [ ] A malformed glob (`filepath.Match` error) yields zero matches and hard-fails rather than erroring or panicking.

**Tests**:
- `internal/resolver/glob_test.go` (or `query_test.go`): `"HasGlobMeta detects *, ?, and ["` and `"HasGlobMeta is false for plain and path-like strings"` (`api`, `~/Code/blog`, `api-x7Kd9a`).
- `"it expands a session glob against the user-visible set"` — lister returns `["api-1","api-2","web-3"]`, resolve `"api-*"`, assert `*SessionResult{Domain:"glob"}` with `Name == "api-1"`.
- `"a glob matching zero user-visible sessions hard-fails"` — lister returns `["web-3"]`, resolve `"api-*"`, assert `*MissResult`.
- `"a glob matching only internal sessions counts as zero"` — lister returns the already-filtered set (no `_portal-*`), resolve `"_portal-*"`, assert `*MissResult`.
- `"a glob skips the directory chain even when a same-named alias exists"` — alias map `{"api-*": <dir>}` (contrived), lister returns `["api-1"]`, resolve `"api-*"`, assert `*SessionResult` (glob), proving the alias was never consulted.
- `"a path with glob metacharacters is unreachable as a bare positional"` — resolve `"~/tmp/foo[1]"` (or `"foo[1]"`) with lister returning no matching session, assert `*MissResult` — never a `PathResult`.
- `"a malformed glob yields zero matches"` — resolve `"foo["` with any lister, assert `*MissResult` (no panic).

**Edge Cases**:
- Glob matching only `_`-prefixed internal sessions ⇒ zero (filtered out of `ListSessionNames`).
- Path with glob metacharacters (`foo[1]`, `~/tmp/foo[1]`) ⇒ zero-match hard-fail; reachable only via `-p` (Phase 2).
- Glob takes precedence over a same-named alias/dir — the pre-check runs first and returns, never falling into the chain.
- Multi-match at single-target arity ⇒ first match (Phase 1); the burst fan-out is Phase 3.
- Malformed glob (`filepath.Match` returns `ErrBadPattern`) ⇒ treat as no-match ⇒ hard fail.

**Context**:
> Spec § Target resolution precedence, step 1: "If the target contains glob metacharacters (`*`, `?`, `[…]`), it is session-domain by construction: expand it against live session names and skip the chain below entirely… Zero matches ⇒ unresolvable ⇒ hard fail." Spec § Glob targets: "Glob, not regex"; "A directory path whose name contains glob metacharacters (e.g. `~/tmp/foo[1]`) is unreachable as a bare positional — the glob pre-check treats it as a session glob, it matches zero sessions, and it hard-fails. Reach it with `-p <dir>`." Spec § Glob targets: expansion "produces K targets that join the target list" — that K-target join is the multi-target burst, **deferred to Phase 3**; Phase 1 delivers the pre-check, single-target expansion, and zero-match hard-fail only.
>
> Ambiguity note: the spec defines multi-match glob behaviour only in the multi-target burst context (Phase 3). For a single-target Phase-1 `open`, this task attaches the first match; the spec is silent on single-target multi-match, so "first match" is a planner decision chosen to keep Phase 1 testable and forward-compatible with the Phase 3 burst (which will fan out to a window per match). The glob-miss reuses the Task 1.2 single-target miss message (`nothing resolved for '<glob>' — try -f <glob>`) since a glob miss is still a single-target total miss.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § Target resolution precedence (Glob pre-check); § Multi-Target Burst Mechanics — Glob targets.

---

## cli-verb-surface-redesign-1-4 | approved

### Task 1.4: `resolve` log component — INFO decision line

**Problem**: When resolution guesses (e.g. a wrong zoxide match silently mints a session), there is no reliable confirmation surface (outside tmux `open` exec-replaces itself; inside tmux output lands in the switched-away pane). The one locked observability addition is a durable log line so a confusing guess is reconstructable from `portal.log`. `open` owns no log component today, so this requires a governed amendment adding a new `resolve` component to the closed taxonomy.

**Solution**: Bind the new `resolve` component once in `cmd/open.go` (`var resolveLogger = log.For("resolve")`) and emit exactly one INFO decision line per bare positional resolved through the guessing chain, carrying attrs `target`, `domain`, and `resolved_path`. Gate emission on the resolver's glob predicate so glob (and, later, pinned) targets — which are deterministic — emit no line. Emit on a miss too (`domain=miss`, empty `resolved_path`).

**Outcome**: `open blog` that resolves via zoxide writes one INFO record `resolve: resolved target=blog domain=zoxide resolved_path=/…/Code/blog`; a session hit records `domain=session resolved_path=<session name>`; a total miss records `domain=miss resolved_path=""`; a glob target records nothing. `internal/resolver` stays log-free.

**Do**:
- In `cmd/open.go`, add a package-level binding mirroring `spawn.go`'s `var spawnLogger = log.For("spawn")`:
  ```go
  var resolveLogger = log.For("resolve")
  ```
- In `openCmd.RunE`, after `qr.Resolve(query)` succeeds (i.e. returns a result with `err == nil`), emit the decision line once, gated on the glob predicate, reading the domain + resolved value off the result:
  ```go
  if !resolver.HasGlobMeta(query) {
      resolveLogger.Info("resolved",
          "target", query,
          "domain", domainOf(result),        // "session" | "path" | "alias" | "zoxide" | "miss"
          "resolved_path", resolvedPathOf(result)) // session name for SessionResult; dir for PathResult; "" for MissResult
  }
  ```
  Derive `domain`/`resolved_path` from the result type: `*SessionResult` → `domain = r.Domain` (`"session"`), `resolved_path = r.Name`; `*PathResult` → `domain = r.Domain` (`"path"`/`"alias"`/`"zoxide"`), `resolved_path = r.Path`; `*MissResult` → `domain = "miss"`, `resolved_path = ""`.
- Emit the line for the successful-classification path only. On a mid-chain hard error from `Resolve` (`*DirNotFoundError`), return the error without emitting a decision line (the classification did not complete). The user-facing hard-fail stderr message (Task 1.2) is separate from this log line.
- Keep `internal/resolver` free of any logging import — the component is bound and emitted only in `cmd/open.go`.
- Use message string `"resolved"` (stable, greppable) — INFO level, consistent with the existing per-`open` `process: exec` INFO line.

**Acceptance Criteria**:
- [ ] A session-name hit emits one INFO `resolve` record with `domain=session` and `resolved_path=<session name>`.
- [ ] A path/alias/zoxide hit emits one INFO `resolve` record with the matching `domain` and `resolved_path=<resolved dir>`.
- [ ] A total miss emits one INFO `resolve` record with `domain=miss` and an empty `resolved_path` (in addition to the separate stderr hard-fail error).
- [ ] A glob target (`HasGlobMeta(target) == true`) emits NO `resolve` record.
- [ ] The record's level is INFO (reconstructable after the fact; DEBUG would be silent by default).
- [ ] `internal/resolver` imports no logging package — the binding and emission live only in `cmd/open.go`.

**Tests** (use `log.SetTestHandler(t, h)` with the in-file capturing handler pattern already in `cmd/open_test.go`, which remembers the `component` attr delivered via `WithAttrs`):
- `"it logs domain=session and resolved_path=<name> on a session hit"` — resolve `open dev` against a session set containing `dev`; assert one record component=`resolve`, level=INFO, `target=dev`, `domain=session`, `resolved_path=dev`.
- `"it logs domain=zoxide and the resolved dir on a zoxide mint"` — assert `domain=zoxide`, `resolved_path=<dir>`.
- `"it logs domain=miss with empty resolved_path on a total miss"` — assert `domain=miss`, `resolved_path=""`, and that the command still returns the hard-fail error.
- `"it emits no resolve line for a glob target"` — resolve `open 'api-*'`; assert zero records with component=`resolve`.
- `"the resolve line is INFO level"` — assert `record.Level == slog.LevelInfo`.
- `internal/resolver` guard: `"resolver imports no logging package"` — a small assertion (or rely on the existing source-discipline convention) that `internal/resolver` does not reference `internal/log`.

**Edge Cases**:
- Session hit logs `resolved_path` = the session name (not a directory) — the attr is overloaded per the spec ("resolved directory, or resolved session name for a session hit").
- Miss logs `domain=miss` with empty `resolved_path`; the stderr hard-fail is a separate surface, not the log line.
- Glob targets emit no line — gate on `resolver.HasGlobMeta(target)`, which is deterministic (glob expansion is not a guess). Pins (`-s`/`-p`/`-z`/`-a`, Phase 2) will likewise emit no line, but they do not exist yet.
- One line per resolved guessing-chain target (single-target here; the multi-target burst emits one per such target in Phase 3).

**Context**:
> Spec § Wrong-guess feedback — tmux is the receipt: "`open` logs its resolution decision… The line is emitted from the `open` command body (`cmd/open.go`), where resolution is driven — `internal/resolver` stays a pure, log-free library." "This requires a governed amendment to Portal's closed log taxonomy: this feature adds one new component, `resolve`… with attr keys `target` (raw input), `domain` (session / path / alias / zoxide, or `miss` on a total miss), and `resolved_path` (resolved directory, or resolved session name for a session hit; empty on a miss). This is a spec-recorded amendment, not a call-site invention." Line behaviour: "Level: INFO"; "Guessing-chain targets only… Explicit pins (`-s`/`-p`/`-z`/`-a`) and glob targets are deterministic — no guessing — so they emit no `resolve` line"; "Emitted on a miss too — a total miss uses `domain = miss` with an empty `resolved_path`"; "One line per resolved guessing-chain target."
>
> `resolve` is added to the closed component set by this spec amendment. Note that `log.For` is convention-based (no runtime allowlist guard exists in `internal/log`), so the binding needs no enum update; the CLAUDE.md component catalog is documentation to be kept in sync out-of-band.
>
> Ambiguity note: the spec does not state whether a mid-chain hard error (`DirNotFoundError`, alias→nonexistent dir) emits a `resolve` line. This task chooses to emit the line only on a completed classification (session/path/alias/zoxide/miss) and to surface `DirNotFoundError` as the error without a decision line, since no domain decision was reached. Flag for review if a `domain=alias` line on that error path is wanted.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § Wrong-guess feedback — tmux is the receipt (the `resolve` component amendment and the line's behaviour).

---

## cli-verb-surface-redesign-1-5 | approved

### Task 1.5: `-f/--filter` picker redirect + mutual exclusivity

**Problem**: Removing the implicit picker-with-filter fallback (Task 1.2) leaves the filtered-picker mechanic without an explicit door. The redesign adds `-f/--filter <text>` as the sole non-composing flag: "skip resolution, open the picker pre-filtered." It must be mutually exclusive with a positional target (a usage error otherwise) and must not disturb the no-arg picker.

**Solution**: Add a `-f/--filter` string flag to `openCmd`. When set, skip resolution entirely and launch the TUI on the Sessions page pre-filled with the filter text via the existing `openTUI` (whose `initialFilter` parameter already exists). Reject `-f` combined with a positional target as a usage error, and reject an empty `-f` value (consistent with `-e ""`). Preserve the no-arg picker unchanged.

**Outcome**: `open -f blog` opens the Sessions picker pre-filtered to "blog" and never resolves; `open -f blog api` (filter + positional) is a usage error (exit 2); `open` with no args still launches the picker exactly as before.

**Do**:
- In `cmd/open.go` `init()`, register the flag: `openCmd.Flags().StringP("filter", "f", "", "open the picker pre-filtered by <text> (skips resolution)")`.
- In `openCmd.RunE`, handle `-f` **before** resolution (before `buildQueryResolver`/`Resolve`):
  - Read `filterVal, _ := cmd.Flags().GetString("filter")` and `filterChanged := cmd.Flags().Changed("filter")`.
  - If `filterChanged`:
    - If a positional target is present (`destination != ""` after `parseCommandArgs`) → `return NewUsageError("cannot use -f/--filter with a target")` (exit 2). Scope: positionals only — the pin flags (`-s`/`-p`/`-z`/`-a`) do not exist until Phase 2, so their mutual exclusion with `-f` is out of scope here.
    - If `filterVal == ""` → `return NewUsageError("-f/--filter value must not be empty")` (mirrors the existing empty-`-e` guard).
    - Otherwise → `return openTUIFunc(cmd, filterVal, command, serverWasStarted(cmd))`. This threads any present `-e`/`--` command straight through, preserving today's "command present ⇒ picker specializes to Projects" behaviour (the filtered-Projects variant the spec assigns to Phase 2 falls out for free — no special-casing needed here).
- Leave the no-arg path (`destination == ""` and `!filterChanged`) exactly as today: `return openTUIFunc(cmd, "", command, serverWasStarted(cmd))`.
- Do not route `-f` through the resolver — it is a picker redirect, not a target.

**Acceptance Criteria**:
- [ ] `open -f <text>` (no positional) calls `openTUIFunc` with `initialFilter == <text>` and never invokes the query resolver.
- [ ] `open -f <text> <positional>` returns a `UsageError` (exit 2); the resolver and picker are not invoked.
- [ ] `open -f ""` (explicitly empty) returns a `UsageError` (exit 2).
- [ ] `open` with no args and no `-f` still launches the picker with an empty filter — unchanged (regression guard).
- [ ] `-f` skips resolution: no `resolve` log line is emitted for a `-f` invocation (resolution never runs).

**Tests** (`cmd/open_test.go`, overriding `openTUIFunc` to capture args; inject `bootstrapDeps` with a nop runner as the existing tests do):
- `"open -f text opens the picker pre-filtered and skips resolution"` — override `openTUIFunc` to capture `initialFilter`; Execute `open -f blog`; assert captured filter == `"blog"` and the resolver seam (`openDeps`) was never consulted (e.g. a failing/should-not-be-called resolver still yields no error).
- `"open -f with a positional target is a usage error"` — Execute `open -f blog api`; assert the error is a `*UsageError` and `openTUIFunc` was not called.
- `"open -f with an empty value is a usage error"` — Execute `open -f ""` (or `open --filter=`); assert `*UsageError`.
- `"open with no args still launches the picker (regression)"` — Execute `open`; assert `openTUIFunc` called with empty `initialFilter`.
- `"open -f text -e cmd threads the command to the picker"` — Execute `open -f web -e claude`; assert captured `initialFilter=="web"` and captured `command==["claude"]` (preserving today's Projects-mode specialization).

**Edge Cases**:
- `-f` + positional target ⇒ usage error (exit 2). Scoped to positionals only in Phase 1; the pin flags do not exist yet, so `-f`↔pin exclusivity is Phase 2.
- Empty `-f` value ⇒ usage error (planner decision: the spec does not explicitly define empty `-f`; this mirrors the existing empty-`-e` guard for consistency — flag for review if "empty `-f` opens the unfiltered picker" is preferred instead).
- `-f` alone ⇒ Sessions picker pre-filled; the user can toggle to Projects from there (the command variant that starts in Projects mode is the existing command-present behaviour).
- No-arg `open` ⇒ picker unchanged — the regression this feature must not break.

**Context**:
> Spec § `-f/--filter` is the sole non-composing flag: "`-f` is not a target — it is a 'skip resolution, open the picker pre-filtered' redirect. It is mutually exclusive with positional targets and with every other pin flag (usage error otherwise). Plain `-f <text>` (no command) opens the picker on the default Sessions page with the text pre-filled — matching the removed implicit picker-with-filter fallback it replaces." Spec § Miss handling names `-f blog` as the escape hatch the miss message points at, so this flag is what makes that suggestion real. Spec § Command passthrough / picker: "`-f <text> -e <cmd>` likewise coheres (filtered Projects picker running the command)."
>
> Scope note: mutual exclusivity with the pin flags (`-s`/`-p`/`-z`/`-a`) is Phase 2 (the pins do not exist in Phase 1); this task enforces exclusivity against positional targets only. The `initialFilter` parameter already exists on `openTUI` (`cmd/open.go`) and on `tui.Build` via `tui.Deps.InitialFilter`, so no TUI change is required — this is purely a flag + branch in `openCmd.RunE`.
>
> Ambiguity note: the spec is silent on an explicitly empty `-f` value. This task treats it as a usage error to match the empty-`-e` precedent; the alternative (open the unfiltered picker) is noted for review.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Flags & Command Passthrough (`-f/--filter`); § Miss handling — total miss is a hard fail (the `-f` escape hatch).
