---
phase: 2
phase_name: "Domain-pinning flags & mint-scoped command passthrough"
total: 7
---

## cli-verb-surface-redesign-2-1 | approved

### Task 2.1: `-s/--session` pin — session-domain attach, never mints, hard-fails on miss

**Problem**: Phase 1 gave `open` a bare-positional resolution chain (glob pre-check → exact session → path → alias → zoxide). There is no way to name a target *explicitly* as session-domain — needed for scriptability, for spawned windows (Phase 3 execs `open --session <name> --ack …`), and to reach a session whose name would otherwise be shadowed by a higher-precedence directory match. The spec's `-s/--session` pin fills this: it pins the session domain, **attaches** on a hit, **never mints**, and **hard-fails** on a miss without ever popping the picker.

**Solution**: Add a `-s/--session <name-or-glob>` string flag to `openCmd`. When set, resolve the value against the **user-visible** session set only (exact name, or `filepath.Match` glob expansion) via a new `QueryResolver.ResolveSessionPin` method, then dispatch a session-domain hit to the existing inside/outside connector (`openSessionFunc`, from Phase 1). A miss returns a plain hard-fail error (exit 1); the pin never falls through to path/alias/zoxide and never opens the TUI. Introduce a shared `openResolved` dispatch helper so this pin, the later mint pins (2-2/2-3/2-4), and the bare-positional path all route a resolved result to the same outcome switch.

**Outcome**: `open -s api-x7Kd9a` attaches the running session (`switch-client` inside tmux / exec `tmux attach-session -t =<name>` outside); `open -s 'api-*'` (glob) attaches the sole/first user-visible match; `open -s _portal-saver`, `open -s nope`, and an empty session set all hard-fail with `No session found: <value>` (exit 1) and never touch path/alias/zoxide or the picker; the pin emits no `resolve` log line (it is deterministic).

**Do**:
- In `internal/resolver/query.go`, add `func (qr *QueryResolver) ResolveSessionPin(query string) (QueryResult, error)`:
  - Fetch the user-visible set once via `qr.sessions.ListSessionNames()` (Phase 1's `SessionLister`; a nil/error/empty result collapses to "no sessions" exactly as the bare path does).
  - If `HasGlobMeta(query)` (Phase 1 predicate): `matches := MatchSessions(query, names)`; on `len(matches) == 0` return a session-not-found hard error, else return `&SessionResult{Name: matches[0], Domain: "glob"}, nil`.
  - Otherwise if `slices.Contains(names, query)` return `&SessionResult{Name: query, Domain: "session"}, nil`; else return the session-not-found hard error.
  - The hard error is a plain error carrying `fmt.Errorf("No session found: %s", query)` — the exact wording the retired `attach` command used (`cmd/attach.go:55`), so `-s` is behaviourally identical to `attach` (Phase 5 deletes `attach` and points at `open --session`). It is NOT a `UsageError` (a missing session is a runtime resolution failure → exit 1).
  - This method never consults `qr.aliases` / `qr.zoxide` / `qr.dirValidator` — session-domain only.
- In `cmd/open.go`:
  - Register the flag in `init()`: `openCmd.Flags().StringP("session", "s", "", "attach the named session or session glob (session-domain; never mints)")`.
  - Introduce a shared dispatch helper reused by every resolved single target:
    ```go
    // openResolved dispatches a successfully-resolved target to its outcome:
    // a session-domain hit attaches, a directory-domain hit mints. Shared by the
    // bare-positional path and every domain pin. (Task 2-6 adds the mint-scoped
    // command guard here.)
    func openResolved(cmd *cobra.Command, result resolver.QueryResult, command []string) error {
        switch r := result.(type) {
        case *resolver.SessionResult:
            return openSessionFunc(cmd, r.Name)
        case *resolver.PathResult:
            return openPathFunc(cmd, r.Path, command)
        default:
            return fmt.Errorf("unexpected resolution result: %T", result)
        }
    }
    ```
    Consolidate the Phase-1 inline `case *SessionResult` / `case *PathResult` arms in `openCmd.RunE` into this helper; keep the `*MissResult` arm inline in the bare path (pins never yield `*MissResult`).
  - In `openCmd.RunE`, add the pin dispatch block **after** the `-f` handling (Phase 1) and **before** the no-target picker early-return (`if destination == "" { return openTUIFunc(...) }`). Relocate that no-target early-return below the pin block so a `-s` invocation (which has an empty positional `destination`) resolves the pin instead of opening the picker:
    ```go
    if cmd.Flags().Changed("session") {
        sessionVal, _ := cmd.Flags().GetString("session")
        qr, err := buildQueryResolver()   // Phase-1 helper (now wires the SessionLister)
        if err != nil { return err }
        result, err := qr.ResolveSessionPin(sessionVal)
        if err != nil { return err }       // hard-fail: No session found: <value>
        return openResolved(cmd, result, command)
    }
    ```
  - Do NOT emit a `resolve` log line for the pin (Phase 1 gates that line to the bare-positional path only; the pin path never reaches it).
- Leave the command→attach interaction to Task 2-6 (the "command + attach target ⇒ usage error" guard lands in `openResolved`); do not thread `command` into the session attach here.

**Acceptance Criteria**:
- [ ] `open -s <exact-user-visible-name>` returns `*SessionResult{Domain:"session"}` and routes to `openSessionFunc` (attach) — never `openPathFunc`/mint, never `openTUIFunc`/picker.
- [ ] `open -s '<glob>'` expands against the user-visible set via `filepath.Match`; a single/multi match attaches the first match (`Domain:"glob"`); the multi-match window fan-out is deferred to Phase 3.
- [ ] `-s` matching only a `_`-prefixed internal session (`_portal-saver`, `_portal-bootstrap`) is a miss — those names are absent from `ListSessionNames`, so it hard-fails exactly as a nonexistent name would.
- [ ] A `-s` miss (no exact match, zero glob matches, or empty session set) returns `No session found: <value>` as a plain error (exit 1); the TUI picker is never launched and path/alias/zoxide are never consulted.
- [ ] Attach uses `switch-client` inside tmux and exec `tmux attach-session -t =<name>` outside — delegated to the already-tested `buildSessionConnector` via `openSessionFunc`.
- [ ] `-s` emits no `resolve` log record (deterministic pin).

**Tests**:
- `internal/resolver/query_test.go` (or new `pin_test.go`): `"ResolveSessionPin returns SessionResult for an exact user-visible hit"` — lister `["api-x7Kd9a","web-3fJk"]`, `ResolveSessionPin("api-x7Kd9a")` → `*SessionResult{Name:"api-x7Kd9a",Domain:"session"}`.
- `"ResolveSessionPin expands a glob against the user-visible set"` — lister `["api-1","api-2","web-3"]`, `ResolveSessionPin("api-*")` → `*SessionResult{Domain:"glob",Name:"api-1"}`.
- `"ResolveSessionPin hard-fails on zero matches"` — lister `["web-3"]`, `ResolveSessionPin("api-9")` → `err.Error()=="No session found: api-9"`, result nil.
- `"ResolveSessionPin never matches an internal session"` — lister returns the filtered set (no `_portal-saver`), `ResolveSessionPin("_portal-saver")` → `No session found: _portal-saver`.
- `"ResolveSessionPin never consults aliases/zoxide"` — inject alias `{"api-9": <dir>}` + a zoxide that would match; `ResolveSessionPin("api-9")` still misses (proves session-only).
- `"ResolveSessionPin treats an empty session set as a miss"` — lister `[]`, any query → hard error.
- `cmd/open_test.go`: `"open -s routes an exact session hit to the connector"` — set `openDeps.SessionLister` to a fake returning `["dev"]`, override `openSessionFunc` to capture, Execute `open -s dev`; assert captured name `"dev"` and `openPathFunc`/`openTUIFunc` NOT called.
- `cmd/open_test.go`: `"open -s with no matching session hard-fails and does not open the picker"` — fake lister `["web"]`, override `openTUIFunc` to fail the test if called; Execute `open -s api`; assert `err.Error()=="No session found: api"` and `openTUIFunc` not called.
- `cmd/open_test.go`: `"open -s emits no resolve log line"` — install the capturing handler (`log.SetTestHandler`), Execute `open -s dev`; assert zero records with component `resolve`.

**Edge Cases**:
- `-s` matching only a `_`-prefixed internal session ⇒ miss (user-visible `ListSessionNames` view; never `HasSession`, which would see internal sessions).
- Empty session set ⇒ hard-fail (no picker).
- `-s` bypasses the guessing chain: it never tries path/alias/zoxide even when a same-named dir or alias exists.
- Session glob under `-s` expanding to >1 session ⇒ single-target attaches the first match; the per-match burst is Phase 3.
- Inside-tmux `switch-client` vs outside exec attach — selected by `buildSessionConnector` (unchanged from Phase 1).

**Context**:
> Spec § Domain-pinning flags: `-s/--session <name-or-glob>` pins "exact session / session glob", semantics "attach; hard fail on miss; never mints". Spec § Pinned-domain contract — never falls back to the picker: "Every domain pin (`-s`, `-p`, `-z`, `-a`) hard-fails on unresolvable and never falls back to the TUI picker — a spawned window or script must never pop a TUI. `--session` never mints (a bare name has no directory to mint from)." Spec § Session set — user-visible only: the `-s/--session` pin "matches only against the user-visible session set … never matchable as `open` targets" for internal sessions.
>
> Ambiguity / planner decision (flag for review): the spec fixes no exact miss-message string for `-s`. This task reuses `No session found: <value>` — the verbatim string the retired `attach` command emitted (`cmd/attach.go:55`) — so `open --session` is byte-identical to `attach` on the miss path, easing the Phase 5 `attach`→`open --session` retirement. The bare-positional miss's `-f` suggestion is deliberately NOT reused here (`-f` is a picker redirect, mutually exclusive with pins, so suggesting it on a pin miss would be contradictory).
>
> Interim note: the "command + `-s` attach ⇒ usage error" guard lands in Task 2-6 (inside `openResolved`); until then a command alongside `-s` is silently ignored on the attach, matching Phase 1's unguarded bare-session-attach behaviour. The `-f`↔`-s` mutual-exclusion lands in Task 2-5; until then `open -f x -s y` hits the earlier `-f` branch.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Flags & Command Passthrough (Domain-pinning flags; Pinned-domain contract); § `portal open` — Grammar & Target Resolution (Session set — user-visible only); § `attach` — Retired (Spawned-window contract).

---

## cli-verb-surface-redesign-2-2 | approved

### Task 2.2: `-p/--path` pin — path-domain mint, dir must exist

**Problem**: A directory whose name contains glob metacharacters (e.g. `~/tmp/foo[1]`) is **unreachable as a bare positional** — Phase 1's glob pre-check treats it as a session glob, matches zero sessions, and hard-fails. The spec's `-p/--path` pin is the explicit escape: it pins the path domain, bypasses the glob pre-check entirely, mints a fresh session at the (existing) directory, and hard-fails if the directory does not exist — never popping the picker.

**Solution**: Add a `-p/--path <dir>` string flag to `openCmd`. When set, resolve the value through a new `QueryResolver.ResolvePathPin` that reuses `resolver.ResolvePath` (tilde/relative expansion + existence + is-directory validation), then dispatch the resulting `*PathResult` to `openPathFunc` (mint), threading any `-e`/`--` command. `ResolvePath` operates on the literal path, so glob metacharacters in the directory name are never expanded and the dir is reachable.

**Outcome**: `open -p ~/tmp/foo[1]` (an existing dir literally named `foo[1]`) mints a fresh `{project}-{nanoid}` session there; `open -p ~/Code/blog` mints at the expanded/validated absolute path; `open -p /gone` hard-fails with `Directory not found: /gone` (exit 1) and never opens the picker; `-p` never consults session/alias/zoxide matching.

**Do**:
- In `internal/resolver/query.go`, add `func (qr *QueryResolver) ResolvePathPin(dir string) (QueryResult, error)`:
  - `resolved, err := ResolvePath(dir)` (in `internal/resolver/path.go` — expands tilde via `ExpandTilde`, `filepath.Abs`, `os.Stat`, rejects non-existent with `Directory not found: <abs>` and non-directories with `not a directory: <abs>`).
  - On error return `(nil, err)` (hard-fail, exit 1). On success return `&PathResult{Path: resolved, Domain: "path"}, nil`.
  - This method calls neither `HasGlobMeta`/`MatchSessions`, `qr.sessions`, `qr.aliases`, nor `qr.zoxide` — path-domain only. (It does not go through `qr.validatedPath`; `ResolvePath` already validates existence and additionally rejects a non-directory file, which the pin should surface.)
- In `cmd/open.go`:
  - Register the flag in `init()`: `openCmd.Flags().StringP("path", "p", "", "mint a new session at the given directory (path-domain; dir must exist)")`.
  - In `openCmd.RunE`, add a pin branch alongside the Task 2-1 `-s` block (after `-f` handling, before the no-target picker return):
    ```go
    if cmd.Flags().Changed("path") {
        pathVal, _ := cmd.Flags().GetString("path")
        qr, err := buildQueryResolver()
        if err != nil { return err }
        result, err := qr.ResolvePathPin(pathVal)
        if err != nil { return err }        // hard-fail: Directory not found / not a directory
        return openResolved(cmd, result, command)   // PathResult → openPathFunc (mint), threads command
    }
    ```
  - Do NOT emit a `resolve` log line for the pin (deterministic).

**Acceptance Criteria**:
- [ ] `open -p <existing-dir>` returns `*PathResult{Domain:"path"}` and mints via `openPathFunc` (fresh session, never attach, never picker).
- [ ] A directory whose name contains glob metacharacters (`~/tmp/foo[1]`) is reachable via `-p`: `ResolvePath` stats the literal path, so `[1]` is not expanded and the mint proceeds (contrast: the same value as a bare positional hard-fails via the Phase-1 glob pre-check).
- [ ] Tilde and relative paths are expanded/absolutised by the reused `ResolvePath` (`~` → home, `.`/relative → absolute).
- [ ] A non-existent dir hard-fails with `Directory not found: <abs>` (exit 1); a path pointing at a file hard-fails with `not a directory: <abs>`; the picker is never launched on either.
- [ ] `-p` never runs session/alias/zoxide matching (path-domain only) and emits no `resolve` log record.
- [ ] A present `-e`/`--` command threads into the minted session (`openPathFunc(cmd, path, command)`).

**Tests**:
- `internal/resolver/query_test.go`: `"ResolvePathPin returns PathResult for an existing dir"` — pass `t.TempDir()`, assert `*PathResult{Path:<abs>,Domain:"path"}`.
- `"ResolvePathPin reaches a dir whose name contains glob metacharacters"` — create `t.TempDir()/foo[1]` with `os.MkdirAll`, `ResolvePathPin(<dir>/foo[1])` → `*PathResult` at that literal path (no expansion, no error).
- `"ResolvePathPin hard-fails for a non-existent dir"` — `ResolvePathPin("/nonexistent/xyz")` → error `Directory not found: /nonexistent/xyz`, result nil.
- `"ResolvePathPin hard-fails for a file"` — write a temp file, `ResolvePathPin(<file>)` → error `not a directory: <file>`.
- `"ResolvePathPin expands tilde"` — `ResolvePathPin("~")` → `*PathResult{Path:<home>}` (home is a dir).
- `cmd/open_test.go`: `"open -p mints at an existing dir and does not open the picker"` — override `openPathFunc` to capture, override `openTUIFunc` to fail if called; Execute `open -p <tempdir>`; assert captured path `==<abs tempdir>`.
- `cmd/open_test.go`: `"open -p with a glob-named dir mints (bypasses the glob pre-check)"` — create `<tempdir>/foo[1]`, Execute `open -p <tempdir>/foo[1]`; assert `openPathFunc` captured that path (and no miss/error).
- `cmd/open_test.go`: `"open -p threads a command into the mint"` — Execute `open -p <tempdir> -e claude`; assert the captured command reaching `openPathFunc` is `["claude"]`.

**Edge Cases**:
- A dir whose name contains glob metacharacters (`~/tmp/foo[1]`) is reachable via `-p` (bypasses the Phase-1 glob pre-check) — the whole reason `-p` is the documented escape for such names.
- Non-existent dir ⇒ `Directory not found` hard-fail (exit 1), never the picker.
- File (not a directory) ⇒ `not a directory` hard-fail.
- Tilde/relative expansion is reused from `ResolvePath` (single source of truth for path expansion).
- `-p` never runs session/alias/zoxide matching.

**Context**:
> Spec § Domain-pinning flags: `-p/--path <dir>` pins "directory path", semantics "mint new session; dir must exist". Spec § Glob targets: "A directory path whose name contains glob metacharacters (e.g. `~/tmp/foo[1]`) is unreachable as a bare positional — the glob pre-check treats it as a session glob, it matches zero sessions, and it hard-fails. Reach it with `-p <dir>`, which pins the path domain and bypasses glob detection." Spec § Pinned-domain contract: `-p` "mints per Axiom 2 on a hit and hard-fails on a miss" and "never falls back to the TUI picker."
>
> `ResolvePath` (`internal/resolver/path.go`) already implements tilde/relative expansion + existence + is-directory checks and emits the exact `Directory not found:`/`not a directory:` strings, so the pin reuses it rather than re-implementing validation — keeping expansion behaviour single-sourced with the bare-positional path branch. The mint itself is unchanged Phase-1 `openPathFunc` (`QuickStart`/`CreateFromDir`, always a fresh `{project}-{nanoid}`).

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Flags & Command Passthrough (Domain-pinning flags; Pinned-domain contract); § `portal open` — Multi-Target Burst Mechanics (Glob targets — the glob-named-dir escape).

---

## cli-verb-surface-redesign-2-3 | approved

### Task 2.3: `-a/--alias` pin — alias-domain mint, key globs, shadow bypass, hard-fails on unknown key

**Problem**: An alias key can be *shadowed* by a same-named session (session is higher in the precedence chain), leaving no bare-positional way to reach the aliased directory. The spec's `-a/--alias` pin is the only way to reach a shadowed alias key, rounds out the four resolution domains, and — because alias keys are a finite Portal-owned namespace — supports **key globs** (`-a 'workflow-*'`). It mints at the aliased dir on a hit and hard-fails on an unknown key without popping the picker.

**Solution**: Add an `-a/--alias <key-or-glob>` string flag to `openCmd`. When set, resolve the value through a new `QueryResolver.ResolveAliasPin` that looks the key up directly in the alias store (bypassing the session→path→alias precedence), or — when the value is a glob — expands it against the enumerated alias key namespace via `filepath.Match`, then validates the resolved directory exists and mints via `openPathFunc`. Enumeration requires exposing the alias keys, so extend the resolver's alias dependency with a `Keys()` method (satisfied by `*alias.Store`).

**Outcome**: `open -a api` mints at the `api` alias's directory even when a session named `api` exists (the pin bypasses precedence); `open -a 'workflow-*'` matching one key mints at its dir; an unknown key (or zero glob matches) hard-fails with `No alias found: <value>` (exit 1) and never opens the picker; an alias whose directory no longer exists on disk hard-fails with `Directory not found: <dir>`.

**Do**:
- In `internal/alias/store.go`, add `func (s *Store) Keys() []string` returning the alias names sorted (derive from the internal `s.aliases` map, mirroring `List`; e.g. collect keys then `slices.Sort`). This exposes the finite key namespace for glob enumeration without leaking `[]Alias` into the resolver.
- In `internal/resolver/query.go`, extend the alias seam so the pin can enumerate keys:
  ```go
  type AliasLookup interface {
      Get(name string) (string, bool)
      Keys() []string   // NEW — the finite Portal-owned alias-key namespace, for -a key-glob matching
  }
  ```
  Update the cmd/resolver test fakes (`testAliasLookup` in `cmd/open_test.go`, the `AliasLookup` fakes in `query_test.go`) to implement `Keys()` (return the map's sorted keys).
- Add `func (qr *QueryResolver) ResolveAliasPin(key string) (QueryResult, error)`:
  - If `HasGlobMeta(key)`: `matches := matchKeys(key, qr.aliases.Keys())` where `matchKeys` applies `filepath.Match(pattern, key)` per key (a `filepath.Match` error counts as no match, mirroring `MatchSessions`); on `len(matches) == 0` return the unknown-key hard error, else use `matches[0]` as the resolved key.
  - Otherwise the resolved key is `key` itself if `qr.aliases.Get(key)` reports found; on not-found return the unknown-key hard error.
  - Look up the resolved key's path (`qr.aliases.Get(resolvedKey)`) and validate it on disk: reuse the same disk-existence validation the bare alias arm uses (`qr.validatedPath`) so a gone directory yields `*DirNotFoundError` (`Directory not found: <dir>`); set the result `Domain: "alias"`.
  - The unknown-key hard error is a plain error `fmt.Errorf("No alias found: %s", key)` (exit 1; NOT a `UsageError`). The method never consults `qr.sessions` or `qr.zoxide`.
- In `cmd/open.go`:
  - Register the flag in `init()`: `openCmd.Flags().StringP("alias", "a", "", "mint a new session at the given alias key or key glob (alias-domain)")`.
  - In `openCmd.RunE`, add the pin branch alongside 2-1/2-2 (after `-f` handling, before the no-target picker return):
    ```go
    if cmd.Flags().Changed("alias") {
        aliasVal, _ := cmd.Flags().GetString("alias")
        qr, err := buildQueryResolver()
        if err != nil { return err }
        result, err := qr.ResolveAliasPin(aliasVal)
        if err != nil { return err }        // hard-fail: No alias found / Directory not found
        return openResolved(cmd, result, command)   // PathResult → openPathFunc (mint), threads command
    }
    ```
  - Do NOT emit a `resolve` log line (deterministic pin).

**Acceptance Criteria**:
- [ ] `open -a <key>` mints at the aliased dir via `openPathFunc` even when a same-named session exists (the pin bypasses the session→path→alias precedence and never checks sessions).
- [ ] `open -a '<glob>'` matches over the enumerated alias key namespace (`Keys()` + `filepath.Match`); a single match mints; multi-match window fan-out is deferred to Phase 3 (single-target mints the first match).
- [ ] An unknown key, or a glob matching zero keys, hard-fails with `No alias found: <value>` (exit 1); the picker is never launched.
- [ ] An alias whose directory no longer exists on disk hard-fails with `Directory not found: <dir>` (via the shared disk validation) — distinct from the unknown-key miss.
- [ ] `-a` never consults session or zoxide matching and emits no `resolve` log record.
- [ ] A present `-e`/`--` command threads into the minted session.

**Tests**:
- `internal/alias/store_test.go`: `"Keys returns sorted alias names"` — seed `{"b":.., "a":..}`, assert `Keys() == ["a","b"]`.
- `internal/resolver/query_test.go`: `"ResolveAliasPin mints at the aliased dir for a known key"` — alias `{"api": <existing dir>}`, `ResolveAliasPin("api")` → `*PathResult{Path:<dir>,Domain:"alias"}`.
- `"ResolveAliasPin bypasses a shadowing session"` — lister `["api"]` (a session named `api`), alias `{"api": <dir>}`, `ResolveAliasPin("api")` → `*PathResult` (proves sessions are never consulted).
- `"ResolveAliasPin expands a key glob to a single match"` — aliases `{"workflow-a":<dir>,"other":..}`, `ResolveAliasPin("workflow-*")` → `*PathResult` at `workflow-a`'s dir.
- `"ResolveAliasPin hard-fails on an unknown key"` — empty aliases, `ResolveAliasPin("nope")` → `No alias found: nope`, result nil.
- `"ResolveAliasPin hard-fails on a glob matching zero keys"` — aliases `{"api":..}`, `ResolveAliasPin("zzz-*")` → `No alias found: zzz-*`.
- `"ResolveAliasPin errors when the aliased dir is gone"` — alias `{"api":"/gone"}`, dir validator rejects `/gone`, `ResolveAliasPin("api")` → `*DirNotFoundError` (`Directory not found: /gone`).
- `cmd/open_test.go`: `"open -a mints and never opens the picker"` — `openDeps` with alias `{"api": <tempdir>}` + validator marking `<tempdir>` existing, override `openPathFunc`/`openTUIFunc`; Execute `open -a api`; assert `openPathFunc` captured `<tempdir>` and `openTUIFunc` not called.
- `cmd/open_test.go`: `"open -a on an unknown key hard-fails without the picker"` — empty aliases; Execute `open -a nope`; assert `err.Error()=="No alias found: nope"` and `openTUIFunc` not called.

**Edge Cases**:
- Alias key shadowed by a same-named session — `-a` mints at the aliased dir (bypasses precedence; sessions never consulted).
- Unknown key ⇒ hard-fail, never the picker.
- Key glob single-match mints; multi-match ⇒ Phase 3 burst (single-target mints first match).
- Glob matches over the finite Portal-owned key namespace enumerated via `Keys()` (not the filesystem).
- Aliased dir no longer on disk ⇒ `Directory not found` error (distinct from unknown-key miss).

**Context**:
> Spec § Domain-pinning flags: `-a/--alias <key-or-glob>` pins "alias key / key glob", semantics "mint at aliased dir; hard fail on unknown key"; "`-a` is the only way to reach an alias key shadowed by a same-named session, and rounds out the four resolution domains." Spec § Glob targets: "`-a` accepts key globs (alias keys are a finite Portal-owned namespace: `-a 'workflow-*'`)." Spec § Pinned-domain contract: "never falls back to the TUI picker."
>
> Ambiguity / planner decision (flag for review): the spec fixes no exact unknown-key miss string. This task uses `No alias found: <value>`, matching the capitalised `No session found:` / `Directory not found:` house style of the existing user-facing errors. The gone-directory path reuses the existing `*DirNotFoundError` (`Directory not found: <dir>`) from `qr.validatedPath`, so an on-disk-missing alias dir is distinguishable from an unknown key. Extending `AliasLookup` with `Keys()` is the minimal way to enumerate the finite key namespace without the resolver importing `internal/alias`; `*alias.Store` gains the method and the test fakes are updated in lockstep.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Flags & Command Passthrough (Domain-pinning flags; Pinned-domain contract); § `portal open` — Multi-Target Burst Mechanics (Glob targets — `-a` key globs).

---

## cli-verb-surface-redesign-2-4 | approved

### Task 2.4: `-z/--zoxide` pin — zoxide-domain mint, explicit not-installed error, hard-fails on no match

**Problem**: In the bare-target chain, any zoxide error (not installed, or no match) is swallowed — resolution silently falls through to the next domain and ultimately to a total miss. The spec's `-z/--zoxide` pin makes zoxide's outcome explicit: it mints at zoxide's best match on a hit, **errors distinctly when zoxide is not installed** (`ErrZoxideNotInstalled`, so a script sees why), hard-fails on no match, and never pops the picker.

**Solution**: Add a `-z/--zoxide <query>` string flag to `openCmd`. When set, resolve the value through a new `QueryResolver.ResolveZoxidePin` that queries zoxide, surfaces `ErrZoxideNotInstalled` verbatim (distinct from the bare chain's silent fall-through), treats a no-match as a hard-fail, validates the resolved directory exists, and dispatches a `*PathResult` to `openPathFunc` (mint).

**Outcome**: `open -z blog` mints at zoxide's best-match dir (after verifying it exists on disk); `open -z blog` with zoxide absent fails with `zoxide is not installed` (exit 1); `open -z zzz` with no zoxide match hard-fails with `No zoxide match for: zzz` (exit 1); neither error opens the picker; `-z` never consults session/path/alias matching.

**Do**:
- In `internal/resolver/query.go`, add `func (qr *QueryResolver) ResolveZoxidePin(query string) (QueryResult, error)`:
  - `path, err := qr.zoxide.Query(query)`.
  - On error: if `errors.Is(err, ErrZoxideNotInstalled)` return `(nil, ErrZoxideNotInstalled)` (surface it explicitly — this is the whole point of the pin); otherwise (e.g. `ErrNoMatch`) return the no-match hard error `fmt.Errorf("No zoxide match for: %s", query)` (exit 1).
  - On success validate the dir exists before minting: reuse `qr.validatedPath(path)` (returns `*DirNotFoundError` if zoxide's best match points at a gone directory), and set `Domain: "zoxide"` on the resulting `*PathResult`.
  - The method never consults `qr.sessions` or `qr.aliases` — zoxide-domain only. (`ZoxideResolver.Query` in `internal/resolver/zoxide.go` already returns `ErrZoxideNotInstalled` when `zoxide` is absent from PATH and `ErrNoMatch` on a non-zero exit.)
- In `cmd/open.go`:
  - Register the flag in `init()`: `openCmd.Flags().StringP("zoxide", "z", "", "mint a new session at zoxide's best match (zoxide-domain; explicit error if zoxide is not installed)")`.
  - In `openCmd.RunE`, add the pin branch alongside 2-1/2-2/2-3 (after `-f` handling, before the no-target picker return):
    ```go
    if cmd.Flags().Changed("zoxide") {
        zoxideVal, _ := cmd.Flags().GetString("zoxide")
        qr, err := buildQueryResolver()
        if err != nil { return err }
        result, err := qr.ResolveZoxidePin(zoxideVal)
        if err != nil { return err }        // hard-fail: zoxide not installed / no match / dir gone
        return openResolved(cmd, result, command)   // PathResult → openPathFunc (mint), threads command
    }
    ```
  - Do NOT emit a `resolve` log line (deterministic pin).

**Acceptance Criteria**:
- [ ] `open -z <query>` resolving to an existing dir returns `*PathResult{Domain:"zoxide"}` and mints via `openPathFunc` (never attach, never picker).
- [ ] With zoxide not installed, `open -z <query>` returns `ErrZoxideNotInstalled` (`zoxide is not installed`, exit 1) — explicitly, distinct from the bare-target chain which silently falls through on the same condition.
- [ ] With zoxide installed but no match, `open -z <query>` hard-fails with `No zoxide match for: <query>` (exit 1); the picker is never launched.
- [ ] The resolved dir is validated on disk before minting; a gone best-match dir yields `Directory not found: <dir>`.
- [ ] `-z` never runs session/path/alias matching (zoxide-domain only) and emits no `resolve` log record.
- [ ] A present `-e`/`--` command threads into the minted session.

**Tests**:
- `internal/resolver/query_test.go`: `"ResolveZoxidePin mints at an existing best-match dir"` — zoxide fake returns `<existing dir>`, validator marks it existing, `ResolveZoxidePin("blog")` → `*PathResult{Path:<dir>,Domain:"zoxide"}`.
- `"ResolveZoxidePin surfaces ErrZoxideNotInstalled explicitly"` — zoxide fake returns `("", ErrZoxideNotInstalled)`, assert `errors.Is(err, ErrZoxideNotInstalled)` and result nil.
- `"ResolveZoxidePin hard-fails on no match"` — zoxide fake returns `("", ErrNoMatch)`, assert `err.Error()=="No zoxide match for: zzz"`.
- `"ResolveZoxidePin errors when the best-match dir is gone"` — zoxide returns `/gone`, validator rejects it, assert `*DirNotFoundError`.
- `"ResolveZoxidePin never consults sessions/aliases"` — inject a session + alias that would match the query; still resolves purely via zoxide.
- `cmd/open_test.go`: `"open -z mints and does not open the picker"` — `openDeps` zoxide → `<tempdir>` (validator existing), override `openPathFunc`/`openTUIFunc`; Execute `open -z blog`; assert `openPathFunc` captured `<tempdir>`, `openTUIFunc` not called.
- `cmd/open_test.go`: `"open -z with zoxide absent errors explicitly and does not open the picker"` — `openDeps` zoxide returns `ErrZoxideNotInstalled`; Execute `open -z blog`; assert `errors.Is(err, resolver.ErrZoxideNotInstalled)` and `openTUIFunc` not called.
- `cmd/open_test.go`: `"open -z with no match hard-fails"` — zoxide returns `ErrNoMatch`; Execute `open -z zzz`; assert `err.Error()=="No zoxide match for: zzz"`.

**Edge Cases**:
- zoxide not installed ⇒ explicit `ErrZoxideNotInstalled` (distinct from the bare chain's silent fall-through to the next domain / total miss).
- No match ⇒ hard-fail (`No zoxide match for: <query>`), never the picker.
- Resolved dir validated to exist before mint (gone best-match ⇒ `Directory not found`).
- `-z` never runs session/path/alias matching.

**Context**:
> Spec § Domain-pinning flags: `-z/--zoxide <query>` pins "zoxide best match", semantics "mint at matched dir; hard fail on no match; explicit error if zoxide not installed". Note: "`-z` differs from the guessing chain on zoxide-absence: pinned `-z` errors when zoxide is not installed (`ErrZoxideNotInstalled`), whereas the bare-target chain treats any zoxide error as 'continue to next domain' (falls through silently)." Spec § Pinned-domain contract: "never falls back to the TUI picker."
>
> `ErrZoxideNotInstalled` and `ErrNoMatch` already exist (`internal/resolver/zoxide.go`) and are returned by `ZoxideResolver.Query`, so the pin distinguishes them via `errors.Is`. Ambiguity / planner decision (flag for review): the no-match hard-fail string `No zoxide match for: <query>` is not spec-fixed; it follows the capitalised house style and stays distinct from the not-installed error so a script can tell the two apart. The dir-existence validation reuses `qr.validatedPath` (same as the bare zoxide arm), so a gone best-match dir surfaces `Directory not found`.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Flags & Command Passthrough (Domain-pinning flags — `-z` note; Pinned-domain contract).

---

## cli-verb-surface-redesign-2-5 | approved

### Task 2.5: `-f/--filter` mutual exclusivity extended to all pin flags

**Problem**: Phase 1 made `-f/--filter` mutually exclusive with a positional target only (the pin flags did not exist yet). Now that `-s`/`-p`/`-z`/`-a` exist (Tasks 2-1..2-4), the spec's "`-f` is the sole non-composing flag" contract must be completed: `-f` combined with *any* pin is a usage error. The one allowed companion is the command (`-e`/`--`), which specialises the picker to filtered-Projects mode rather than conflicting with it.

**Solution**: Extend the existing `-f` handling in `openCmd.RunE` so that, when `-f` is set, the presence of any pin flag (`--session`/`--path`/`--zoxide`/`--alias`) — in addition to the already-rejected positional target — produces a `UsageError` (exit 2). Leave the `-f` + command path untouched (allowed → routed to the picker, threading the command).

**Outcome**: `open -f blog -s api` (and `-p`/`-z`/`-a` variants, singly or in combination) is a usage error (exit 2) — the resolver and picker are never invoked; `open -f blog api` (filter + positional) stays a usage error (Phase 1 regression); `open -f blog` alone still opens the Sessions picker pre-filtered (regression); `open -f web -e claude` is allowed (filtered picker, command threaded).

**Do**:
- In `cmd/open.go` `openCmd.RunE`, in the `-f` branch (the block guarded by `cmd.Flags().Changed("filter")`, before resolution), add a pin-conflict check alongside the existing positional-conflict check:
  - Compute `anyPin := cmd.Flags().Changed("session") || cmd.Flags().Changed("path") || cmd.Flags().Changed("zoxide") || cmd.Flags().Changed("alias")`.
  - If `destination != ""` (positional present) **or** `anyPin` → `return NewUsageError("cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)")`.
  - Keep the existing empty-value guard (`filterVal == "" → NewUsageError`) and the allowed path: otherwise `return openTUIFunc(cmd, filterVal, command, serverWasStarted(cmd))` (command, if present, threads through — the filtered-Projects case owned by Task 2-7).
- Ensure this `-f` branch runs **before** the pin dispatch blocks (2-1..2-4) so `-f -s x` is rejected rather than resolving the pin. (Phase 1 already placed `-f` handling before resolution; keep it first.)
- The command is deliberately NOT part of the exclusivity test — `-f`+command is allowed.

**Acceptance Criteria**:
- [ ] `open -f <text> -s <v>`, `open -f <text> -p <v>`, `open -f <text> -z <v>`, `open -f <text> -a <v>` each return a `*UsageError` (exit 2); the resolver and `openTUIFunc` are not invoked.
- [ ] `open -f <text> -s <v> -p <v2>` (multiple pins alongside `-f`) still returns a `*UsageError`.
- [ ] `open -f <text> <positional>` remains a `*UsageError` (Phase 1 regression).
- [ ] `open -f <text> -e <cmd>` (and `-- <cmd>`) is NOT an exclusivity violation — it calls `openTUIFunc` with the filter text and the command (the filtered-Projects path, verified end-to-end in Task 2-7).
- [ ] `open -f <text>` alone still opens the Sessions picker pre-filtered (regression); no pin/positional means no usage error.
- [ ] `open -f ""` (empty value) remains a `*UsageError` (Phase 1 regression).

**Tests** (`cmd/open_test.go`; override `openTUIFunc` to capture/fail, inject `bootstrapDeps` with a nop runner):
- `"open -f with -s is a usage error"` — Execute `open -f blog -s api`; assert `*UsageError` and `openTUIFunc` not called. Repeat parameterised for `-p <tempdir>`, `-z q`, `-a key`.
- `"open -f with multiple pins is a usage error"` — Execute `open -f blog -s api -p <tempdir>`; assert `*UsageError`.
- `"open -f with a positional is a usage error (regression)"` — Execute `open -f blog api`; assert `*UsageError`.
- `"open -f alone opens the filtered Sessions picker (regression)"` — Execute `open -f blog`; assert `openTUIFunc` captured `initialFilter=="blog"` and no error.
- `"open -f with a command is allowed (not an exclusivity violation)"` — Execute `open -f web -e claude`; assert `openTUIFunc` captured `initialFilter=="web"` and `command==["claude"]`, no error.
- `"open -f empty value is a usage error (regression)"` — Execute `open --filter=`; assert `*UsageError`.

**Edge Cases**:
- `-f` + each of `-s`/`-p`/`-z`/`-a` ⇒ usage error (exit 2).
- `-f` + positional ⇒ usage error (Phase 1 regression).
- `-f` + `-e`/`--` command ⇒ allowed (filtered-Projects picker; not a violation).
- `-f` alone ⇒ Sessions picker pre-filled (regression).
- Multiple pins alongside `-f` ⇒ still a usage error.

**Context**:
> Spec § `-f/--filter` is the sole non-composing flag: "`-f` is not a target — it is a 'skip resolution, open the picker pre-filtered' redirect. It is mutually exclusive with positional targets and with every other pin flag (usage error otherwise)." Spec § Target-set composition: "`-f` is the sole non-composing flag (picker redirect; exclusive with all targets and pins)." Spec § Command passthrough / picker: "The command variant `-f <text> -e <cmd>` is the stated exception: a filtered Projects (mint-only) picker" — so command is explicitly allowed alongside `-f`.
>
> This task closes the Phase-1 scope note (task 1-5 enforced `-f`↔positional only; the pins did not exist). It is purely a guard extension in the existing `-f` branch — no resolver or TUI change.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Flags & Command Passthrough (`-f/--filter` is the sole non-composing flag); § `portal open` — Multi-Target Burst Mechanics (Target-set composition).

---

## cli-verb-surface-redesign-2-6 | approved

### Task 2.6: Command (`-e`/`--`) is mint-scoped — reject on attach targets

**Problem**: A command (`-e`/`--`) is the "open this project with claude running" mechanism, fed to a freshly-minted session's clean pane. An **existing (attach) session has no safe command-injection channel** (`send-keys` corrupts a busy pane; `respawn-pane -k` destroys running work), so a command must never run in an attach target. Phase 1 wired command→mint threading but left command-on-attach silently ignored; the spec requires it to be a **usage error**. Likewise a single target that is all-attach (zero mint targets) plus a command is a usage error.

**Solution**: In the shared `openResolved` dispatch (introduced in Task 2-1), reject a command when the resolved target is a session-domain hit (`*SessionResult`) — for both a bare session name and a `-s` pin — with a `UsageError` (exit 2). Preserve the already-correct parse-time guards (both `-e` and `--` ⇒ usage error; empty command value ⇒ usage error) and the command→mint threading, adding regression coverage.

**Outcome**: `open <exact-session>  -e claude` and `open -s <session> -e claude` are usage errors ("a command needs a new session to run in"); `open <mint-target> -e claude` still mints and runs the command; `open -e vim -- claude` (both spellings) and `open -e ""`/`open --` (empty command) stay usage errors.

**Do**:
- In `cmd/open.go`, add the mint-scoped command guard inside `openResolved` (the shared helper from Task 2-1), in the `*SessionResult` arm:
  ```go
  case *resolver.SessionResult:
      if len(command) > 0 {
          return NewUsageError("a command (-e/--) can only run in a newly-created session, not an existing one")
      }
      return openSessionFunc(cmd, r.Name)
  ```
  Because both the bare-positional path and the `-s` pin dispatch through `openResolved`, this single guard covers both. The `*PathResult` (mint) arm keeps threading `command` into `openPathFunc` unchanged.
- Confirm (no code change; add regression tests) that `parseCommandArgs` already enforces:
  - both `-e` and `--` present ⇒ `NewUsageError("cannot use both -e/--exec and -- to specify a command")`;
  - empty `-e` value ⇒ `NewUsageError("-e/--exec value must not be empty")`; `--` with no trailing args ⇒ `NewUsageError("no command specified after --")`.
- Do NOT reject a command when there is **no target at all** — that case routes to the Projects (mint-only) picker (Task 2-7). The guard fires only when a target resolved to a `*SessionResult`. (At single-target arity, "zero mint targets + a command" is exactly "the sole target is an attach", so the same guard covers it. Multi-target zero-mint — an all-attach explicit set like `open api web -e cmd` — is deferred to Phase 3.)

**Acceptance Criteria**:
- [ ] `open <exact-session-name> -e <cmd>` (bare session attach + command) returns a `*UsageError` (exit 2); no attach happens.
- [ ] `open -s <session> -e <cmd>` (pin attach + command) returns a `*UsageError` (exit 2).
- [ ] `open <mint-target> -e <cmd>` (path/alias/zoxide or `-p`/`-a`/`-z`) still mints and threads the command into `openPathFunc` (regression).
- [ ] `open -e <cmd> -- <cmd2>` (both spellings) returns a `*UsageError` (preserved).
- [ ] `open -e ""` and `open --` (empty command) return a `*UsageError` (preserved).
- [ ] The guard does not fire when there is no target (that path opens the Projects picker — Task 2-7).

**Tests** (`cmd/open_test.go`):
- `"open bare-session-attach with a command is a usage error"` — `openDeps.SessionLister` fake `["dev"]`; override `openSessionFunc` to fail if called; Execute `open dev -e claude`; assert `*UsageError` and `openSessionFunc` not called.
- `"open -s attach with a command is a usage error"` — same lister; Execute `open -s dev -e claude`; assert `*UsageError`.
- `"open mint target with a command still threads the command (regression)"` — `openDeps` alias `{"api": <tempdir>}` (validator existing); override `openPathFunc` to capture; Execute `open api -e claude`; assert captured command `["claude"]`.
- `"open -p with a command threads the command (regression)"` — Execute `open -p <tempdir> -e claude`; assert `openPathFunc` command `["claude"]`.
- `"both -e and -- is a usage error (preserved)"` — Execute `open -e vim -- claude`; assert `*UsageError` with the existing message.
- `"empty -e value is a usage error (preserved)"` — Execute `open -e ""`; assert `*UsageError`.
- `"-- with no command is a usage error (preserved)"` — Execute `open --`; assert `*UsageError`.

**Edge Cases**:
- Command + a session-resolving target (attach) ⇒ usage error — for a bare session name and for a `-s` pin (both flow through `openResolved`).
- Single-target command + zero mint targets ⇒ usage error (identical to the attach case at single-target arity); the multi-target all-attach zero-mint case is deferred to Phase 3.
- Both `-e` and `--` ⇒ usage error (preserved parse-time guard).
- Empty command value (`-e ""` / `--` with no args) ⇒ usage error (preserved).
- Command + mint target threads into the mint (Phase-1 wiring; regression).

**Context**:
> Spec § Command passthrough (`-e` / `--`) — mint-scoped: "The command targets mint surfaces only. A freshly-minted session has a clean pane to be the command's process. An existing (attach) session has no safe injection channel … so a command can never run in an attach target." "`-e` and `--` are two spellings of the same single command — specifying both is a usage error." "Zero mint targets + a command ⇒ usage error. `open api web -e claude` (all existing sessions) → error: the command has no new session to run in. (Erroring beats silently dropping the command.)" "A command with no target is not this case — it opens the picker in mint-only (Projects) mode."
>
> Ambiguity / planner decision (flag for review): the spec fixes no exact command-on-attach usage string; this task uses "a command (-e/--) can only run in a newly-created session, not an existing one", consistent with the existing `-e/--exec …` usage-error house style. The multi-target all-attach zero-mint error (`open api web -e cmd`) is Phase 3 (it requires the target-set union); Phase 2 covers the single-target arity where "attach target + command" fully expresses the rule.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Flags & Command Passthrough (Command passthrough (`-e` / `--`) — mint-scoped; Command-injection-safety note).

---

## cli-verb-surface-redesign-2-7 | approved

### Task 2.7: Command with no target → Projects (mint-only) picker with banner

**Problem**: Task 2-6 makes "command + attach/zero-mint target" a usage error. The one case that must NOT become a usage error is a command with **no target at all** (`open -e claude`): the spec preserves today's behaviour of opening the picker restricted to Projects (mint-only) mode with a `Pick a project to run <cmd>` banner — because Projects only ever mint a fresh, clean session, the command always lands somewhere coherent. This task pins that exemption and the filtered variant.

**Solution**: Ensure `openCmd.RunE` routes a command with no target (and no pin) to `openTUIFunc(cmd, "", command, …)` — the existing Phase-1 wiring that builds the Projects-mode model — and that `-f <text> -e <cmd>` routes to `openTUIFunc(cmd, <text>, command, …)` (filtered Projects). The TUI already renders the Projects page and the `Pick a project to run` banner + command chip when a command is pending; this task is primarily a guardrail/regression: the no-target command path must reach the picker (never the Task 2-6 usage error), with the banner wording verified exactly.

**Outcome**: `open -e claude` / `open -- claude` (no target) opens the picker in Projects mode showing the `Pick a project to run` banner beside the `claude` command chip — not a usage error; `open -f api -e claude` opens the *filtered* Projects picker (distinct from `open -f api`, which lands on the Sessions page); every selection in Projects-only mode mints a fresh session.

**Do**:
- In `cmd/open.go` `openCmd.RunE`, keep/confirm the no-target early-return positioned **after** the `-f` branch (2-5) and the pin blocks (2-1..2-4) and **before** bare-positional resolution: when no positional (`destination == ""`) and no pin flag changed, `return openTUIFunc(cmd, "", command, serverWasStarted(cmd))`. When a command is present it threads through into Projects mode; when absent this is the unchanged no-arg picker.
- Confirm the `-f` branch (2-5, allowed-command path) already threads the command: `return openTUIFunc(cmd, filterVal, command, serverWasStarted(cmd))` — this is the `-f <text> -e <cmd>` filtered-Projects route.
- No TUI change required: `buildTUIModel`/`tui.Build` already switch to `PageProjects` and set command-pending when `command` is non-empty (`TestBuildTUIModel` "command creates model in command-pending mode" → `ActivePage()==PageProjects`), and `internal/tui/notice_band.go` renders `commandBandText = "Pick a project to run"` beside the command chip. Verify the wording is exactly `Pick a project to run` (the spec's `Pick a project to run <cmd>` = this fixed text + the command rendered in the chip).
- Guard against regression from Task 2-6: the mint-scoped command guard must fire only on a resolved `*SessionResult`, never on the no-target path — so `open -e claude` reaches the picker, not the usage error.

**Acceptance Criteria**:
- [ ] `open -e <cmd>` (no target) calls `openTUIFunc` with empty filter and the command; it is NOT a usage error.
- [ ] `open -- <cmd>` (no target) behaves identically (Projects-mode picker, command threaded).
- [ ] `open -f <text> -e <cmd>` calls `openTUIFunc` with `initialFilter==<text>` and the command (filtered Projects), distinct from `open -f <text>` (no command → Sessions page).
- [ ] The command-pending picker renders the banner text exactly `Pick a project to run` with the command in the chip (verified against `internal/tui/notice_band.go`'s `commandBandText`); Projects-only mode always mints a fresh session.
- [ ] The Task 2-6 mint-scoped guard does not fire on the no-target path.

**Tests** (`cmd/open_test.go`; override `openTUIFunc` to capture `initialFilter`+`command`):
- `"open -e cmd with no target opens the Projects picker (not a usage error)"` — Execute `open -e claude`; assert `openTUIFunc` captured `initialFilter==""` and `command==["claude"]`, error nil (NOT a `*UsageError`).
- `"open -- cmd with no target opens the Projects picker"` — Execute `open -- claude --resume`; assert captured `command==["claude","--resume"]`, error nil.
- `"open -f text -e cmd opens the filtered Projects picker"` — Execute `open -f api -e claude`; assert captured `initialFilter=="api"` and `command==["claude"]`.
- `"open -f text alone lands on the Sessions page (no command)"` — Execute `open -f api`; assert captured `initialFilter=="api"` and `command` nil (contrast with the filtered-Projects case).
- `internal/tui` (existing coverage) confirms `buildTUIModel(cfg,"",["claude"])` yields `ActivePage()==PageProjects` and `CommandPending()==true`; add/keep an assertion that the rendered command-pending view contains `Pick a project to run` (mirroring `internal/tui/model_test.go`).

**Edge Cases**:
- `open -e <cmd>` / `open -- <cmd>` with no target ⇒ Projects picker with the `Pick a project to run <cmd>` banner (preserved; NOT a usage error).
- `-f <text> -e <cmd>` ⇒ filtered Projects picker (distinct from `-f` alone → Sessions page).
- Banner wording exactly `Pick a project to run` (fixed text) + the command chip.
- Projects-only mode always mints a fresh session (no attach in that mode).

**Context**:
> Spec § Mint-only command with no target → picker in Projects mode: "`open -e <cmd>` / `open -- <cmd>` with no target opens the picker restricted to Projects (mint-only) mode, with a `Pick a project to run <cmd>` banner. This is preserved exactly from today's behavior — not a usage error." "A pending command switches the picker into Projects mode, and Projects only ever mint a fresh session — so the command always lands in a clean session." "`-f <text> -e <cmd>` likewise coheres (filtered Projects picker running the command). The command's only error case is zero mint targets (all-attach explicit set, e.g. `open api web -e cmd`)."
>
> This behaviour is already wired: Phase-1 task 1-5 threads the command through the `-f` and no-arg picker branches, and `internal/tui` puts the model in `PageProjects`/command-pending with the `Pick a project to run` banner (`notice_band.go` `commandBandText`) whenever `command` is non-empty. This task is the guardrail that Task 2-6's usage-error path does not swallow the no-target case, plus exact-wording verification (grep-verified: the constant is `"Pick a project to run"`, with the command rendered separately in an accent chip — do not paraphrase or re-embed `<cmd>` into the constant).

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Multi-Target Burst Mechanics (Mint-only command with no target → picker in Projects mode); § `portal open` — Flags & Command Passthrough (`-f/--filter`).
