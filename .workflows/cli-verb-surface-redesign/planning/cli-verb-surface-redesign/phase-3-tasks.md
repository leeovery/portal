---
phase: 3
phase_name: "Multi-target burst (absorb / net-N)"
total: 8
---

## cli-verb-surface-redesign-3-1 | approved

### Task 3.1: `--ack` receiver flag on `open` (hidden) + best-effort marker write before handoff

**Problem**: The multi-target burst confirms each spawned window via a `@portal-spawn-<batch>-<token>` server-option receipt the window writes just before it execs into tmux. Today only `attach --spawn-ack` (`cmd/attach.go`) writes that receipt. Phase 3 makes every spawned window exec the `open` grammar (`open --session … --ack …` / `open --path … --ack …`, Task 3-5), so `open` itself must gain the receipt-writing behaviour. Without it the parent burst never sees an ack and classifies every window failed.

**Solution**: Add a hidden `--ack <batch>:<token>` string flag to `openCmd`, mirroring the retired-until-Phase-5 `attach --spawn-ack` contract but properly hidden via Cobra `MarkHidden`. Parse and validate the flag value fast (before touching tmux); a malformed value is a usage error (exit 2). When set, the spawned Portal process — as its last act before the connect/mint handoff, and on **both** the `--session` attach receiver and the `--path` mint receiver — writes `@portal-spawn-<batch>-<token>` as a tmux server option, best-effort (still connects/mints on write failure). Leave `attach --spawn-ack` untouched (deleted in Phase 5).

**Outcome**: `portal open --session dev --ack b7:tk9` writes the `@portal-spawn-b7-tk9` server option, then attaches; `portal open --path /existing/dir --ack b7:tk9` writes the same marker, then mints; `open --ack not-a-pair` exits 2 with a usage error before any tmux call; `--ack` is absent from `portal open --help` and completion but visible in `ps`.

**Do**:
- In `cmd/open.go` `init()`, register and hide the flag, mirroring `cmd/attach.go:93` but hidden:
  ```go
  openCmd.Flags().String("ack", "", "internal: <batch>:<token> — write the @portal-spawn-<batch>-<token> ack marker before connecting")
  _ = openCmd.Flags().MarkHidden("ack")
  ```
  (`MarkHidden` removes it from `--help` and completion; it still appears in the process argv / `ps` — acceptable per spec, internal-not-secret.)
- Extend `OpenDeps` (Phase 1) with an `AckWriter spawn.AckWriter` field, and add a production builder mirroring `buildAttachDeps` (`cmd/attach.go:81`): when `openDeps != nil && openDeps.AckWriter != nil` use it, else `spawn.NewServerOptionAckChannel(tmuxClient(cmd), tmuxClient(cmd))` (the shared `*tmux.Client` satisfies both the writer and lister seams — see `internal/spawn/ack.go`).
- In `openCmd.RunE`, parse `--ack` **before** resolution and before touching tmux, exactly as `cmd/attach.go:43-52` does:
  ```go
  ackVal, _ := cmd.Flags().GetString("ack")
  var ackBatch, ackToken string
  ackRequested := ackVal != ""
  if ackRequested {
      b, t, ok := spawn.ParseSpawnAckFlag(ackVal)   // internal/spawn/ackid.go:84
      if !ok {
          return NewUsageError("open: --ack must be <batch>:<token>")
      }
      ackBatch, ackToken = b, t
  }
  ```
  A malformed value (`ParseSpawnAckFlag` returns `ok == false` for a missing colon / empty batch / empty token) → `*UsageError` → exit 2 (per `main.classify`), returned before any tmux read/mutation.
- Perform the marker write as the **last act before the handoff**, on both single-target receivers. Route it through the shared `openResolved` dispatch (Phase 2, Task 2-1) so it covers both a `*SessionResult` (attach) and a `*PathResult` (mint) in one place: after the Task-2-6 mint-scoped command guard, and immediately before `openSessionFunc` / `openPathFunc`, write best-effort when `ackRequested`:
  ```go
  if ackRequested {
      if err := ackWriter.Write(ackBatch, ackToken); err != nil {   // ServerOptionAckChannel.Write → SetServerOption (ack.go:79)
          resolveLogger.Debug("ack marker write failed", "batch", ackBatch, "detail", err.Error())
      }
      // do NOT return on error — fall through to connect/mint (false-negative, no orphan)
  }
  ```
  Thread `ackBatch`/`ackToken`/`ackWriter` into `openResolved` (or a small `ackReceipt` struct) so the write sits strictly between the exists/resolve check and the terminal connect/mint. For the mint receiver the marker must be written **before** `openPathFunc` (which, outside tmux, `syscall.Exec`s into tmux and never returns) — the same pre-exec ordering `cmd/attach.go` uses.
- Do NOT change `cmd/attach.go` — `attach --spawn-ack` stays a fully-working parallel receiver until Phase 5.

**Acceptance Criteria**:
- [ ] `open --session <existing> --ack <b>:<t>` writes `@portal-spawn-<b>-<t>` via the `AckWriter` seam, then attaches (marker write strictly before the connector call).
- [ ] `open --path <existing-dir> --ack <b>:<t>` writes the same marker, then mints (marker write strictly before `openPathFunc`).
- [ ] A malformed `--ack` value (`open --ack foo`, `open --ack :t`, `open --ack b:`) returns a `*UsageError` (exit 2) **before** any tmux read or mutation; the `AckWriter` is never called and no connect/mint occurs.
- [ ] A failing marker write (AckWriter returns an error) still connects/mints (best-effort, false-negative, no orphan) and emits a DEBUG breadcrumb only.
- [ ] `--ack` does not appear in `portal open --help` output or in generated completion (MarkHidden), but is present in the running process argv.
- [ ] `cmd/attach.go` and its `--spawn-ack` flag are unchanged (safety net until Phase 5).

**Tests** (`cmd/open_test.go`; inject `openDeps` with a `SessionLister` fake, an `AckWriter` fake — `spawntest.FakeAckChannel` satisfies `spawn.AckWriter` — a captured `openSessionFunc`/`openPathFunc`, and a `DirValidator`/alias fakes as needed; all unit-lane, no real tmux):
- `"open --session with --ack writes the marker before attaching"` — lister `["dev"]`, fake AckWriter; Execute `open --session dev --ack b7:tk9`; assert the writer recorded `Write("b7","tk9")` and `openSessionFunc` captured `"dev"`, with the write ordered before the connect.
- `"open --path with --ack writes the marker before minting"` — Execute `open --path <tempdir> --ack b7:tk9`; assert `Write("b7","tk9")` recorded and `openPathFunc` captured `<tempdir>`.
- `"open --ack with a malformed value is a usage error before touching tmux"` — Execute `open --session dev --ack nope`; assert `*UsageError`, the AckWriter was never called, and `openSessionFunc` was not called. Parameterise over `nope`, `:t`, `b:`, `""`-via-`--ack=`.
- `"a failing ack write still connects"` — AckWriter returns an error; Execute `open --session dev --ack b:t`; assert `openSessionFunc` still captured `"dev"` and the command returned nil (no error surfaced from the write).
- `"--ack is hidden from help"` — assert `openCmd.Flags().Lookup("ack").Hidden == true`.

**Edge Cases**:
- Malformed `<batch>:<token>` (missing colon, empty half) ⇒ usage error (exit 2) before any tmux call — validated via `spawn.ParseSpawnAckFlag`, identical to `attach`.
- Write failure ⇒ still connects/execs (false-negative; the parent's poll times out and classifies the window failed, leave-what-opened; **no orphan session is created**).
- Rides both receivers: `--session` (attach) and `--path` (mint) — the write sits in the shared `openResolved` dispatch so a single site covers both, always as the last act before the handoff.
- Hidden via `MarkHidden` (gone from `--help`/completion) yet visible in `ps` — internal, not secret.
- `attach --spawn-ack` is left fully intact (Phase 5 deletes it).

**Context**:
> Spec § Hidden `--ack` flag: "`open --ack <batch>:<token>` is an internal receipt flag used by spawned host windows, marked hidden via Cobra `MarkHidden` (gone from `--help` and completion). It remains visible in `ps` … Its behavior: the spawned Portal process, as its last act before exec'ing into tmux, writes `@portal-spawn-<batch>-<token>` as a tmux server option … The write is best-effort: the process still execs into tmux even if the write fails … A failed write therefore produces a false negative — the window is up but the parent's poll sees no receipt within its timeout and classifies it failed (leave-what-opened applies; no orphan is created). … Today's equivalent flag `--spawn-ack` is only labelled 'internal:' in help text, not actually hidden — the redesign hides it properly and renames it `--ack`." Spec § Command Surface Summary (Hidden): `portal open --ack <batch>:<token>` invoked by spawned host windows.
>
> The existing `attach --spawn-ack` receiver (`cmd/attach.go`) is the exact template: parse+validate fast via `spawn.ParseSpawnAckFlag` (usage error on malformed), check the session exists / resolve the target, then write the marker best-effort as the last pre-handoff act (never returning on write failure). `open`'s receiver differs only in that it must serve the mint (`--path`) receiver as well as attach (`--session`), so the write is placed in the shared `openResolved` dispatch. The marker name and value are produced by `spawn.NewServerOptionAckChannel.Write` (`@portal-spawn-<batch>-<token>` = `"1"`). Do not touch `attach`/`--spawn-ack` — it is a Phase-5 deletion.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Flags & Command Passthrough (Hidden `--ack` flag); § Command Surface Summary (Hidden).

---

## cli-verb-surface-redesign-3-2 | approved

### Task 3.2: Raw `os.Args` scan → ordered domain-tagged target-set union (no dedup)

**Problem**: The multi-target burst opens surfaces in **command-line order** — the trigger absorbs the *first* target as typed — and pins interleave freely with positionals (`open -s api ~/Code/new blog`). Cobra splits argv into positional and flag buckets, discarding the interleaved order and (for a single-value `StringP` pin like `-s`) collapsing repeats to the last value. So the true left-to-right target order and repeated pins cannot be recovered from cobra's parsed state; they must be recovered by a raw scan of the process argv, while cobra stays the source of truth for flag validation.

**Solution**: Add a pure, cmd-local function that scans the `open` invocation's raw argv tail and emits an ordered slice of domain-tagged targets — one element per positional and per `-s`/`-p`/`-z`/`-a` occurrence, in the exact order they appear — excluding the `-e`/`-f`/`--ack` flag values and everything after `--`. It classifies each token by the same flag set cobra knows (so the two never disagree) and performs no resolution and no dedup.

**Outcome**: `open -s api ~/Code/new blog` yields three ordered targets `[session:api, bare:~/Code/new, bare:blog]`; `open -s a -s b` yields `[session:a, session:b]` (repeats honoured); `open -e claude ~/new` yields the sole target `[bare:~/new]` (the `-e` value `claude` excluded); `open ~/a -- claude --resume` yields `[bare:~/a]` (everything after `--` excluded); a single-target invocation yields a one-element list.

**Do**:
- Add a cmd-local target type and a pure scanner in a new `cmd/open_targets.go`:
  ```go
  // openTarget is one ordered element of the burst target-set union: a positional
  // (Domain == "") or a domain pin value (Domain ∈ {"session","path","zoxide","alias"}).
  type openTarget struct {
      Domain string // "" bare positional | "session" | "path" | "zoxide" | "alias"
      Value  string
  }

  // orderedOpenTargets recovers the left-to-right target-set union from the open
  // invocation's raw argv tail (production: os.Args trimmed to after the "open"
  // subcommand token). It is a pure state machine over the SAME flag set cobra
  // validates, so the two never disagree; cobra remains the flag-validation source
  // of truth (this runs inside RunE, after a successful parse).
  func orderedOpenTargets(argv []string) []openTarget { … }
  ```
- The state machine classifies each token in `argv` left-to-right:
  - `--` ⇒ stop; every remaining token is command-passthrough, never a target.
  - A value-taking **domain pin** — `-s`/`--session`, `-p`/`--path`, `-z`/`--zoxide`, `-a`/`--alias`:
    - equals form (`-s=api`, `--session=api`) ⇒ emit `{Domain, <inline value>}`.
    - space form (`-s api`) ⇒ consume the **next** token as the value, emit `{Domain, <next>}` (the value token is attributed to the pin, **never** counted as a positional).
  - A value-taking **non-target** flag — `-e`/`--exec`, `-f`/`--filter`, `--ack`: consume its value (inline or next token) and emit **nothing** (excluded from the target list).
  - Any other non-flag token ⇒ a bare positional ⇒ emit `{Domain:"", Value:<token>}`.
  - Preserve order; **no dedup** (append every emission).
- Map short↔long spellings to one domain via a small table (`-s`/`--session` → `"session"`, etc.). Value pins are written separately per the spec (no bundled `-sf`-style value-pin combining), so the scanner need not split combined single-letter clusters for value pins.
- Production wiring: obtain the argv tail by trimming `os.Args` up to and including the first token equal to the `open` command's `Name()` (`"open"`) — the first `"open"` is always the subcommand (a positional literally named `open` can only appear *after* it), so this boundary is robust. Keep `orderedOpenTargets` a pure function over the already-trimmed slice so it is unit-testable without `os.Args`; the trim is the single production-wiring line.
- Do NOT resolve, classify attach-vs-mint, expand globs, or dedup here — this task only recovers order + domain tags. Resolution/classification is Task 3-3.

**Acceptance Criteria**:
- [ ] A pin value in either form — `-s api`, `-s=api`, `--session=api` — is attributed to that pin's domain and never emitted as a bare positional.
- [ ] `-e <cmd>` / `--exec=<cmd>` and its value are excluded from the target list; likewise `-f`/`--filter` and `--ack` values.
- [ ] Every token after `--` is excluded from the target list.
- [ ] Pins repeat freely and interleave with positionals; order is preserved exactly and duplicates are never collapsed (`open -s a blog -s a` → `[session:a, bare:blog, session:a]`).
- [ ] A single positional or single pin yields a one-element ordered list; zero targets yields an empty slice.
- [ ] The scan classifies by the same flag set cobra knows; it performs no resolution, no glob expansion, no mint, and no tmux access.

**Tests** (`cmd/open_targets_test.go`; pure-function unit tests over hand-built argv slices — unit lane, no cobra/tmux):
- `"it recovers interleaved pins and positionals in order"` — `["-s","api","~/Code/new","blog"]` → `[{session,api},{"",~/Code/new},{"",blog}]`.
- `"it attributes -s=api and --session=api values to the pin"` — `["-s=api"]` and `["--session=api"]` each → `[{session,api}]`, never a positional.
- `"it excludes the -e value from targets"` — `["-e","claude","~/new"]` → `[{"",~/new}]`.
- `"it excludes everything after --"` — `["~/a","--","claude","--resume"]` → `[{"",~/a}]`.
- `"it excludes the -f and --ack values"` — `["-f","blog"]` → `[]`; `["--session","dev","--ack","b:t"]` → `[{session,dev}]`.
- `"it honours repeated pins with no dedup"` — `["-s","a","-s","b"]` → `[{session,a},{session,b}]`; `["-a","a","blog","-a","a"]` → `[{alias,a},{"",blog},{alias,a}]`.
- `"a single target yields a one-element list"` — `["blog"]` → `[{"",blog}]`; `["-p","/x"]` → `[{path,/x}]`.
- `"zero targets yields an empty slice"` — `["-f","x"]` and `[]` → `[]`.

**Edge Cases**:
- `-s api` / `-s=api` / `--session=api` — value attributed to the pin, never a positional.
- `-e <cmd>` value and everything after `--` excluded from targets.
- Pins repeat and interleave with positionals; order preserved; no dedup.
- Value pins never bundled (`-sf`-style value-pin combining is not a spec form).
- Cobra stays the flag-validation source of truth — the scan only recovers order (runs inside RunE after a clean parse); unknown flags never reach it.
- Single-target arity ⇒ a one-element list (the downstream N=1 fall-through is Task 3-4).

**Context**:
> Spec § Target-set composition: "The target set is the union of (all positionals + every `-s`/`-p`/`-z`/`-a` occurrence). … Pins repeat freely … Pins mix across domains and with positionals … `-f` is the sole non-composing flag." Spec § Argv parsing contract (target ordering): "Cobra remains the source of truth for flag validation, value binding, `-f` mutual exclusion, and rejecting unknown flags. Target ordering is recovered by a raw `os.Args` scan layered on top … Both value forms are recognized for each pin — `-s api` (space) and `-s=api` / `--session=api` (equals) — and the value token is attributed to that pin, never counted as a positional. `-e <cmd>` and its value are not targets and are excluded … `--` terminates flag/target parsing; every token after `--` is command-passthrough args, never a target. Value-taking pins are written separately, each with its own value — no bundled `-sf`-style combining for value pins. The ordered target list is the sequence of positionals and pin-values in the exact left-to-right order they appear in `os.Args`; the trigger absorbs the first element of that list. The raw scan only recovers order — it classifies each token by the same flag set cobra knows, so the two never disagree." Spec § trigger absorbs the first target: "the implementation reads `os.Args` rather than cobra's split positional/flag buckets to preserve true order."
>
> The single-value `StringP` registration of the pins (Phase 2) collapses repeats to the last value in cobra's storage, so the raw scan — not `cmd.Flags().GetString` — is the only way to recover `-s a -s b` as two targets. The scan is a pure function over the argv tail so it is unit-testable; production trims `os.Args` to after the first `"open"` token (always the subcommand). Cobra still validates: `RunE` only runs after a clean parse, so a token reaching the scanner is always a valid flag or a positional.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Multi-Target Burst Mechanics (Target-set composition; Argv parsing contract — target ordering; The trigger absorbs the first target).

---

## cli-verb-surface-redesign-3-3 | approved

### Task 3.3: Read-only resolve/classify engine → ordered attach/mint surfaces + glob/alias-glob K-expansion + literal-dir reduction

**Problem**: The ordered domain-tagged targets (Task 3-2) must be turned into an ordered list of concrete **surfaces** — each an *attach* (existing session) or a *mint* (literal existing directory) — before the burst can open anything. Globs and alias-key globs expand to K surfaces that join the list in place; mint targets must be reduced to a **literal existing directory** at resolve time so a spawned window never re-resolves (an alias/zoxide query could re-resolve differently mid-burst). This whole step must be **strictly read-only**: no session is minted and no tmux state is mutated, so an unresolvable target can abort the burst with nothing created (Task 3-4).

**Solution**: Add a cmd-layer engine that drives per-target resolution through the Phase-1/2 resolver primitives (extended with K-returning variants), producing an ordered `[]spawn.Surface` plus an ordered miss list. A bare positional runs the Phase-1 chain (glob pre-check → exact session → path → alias → zoxide); pins skip straight to their Phase-2 domain. Session globs (bare or `-s`) expand to K attach surfaces; `-a` key globs expand to K mint surfaces; every mint surface carries the reduced literal existing dir. The engine emits the Phase-1 `resolve` INFO log line for bare guessing-chain targets only (globs/pins stay deterministic → no line).

**Outcome**: `open 'api-*' blog` with sessions `api-1`,`api-2` yields `[attach:api-1, attach:api-2, <blog resolved>]`; `open -a 'workflow-*'` with keys `workflow-a`,`workflow-b` yields `[mint:/abs/dir-a, mint:/abs/dir-b]`; `open -z blog` yields `[mint:/abs/blog]` (reduced literal dir, never the zoxide query); `open 'foo[1]'` (glob metachars, no session match) yields zero surfaces + one miss; nothing is minted and no tmux option is set by this step.

**Do**:
- Define the surface type in `internal/spawn` (consumed by the Burster argv builder in Task 3-5 and the dispatch in Task 3-6), new file `internal/spawn/surface.go`:
  ```go
  type SurfaceKind int
  const ( SurfaceAttach SurfaceKind = iota + 1; SurfaceMint )
  // Surface is one resolved open target: an attach to an existing session, or a
  // mint at a literal existing directory. Value is the session name (attach) or
  // the reduced literal existing dir (mint).
  type Surface struct { Kind SurfaceKind; Value string }
  ```
- Add K-returning resolver variants (new, alongside the Phase-1/2 single-result methods in `internal/resolver/query.go`) that return **misses as data** (a `*MissResult`) rather than hard-erroring, so the multi-target engine can aggregate (Task 3-4):
  - `ResolveBareAll(query string) ([]QueryResult, error)` — the Phase-1 chain, glob-aware: if `HasGlobMeta(query)` expand `MatchSessions(query, ListSessionNames())` to K `*SessionResult{Domain:"glob"}` (zero ⇒ one `*MissResult`); else run the single chain (exact session → path → alias → zoxide) returning a 1-slice (`*SessionResult`/`*PathResult`/`*MissResult`). A mid-chain `*DirNotFoundError` still propagates as an error (hard, distinct from a miss — matching Phase 1).
  - `ResolveSessionPinAll(query string) ([]QueryResult, error)` — exact ⇒ 1 `*SessionResult`; glob ⇒ K `*SessionResult{Domain:"glob"}`; zero ⇒ one `*MissResult`. Refactor the Phase-2 `ResolveSessionPin` to delegate here and convert a `*MissResult` into its `No session found: <q>` hard error, so the two share one match rule.
  - `ResolveAliasPinAll(query string) ([]QueryResult, error)` — exact key ⇒ 1 mint (validated literal dir); key glob ⇒ K mints over `aliases.Keys()`; zero ⇒ one `*MissResult`; an alias whose dir is gone ⇒ `*DirNotFoundError` (hard). Refactor `ResolveAliasPin` to delegate + convert the `*MissResult` into `No alias found: <q>`.
  - `-p` (`ResolvePathPin`) and `-z` (`ResolveZoxidePin`) stay single-result (no glob) — the engine wraps their hard errors into a miss/hard split (see below).
- Ensure every `*PathResult` produced carries the **reduced literal existing dir**: `ResolvePath`/`validatedPath` already return the absolute on-disk directory (Phase 1/2), so a mint surface's `Value` is that literal path — an alias/zoxide/`-p`/bare-path all collapse to the same literal-dir form, and the value handed to a spawned window is `--path <literal-dir>`, which cannot re-resolve.
- Add the cmd-layer engine in `cmd/open.go` (or `cmd/open_targets.go`):
  ```go
  // resolveOpenSurfaces classifies the ordered target-set union into ordered
  // attach/mint surfaces, read-only. It emits the Phase-1 `resolve` INFO line for
  // each BARE guessing-chain target (hit or miss); globs and pins are deterministic
  // and emit none. Returns surfaces in target order (K-expanded joins in place) plus
  // the ordered miss list; a hard resolver error (DirNotFound, zoxide-not-installed)
  // returns immediately (it is not a soft miss).
  func resolveOpenSurfaces(qr *resolver.QueryResolver, targets []openTarget) (surfaces []spawn.Surface, misses []openMiss, err error) { … }
  ```
  Per ordered target dispatch on `Domain`:
  - `""` (bare) ⇒ `ResolveBareAll`; emit the `resolve` line (Task 1-4 gating: only when `!HasGlobMeta(target.Value)`), then map each `QueryResult` → `Surface` / miss.
  - `"session"` ⇒ `ResolveSessionPinAll` (no resolve line).
  - `"path"` ⇒ `ResolvePathPin` (single; a non-directory value is a **hard** usage-style error → return it; a non-existent `-p` dir is surfaced as a miss entry — see the ambiguity note).
  - `"zoxide"` ⇒ `ResolveZoxidePin` (single; `ErrZoxideNotInstalled` is a hard error → return immediately; a no-match is a miss entry).
  - `"alias"` ⇒ `ResolveAliasPinAll`.
  - Map `*SessionResult` → `Surface{SurfaceAttach, r.Name}`; `*PathResult` → `Surface{SurfaceMint, r.Path}`; `*MissResult` → an `openMiss{Domain, Value}` entry (carrying the raw target + its domain so Task 3-4 can format the right message).
- Keep this step strictly read-only: it calls only the resolver's read methods and `ListSessionNames`; it never calls `openPath`/`openSession`/`CreateFromDir`/`QuickStart` or any tmux set-option. The only side effect is the `resolve` INFO log line (a decision record, not a mutation), emitted from `cmd/open.go` (the Phase-1 `resolveLogger`), keeping `internal/resolver` log-free.

**Acceptance Criteria**:
- [ ] A bare non-glob target runs the Phase-1 chain (exact session → path → alias → zoxide) producing one attach or mint surface, or a miss; a pin skips straight to its Phase-2 domain.
- [ ] A session glob (bare or `-s`) expands to K attach surfaces joining the list in place; zero matches produces a miss (no surface).
- [ ] An `-a` key glob expands to K mint surfaces over the enumerated alias-key namespace; each carries its reduced literal dir.
- [ ] Overlapping globs may yield duplicate surfaces (`open 'api-*' 'api-1'`) — honoured, never deduped.
- [ ] Every mint surface's `Value` is a literal existing directory (alias/zoxide/`-p`/bare-path all reduced), never an alias key or zoxide query.
- [ ] A directory path whose name contains glob metacharacters is unreachable as a bare positional (captured by the glob pre-check → zero session matches → miss) and reachable only via `-p` (which mints at the literal dir).
- [ ] The engine performs no mint and no tmux mutation (strictly read-only); a bare guessing-chain target emits one `resolve` INFO line (hit or miss), globs/pins emit none; `internal/resolver` imports no logging.

**Tests** (`internal/resolver/query_test.go` for the `…All` variants; `cmd/open_test.go` for the engine + resolve-log gating — unit lane, resolver + lister + alias fakes, `log.SetTestHandler` for the log assertions):
- `"ResolveBareAll expands a session glob to K SessionResults"` — lister `["api-1","api-2","web-3"]`, `ResolveBareAll("api-*")` → 2 `*SessionResult{Domain:"glob"}` in order.
- `"ResolveBareAll returns a single chain result for a non-glob"` — alias `{"api":<dir>}`, `ResolveBareAll("api")` → 1 `*PathResult{Domain:"alias",Path:<dir>}`.
- `"ResolveBareAll returns a MissResult for zero glob matches"` — lister `["web"]`, `ResolveBareAll("api-*")` → 1 `*MissResult{Target:"api-*"}`.
- `"ResolveAliasPinAll expands a key glob to K mints with literal dirs"` — keys `{"workflow-a":<d1>,"workflow-b":<d2>,"other":<d3>}` (validator existing), `ResolveAliasPinAll("workflow-*")` → 2 `*PathResult` at `<d1>`,`<d2>`.
- `"a mint surface carries the reduced literal dir"` — engine over `[{zoxide,blog}]` with zoxide→`<abs>` → `[]spawn.Surface{{SurfaceMint,<abs>}}`.
- `"resolveOpenSurfaces preserves order and never dedupes"` — targets `[{"",api-*},{"",api-1}]` (glob overlaps the literal) → surfaces include `api-1` twice.
- `"the engine emits a resolve line for a bare guessing-chain hit but not for a glob or pin"` — over `[{"",blog}(→zoxide), {"",api-*}, {session,dev}]`, assert exactly one `resolve` record (`target=blog domain=zoxide`), none for the glob or the pin.
- `"a bare glob path is unreachable and yields a miss"` — `resolveOpenSurfaces([{"",~/tmp/foo[1]}])` with no matching session → one miss, zero surfaces; `resolveOpenSurfaces([{path,~/tmp/foo[1]}])` with that dir existing → one mint surface.
- `"the engine is read-only"` — inject a mint seam that fails the test if called; run the engine over a mixed set; assert it is never invoked.

**Edge Cases**:
- Bare runs the Phase-1 chain; pins skip to their Phase-2 domain.
- Session glob (bare or `-s`) expands to K user-visible attach surfaces joining in place; zero-match = miss.
- `-a` key glob expands to K alias mints (over the finite Portal-owned key namespace).
- Overlapping globs may duplicate surfaces — honoured, never deduped.
- Mint targets reduced to a literal existing dir so the spawned window never re-resolves.
- Glob-metacharacter dir path unreachable as bare (glob pre-check → zero → miss), reachable only via `-p`.
- Strictly read-only — no mint, no tmux mutation; the only side effect is the `resolve` decision line for bare guessing-chain targets.

**Context**:
> Spec § Glob targets: "A bare target containing glob metacharacters … is session-domain by construction … Expansion produces K targets that join the target list (`open 'agentic-workflows-*' blog` → K+1 surfaces …). Zero matches ⇒ unresolvable ⇒ atomic hard fail. … `-a` accepts key globs … A directory path whose name contains glob metacharacters … is unreachable as a bare positional … Reach it with `-p <dir>`." Spec § The trigger absorbs the first target … no dedup: "overlapping globs (`open 'api-*' 'api-1'`) can produce a duplicate surface; honored, not deduped." Spec § Burst exec-argv & mint responsibility: "Mint target … the parent reduces it to a literal existing directory at resolve time, then bakes `portal open --path <literal-dir> --ack …`. Alias/zoxide queries never travel to the window (they could re-resolve differently mid-burst); only the resolved literal dir does, and `--path` cannot diverge." Spec § Atomic pre-flight: "Pre-flight is a read-only resolve of the whole target set. … any target unresolvable ⇒ nothing opens, nothing created." Spec § Wrong-guess feedback: "One line per resolved guessing-chain target — a multi-target burst emits one per such target."
>
> The K-returning variants extend the Phase-1/2 single-result methods (which return the *first* match / hard-fail on miss); the multi-target engine needs the *full* expansion and misses-as-data for aggregation, so the Phase-2 single methods are refactored to delegate to the `…All` variants + convert a `*MissResult` into their existing hard-fail string (single-source the match rule). Literal-dir reduction is free: `ResolvePath`/`validatedPath` already yield the absolute on-disk dir, so every mint `Value` is a literal path.
>
> Ambiguity note (flag for review): the spec does not fix how a `-p`/`-z` **hard** error (a non-existent `-p` dir, `ErrZoxideNotInstalled`) participates in the *aggregated* multi-target abort — Phase 2 makes them immediate hard errors at single-target arity. This task treats `ErrZoxideNotInstalled` and a non-directory `-p` value as immediate hard errors (they are environment/usage faults, not "target not found"), while a `-p` non-existent dir and a `-z` no-match are collected as **miss entries** so a mixed set reports them alongside other misses (Task 3-4). Confirm whether all pin faults should instead abort on the first hard error.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Multi-Target Burst Mechanics (Glob targets; Burst exec-argv & mint responsibility; Atomic pre-flight); § Wrong-guess feedback — tmux is the receipt.

---

## cli-verb-surface-redesign-3-4 | approved

### Task 3.4: Atomic aggregated pre-flight abort (report every miss; single-target `-f` carve-out)

**Problem**: The burst must be **atomic**: if any target in the whole union is unresolvable, nothing opens and nothing is minted. And the abort must report **every** unresolvable target (not just the first) so one re-run fixes them all. The single-target `-f` escape-hatch suggestion (Phase 1) must appear **only** at single-target arity — `-f` is mutually exclusive with targets, so it cannot carry a multi-target intent — while a multi-target miss lists every miss without `-f`. Finally, an all-hit set of size 1 must fall through to the ordinary single-target connect, not spin up a burst.

**Solution**: In `openCmd.RunE`, gate the burst path on the scanned target list (Task 3-2), run the read-only classify engine (Task 3-3) over the union, and branch on the miss/surface counts: any miss ⇒ atomic abort with a message naming every miss (single-target keeps the Phase-1/2 message incl. `-f`; multi-target aggregates without `-f`); zero misses and exactly one surface ⇒ the ordinary single-target connect; zero misses and ≥2 surfaces ⇒ the net-N burst (Task 3-6).

**Outcome**: `open blog nope api-9` (two misses) hard-fails listing both misses so one re-run fixes both, and nothing opens; `open api ~/gone` (one miss in a two-target set) aborts atomically (the `api` attach never happens); `open blog` (single miss) keeps `nothing resolved for 'blog' — try -f blog`; `open api-only-one-match` (single all-hit) connects via the single-target path, not the burst; `open 'api-*'` matching zero counts as a miss.

**Do**:
- In `openCmd.RunE`, after the Phase-1/2 `-f` handling and command parsing, compute the scanned targets `targets := orderedOpenTargets(<argv tail>)` (Task 3-2). Decide the routing:
  - Engage the burst/engine path when `len(targets) >= 2` **or** (`len(targets) == 1` and `resolver.HasGlobMeta(targets[0].Value)`) — the single-glob case must expand (Phase 3 overrides Phase 1's single-target first-match). Otherwise fall through to the existing Phase-1/2 single-target branches (pins + bare resolve) untouched, preserving their behaviour and tests.
- On the engine path, run `surfaces, misses, err := resolveOpenSurfaces(qr, targets)` (Task 3-3). Handle `err != nil` (a hard resolver error, e.g. `ErrZoxideNotInstalled`) by returning it immediately (exit 1) — a hard error is not the soft-miss aggregation.
- Aggregate misses atomically:
  ```go
  if len(misses) > 0 {
      return missAbortError(targets, misses)   // nothing opened, nothing minted
  }
  ```
  `missAbortError` formats by arity:
  - **Single-target** (`len(targets) == 1`): reproduce the Phase-1/2 message byte-for-byte from the miss's domain — bare ⇒ `nothing resolved for '<v>' — try -f <v>`; `-s` ⇒ `No session found: <v>`; `-a` ⇒ `No alias found: <v>`; `-z` no-match ⇒ `No zoxide match for: <v>`; `-p` non-existent ⇒ `Directory not found: <v>`. (This is exactly the single-target case; it keeps the `-f` suggestion only for the bare miss.)
  - **Multi-target** (`len(targets) >= 2`): a single aggregated error naming **every** miss and **omitting** `-f`, e.g. `nothing resolved for: 'blog', 'nope', 'api-9'` (using `spawn.QuoteJoin` for the quoted list so the wording is single-sourced with the burst's other one-line messages). A plain error (exit 1), not a `*UsageError`.
- Branch on surface count when there are no misses:
  - `len(surfaces) == 1` ⇒ single-target connect: route the sole surface through `openResolved` (attach → connector; mint → `openPathFunc`, threading the command per Task 3-7) — the N=1 all-hit fall-through, **not** the burst.
  - `len(surfaces) >= 2` ⇒ the net-N burst (Task 3-6), passing the ordered surfaces + command.
- Keep the abort strictly before any window/adapter/detector work: the read-only resolve is the atomic guarantee (nothing minted, no marker written, no adapter touched) exactly per the spec.

**Acceptance Criteria**:
- [ ] Multiple misses in one invocation are all reported in a single aggregated error (one re-run can fix them all); nothing opens or mints.
- [ ] Any single miss in a mixed (partly-resolvable) set aborts the whole set atomically — no attach, no mint, no marker, no adapter call.
- [ ] A single-target miss keeps the exact Phase-1/2 message for its domain (bare keeps the `-f` suggestion; pins keep their `No … found` / `Directory not found` strings).
- [ ] A multi-target miss omits the `-f` suggestion and lists every miss.
- [ ] A zero-match glob (bare or `-s`/`-a`) counts as a miss and participates in the abort.
- [ ] An all-hit set that resolves to exactly one surface falls through to the ordinary single-target connect, never the burst; an all-hit set of ≥2 surfaces dispatches the burst.

**Tests** (`cmd/open_test.go`; inject `openDeps` (lister/alias/zoxide/validator fakes) + captured `openPathFunc`/`openSessionFunc`/a fake burst seam that fails the test if a window opens; unit lane):
- `"a multi-target set with two misses reports both and opens nothing"` — targets `blog nope` both unresolvable; assert the error names both and the burst/mint seams were never called.
- `"one miss in a mixed set aborts atomically"` — `open api ~/gone` (api resolves to a session, `~/gone` misses); assert error, and neither the `api` attach nor any mint/adapter ran.
- `"a single-target bare miss keeps the -f suggestion"` — `open blog` (miss) → `err.Error()=="nothing resolved for 'blog' — try -f blog"`.
- `"a single-target -s miss keeps the No session found message"` — `open -s nope` (miss) → `"No session found: nope"`, no `-f`.
- `"a multi-target miss omits -f"` — `open blog nope` → error contains both quoted names and does NOT contain `-f`.
- `"a zero-match glob counts as a miss"` — `open 'api-*' blog` with no `api-*` sessions but `blog` resolvable → atomic abort naming `api-*`.
- `"N=1 all-hit connects via the single-target path, not the burst"` — `open api` (resolves to one mint) → `openPathFunc` called once, burst seam never invoked.
- `"N≥2 all-hit dispatches the burst"` — `open api blog` (both resolve) → the burst seam is invoked with 2 ordered surfaces, `openPathFunc`/`openSessionFunc` not called directly.

**Edge Cases**:
- Multiple misses all reported (one re-run fixes all).
- Any single miss in a mixed set aborts atomically (nothing opens/mints).
- Single-target miss keeps the `-f` suggestion (Phase 1); multi-target miss omits `-f`.
- Zero-match glob counts as a miss.
- N=1 all-hit falls through to the single-target connect, not the burst (a single glob matching exactly one session also lands here).
- A hard resolver error (`ErrZoxideNotInstalled`, non-directory `-p`) returns immediately (exit 1), distinct from the soft-miss aggregation.

**Context**:
> Spec § Atomic pre-flight & partial failure: "Pre-flight is a read-only resolve of the whole target set. Any target unresolvable ⇒ atomic abort: nothing opens, nothing is created. The abort reports every unresolvable target (not just the first), so one re-run can fix them all. The `-f <text>` suggestion in the miss message appears only in the single-target case — `-f` is mutually exclusive with targets, so it cannot carry a multi-target intent." Spec § Glob targets: "Zero matches ⇒ unresolvable ⇒ atomic hard fail — no special case." Spec § Burst exec-argv & mint responsibility: "The atomic guarantee is precisely the read-only resolve: any target unresolvable ⇒ nothing opens, nothing created."
>
> The single-glob-multi-match override of Phase 1 (Task 1-3 attached the first match at single-target arity as a forward-compatible placeholder; the spec's real behaviour is K-expansion) is realised here: the routing gate expands a single glob so `open 'api-*'` with ≥2 matches becomes a burst, while a glob matching exactly one still lands on the single-target connect. Non-glob single targets keep the untouched Phase-1/2 path, so their resolve-log + miss-message + pin-dispatch tests stay green.
>
> Ambiguity note (flag for review): the spec fixes no exact wording for the *aggregated* multi-target miss line. This task uses `nothing resolved for: '<a>', '<b>', …` via `spawn.QuoteJoin`, echoing the single-target `nothing resolved for '<x>'` stem without the (arity-inappropriate) `-f` suffix. Adjust if a per-domain aggregated breakdown is preferred.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Multi-Target Burst Mechanics (Atomic pre-flight & partial failure; Glob targets; Burst exec-argv & mint responsibility).

---

## cli-verb-surface-redesign-3-5 | approved

### Task 3.5: Spawned-window `open`-grammar argv composition (attach `--session` / mint `--path <literal-dir>`) + Burster surface-spec input

**Problem**: Each spawned host window must run the **same `open` grammar a human would** — a single pinned target plus the hidden `--ack` — with no bespoke burst-only path. Today `internal/spawn/command.go`'s `composeAttachArgv` bakes only the retired-verb `attach <session> --spawn-ack …` shape and the `Burster.Run` input is a flat `[]string` of session names, so it cannot express a mint window (`--path <literal-dir>`) or a mixed attach/mint set. The composition must be generalized to the `open` grammar over the Task-3-3 surfaces, while keeping the still-present `spawn` CLI and the picker's in-process burst green.

**Solution**: Generalize the single argv builder to emit the `open` grammar from a `spawn.Surface`: an attach surface → `env -u TMUX -u TMUX_PANE PATH=… <exe> open --session <name> --ack <batch>:<token>`; a mint surface → `… open --path <literal-dir> --ack <batch>:<token>`. Change the `Burster.Run` input from `[]string` to `[]Surface`. Carry the legacy all-attach callers (spawn CLI `runSpawn`, picker `dispatchBurst`) forward by wrapping their session-name lists into all-attach `[]Surface`, so both converge onto `open --session --ack` (behaviourally identical to the old `attach --spawn-ack` since Phase 1/2 made `open --session` == `attach`).

**Outcome**: An attach surface composes `["/usr/bin/env","-u","TMUX","-u","TMUX_PANE","PATH=<path>","<exe>","open","--session","<name>","--ack","<batch>:<token>"]`; a mint surface composes the same head with `"open","--path","<literal-dir>","--ack","<batch>:<token>"`; a name/dir with spaces stays one argv element; `--ack <value>` is two discrete elements; `TMUX`/`TMUX_PANE` are stripped; the exe is `os.Executable()` so the warm-command latch stays satisfied; the spawn CLI and picker burst still open windows (now via `open --session --ack`) with their parity tests updated.

**Do**:
- Generalize the builder in `internal/spawn/command.go` (adapt `composeAttachArgv`, keeping the env/PATH/TMUX-strip head verbatim; rename to `composeOpenArgv` and switch the tail to the `open` grammar per surface):
  ```go
  func composeOpenArgv(exePath, path string, s Surface, batch, token string) []string {
      head := []string{"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE", "PATH=" + path, exePath, "open"}
      switch s.Kind {
      case SurfaceMint:
          head = append(head, "--path", s.Value)     // literal existing dir (Task 3-3 reduction)
      default: // SurfaceAttach
          head = append(head, "--session", s.Value)  // session name
      }
      return append(head, "--ack", FormatSpawnAckFlag(batch, token))  // TWO discrete elements
  }
  ```
  Preserve every load-bearing fragment from the current `composeAttachArgv` doc: `/usr/bin/env` prefix, `-u TMUX -u TMUX_PANE` strip (the spawned N−1 MUST run out of tmux), `PATH=<path>` as the sole injected var, `exePath` = the picker's own absolute binary (NOT a bare `portal` PATH lookup) so the version-gated warm-command latch (`state.BootstrappedLatchSatisfied`) stays satisfied and each spawned `open` takes the abridged fast-path, and the session/dir as a discrete element (a space never needs quoting). The command-passthrough tail (mint windows only) is Task 3-7 — leave a clear seam for it (e.g. accept the command as a later parameter or append at the call site).
- Change `Burster.Run` (`internal/spawn/burst.go:133`) to take `external []Surface` instead of `external []string`, and call `composeOpenArgv(exePath, path, external[i], batch, token)`. Keep the token/batch generation, the per-window await, the permission early-stop, and the progress callback unchanged. `WindowResult.Session` should carry a human-readable label for the surface (the session name for attach, the literal dir for mint) so the existing `PartitionResults`/log messages still name the window.
- Carry the legacy all-attach callers forward (no forked builder):
  - `cmd/spawn.go` `runSpawn`: wrap its `external []string` (session names) into `[]spawn.Surface{{SurfaceAttach, name}, …}` before `NewBurster(...).Run(...)`. Its spawned windows now exec `open --session --ack` instead of `attach --spawn-ack`.
  - `internal/tui` `burstRunner.run` / `dispatchBurst`: wrap its `external []string` into all-attach `[]spawn.Surface` at the `burster.Run` call site.
- Update the parity/argv tests in lockstep: `internal/spawn/command_test.go`, `internal/spawn/burst_test.go`, and the picker's `internal/tui/burst_dispatch_test.go` `spawnedSession` helper (reads the element after `"attach"`) → read the element after `"--session"`. The spawn↔picker byte-identical argv parity assertions stay, now over the `open --session --ack` shape.
- Leave `attach --spawn-ack` (`cmd/attach.go`) intact — it is no longer the burst exec target but remains a working command until Phase 5.

**Acceptance Criteria**:
- [ ] An attach surface composes `env -u TMUX -u TMUX_PANE PATH=<path> <exe> open --session <name> --ack <batch>:<token>` (exact element order).
- [ ] A mint surface composes `… <exe> open --path <literal-dir> --ack <batch>:<token>` — the reduced literal dir, never an alias key or zoxide query.
- [ ] A session name or directory containing a space is a single argv element; `--ack` and its `<batch>:<token>` value are two discrete elements (never `--ack=…`, never quoted).
- [ ] `TMUX` and `TMUX_PANE` are stripped and `PATH` is the only injected var; the exe is the resolved `os.Executable()` absolute path so the warm-command latch stays satisfied.
- [ ] `Burster.Run` accepts `[]Surface`; the spawn CLI and picker burst still open windows (all-attach surfaces → `open --session --ack`) with byte-identical argv parity between the two callers, verified by the updated parity tests.
- [ ] `cmd/attach.go` / `--spawn-ack` are unchanged.

**Tests** (`internal/spawn/command_test.go` + `burst_test.go`, unit lane via `spawntest` fakes — no real terminal/tmux/binary):
- `"composeOpenArgv builds the open --session --ack shape for an attach surface"` — `Surface{SurfaceAttach,"api x"}` → asserts the full slice incl. the space-containing name as one element.
- `"composeOpenArgv builds the open --path --ack shape for a mint surface"` — `Surface{SurfaceMint,"/Code/new dir"}` → `… open --path "/Code/new dir" --ack b:t`.
- `"composeOpenArgv strips TMUX/TMUX_PANE and injects only PATH"` — assert the `-u TMUX -u TMUX_PANE` pair and a single `PATH=` element.
- `"--ack is two discrete argv elements"` — assert `argv[len-2]=="--ack"` and `argv[len-1]=="b:t"`.
- `"Burster.Run composes an open-grammar argv per external surface"` — a mixed `[]Surface{attach,mint}` through the fake adapter → `adapter.Calls[0]` has `--session`, `[1]` has `--path`.
- `"the spawn CLI + picker burst still open all-attach windows via open --session --ack"` (updated parity) — the CLI `runSpawn` and the picker `dispatchBurst` over the same session list produce byte-identical `open --session --ack` argv.
- Integration (`//go:build integration`, only if exercising the real exec path): a real spawned `open --session … --ack …` window brings up the session and writes its marker — but the composition itself is fully covered in the unit lane; add integration only where a built binary is actually exec'd (via `portalbintest` + `portaltest.IsolateStateForTest`).

**Edge Cases**:
- Attach → `open --session <name> --ack …`; mint → `open --path <literal-dir> --ack …` (never alias/zoxide — the literal dir is the Task-3-3 reduction).
- Name/dir with spaces = one argv element; `--ack` value = two discrete elements.
- `TMUX`/`TMUX_PANE` stripped; `os.Executable()` keeps the warm-latch satisfied (version-identical binary → abridged attach, not a full bootstrap).
- Minting happens at window exec time, not the parent (the parent only composes `--path <dir>`; no pre-mint → no orphan) — the composition never mints.
- Legacy `spawn` CLI + picker burst stay green: all-attach windows converge onto `open --session --ack` (one builder, no fork).

**Context**:
> Spec § Burst exec-argv & mint responsibility: "Each spawned window runs the same `open` grammar a human would — one pinned target + the hidden `--ack` — no bespoke burst-only path. Window argv, per surface: Attach target … → `portal open --session <name> --ack <batch>:<token>`. Mint target … → the parent reduces it to a literal existing directory at resolve time, then bakes `portal open --path <literal-dir> --ack <batch>:<token>`. … Minting happens in each window, not the parent — no pre-minting." Spec § attach — Retired: "the exec target of every spawned host window → `portal open --session <name> --ack <batch>:<token>`." Spec § Spawned-window contract (pinned `open`): "Spawned host windows exec `portal open --session <name> --ack <batch>:<token>`."
>
> The current `composeAttachArgv` (`internal/spawn/command.go`) already encodes the load-bearing env-self-sufficiency: `/usr/bin/env -u TMUX -u TMUX_PANE PATH=<path> <exe> …` with `os.Executable()` keeping the warm-command latch satisfied (its own doc comment cites this). Generalizing it to the `open` grammar and switching `Burster.Run` to `[]Surface` is the minimal change; the legacy all-attach callers are carried forward by wrapping their name lists into all-attach surfaces rather than forking a second builder (the orchestrator's explicit constraint), so the still-present spawn CLI and picker burst keep working and their parity tests are updated in lockstep. `FormatSpawnAckFlag` (`internal/spawn/ackid.go`) already renders the colon-joined value.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Multi-Target Burst Mechanics (Burst exec-argv & mint responsibility); § `attach` — Retired (Spawned-window contract).

---

## cli-verb-surface-redesign-3-6 | approved

### Task 3.6: Net-N dispatch — trigger absorbs first, spawns N−1 external, connects last

**Problem**: `open <t1> … <tN>` must open N surfaces continuously in N: the invoking terminal becomes one surface and N−1 host windows spawn — net N, never N+1. The trigger absorbs the **first** target in command-line order (distinct from the legacy `spawn` CLI's *trailing*-trigger `SplitNetN`), and — load-bearing outside tmux — connects **last**, after all N−1 spawns are issued (an early `exec attach` would replace the Portal process and destroy the burster). The current session is never special-cased; duplicates are honoured.

**Solution**: Add a `runOpenBurst` orchestrator in `cmd/open.go` (analogous to `cmd/spawn.go`'s `runSpawn`) that takes the ordered surfaces (Task 3-4), splits **first-trigger** via a new `spawn` split distinct from the trailing-trigger `SplitNetN`, detects+resolves the host terminal (reusing `buildProductionSpawnSeams`), spawns the N−1 non-trigger external surfaces via the Task-3-5 Burster, then self-connects the trigger **last** (attach → `buildSessionConnector`; mint → local mint, Task 3-7). N=1 degenerates to a plain single connect (already routed by Task 3-4).

**Outcome**: `open api blog web` (all resolvable) attaches this terminal to the first surface `api` and opens `blog`,`web` as two host windows (net 3, never 4); the trigger's `exec attach`/`switch-client` runs after both spawns are issued; `open api api api` opens two host windows plus the self-connect (three mirrored `api` clients); a current session absent from the set is left detached with no window.

**Do**:
- Add the first-element split to `internal/spawn` (new, distinct from `split.go`'s trailing-trigger `SplitNetN` — leave that untouched for the legacy callers):
  ```go
  // SplitTriggerFirst splits an ordered surface list into the trigger (the FIRST
  // surface — the invoking terminal absorbs it) and the N−1 external surfaces the
  // burst opens. Precondition len(ordered) >= 1. Distinct from SplitNetN, which puts
  // the trigger LAST (the spawn CLI / picker convention).
  func SplitTriggerFirst(ordered []Surface) (trigger Surface, external []Surface) {
      return ordered[0], ordered[1:]
  }
  ```
- Add `runOpenBurst(cmd *cobra.Command, surfaces []spawn.Surface, command []string, deps *OpenBurstDeps) error` in `cmd/open.go`, modelled on `runSpawn` (`cmd/spawn.go:116`) but first-trigger and attach/mint-aware:
  1. `trigger, external := spawn.SplitTriggerFirst(surfaces)`.
  2. Detect the host terminal + resolve the adapter (reuse the shared `buildProductionSpawnSeams(client)` bundle: `Detector.Detect()` → `Resolve(id)`), exactly as `runSpawn` does. On `ResolutionUnsupported`, the burst cannot open the N−1 windows → **atomic no-op** via `spawn.UnsupportedNoopMessage(id)` (nothing opens, **no self-connect** — the trigger does NOT open-here), mirroring `runSpawn`'s unsupported gate. **Resolved decision (2026-07-18):** block the multi-target burst outright on an unsupported/remote terminal — nothing opens at all, trigger does not half-connect. See the decision note below.
  3. Spawn the N−1 external surfaces FIRST: `batch, results, err := deps.NewBurster(adapter).Run(context.Background(), external, nil)` (Task 3-5 Burster; `nil` progress = CLI parity with `runSpawn`).
  4. Clean the batch markers on every post-burst path and BEFORE the self-connect handoff (`_ = deps.Ack.Clean(batch)`), exactly as `runSpawn` does (a point of no return outside tmux) — Task 3-8 owns the leave-what-opened/partial-failure branch that decides whether to self-connect.
  5. Self-connect the trigger **LAST**: attach surface → `deps.Connector.Connect(trigger.Value)` (inside tmux `switch-client`, outside `exec attach` — `buildSessionConnector`); mint surface → local mint (Task 3-7 wires `openPath`). The connect is issued only after all spawns, so `exec attach` replacing the process never destroys an un-issued spawn.
- Do NOT special-case the current session: it gets a window only if it appears in the target set (as any other surface would); the trigger's landing spot is immaterial ("it doesn't matter where the terminal ends up, as long as they all open"). Honour duplicates (no dedup — the ordered surface list is taken literally).
- The inside/outside-tmux split only selects the trigger's connector (`switch-client` vs `exec attach`); the N−1 external windows always run the spawned `open …` argv (which is out-of-tmux by construction, Task 3-5).
- Introduce `OpenBurstDeps` (the burst seams for `open`, mirroring `SpawnDeps`: `Detector`, `Resolve`, `Connector`, `ExePath`, `Getenv`, `Exists`/not needed since resolve is read-only, `Ack`, `NewBurster`, `Logger`, plus a local-mint seam for Task 3-7), defaulting from `buildProductionSpawnSeams` so a test injects fakes and drives the whole burst without a real terminal.
- N=1 is not reached here — Task 3-4 routes a single surface to the plain single-target connect; `runOpenBurst` is only entered with `len(surfaces) >= 2`.

**Acceptance Criteria**:
- [ ] The trigger is the FIRST surface in command-line order; the remaining N−1 are the external set opened before the trigger connects.
- [ ] The N−1 external windows are spawned FIRST and the trigger self-connects LAST (after all spawns are issued) — verified by ordering, load-bearing for the outside-tmux `exec attach`.
- [ ] The first-element split is a distinct function from the legacy trailing-trigger `SplitNetN` (which stays wired for the spawn CLI / picker).
- [ ] The current session is never special-cased: it gets a window only if it appears in the set; if it is the first surface the self-connect is a no-op switch; if elsewhere it moves + gets its own window; if absent it is left detached with no window.
- [ ] Duplicate surfaces are honoured, never deduped (`open api api api` → two external windows + the self-connect).
- [ ] The inside/outside-tmux split only selects the trigger's connector; N=1 does not reach the burst (plain single connect via Task 3-4).

**Tests** (`cmd/open_test.go`; inject `OpenBurstDeps` with a `spawntest.FakeAdapter`/`FakeAckChannel`, a fake `Connector` capturing the self-connect target, a fake `Detector`/`Resolve` returning a supported native adapter — unit lane, the same injection pattern as `cmd/spawn_test.go`):
- `"the trigger is the first surface and the external set is the rest, in order"` — surfaces `[api, blog, web]` → external `[blog, web]`, trigger `api`; assert the fake adapter opened `blog` then `web`, never `api`.
- `"the trigger connects last, after all spawns are issued"` — record call order; assert both `OpenWindow` calls precede the `Connector.Connect("api")`.
- `"SplitTriggerFirst is distinct from SplitNetN"` — unit test in `internal/spawn/split_test.go`: `SplitTriggerFirst([a,b,c])` → `(a,[b,c])` vs `SplitNetN([a,b,c])` → `([a,b],c)`.
- `"duplicates are honoured"` — surfaces `[api, api, api]` → 2 external `api` windows + a self-connect to `api`, no dedup.
- `"the current session is not special-cased"` — with a fake inside-tmux current session not in the set, assert it is neither opened as a window nor connected; the trigger self-connects to the first surface via `switch-client`.
- `"an unsupported terminal is an atomic no-op"` — Resolve → `ResolutionUnsupported`; assert no window opened and no self-connect, error names the identity via `spawn.UnsupportedNoopMessage`.
- Integration (`//go:build integration`) for the real exec/handoff path only, via `portalbintest` + `portaltest.IsolateStateForTest` — the ordering/split logic is fully unit-covered above.

**Edge Cases**:
- Trigger = first in command-line order; N−1 spawned first; trigger self-connects LAST (load-bearing outside tmux).
- First-element split distinct from legacy trailing-trigger `SplitNetN`.
- Current session never special-cased: window only if in set; first → no-op switch; elsewhere → moves + its own window; absent → left detached.
- Duplicates honoured, never deduped.
- Inside/outside split only selects the trigger's connector.
- N=1 degenerates to a plain single connect (routed by Task 3-4, not this orchestrator).
- Unsupported/remote terminal at N≥2 ⇒ **atomic no-op, nothing opens, trigger does not half-connect** (resolved 2026-07-18; parity with `runSpawn`).

**Context**:
> Spec § The trigger absorbs the first target, unconditionally; no dedup: "The trigger (invoking terminal) takes the first target in command-line order … and every remaining target opens a window. If the current session happens to be the first target → a no-op switch … If the current session is elsewhere in the set … the terminal moves to the first target, and the current session gets its own window because it is a target … If the current session is absent from the set … it is simply left as a detached session with no surface. It is not given a window. … No current-session detection, no special-casing … Execution order — the trigger connects last. … The N−1 non-trigger surfaces are spawned first; the trigger self-connects (`switch-client` inside / `exec attach` outside) last, after all spawns are issued. This ordering is load-bearing outside tmux: `exec attach` replaces the Portal process, so connecting the trigger before the spawns would destroy the burster and open only one surface." Spec § no dedup: "`open api api api` = three host windows all showing `api`."
>
> `runSpawn` (`cmd/spawn.go`) is the structural template (pre-flight → detect → resolve → spawn N−1 → connect trigger last, cleaning markers before the self-connect), but its split is trailing-trigger (`SplitNetN`) and all-attach. `open`'s burst is first-trigger and attach/mint-aware, so it needs the distinct `SplitTriggerFirst` and the local-mint trigger branch (Task 3-7). The detection/resolution/unsupported gate reuses the shared `buildProductionSpawnSeams` bundle so the CLI and picker cannot drift.
>
> **Resolved decision (2026-07-18, user):** on an unsupported/remote host terminal a multi-target `open` (N≥2) is **blocked outright** — an atomic no-op naming the identity, nothing opens, and the trigger does **not** open-here (no half-connect). This is consistent with the decided "multi-select fully disabled on unsupported terminals" direction captured in the two out-of-scope inbox bugs below, and matches `runSpawn`'s existing N≥2 unsupported gate. Rejected alternative: "connect the trigger (open-here), skip the N−1" — the half-measure the TUI decision explicitly rejected.
>
> **Out-of-scope cross-reference (not built in this feature):** the deeper remote/unsupported work lives in two active inbox bug reports, NOT in cli-verb-surface-redesign (`internal/spawn` detection is out of scope per the spec): `.workflows/.inbox/bugs/2026-07-15--remote-trigger-spawns-on-local-terminal.md` (the trigger-locality detection gate — a remote trigger with a local client attached must resolve unsupported) and `.workflows/.inbox/bugs/2026-07-16--persistent-no-host-terminal-banner.md` (block multi-select `m`-entry outright + the banner split). This task only ensures the CLI multi-target burst takes the honest block on whatever `Detect()`/`Resolve` reports today; fixing the detection gate itself is those bugs' job.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Multi-Target Burst Mechanics (The trigger absorbs the first target, unconditionally; no dedup).

---

## cli-verb-surface-redesign-3-7 | approved

### Task 3.7: Command rides mint windows only, byte-identical (+ trigger local-mint parity + multi-target zero-mint usage error)

**Problem**: A command (`-e`/`--`) is fed to a freshly-minted session's clean pane; an attach target has no safe injection channel (Phase 2). In a multi-target burst the command must ride **mint** windows only (appended after `--ack`), never attach windows, and must be carried **byte-identically** (no word-splitting) so the trigger's local mint and every spawned mint window run the same command. When the trigger itself is a command-carrying mint it mints **locally** (no spawned window) via the same `CreateFromDir`/`QuickStart` path. A multi-target set with a command but **zero** mint targets (all-attach) is a usage error.

**Solution**: Thread the parsed command (Phase 2 `parseCommandArgs`) into the burst so the Task-3-5 argv builder appends it — as a multi-token passthrough after `--ack` — only on **mint** surfaces; attach surfaces get no command. In `runOpenBurst`, when the trigger surface is a mint carrying the command, mint it locally via `openPath` (`CreateFromDir`/`QuickStart`), feeding the command identically. Before dispatching the burst, if a command is present and the surface set has zero mint surfaces, return a usage error.

**Outcome**: `open api ~/new -e claude` attaches `api` (no command) **and** mints `~/new` running claude in a second surface; `open ~/a ~/b -e "npm run dev"` runs the single-string `npm run dev` command byte-identically in both mints (trigger `~/a` local, `~/b` spawned); `open api web -e claude` (all-attach) is a usage error; the Phase-2 `-e`/`--` exclusivity and empty-command usage errors are preserved.

**Do**:
- Extend the Task-3-5 builder to append the command on mint surfaces only: `composeOpenArgv(exePath, path, s, batch, token, command)` appends `command` as discrete elements after `--ack` **iff** `s.Kind == SurfaceMint`; attach surfaces never carry it. Carry the command as authored — a single `-e "npm run dev"` is one element preserved verbatim (no word-splitting), matching `parseCommandArgs` which stores `-e`'s value as `[]string{execFlag}` (one element) and `--`'s args as the raw multi-token slice. So the mint window argv is `… open --path <literal-dir> --ack <batch>:<token> -- <cmd> args…` (use the `--` passthrough form so the spawned `open` parses it as its own command).
- In `runOpenBurst` (Task 3-6), before dispatch, add the multi-target zero-mint guard:
  ```go
  if len(command) > 0 && !anyMint(surfaces) {
      return NewUsageError("a command (-e/--) needs a new session to run in; every target is an existing session")
  }
  ```
  (`anyMint` scans for `SurfaceMint`.) This is the multi-target arity of the Phase-2 single-target "command + attach ⇒ usage error"; the single-target arity is already covered by Task 2-6's `openResolved` guard.
- Trigger local-mint parity: in the self-connect step, when `trigger.Kind == SurfaceMint`, mint locally via `openPathFunc(cmd, trigger.Value, command)` (the same `CreateFromDir`/`QuickStart` path a spawned mint window takes) instead of spawning a window — the trigger is the invoking terminal, so its mint happens in-process, feeding the command as the pane's initial process, byte-identically to the spawned mints. When `trigger.Kind == SurfaceAttach`, connect via `buildSessionConnector` with **no** command (an attach can't take a command; a command-carrying set would have been caught by the zero-mint guard only if ALL were attach — a mixed set with an attach trigger + mint externals is valid, the trigger just attaches without the command).
- Preserve the Phase-2 parse-time guards unchanged (regression coverage): both `-e` and `--` ⇒ usage error; empty `-e`/bare `--` ⇒ usage error (all in `parseCommandArgs`, `cmd/open.go:166`).

**Acceptance Criteria**:
- [ ] The command is appended after `--ack` on MINT windows only; attach windows never carry it.
- [ ] A mixed set attaches the attach targets bare and mints the mint targets running the command (`open api ~/new -e claude` → attach `api`, mint `~/new` running claude).
- [ ] The command is byte-identical across surfaces with no word-splitting (`-e "npm run dev"` stays one unit), so the trigger's local mint and every spawned mint run the same command.
- [ ] A trigger that is a command-carrying mint mints locally via `openPath` (`CreateFromDir`/`QuickStart`), feeding the command identically — no spawned window for the trigger.
- [ ] A multi-target set with a command and zero mint targets (all-attach) returns a `*UsageError` (exit 2); nothing opens.
- [ ] The Phase-2 `-e`/`--` exclusivity and empty-command usage errors are preserved (regression).

**Tests** (`cmd/open_test.go` + `internal/spawn/command_test.go`; unit lane, fakes):
- `"the command rides mint windows only"` — burst over `[attach:api, mint:/new]` with command `["claude"]` → the spawned argv for `api` has no `--`/command; the argv for `/new` ends `--ack b:t -- claude`.
- `"the command is byte-identical with no word-splitting"` — command `["npm run dev"]` (single `-e` string) → the mint argv carries `-- "npm run dev"` as one element; a second mint carries the identical element.
- `"a command-carrying trigger mint mints locally"` — surfaces `[mint:/a, mint:/b]` command `["claude"]`; assert `openPathFunc` captured `("/a", ["claude"])` (trigger local mint) and the fake adapter opened `/b` with `-- claude`.
- `"a multi-target all-attach set with a command is a usage error"` — `open api web -e claude` → `*UsageError`, no window opened, no self-connect.
- `"a mixed set with an attach trigger + mint external attaches the trigger bare"` — surfaces `[attach:api, mint:/new]` command `["claude"]` → trigger `api` connects with no command; `/new` spawns with `-- claude`.
- `"the -e/-- exclusivity and empty-command guards are preserved"` — `open ~/a ~/b -e vim -- claude` and `open ~/a -e ""` → `*UsageError` (Phase-2 regression).

**Edge Cases**:
- Appended after `--ack` on MINT windows only; attach windows never carry it.
- Mixed set attaches bare + mints running the command.
- Byte-identical, no word-splitting (`-e "npm run dev"` one unit) so local + spawned mints run identical commands.
- Trigger that is a command-carrying mint mints locally via `CreateFromDir`/`QuickStart`.
- Multi-target zero-mint + command (all-attach) → usage error.
- `-e`/`--` exclusivity + empty-command usage errors preserved from Phase 2.

**Context**:
> Spec § Command passthrough (`-e` / `--`) — mint-scoped: "The command targets mint surfaces only. … Mixed sets are allowed; the command is scoped to the mint targets. … `open api ~/new -e claude` → attach `api` as-is and mint `~/new` running claude, in two surfaces. Zero mint targets + a command ⇒ usage error. `open api web -e claude` (all existing sessions) → error." Spec § Burst exec-argv & mint responsibility, point 3: "Command passthrough rides mint windows only. When a command is present … it is appended to each mint window's argv in the multi-token passthrough form, after `--ack`: `portal open --path <literal-dir> --ack <batch>:<token> -- <cmd> args…`. Attach windows never carry the command. When the trigger surface is itself a mint target carrying the command, the trigger mints locally (no spawned window) and feeds the command to `CreateFromDir` / `QuickStart` as the pane's initial process — the same path a spawned mint window takes. Command parity — no word-splitting. The command is carried to every mint surface as authored: a single `-e "npm run dev"` string is preserved as one unit, never split into separate tokens. The trigger's local mint and every spawned mint window therefore run byte-identical commands."
>
> `parseCommandArgs` (`cmd/open.go:166`) already yields the command as a `[]string` (one element for `-e`, the raw multi-token slice for `--`) with the exclusivity + empty-command guards, so no re-parse is needed — the multi-target path threads that same slice. The trigger local mint reuses `openPath` (`QuickStart`/`CreateFromDir`), which is exactly what a spawned `open --path … -- <cmd>` re-enters, so the two are byte-identical by construction. The multi-target zero-mint guard is the multi-target arity of Task 2-6's single-target attach-command guard.

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Flags & Command Passthrough (Command passthrough (`-e` / `--`) — mint-scoped); § `portal open` — Multi-Target Burst Mechanics (Burst exec-argv & mint responsibility).

---

## cli-verb-surface-redesign-3-8 | approved

### Task 3.8: Leave-what-opened partial failure + per-window ~8s ack timeout + `portal.log` outcomes

**Problem**: Past the read-only pre-flight, a per-window failure must be **leave-what-opened**: opened windows stay (Portal doesn't own/tear-down host windows), failed/un-acked surfaces don't auto-retry, and the trigger still connects to its own first-target surface **independent of** other windows' ack failures (skipped only if its own target fails at connect). Each window's outcome must be recorded in `portal.log` (the durable surface — stderr is swallowed on attach). A permission-required result stops the burst and is surfaced once. Batch markers must be cleaned on every terminal path before the self-connect.

**Solution**: Reuse the existing `spawn` burst machinery in `runOpenBurst` (Task 3-6): the `Burster` already applies the per-window ~8s `spawnAckTimeout` (timer per window's own spawn) and the permission early-stop; wire the outcome to `spawn.PartitionResults`/`FirstPermission`, log via `spawn.LogBatchSummary`/`LogPermission`/`LogWindowResults`, clean the batch via `Ack.Clean(batch)` before the self-connect, and connect the trigger independent of the external windows' outcomes (skipping only if the trigger's own connect fails).

**Outcome**: `open api blog web` where `web`'s window never acks within ~8s → `api` still attaches and `blog` stays open; the un-acked `web` is recorded in `portal.log` and not auto-retried; a permission-required window stops the burst and surfaces the driver guidance once; the batch's `@portal-spawn-*` markers are cleaned before the trigger's `exec attach`.

**Do**:
- Reuse the Burster's per-window ack timeout unchanged: `spawnAckTimeout = 8 * time.Second` (`internal/spawn/burst.go:30`), the timer starting at each window's own spawn (`awaitToken`), so cumulative sequential delay never eats a later window's budget. No new timeout logic — `runOpenBurst` gets it for free from `deps.NewBurster(...).Run(...)`.
- After `Run` returns `(batch, results, err)`:
  - `_ = deps.Ack.Clean(batch)` on every post-burst path and BEFORE the self-connect handoff (parity with `runSpawn`, `cmd/spawn.go:169`) — a point of no return outside tmux.
  - `confirmed, failed := spawn.PartitionResults(results)` (the single count-semantics chokepoint).
  - Permission-required precedence: `if perm, ok := spawn.FirstPermission(results); ok { spawn.LogWindowResults(logger, results); spawn.LogPermission(logger, id, resolution, perm.Result.Detail); … }` — the burst already stopped on the first wall (same `(source,target)` grant), so surface the guidance ONCE. Decide the trigger connect independently (see below).
  - Otherwise `spawn.LogBatchSummary(logger, id, resolution, results, total, triggerConnected, batch)` — one INFO `opened N/N` cycle summary + per-window DEBUG detail, where `total = N` (all surfaces incl. the trigger) and `triggerConnected` reflects whether the trigger's own connect succeeded.
- Leave-what-opened: opened (confirmed) external windows stay; failed/un-acked surfaces are NOT auto-retried and NOT torn down (Portal has no host-window teardown seam). Unlike the picker's "stay marked for retry" affordance, the CLI has no retry state — a failed window is simply recorded.
- Trigger self-connect independence: the trigger connects to its own first surface **regardless** of other windows' ack failures (its target is unrelated to theirs). It is skipped ONLY if the trigger's own target fails at connect (e.g. an attach session that vanished between pre-flight and connect, or a local mint that errors) — in which case, outside tmux, Portal returns to the shell without attaching (no `exec`, a plain error/return). A best-effort stderr summary is emitted but is only directly visible in this skip case (on a successful attach it is swallowed by the alternate screen / switched-away pane — the "tmux is the receipt" constraint).
- `portal.log` is the durable surface: all per-window/batch/permission outcomes go through the shared `spawn` logemit helpers (`internal/spawn/logemit.go`) under the `spawn` log component (`deps.Logger = log.For("spawn")`, from `buildProductionSpawnSeams`). The stderr summary is best-effort and swallowed on a successful attach.
- Permission-required stops the burst (no further external spawns): the `Burster.Run` loop already breaks on `OutcomePermissionRequired` (`internal/spawn/burst.go:177`); `runOpenBurst` surfaces `perm.Result.Guidance` once (logged via `LogPermission`, best-effort stderr). **Resolved decision (2026-07-18): the trigger still self-connects** — a permission wall on an *external* window is not the trigger's own target failing, so under the spec's trigger-independence rule the trigger connects to its own viable surface (the guidance rides `portal.log`). This is a deliberate divergence from `runSpawn` (which does not self-attach on a not-all-confirmed batch). The trigger is skipped only if the trigger's OWN target fails at connect.

**Acceptance Criteria**:
- [ ] Opened (confirmed) external windows stay in place; failed/un-acked surfaces are not auto-retried and not torn down.
- [ ] The per-window ack timeout is ~8s, timed from each window's own spawn (reused `spawnAckTimeout` + `awaitToken`); cumulative earlier-window delay never eats a later window's budget.
- [ ] The trigger connects to its own first-target surface independent of other windows' failures, and is skipped only if its own target fails at connect (outside tmux → returns to the shell without attaching).
- [ ] Each window's outcome is recorded in `portal.log` via the shared `spawn` logemit helpers; the stderr summary is best-effort and swallowed on a successful attach.
- [ ] A permission-required window stops the burst and its guidance is surfaced exactly once (no double-report as a generic failed-window line).
- [ ] The batch's `@portal-spawn-*` markers are cleaned on every terminal path before the trigger's self-connect.

**Tests** (`cmd/open_test.go`; inject `OpenBurstDeps` with `spawntest.FakeAdapter`/`FakeAckChannel` + a manual clock (as `cmd/spawn_test.go`'s `withBurster` does) + a fake `Connector` + a `logtest` sink — unit lane; the real ~8s wall is driven by the injected clock, never real time):
- `"an un-acked external window leaves the others open and still connects the trigger"` — external `[blog, web]` with `web` never acking; assert `blog` stayed (confirmed), `web` recorded failed, and the trigger `api` still connected.
- `"the per-window timeout is timed from each window's own spawn"` — drive the manual clock so window 1's spawn + delay does not consume window 2's budget; assert window 2 still gets its full `spawnAckTimeout`.
- `"the trigger connect is independent of external failures"` — all externals fail/timeout; assert `Connector.Connect(trigger)` still ran (trigger's own target is viable).
- `"the trigger self-connect is skipped when its own target fails at connect"` — the trigger's attach session vanished (connector returns an error) → no exec, a plain error returned; the stderr summary is emitted.
- `"each outcome is recorded in portal.log"` — assert the `spawn` component INFO `opened N/N` + per-window records via the `logtest` sink.
- `"a permission-required window stops the burst and surfaces guidance once"` — one external returns `PermissionRequired`; assert the burst stopped, `LogPermission` fired once, and the generic failed-window line did not also fire.
- `"batch markers are cleaned before the self-connect"` — assert `Ack.Clean(batch)` recorded before `Connector.Connect`.
- Integration (`//go:build integration`, `portaltest.IsolateStateForTest` + `portalbintest`) only for a real exec'd burst end-to-end; the orchestration/timeout/logging is fully unit-covered above via injected clock + fakes.

**Edge Cases**:
- Opened windows stay (no teardown); failed/un-acked surfaces don't auto-retry (the CLI has no picker-style "stay marked" affordance).
- Per-window ~8s timeout timed from each window's own spawn.
- Trigger connects independent of other windows' failures; skipped only if its own target fails at connect (outside tmux returns to the shell).
- Each outcome in `portal.log`; stderr summary swallowed on attach (log is the durable surface).
- Permission-required stops the burst, surfaced once.
- Batch markers cleaned on every terminal path before self-connect.

**Context**:
> Spec § Atomic pre-flight & partial failure: "Past the resolve, per-window failure is leave-what-opened. Opened windows stay … and failed/un-acked surfaces don't retry automatically. The trigger connects to its own first-target surface whenever that surface is viable — independent of other windows' ack failures … those failures don't cost the trigger its landing. The trigger's self-connect is skipped only if its own target fails at connect … when that happens, outside tmux Portal returns to the shell without attaching. … Where failures are reported — `portal.log` is the durable surface. The burst records each window's outcome in `portal.log`; that is the reliable record. When the trigger attaches, any stderr the burster prints just before connecting is swallowed by the attach … A best-effort stderr summary is still emitted, directly visible only in the skip case. … Per-window ack timeout (~8s). The parent polls for each window's `@portal-spawn-<batch>-<token>` receipt with a per-window timeout of ~8s, the timer starting at that window's own spawn so cumulative sequential delay never eats a later window's budget." Spec § Burst exec-argv & mint responsibility: "each surface opens/mints itself at exec time under leave-what-opened; a window that never comes up never mints, so there are no orphaned detached sessions."
>
> This task is almost entirely wiring of existing `spawn` machinery: the per-window `spawnAckTimeout` (`burst.go:30`) + `awaitToken` per-window timer, the permission early-stop (`burst.go:177` + `FirstPermission`), the `PartitionResults` count semantics, the `LogBatchSummary`/`LogPermission`/`LogWindowResults` emitters (`logemit.go`), and the `Ack.Clean(batch)` before self-connect — all already used by `runSpawn` (`cmd/spawn.go`). The one genuine `open`-specific behaviour is the trigger-independence rule (the trigger connects regardless of external failures, unlike `runSpawn`'s "self-attach only when EVERY external confirms").
>
> **Resolved decision (2026-07-18, user):** `runSpawn` skips the trigger self-attach on ANY not-all-confirmed batch, but the spec's `open` rule makes the trigger connect independent of external failures (skipped only if its OWN target fails). This task implements the spec's `open` rule (a divergence from `runSpawn`), so a burst where an external window failed — **including a permission-required wall on an external window** — still connects the trigger when the trigger's own target is viable; the permission guidance is surfaced once to `portal.log` (and best-effort stderr). The spec is explicit on this (§ Atomic pre-flight & partial failure: "The trigger connects to its own first-target surface whenever that surface is viable … independent of other windows' ack failures … skipped only if its own target fails at connect"), so no half-measure — the trigger lands. (The distinct *unsupported-terminal* case — Task 3-6 — is where the whole burst is blocked and the trigger does not connect; a permission wall on a *supported* terminal is not that case.)

**Spec Reference**: `.workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` — § `portal open` — Multi-Target Burst Mechanics (Atomic pre-flight & partial failure; Burst exec-argv & mint responsibility).
