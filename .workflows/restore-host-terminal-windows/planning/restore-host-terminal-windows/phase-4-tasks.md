---
phase: 4
phase_name: Config Escape Hatch — terminals.json
total: 6
---

## restore-host-terminal-windows-4-1 | approved

### Task 4.1: `terminals.json` store — load + tolerant decode

**Problem**: The config-override tier needs to read the user-authored `terminals.json` escape hatch, but that file is shipped, hand-edited, and drives command execution — so a malformed or unreadable file must degrade safely (fall through to native → unsupported) and **never crash the picker**. Nothing in `internal/spawn` yet reads or models `terminals.json`, and the resolver (Task 4.6) has no config to consult.

**Solution**: Add `internal/spawn/terminalsconfig.go` — the JSON model for `terminals.json` (`TerminalsConfig` map of identity-key → `TerminalEntry` → `Capabilities{ Open *Recipe }`, where `Recipe` carries `argv`/`script`) plus a read-only `TerminalsStore` whose `Load` follows Portal's tolerant-decode convention (like `internal/hooks/store.go`): a missing file yields an empty config with no WARN, and an unreadable or malformed file yields an empty config after a `spawn`-component WARN. Unknown capability sub-keys (`introspect`/`place`) are dropped for free by `encoding/json`.

**Outcome**: `spawn.NewTerminalsStore(path).Load()` returns a usable (possibly empty) `TerminalsConfig` for every input: absent file → `TerminalsConfig{}` silently; unreadable file (e.g. permission-denied) → `TerminalsConfig{}` + one `spawn` WARN; malformed JSON → `TerminalsConfig{}` + one `spawn` WARN; a well-formed file with future `introspect`/`place` sub-keys parses cleanly with those keys ignored and only `commands.open` retained. `Load` performs reads only — it never creates, truncates, or writes the file. Fully unit-tested with fabricated files under `t.TempDir()`; WARNs asserted via the `logtest.Sink` capture handler.

**Do**:
- Create `internal/spawn/terminalsconfig.go`:
  - JSON model (mirroring the spec's schema `{"<key>": {"commands": {"open": {"argv":[…]}|{"script":"…"}}}}`):
    - `type Recipe struct { Argv []string `json:"argv"`; Script string `json:"script"` }`.
    - `type Capabilities struct { Open *Recipe `json:"open"` }` — `Open` is a pointer so an absent `open` sub-key decodes to `nil` (distinguishable from a present-but-empty recipe). Future `introspect`/`place` sub-keys are simply **not fields**, so `encoding/json` drops them (no `DisallowUnknownFields`) — the forward-compat "unknown capability sub-keys ignored" rule for free.
    - `type TerminalEntry struct { Commands Capabilities `json:"commands"` }`.
    - `type TerminalsConfig map[string]TerminalEntry` — key is whatever identity form the user pasted (friendly alias / `.app` name / raw bundle id / `*`-glob); matched in Task 4.3.
  - `type TerminalsStore struct { path string }` and `func NewTerminalsStore(path string) *TerminalsStore { return &TerminalsStore{path: path} }` (the path is resolved via `configFilePath("PORTAL_TERMINALS_FILE", "terminals.json")` at the cmd/wiring layer in Task 4.6 — the store itself just takes a path, exactly like `hooks.NewStore`).
  - `func (s *TerminalsStore) Load() TerminalsConfig`:
    1. `data, err := os.ReadFile(s.path)`.
    2. On `err`: `if errors.Is(err, os.ErrNotExist) { return TerminalsConfig{} }` (missing → empty, **no WARN** — an unconfigured install is the normal case). Any other read error (unreadable/permission) → emit the `spawn` WARN and `return TerminalsConfig{}`.
    3. `var cfg TerminalsConfig; if err := json.Unmarshal(data, &cfg); err != nil { <spawn WARN>; return TerminalsConfig{} }` (malformed → whole file ignored, per spec).
    4. `if cfg == nil { cfg = TerminalsConfig{} }; return cfg` (a literal JSON `null` decodes to a nil map — normalise so callers never nil-panic).
  - Do **not** add a `Save`/write method: `terminals.json` is read-only at spawn time (user-authored). `Load` must touch the filesystem for reads only.
  - Emit WARNs through the package-level `spawn`-component logger (`log.For("spawn")`, the component registered in Phase 1; reuse the existing package `logger`/`spawnLogger` var if Phase 1 bound one). Keep to the **closed spawn attr set** — carry the OS/config-specific reason in the opaque `detail` attr, e.g. `logger.Warn("terminals.json unreadable", "detail", err.Error())` and `logger.Warn("terminals.json malformed", "detail", err.Error())`. Do **not** invent a new attr key for the path/error.
- Add `internal/spawn/terminalsconfig_test.go` (unit lane, white-box `package spawn`):
  - Missing file: point the store at a non-existent path under `t.TempDir()`; assert `Load()` returns an empty (`len==0`) config, no file is created afterward, and the `logtest.Sink` captured **zero** WARN records.
  - Unreadable file: write a file then `os.Chmod(path, 0o000)` (register a `t.Cleanup` to restore perms so `TempDir` teardown succeeds); assert empty config + exactly one `spawn` WARN. (If the CI/dev environment runs as root and 0o000 is still readable, an alternative unreadable shape is a directory at the path — `os.Mkdir(path)` then `Load` — which `ReadFile` rejects as a non-file; either shape proves the non-ENOENT read-error branch.)
  - Malformed JSON: write `{ not valid json`; assert empty config + exactly one `spawn` WARN whose `detail` is non-empty.
  - Unknown capability sub-keys: write `{"com.example.MyTerm":{"commands":{"open":{"argv":["kitty","{command}"]},"introspect":{"foo":1},"place":{"bar":2}}}}`; assert the entry parses, `Commands.Open` is non-nil with the `argv`, and no error/WARN (the `introspect`/`place` keys are silently dropped).
  - Read-only proof: snapshot the file bytes (or mtime) before `Load` on a valid file and assert they are unchanged after; for the missing-file case assert no file exists at the path after `Load`.

**Acceptance Criteria**:
- [ ] `Load` on a non-existent path returns an empty config and emits **no** WARN, and no file is created.
- [ ] `Load` on an unreadable file returns an empty config and emits exactly one `spawn`-component WARN (reason carried in `detail`).
- [ ] `Load` on a malformed-JSON file returns an empty config and emits exactly one `spawn`-component WARN.
- [ ] A valid entry with future `introspect`/`place` capability sub-keys parses successfully with those sub-keys ignored; `Commands.Open` retains the `open` recipe.
- [ ] `Load` performs reads only — it never creates, truncates, or writes the file (verified for both the valid and the missing-file cases).
- [ ] `Load` never returns a nil map (a JSON `null` normalises to `TerminalsConfig{}`), so callers never nil-panic ranging over it.

**Tests**:
- `"it returns an empty config with no WARN for a missing file"`
- `"it ignores an unreadable file and emits a spawn WARN"`
- `"it ignores a malformed file and emits a spawn WARN"`
- `"it parses a valid entry and ignores unknown capability sub-keys"`
- `"it never writes the file (read-only load)"`
- `"it normalises a JSON null to an empty config"`

**Edge Cases**:
- Missing file → empty result, **no** WARN (the unconfigured install is the normal path).
- Malformed / unreadable file → whole file ignored + one `spawn`-component WARN (degrade safely, never crash the picker).
- Unknown capability sub-keys (`introspect`/`place`) ignored (forward-compat, via `encoding/json` dropping unmodeled keys).
- Read-only: `Load` never writes the file.

**Context**:
> Spec *Config Schema → Location & format*: "`~/.config/portal/terminals.json` — Portal's JSON-store convention (like `projects.json` / `hooks.json`), XDG-resolved via the existing `configFilePath`. Read-only at spawn time; user-authored."
> Spec *Config Schema → Validation & error handling*: "Consistent with Portal's other JSON stores (tolerant decode): **Malformed / unreadable file** → the whole file is ignored; resolution falls through to native adapters → unsupported. A WARN breadcrumb is emitted under the `spawn` component. … **Unknown capability sub-keys** (e.g. a future `introspect` / `place`) → ignored (forward-compat)."
> Spec *Observability & State Footprint → State/daemon footprint*: "**Reads** `terminals.json` (user-authored, read-only at spawn time)."
> `internal/hooks/store.go`'s `Load` is the reference tolerant-decode shape (ENOENT → empty; malformed → empty). Note the deliberate divergence: hooks returns `(map, error)` and stays silent on malformed, whereas this store returns a single `TerminalsConfig` (every failure degrades to empty — there is no error the resolver should act on, it just falls through to native) and **WARNs** on unreadable/malformed per the spec. The `spawn` component + the `detail` attr already exist in the closed taxonomy (Phase 1 / spec *Observability → Attr keys*).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Config Schema → Location & format / Structure / Validation & error handling*; *Observability & State Footprint → State/daemon footprint / Attr keys (`detail`)*.

---

## restore-host-terminal-windows-4-2 | approved

### Task 4.2: Recipe structural validation — exactly-one-of `argv`/`script` + `{command}` presence

**Problem**: A `terminals.json` `open` recipe must carry **exactly one** of `argv` / `script`, and an `argv` template must reference the `{command}` placeholder (a window with no `{command}` would never run the attach). A recipe that violates these rules is a config typo that must be **skipped with a diagnosable WARN** and fall through to native — never crash and never silently degrade to "unsupported" with no breadcrumb. Task 4.1 models the recipe but does not validate it.

**Solution**: Add `internal/spawn/recipe.go` with a pure `validateRecipe(Recipe) (RecipeKind, error)` (the exactly-one-of + `{command}`-presence rules, returning whether the recipe is an argv or script form) and a thin `validRecipeForEntry(key, TerminalEntry) (Recipe, RecipeKind, bool)` that extracts the entry's `open` recipe, validates it, and emits the `spawn`-component WARN (naming the entry key + reason in `detail`) on rejection. Also add the shared `renderCommandString` helper that Tasks 4.4/4.5 use to turn the composed attach argv into the single `{command}` string.

**Outcome**: `validateRecipe` returns `RecipeArgv`/`RecipeScript` for a well-formed recipe and a descriptive error for neither/both/`{command}`-missing; `validRecipeForEntry` returns `ok=false` after a single `spawn` WARN for a structurally-invalid recipe, `ok=false` **without** a WARN for an entry that defines no `open` capability at all (forward-compat: only future capabilities configured), and `ok=true` with the recipe + kind for a valid argv-only or script-only recipe. `renderCommandString(argv)` space-joins the composed argv (matching the native Ghostty embed). All pure and unit-tested; WARNs asserted via `logtest.Sink`.

**Do**:
- Create `internal/spawn/recipe.go`:
  - `type RecipeKind int` with `const ( RecipeArgv RecipeKind = iota + 1; RecipeScript )` — the zero value is an explicit "invalid/none" so a bare `RecipeKind` is never mistaken for a valid form.
  - `func validateRecipe(r Recipe) (RecipeKind, error)`:
    - `hasArgv := len(r.Argv) > 0`; `hasScript := strings.TrimSpace(r.Script) != ""`.
    - both → `return 0, errors.New("recipe declares both argv and script (exactly one required)")`.
    - neither → `return 0, errors.New("recipe declares neither argv nor script (exactly one required)")`.
    - argv form → require the `{command}` placeholder in at least one element: `if !argvHasCommandPlaceholder(r.Argv) { return 0, errors.New("argv recipe omits the {command} placeholder") }`; else `return RecipeArgv, nil`.
    - script form → `return RecipeScript, nil`. (The `{command}`-presence rule does **not** apply to `script`: a script recipe **always** receives `{command}` as `$1` from Portal, so the window can never be left with "no `{command}`" — the placeholder is delivered structurally, not embedded in the script path. See the spec rationale below.)
  - `func argvHasCommandPlaceholder(argv []string) bool` — true iff some element `strings.Contains(el, "{command}")`.
  - `func validRecipeForEntry(key string, e TerminalEntry) (Recipe, RecipeKind, bool)`:
    - `if e.Commands.Open == nil { return Recipe{}, 0, false }` — no `open` capability configured for this entry (e.g. only a future `introspect`); this is **forward-compat, not a typo**, so return `ok=false` with **no WARN** (the resolver falls through to native).
    - `kind, err := validateRecipe(*e.Commands.Open)`; on `err`: `logger.Warn("terminals.json entry rejected", "detail", fmt.Sprintf("%q: %v", key, err)); return Recipe{}, 0, false`.
    - else `return *e.Commands.Open, kind, true`.
  - `func renderCommandString(command []string) string { return strings.Join(command, " ") }` — the single canonical rendering of the composed attach argv (Task 2.3/3.5's env-self-sufficient `/usr/bin/env … attach <session> --spawn-ack <b>:<t>`) into the `{command}` string that argv recipes substitute (Task 4.4) and script recipes deliver as `$1` (Task 4.5). This is the **same** space-join the native Ghostty `ghosttyEmbed` uses, so config and native render `{command}` identically.
  - Emit the WARN through the `spawn`-component logger, carrying `key` + reason only in the opaque `detail` attr (the closed attr set has no dedicated entry-key attr).
- Add `internal/spawn/recipe_test.go` (unit lane, white-box `package spawn`):
  - Pure `validateRecipe`: both-present → err; neither → err; argv without `{command}` → err; valid argv-with-`{command}` → `RecipeArgv,nil`; valid script (non-empty path, no argv) → `RecipeScript,nil`; assert the zero `RecipeKind` on every error.
  - `validRecipeForEntry`: invalid recipe → `ok=false` + exactly one captured `spawn` WARN whose `detail` names the key; `Commands.Open == nil` → `ok=false` + **zero** WARNs; valid argv / valid script → `ok=true` with the recipe + expected kind and zero WARNs.
  - `renderCommandString(["/usr/bin/env","-u","TMUX","PATH=/b","/abs/portal","attach","proj-x","--spawn-ack","b1:t1"])` → the exact space-joined string.

**Acceptance Criteria**:
- [ ] A recipe with **both** `argv` and `script` is invalid; a recipe with **neither** is invalid — both return the zero `RecipeKind` and a descriptive error.
- [ ] An `argv` recipe whose template contains no `{command}` in any element is invalid; a valid argv-only recipe returns `RecipeArgv`; a valid script-only recipe returns `RecipeScript`.
- [ ] `validRecipeForEntry` emits exactly one `spawn` WARN naming the entry key (in `detail`) for a structurally-invalid recipe and returns `ok=false`.
- [ ] `validRecipeForEntry` returns `ok=false` with **no** WARN when the entry declares no `open` capability (forward-compat).
- [ ] The `{command}`-presence rule is applied to `argv` recipes only; a `script` recipe is never rejected for "omitting `{command}`" (it is delivered as `$1`).
- [ ] `renderCommandString` produces the single space-joined command string, identical in form to the native Ghostty embed.

**Tests**:
- `"it rejects a recipe declaring both argv and script"`
- `"it rejects a recipe declaring neither argv nor script"`
- `"it rejects an argv recipe that omits the {command} placeholder"`
- `"it accepts a valid argv-only recipe and a valid script-only recipe"`
- `"it warns once and skips an entry with a structurally invalid open recipe"`
- `"it skips an entry with no open capability without warning (forward-compat)"`
- `"renderCommandString space-joins the composed attach argv"`

**Edge Cases**:
- Neither `argv` nor `script` → invalid + WARN.
- Both present → invalid + WARN.
- `argv` template omits `{command}` → invalid + WARN.
- Valid argv-only / valid script-only → accepted.
- Entry with no `open` capability (only future `introspect`/`place`) → skipped **without** a WARN (forward-compat, distinct from a typo).

**Context**:
> Spec *Config Schema → Validation & error handling*: "**Per-entry recipe** must carry **exactly one** of `argv` / `script`. Neither or both → that entry is invalid and skipped (falls through to native → unsupported), with a WARN naming the entry key. **A recipe template omitting `{command}`** → invalid entry, skipped with a WARN (a window with no `{command}` would never run the attach). … Every rejection emits a `spawn`-component breadcrumb so a config typo is diagnosable rather than silently degrading to 'unsupported.'"
> Spec *Config Schema → Recipe execution contract*: "`script` recipes receive `{command}` as `$1` (first positional arg)." — this is why the `{command}`-presence check is argv-only: Portal always supplies `$1`, so a script recipe can never structurally lack the command. The `{command}` string is "a single, already-resolved command string" — hence `renderCommandString` produces one string, not an argv.
> The entry-key + reason ride in the opaque `detail` attr because the closed `spawn` attr set (`batch`/`terminal`/`bundle_id`/`resolution`/`session`/`ack`/`opened`/`total`/`detail`) has no dedicated key attr — same discipline as the driver-quarantine `detail` usage.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Config Schema → Validation & error handling / Recipe execution contract / Placeholder `{command}`*; *Observability & State Footprint → Attr keys (`detail`)*.

---

## restore-host-terminal-windows-4-3 | approved

### Task 4.3: Identity match + within-config most-specific precedence

**Problem**: A single detected terminal identity can match **several** `terminals.json` keys at once — a raw bundle id, a `.app` name, a friendly alias, and one or more `*`-globs. The resolver must pick a **single, deterministic** winner: the user's specific override must always beat their glob fallback, never the reverse. Nothing yet maps an `Identity` onto config keys or ranks the matches.

**Solution**: Add `internal/spawn/configmatch.go` — a pure `matchConfig(cfg, id)` that scans the config keys, matches each against the identity by form (raw bundle id / `.app` name / friendly alias / `*`-glob, all reducing to bundle-id-family matching via Phase 1's `MatchesFamily`), scores each match by specificity, and returns the single most-specific matching entry (or `ok=false` for no match). Ships the small Portal-blessed friendly-alias→family table (`ghostty`, `warp`).

**Outcome**: `matchConfig(cfg, id)` returns the most-specific matching `(key, entry, true)` for an identity that matches multiple keys — exact raw bundle id beats `.app`/alias beats `*`-glob; among globs a longer literal prefix beats a broader one and a bare `*` catch-all is lowest; a friendly-alias key resolves through the bundle-id family; a wholly-unmatched identity returns `ok=false` (fall through). Pure and fully unit-tested with fabricated configs + identities; no adapter is built and no recipe is validated (validity is Task 4.2, wired in Task 4.6).

**Do**:
- Create `internal/spawn/configmatch.go`:
  - Ship the friendly-alias table: `var friendlyAliases = map[string]string{ "ghostty": "com.mitchellh.ghostty*", "warp": "dev.warp.Warp-*" }` — Portal-shipped aliases for known terminals mapping to their bundle-id family (the spec's two named examples; extend here as more ship). Matching a friendly alias means `MatchesFamily(id.BundleID, friendlyAliases[key])`.
  - `func matchConfig(cfg TerminalsConfig, id Identity) (key string, entry TerminalEntry, ok bool)`:
    - Iterate `cfg`; for each `(k, e)` compute `score, matched := scoreKey(k, id)`; skip non-matches; keep the best score (highest wins), replacing on a strictly-better score. Return the winning key/entry, or `ok=false` if none matched.
  - `func scoreKey(key string, id Identity) (matchScore, bool)` — classify by form and compute a specificity score:
    1. **Glob** (`strings.Contains(key, "*")`): match iff `MatchesFamily(id.BundleID, key)`; score `{tier: tierGlob, literals: countLiterals(key)}` (glob semantics reuse Phase 1's family matcher). `countLiterals` = number of non-`*` runes, so a longer/more-specific glob outscores a broader one and a bare `*` (0 literals) is the lowest of all matches.
    2. **Exact raw bundle id** (no `*`, `key == id.BundleID`): score `{tier: tierBundleID}` (highest tier — the exact id Portal displayed for copy-paste).
    3. **Friendly alias** (no `*`, `key` in `friendlyAliases` and `MatchesFamily(id.BundleID, friendlyAliases[key])`): score `{tier: tierNamed}`.
    4. **`.app` name** (no `*`, `id.Name != "" && key == id.Name`): score `{tier: tierNamed}`.
    5. else `return matchScore{}, false`.
  - Tiers highest-first: `tierBundleID > tierNamed > tierGlob` (e.g. `const ( tierGlob = 1; tierNamed = 2; tierBundleID = 3 )`). Define `type matchScore struct { tier int; literals int }` and a total order `func (s matchScore) better(o matchScore) bool` comparing `tier` desc then `literals` desc. Break a residual exact tie deterministically in `matchConfig` by comparing the key string (e.g. keep the lexicographically-smaller key) so iteration-order over the map never makes the winner nondeterministic.
  - Keep `matchConfig` **pure** — no logging, no recipe validation, no adapter construction. It answers only "which single config key most-specifically matches this identity."
- Add `internal/spawn/configmatch_test.go` (unit lane, white-box `package spawn`): fabricate identities via `NewIdentity(bundleID, name)` and configs via `TerminalsConfig` literals; assert the winning key across the precedence tiers.

**Acceptance Criteria**:
- [ ] An identity matching a raw bundle id, a `.app` name, a friendly alias, **and** a `*`-glob all at once resolves to the **raw bundle id** entry (highest tier).
- [ ] A friendly-alias key (`"ghostty"`) matches a `com.mitchellh.ghostty` identity via the bundle-id family and outranks a bare `*` glob.
- [ ] Among two matching globs, the one with the longer literal prefix wins (`com.mitchellh.*` beats `com.*` beats bare `*`).
- [ ] A bare `*` catch-all is the lowest-precedence match — it wins only when nothing more specific matches.
- [ ] An identity matching **no** key returns `ok=false` (the resolver falls through to native).
- [ ] `matchConfig` is deterministic regardless of Go map iteration order (a residual tier+literals tie resolves by key string).

**Tests**:
- `"it picks the exact raw bundle id over a .app name, alias, and glob"`
- `"it matches a friendly-alias key through the bundle-id family"`
- `"it prefers a longer glob over a broader glob and both over a bare catch-all"`
- `"it selects the bare * catch-all only when nothing more specific matches"`
- `"it returns no match for an identity absent from the config"`
- `"it resolves ties deterministically across map iteration order"`

**Edge Cases**:
- Identity matches several entries → most-specific wins (bundle id > `.app`/alias > glob).
- Friendly-alias key matches via bundle-id family (`ghostty` → `com.mitchellh.ghostty*`).
- Longer/more-specific `*`-glob beats a broader one; bare `*` catch-all lowest.
- No match → `ok=false` (fall through).
- Deterministic under Go map iteration order (tie-break on the key string).

**Context**:
> Spec *Config Schema → Precedence*: "**Within config, most-specific wins** (a single identity can match several entries — a raw bundle id, a `.app` name, a friendly alias, and a `*`-glob). Deterministic order, highest first: **exact raw bundle id** → **exact `.app` name / friendly alias** → **`*`-glob** (a longer/more-specific glob beats a broader one; a bare `*` catch-all is lowest). So a user's specific override always beats their glob fallback, never the reverse."
> Spec *Terminal Identity & Detection → Config keys accepted: layered*: "**Friendly alias** (`ghostty`, `warp`) — Portal-shipped, for *known* terminals; maps to the bundle-id family. **`.app` name** / **raw bundle id** / **`*`-glob** — the escape hatch for custom/unknown terminals. … Internal **matching** stays on bundle-id families; user-facing keys are the friendlier forms."
> `Identity` carries `BundleID` (the raw/matched bundle id) and `Name` (friendly `.app` name), with `MatchesFamily(id.BundleID, familyGlob)` the Phase 1 family matcher (channel-aware, e.g. `dev.warp.Warp-Stable` matches `dev.warp.Warp-*`). Glob keys reuse that matcher, so config glob semantics equal native family semantics. This task returns only the **single** most-specific matching entry; its recipe's structural validity (Task 4.2) and the invalid→native fall-through are composed in Task 4.6 (per the spec: an invalid winning entry "falls through to native → unsupported"). A NULL identity never reaches `matchConfig` — the resolver short-circuits `id.IsNull()` before the config tier (Task 4.6), so a `*` catch-all never hijacks a remote/mosh no-op.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Config Schema → Precedence*; *Terminal Identity & Detection → Config keys accepted: layered / Identity resolution: macOS bundle id, matched as a family*.

---

## restore-host-terminal-windows-4-4 | approved

### Task 4.4: Config `argv` recipe adapter

**Problem**: A matched `argv` recipe must be turned into a working `spawn.Adapter` that opens a host window running the composed attach command. The `{command}` placeholder must be dropped into the recipe as **one literal, already-resolved string** (never shell-split into multiple argv elements), the surrounding argv elements must pass through verbatim, and the recipe's exit status must map into the generic typed `Result` — with config recipes **never** producing `permission-required` (they have no AppleEvent codes to read).

**Solution**: Add the `argvRecipeAdapter` to `internal/spawn/configadapter.go`, plus the shared recipe-execution primitives it and the script adapter (Task 4.5) reuse: a `recipeRunner` exec seam (identical shape to Phase 2's `osascriptRunner`, generically named because config recipes are not osascript-specific), the pure `substituteCommand` argv-template substitution, and `mapRecipeResult` (clean exit → `Success`, any non-clean → `SpawnFailed`, **no** permission branch). `OpenWindow` renders the composed argv to the `{command}` string (Task 4.2's `renderCommandString`), substitutes it into the template, runs the final argv through the seam, and maps the outcome.

**Outcome**: `argvRecipeAdapter{template, runner}.OpenWindow(command)` substitutes the space-joined `{command}` string into every template element containing the `{command}` token (as one literal string, never shell-split), passes all other elements through unchanged, runs the resulting argv through the `recipeRunner`, and returns `Success` on a clean exit or `SpawnFailed` (carrying the opaque combined output in `detail`) on any non-zero exit or execution error — never `PermissionRequired`. Fully unit-tested with a fake runner asserting the composed final argv + the result mapping; a real argv exec is covered by an integration-tagged test.

**Do**:
- Create `internal/spawn/configadapter.go` (shared config-execution infrastructure + the argv adapter; Task 4.5 adds the script adapter to the same file):
  - Runner seam: `type recipeRunner interface { Run(argv []string) (out string, exitCode int, err error) }` — `out` is combined stdout+stderr, `exitCode` the process status (0 on success), `err` a non-exit execution error (e.g. binary not found). Real impl `type execRecipeRunner struct{}` running `exec.Command(argv[0], argv[1:]...)` through `log.CombinedOutputWithContext` (the stderr-preserving boundary helper) and deriving `exitCode` from an `*exec.ExitError` (0 on success). This mirrors Phase 2's `execOsascriptRunner`; keep it a **separate, generically-named seam** (do not refactor the Phase 2 osascript seam — avoid churning approved code).
  - Pure substitution: `func substituteCommand(template []string, commandStr string) []string` — return a new slice where each element is `strings.ReplaceAll(el, "{command}", commandStr)`. This drops the composed command in as **one literal string** wherever `{command}` appears, keeping the element count fixed (never shell-splitting), and leaves elements without the token byte-for-byte unchanged.
  - Pure mapping: `func mapRecipeResult(out string, exitCode int, err error) Result` — `if err == nil && exitCode == 0 { return Success(strings.TrimSpace(out)) }`; else `return SpawnFailed(<opaque combined output / error text>)`. There is **no** `-1712`/`-1743` → `PermissionRequired` branch: a config recipe is a generic argv Portal cannot read AppleEvent codes from, so `permission-required` is structurally unreachable here (it stays native-adapter-only).
  - `type argvRecipeAdapter struct { template []string; runner recipeRunner }` implementing `Adapter`:
    - `func (a *argvRecipeAdapter) OpenWindow(command []string) Result { final := substituteCommand(a.template, renderCommandString(command)); out, code, err := a.runner.Run(final); return mapRecipeResult(out, code, err) }`.
  - (Constructor wiring — `matchConfig` winner + `RecipeArgv` → `&argvRecipeAdapter{recipe.Argv, r.runner}` — lives in the resolver, Task 4.6.)
- Add `internal/spawn/configadapter_argv_test.go` (unit lane, white-box `package spawn`):
  - Substitution: template `["osascript","-e","tell app \"Warp\" to create window with command \"{command}\""]` + a composed command containing spaces → the third element has `{command}` replaced by the whole command string as **one** element (three elements total, never four); an element without `{command}` is unchanged. Also cover a standalone-`{command}` element template (`["kitty","@","launch","{command}"]`) → the `{command}` element becomes exactly the one command string.
  - Result mapping via a fake `recipeRunner` (records the final argv, returns scripted `(out, exitCode, err)`): clean exit (`code=0,err=nil`) → `OutcomeSuccess`; non-zero exit (`code=1,out="…error…"`) → `OutcomeSpawnFailed` with `Detail` carrying the output; execution error (binary not found) → `OutcomeSpawnFailed` (never a panic). Assert the runner received exactly the substituted final argv.
  - Assert `mapRecipeResult` **never** returns `OutcomePermissionRequired` for any `(out, exitCode, err)` combination.
- Add `internal/spawn/configadapter_argv_integration_test.go` (`//go:build integration`): construct an `argvRecipeAdapter` whose template runs a trivial real program (e.g. `["/usr/bin/true","{command}"]` for success and `["/usr/bin/false","{command}"]` for a non-zero exit) via the real `execRecipeRunner`, and assert `OpenWindow` returns `Success` / `SpawnFailed` respectively — the real-exec inch, off the unit lane (no tmux, no daemon, no built portal binary).

**Acceptance Criteria**:
- [ ] `{command}` is substituted into each template element containing it as **one literal string** (never shell-split), keeping the element count fixed; a standalone-`{command}` element becomes exactly the composed command string.
- [ ] Template elements not containing `{command}` pass through byte-for-byte verbatim.
- [ ] `OpenWindow` runs exactly the substituted final argv through the runner (asserted via the fake runner's recorded argv).
- [ ] A clean recipe exit → `OutcomeSuccess`; a non-zero exit → `OutcomeSpawnFailed` with the opaque output in `Detail`; an execution error → `OutcomeSpawnFailed`.
- [ ] `mapRecipeResult` never returns `OutcomePermissionRequired` (config recipes never trigger the permission path).
- [ ] The integration-tagged test exercises a real argv exec and confirms the success / non-zero-exit mappings; the unit lane runs no real recipe.

**Tests**:
- `"it substitutes {command} as one literal element and leaves other elements verbatim"`
- `"it substitutes a standalone {command} element as the whole command string"`
- `"it runs the substituted final argv through the runner"`
- `"it maps a clean exit to success and a non-zero exit to spawn-failed with opaque detail"`
- `"it maps an execution error to spawn-failed"`
- `"it never returns permission-required from a config recipe"`
- `"integration: it opens via a real argv recipe and maps the exit status"` (`//go:build integration`)

**Edge Cases**:
- `{command}` dropped in as one literal string (never shell-split).
- Surrounding argv elements pass through verbatim.
- Non-zero recipe exit → `spawn-failed`.
- Config recipe never returns `permission-required`.

**Context**:
> Spec *Config Schema → Recipe: explicit fields*: "**`argv`** is an **argv array** (not a string), to sidestep shell-quoting hell. The inline field is named `argv` (not `command`) to avoid colliding with the placeholder."
> Spec *Config Schema → Recipe execution contract*: "**`{command}` substitutes as a single, already-resolved command string**, dropped literally into the recipe. Escaping it for an *embedding* context (e.g. inside an AppleScript string) is the recipe author's responsibility — they wrote that AppleScript. The `argv`-array form exists precisely so simple CLI terminals (kitty/wezterm) avoid quoting entirely. … **Recipe failure classification.** … a **non-zero exit** of the recipe process maps to `spawn-failed`; otherwise the window's fate is decided by the **ack** (timeout → failed). `permission-required` is **native-adapter-only** — config recipes never produce it."
> Spec *Spawn Architecture → env self-sufficiency*: "the adapter/recipe just runs the composed command (`{command}`) verbatim, a real **argv**, never shell syntax." — the composed argv is env-self-sufficient (`/usr/bin/env PATH=… <abs>/portal attach <session> --spawn-ack <batch>:<token>`, Tasks 2.3/3.5), so the recipe author never touches PATH/env. `renderCommandString` (Task 4.2) space-joins it into the `{command}` string, exactly as the native Ghostty `ghosttyEmbed` does — Portal session names are `{project}-{nanoid}` (no spaces); any space in a PATH/exe path is the recipe author's escaping responsibility for their embedding context (spec-stated), consistent with the native path.
> The `Success`/`SpawnFailed` constructors and the opaque `Detail` come from Phase 2's `Result` taxonomy; on a `Success` return the Phase 3 burster then awaits the token ack (a clean recipe exit means "the terminal accepted the request"; the ack decides the window's real fate).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Config Schema → Recipe: explicit fields / Placeholder `{command}` / Recipe execution contract*; *Spawn Architecture → Spawned-window environment (env self-sufficiency)*; *Permissions & Error Quarantine → Architectural boundary*.

---

## restore-host-terminal-windows-4-5 | approved

### Task 4.5: Config `script` recipe adapter

**Problem**: A matched `script` recipe points at a user file Portal must execute directly (it carries its own shebang + exec bit), delivering the composed attach command as `$1`. Portal must expand a leading `~` in the path, and a **missing or non-executable** script is an invalid entry that must be skipped with a WARN and fall through to native — never a runtime crash or a silent "unsupported." As with argv recipes, a non-zero exit maps to `spawn-failed` and a script recipe **never** produces `permission-required`.

**Solution**: Add the `scriptRecipeAdapter` and its validating constructor `newScriptRecipeAdapter(key, rawPath, runner)` to `internal/spawn/configadapter.go`. The constructor expands a leading `~` via `resolver.ExpandTilde`, stats the file, and rejects a missing or non-executable script (WARN naming the entry key, `ok=false` → fall through). `OpenWindow` execs the resolved script path with the composed command string as `$1`, reusing Task 4.4's `recipeRunner` seam and `mapRecipeResult`.

**Outcome**: `newScriptRecipeAdapter("com.example.MyTerm", "~/.config/portal/terminals/myterm.sh", runner)` expands `~`, and returns `(adapter, true)` for an existing executable file or `(nil, false)` after a single `spawn` WARN for a missing or non-executable path. The adapter's `OpenWindow(command)` runs `[resolvedScriptPath, renderCommandString(command)]` through the runner — delivering the composed command as `$1` — and maps the exit status (clean → `Success`, non-zero/exec-error → `SpawnFailed`, never `PermissionRequired`). Unit-tested with a fake runner + hermetic temp scripts; a real script exec is integration-tagged.

**Do**:
- In `internal/spawn/configadapter.go`:
  - `func newScriptRecipeAdapter(key, rawPath string, runner recipeRunner) (Adapter, bool)`:
    1. `p := resolver.ExpandTilde(rawPath)` — the single-source-of-truth leading-`~` expansion (`internal/resolver`; no import cycle, resolver does not import spawn).
    2. `info, err := os.Stat(p)`; on `err` (ENOENT or otherwise) → `logger.Warn("terminals.json entry rejected", "detail", fmt.Sprintf("%q: script %q not found: %v", key, p, err)); return nil, false`.
    3. Reject a directory or a file with no exec bit: `if info.IsDir() || info.Mode().Perm()&0o111 == 0 { logger.Warn("terminals.json entry rejected", "detail", fmt.Sprintf("%q: script %q is not executable", key, p)); return nil, false }` (per spec, the escape-hatch script must carry its own exec bit + shebang — Portal executes it directly, not via `sh <path>`).
    4. `return &scriptRecipeAdapter{scriptPath: p, runner: runner}, true`.
  - `type scriptRecipeAdapter struct { scriptPath string; runner recipeRunner }` implementing `Adapter`:
    - `func (a *scriptRecipeAdapter) OpenWindow(command []string) Result { final := []string{a.scriptPath, renderCommandString(command)}; out, code, err := a.runner.Run(final); return mapRecipeResult(out, code, err) }` — the script path is `argv[0]` and the composed command string is `$1` (`argv[1]`); the runner execs the file directly, so the shebang + exec bit apply. `mapRecipeResult` (Task 4.4) never yields `PermissionRequired`.
  - (Resolver wiring — `matchConfig` winner + `RecipeScript` → `newScriptRecipeAdapter(key, recipe.Script, r.runner)`, with `ok=false` treated as a native fall-through — lives in Task 4.6.)
- Add `internal/spawn/configadapter_script_test.go` (unit lane, white-box `package spawn`):
  - `~` expansion: `t.Setenv("HOME", tmpHome)`, create an executable script at `tmpHome/s.sh` (`0o755`), construct via `newScriptRecipeAdapter(key, "~/s.sh", fakeRunner)`; assert `ok=true` and that `OpenWindow` runs `[<tmpHome>/s.sh, <commandString>]` (the recorded final argv proves both the `~` expansion of `argv[0]` and the `$1` delivery).
  - `$1` delivery: assert the recorded `argv[1]` equals `renderCommandString(command)` — the composed command as a single positional arg.
  - Missing script → `newScriptRecipeAdapter` returns `(nil, false)` + exactly one `spawn` WARN naming the key.
  - Non-executable script → create a `0o644` file, assert `(nil, false)` + one WARN.
  - Result mapping via the fake runner: clean exit → `Success`; non-zero exit → `SpawnFailed` (opaque `Detail`); exec error → `SpawnFailed`. Assert no case yields `PermissionRequired`.
- Add `internal/spawn/configadapter_script_integration_test.go` (`//go:build integration`): write a real executable shebang script into `t.TempDir()` that exits 0 (and a sibling that exits non-zero), point a `scriptRecipeAdapter` (real `execRecipeRunner`) at each, and assert `OpenWindow` returns `Success` / `SpawnFailed` and that the script observed its `$1` — the real-exec inch, off the unit lane.

**Acceptance Criteria**:
- [ ] A leading `~` in the script path is expanded via `resolver.ExpandTilde` before the file is stat'd and executed.
- [ ] `OpenWindow` execs the resolved script path with the composed command string as `$1` (a single positional argument), asserted via the recorded final argv.
- [ ] A missing script path → `newScriptRecipeAdapter` returns `(nil, false)` after one `spawn` WARN naming the entry key (the resolver falls through to native).
- [ ] A non-executable script (no exec bit) → `(nil, false)` after one `spawn` WARN.
- [ ] A clean exit → `Success`; a non-zero exit → `SpawnFailed` (opaque `Detail`); an exec error → `SpawnFailed`; never `PermissionRequired`.
- [ ] The integration-tagged test execs a real shebang script and confirms the success / non-zero mappings + `$1`; the unit lane execs no real script.

**Tests**:
- `"it expands a leading ~ in the script path"`
- `"it delivers the composed command as $1 to the script"`
- `"it skips a missing script with a WARN and no adapter"`
- `"it skips a non-executable script with a WARN and no adapter"`
- `"it maps a clean exit to success and a non-zero exit to spawn-failed"`
- `"it never returns permission-required from a script recipe"`
- `"integration: it execs a real shebang script and maps the exit status"` (`//go:build integration`)

**Edge Cases**:
- Leading `~` expanded in the script path.
- `{command}` delivered as `$1`.
- Missing script → invalid + WARN (falls through).
- Non-executable / no exec bit → invalid + WARN (falls through).
- Non-zero exit → `spawn-failed`.
- Never `permission-required`.

**Context**:
> Spec *Config Schema → Recipe execution contract*: "**`script` recipes receive `{command}` as `$1`** (first positional arg). Portal **expands a leading `~`** in the script path and **executes the file directly** (it must carry its exec bit + shebang — the standard for an executable escape-hatch script); a missing / non-executable script is an invalid entry (skipped + WARN, per *Validation & error handling*). … a **non-zero exit** of the recipe process maps to `spawn-failed` … `permission-required` is **native-adapter-only** — config recipes never produce it."
> Spec *Config Schema → Recipe: explicit fields, not magic*: "**`script`** is a path to a file Portal executes." Chosen over auto-detecting "is it a path on disk?" — the field name declares intent, so Portal executes the file directly.
> `resolver.ExpandTilde` (`internal/resolver/path.go`) is the single source of truth for leading-`~` expansion (reused by `project.CanonicalDirKey`); `internal/spawn` may import it (resolver has no dependency on spawn, so no cycle). The `recipeRunner` seam + `mapRecipeResult` + `renderCommandString` are the shared primitives from Tasks 4.4/4.2. The missing/non-executable check is a resolution-time validity gate (an invalid entry falls through to native, Task 4.6), which is why it lives in the constructor and emits the WARN there — distinct from the *structural* validation in Task 4.2 (that check needs no filesystem access; this one does).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Config Schema → Recipe: explicit fields, not magic / Recipe execution contract / Validation & error handling*; *Permissions & Error Quarantine → Architectural boundary*.

---

## restore-host-terminal-windows-4-6 | approved

### Task 4.6: Wire the config tier into the resolver + resolution observability

**Problem**: The config store (4.1), validation (4.2), matching (4.3), and the argv/script adapters (4.4/4.5) exist independently, but the resolver still resolves only `native → unsupported` (Phase 2's placeholder). The pieces must be composed into the spec's full precedence — **config override → native → unsupported** — with config resolving **ahead of** a would-match native, an invalid/absent config entry falling through to native then unsupported, and the resolution outcome (`config` vs `native` vs `unsupported`) logged. The `terminals.json` path must resolve through the existing XDG `configFilePath` chain, and the whole tier must stay read-only with **no** `sessions.json`/daemon/prefs/restore interaction.

**Solution**: Convert Phase 2's free `spawn.ResolveAdapter` into a `spawn.Resolver` (holding the loaded `TerminalsConfig` + the recipe runner) whose `Resolve(id)` short-circuits `IsNull` → unsupported, then tries the config tier (match → validate → build argv/script adapter, `ResolutionConfig`) **before** the native registry (`ResolutionNative`), falling through to `unsupported` when nothing matches. Wire `cmd/spawn.go` to resolve the `terminals.json` path via `configFilePath("PORTAL_TERMINALS_FILE", "terminals.json")`, load it once through `TerminalsStore`, construct the config-aware `Resolver`, and use its `Resolve` as the `spawnDeps.Resolve` default — so the existing Phase 2/3 batch-summary emission logs `resolution=config` for a config match with no new logging code.

**Outcome**: For an identity that matches a valid `terminals.json` entry, `Resolver.Resolve` returns the config adapter + `ResolutionConfig` **even when that same identity would also match the native Ghostty adapter** (config ahead of native). A no-match, an invalid recipe (structural or missing/non-exec script), or an entry with no `open` capability falls through to native (`ResolutionNative`) and then to `unsupported` (`nil, ResolutionUnsupported`) for a NULL/unmatched identity. `portal spawn` on a config-matched terminal logs the batch summary with `resolution=config`. The tier reads only `terminals.json` and never touches `sessions.json`, the daemon capture loop, `prefs.json`, or restore. Unit-tested at the resolver boundary (config-ahead-of-native + the fall-throughs) and at the cmd boundary (the `resolution=config` log attr).

**Do**:
- In `internal/spawn/resolver.go` (refactoring Phase 2's `ResolveAdapter`):
  - `type Resolver struct { Config TerminalsConfig; runner recipeRunner }` and `func NewResolver(cfg TerminalsConfig) *Resolver { return &Resolver{Config: cfg, runner: &execRecipeRunner{}} }` (production runner from Task 4.4; white-box tests set `runner` to a fake).
  - `func (r *Resolver) Resolve(id Identity) (Adapter, Resolution)`:
    1. `if id.IsNull() { return nil, ResolutionUnsupported }` — remote/mosh / no host-local terminal; the config tier (incl. any `*` catch-all) is **not** consulted, so a NULL never resolves to a config adapter.
    2. Config tier: `if a, ok := r.resolveConfig(id); ok { return a, ResolutionConfig }`.
    3. Native tier: iterate the Phase 2 native registry — `if MatchesFamily(id.BundleID, entry.family) { return entry.build(), ResolutionNative }` (relocate the Phase 2 registry loop here unchanged).
    4. `return nil, ResolutionUnsupported`.
  - `func (r *Resolver) resolveConfig(id Identity) (Adapter, bool)`:
    - `key, entry, ok := matchConfig(r.Config, id)` (Task 4.3); `if !ok { return nil, false }`.
    - `recipe, kind, valid := validRecipeForEntry(key, entry)` (Task 4.2 — emits the WARN on a structurally-invalid recipe, silent for a no-`open` entry); `if !valid { return nil, false }` (fall through to native).
    - `switch kind { case RecipeArgv: return &argvRecipeAdapter{template: recipe.Argv, runner: r.runner}, true; case RecipeScript: return newScriptRecipeAdapter(key, recipe.Script, r.runner) }` (Task 4.5's constructor returns `(nil,false)` + WARN for a missing/non-exec script → native fall-through). `default: return nil, false`.
  - Keep `func ResolveAdapter(id Identity) (Adapter, Resolution) { return NewResolver(TerminalsConfig{}).Resolve(id) }` as a thin zero-config wrapper so any Phase 1/2 caller keeps its native→unsupported behaviour (empty config → `resolveConfig` never matches).
  - Note the spec-literal fall-through: `matchConfig` returns the **single** most-specific matching entry; if it is invalid, resolution falls through to **native** (not to a less-specific config entry) — matching the spec's "that entry is invalid and skipped (falls through to native → unsupported)" and this task's "no/invalid config entry falls through to native then unsupported."
- In `cmd/spawn.go` (Phase 2/3 pipeline wiring):
  - Where `spawnDeps.Resolve` defaults are set (the `buildSpawnDeps`/deps-init path), resolve the config path and load it once: `path, err := configFilePath("PORTAL_TERMINALS_FILE", "terminals.json")`; on `err` (rare — undeterminable home/XDG) degrade to an **empty** config (native-only resolution) rather than aborting the command — the escape hatch fails safe. Then `cfg := spawn.NewTerminalsStore(path).Load()` and `resolver := spawn.NewResolver(cfg)`; set the default `Resolve` seam to `resolver.Resolve`. The seam signature `func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` is unchanged, so injected-fake tests are unaffected.
  - The batch-summary emission already logs the `resolution` attr from the returned `Resolution` (Phase 2 Task 2.6 / Phase 3 Task 3.5); `ResolutionConfig`'s string value is `"config"`, so a config match logs `resolution=config` with **no** new emission code. Confirm the value flows through (do not add a second emission).
- In `cmd/config.go`: add `"terminals.json": ""` to `configFileComponents` with a one-line comment that `terminals.json` is **read-only** (not part of the state-mutation audit-trail set) and has **no** old-macOS-path predecessor, so the one-shot migrate is a guaranteed no-op and its breadcrumb is suppressed (mirrors the `prefs.json` precedent). (Leaving it unmapped resolves to `""` by default, but the explicit entry documents intent.)
- Tests:
  - `internal/spawn/resolver_config_test.go` (unit lane, white-box `package spawn`) — set `Resolver.runner` to a fake:
    - Config-ahead-of-native: a `com.mitchellh.ghostty` identity (which the native registry matches) with a config entry keyed `com.mitchellh.ghostty*` (valid argv recipe) → `Resolve` returns the config adapter (type-assert `*argvRecipeAdapter`) + `ResolutionConfig`.
    - No config: empty config, Ghostty identity → `(*ghosttyAdapter, ResolutionNative)`.
    - Invalid config → native: a matching entry with an invalid recipe (both argv+script) for a Ghostty identity → `(*ghosttyAdapter, ResolutionNative)` + a captured WARN.
    - Unmatched config, unknown identity → `(nil, ResolutionUnsupported)`.
    - NULL identity with a `*` catch-all config entry present → `(nil, ResolutionUnsupported)` (config tier skipped for NULL).
    - A valid `script` config entry (executable temp file) → `(*scriptRecipeAdapter, ResolutionConfig)`.
  - `cmd/spawn_test.go` (unit lane, `package cmd`, no `t.Parallel()`): inject `spawnDeps.Resolve` returning `(*spawntest.FakeAdapter, spawn.ResolutionConfig)` (fully hermetic — no real recipe exec) with the fake adapter confirming, and assert the emitted batch summary carries `resolution=config` (via a `logtest.Sink`); inject every other tmux-touching `*Deps` seam (cmd `TestMain` poisons `TMUX`, so a missed injection fails loudly).

**Acceptance Criteria**:
- [ ] For an identity matching a valid config entry that **also** matches native Ghostty, `Resolve` returns the config adapter + `ResolutionConfig` (config ahead of native).
- [ ] With no config entry, a Ghostty identity resolves to the native adapter + `ResolutionNative`; an unmatched/unknown identity resolves to `(nil, ResolutionUnsupported)`.
- [ ] An invalid config recipe (structural, or a missing/non-executable script) for a native-matching identity falls through to `ResolutionNative` (with the Task 4.2/4.5 WARN emitted); for a non-native identity it falls through to `ResolutionUnsupported`.
- [ ] A NULL identity returns `(nil, ResolutionUnsupported)` even when a `*` catch-all config entry exists (config tier skipped for NULL).
- [ ] `portal spawn` on a config-matched terminal logs the batch summary with `resolution=config` (no new emission code — the value flows through the Phase 2/3 summary).
- [ ] `terminals.json` resolves via `configFilePath("PORTAL_TERMINALS_FILE", "terminals.json")` (XDG chain), is loaded once, and the tier imports none of `internal/state` (sessions.json), the daemon, `prefs`, or restore — no `sessions.json`/daemon/prefs/restore interaction.

**Tests**:
- `"it resolves a matching config entry ahead of a would-match native adapter"`
- `"it falls through to native when there is no config entry"`
- `"it falls through past an invalid config entry to native then unsupported"`
- `"it returns unsupported for a NULL identity even with a catch-all config entry"`
- `"it resolves a valid script config entry to the script adapter with resolution=config"`
- `"it logs resolution=config for a config-resolved batch summary"`

**Edge Cases**:
- Config match returns the config adapter ahead of a would-match native.
- No/invalid config entry falls through to native then unsupported.
- `resolution=config` vs `native` (vs `unsupported`) logged.
- NULL identity skips the config tier (no `*` catch-all hijack).
- No `sessions.json`/daemon/prefs/restore interaction (read-only `terminals.json` + the Phase 3 transient markers only).

**Context**:
> Spec *Adapter Contract & Extensibility → Resolution precedence*: "**config override → native adapter → unsupported.** Config can override a built-in too (e.g. Ghostty + a resize). A NULL/unmatched identity → unsupported."
> Spec *Config Schema → Precedence*: "config override → native adapter → unsupported. Config can override a built-in." + within-config most-specific (Task 4.3).
> Spec *Observability & State Footprint → Attr keys*: `resolution` is a closed attr with values `config | native | unsupported`; the `Resolution` string values already match, so a config match logs directly. *State/daemon footprint*: "**Reads** `terminals.json` … **Does not touch** `sessions.json`, the daemon capture loop, `prefs.json`, or the restore machinery." — the config tier honours this: it imports only stdlib + `internal/log` + `internal/resolver` (ExpandTilde) + the spawn package's own types.
> Phase 2 shipped `ResolveAdapter(id)` with a commented Phase-4 placeholder ahead of the native registry; this task fills that placeholder by promoting the resolver to a config-holding struct. The `Resolve` seam in `cmd/spawn.go` keeps its signature, so the detection→resolve→spawn→self-attach pipeline and all injected-fake tests are untouched. `configFilePath`/`configFileComponents` (`cmd/config.go`) already handle the XDG chain + the (no-op) migrate; `PORTAL_TERMINALS_FILE` follows the existing per-file env-var naming (`PORTAL_HOOKS_FILE`, `PORTAL_PROJECTS_FILE`, …).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Adapter Contract & Extensibility → Resolution precedence*; *Config Schema → Precedence / Location & format*; *Observability & State Footprint → Attr keys (`resolution`) / State/daemon footprint*.
