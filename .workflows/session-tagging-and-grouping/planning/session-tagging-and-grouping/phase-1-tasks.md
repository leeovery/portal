---
phase: 1
phase_name: Tag data model & session→directory resolution
total: 8
---

## session-tagging-and-grouping-1-1 | approved

### Task session-tagging-and-grouping-1-1: Add `tags []string` field to Project record

**Problem**: The feature stores tags on the project record, but the `Project` struct in `internal/project/store.go` carries only `{path, name, last_used}` — there is nowhere to persist a directory's tags, and existing `projects.json` files predate any tags field.

**Solution**: Add a `Tags []string` field to the `Project` struct with the JSON key `tags`, marked `omitempty` so the on-disk shape stays clean for un-tagged projects, and confirm via round-trip tests that an existing `projects.json` lacking the field decodes to the zero-tag state with no migration step.

**Outcome**: `Project` carries `Tags []string`; loading a legacy record (no `tags` key, or an explicit `null`, or `[]`) yields an empty/nil `Tags` slice; saving and reloading a record with tags round-trips the exact slice; un-tagged projects continue to behave exactly as today.

**Do**:
- In `/Users/leeovery/Code/portal/internal/project/store.go`, add `Tags []string \`json:"tags,omitempty"\`` to the `Project` struct (after `LastUsed`).
- Do NOT add a migration step, version field, or backfill — a missing `tags` key already decodes to nil via `encoding/json`.
- Confirm `Load` (which already tolerates malformed JSON by returning `[]Project{}`) needs no change; the new field decodes inline.
- Confirm `Save`/`Upsert`/`Rename`/`Remove`/`CleanStale` continue to compile and preserve the `Tags` slice — none of them touch tags yet, but `Upsert`'s found-branch must NOT clobber `Tags` (it currently only updates `Name`/`LastUsed`, which is correct — verify it leaves `Tags` intact).

**Acceptance Criteria**:
- [ ] `Project` struct has a `Tags []string` field with json tag `tags,omitempty`.
- [ ] A `projects.json` whose project objects omit the `tags` key decodes with `Tags == nil` (len 0) and no error.
- [ ] A `projects.json` with `"tags": null` and one with `"tags": []` both decode to a zero-length `Tags`.
- [ ] A record saved with `Tags: ["work","personal"]` reloads with exactly that slice in order.
- [ ] `Upsert` on an existing path updates `Name`/`LastUsed` without dropping a previously-stored `Tags` slice.
- [ ] No migration step is introduced.

**Tests**:
- `"it decodes a legacy record with no tags field to an empty Tags slice"`
- `"it decodes an explicit null tags value to an empty Tags slice"`
- `"it decodes an explicit [] tags value to an empty Tags slice"`
- `"it round-trips a record with multiple tags unchanged"`
- `"it preserves Tags when Upsert updates an existing project's name and last_used"`

**Edge Cases**:
- Missing `tags` field decodes to nil/empty (no migration) — the canonical zero-tag state.
- `null` vs `[]` in JSON both collapse to a zero-length slice.
- `Upsert`'s in-place update of an existing project must not zero an existing `Tags` slice (it only writes `Name` and `LastUsed`).

**Context**:
> Tags are stored on the project record in `~/.config/portal/projects.json`. The existing `Project` record (`{path, name, last_used}`) gains a `tags []string` field. Reuses the existing JSON store machinery: `AtomicWrite` and `configFilePath`. No new store, no new persistence pattern.
> Existing `projects.json` records predate the `tags` field. A missing `tags` field decodes to nil/empty — exactly the zero-tag state. No migration is required; un-tagged projects behave as today.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Tag Data Model & Persistence → Storage, Back-compatibility)

## session-tagging-and-grouping-1-2 | approved

### Task session-tagging-and-grouping-1-2: Tag value normalisation helper (trim + lower-case + reject empty)

**Problem**: A tag value is also the grouping key (a By-Tag heading and the implicit-union dedup key), so its canonical form is load-bearing and must be defined in exactly one place — otherwise dedup, the cross-project union, and By-Tag grouping could each apply slightly different rules and silently split or merge groups.

**Solution**: Add a single normalisation helper in `internal/project` that trims leading/trailing whitespace, lower-cases, and signals whether the result is empty (rejected). This is the sole canonical-form function; every later tag comparison (dedup, union, grouping key) calls it.

**Outcome**: A function such as `NormaliseTag(raw string) (canonical string, ok bool)` returns the trimmed + lower-cased value with `ok == true` for any input that has non-whitespace content, and `("", false)` for empty/whitespace-only input; internal whitespace is preserved; case folds to lower.

**Do**:
- Add the helper to `/Users/leeovery/Code/portal/internal/project/` (e.g. a new `tags.go` file in package `project`), signature `func NormaliseTag(raw string) (string, bool)`.
- Implementation: `trimmed := strings.TrimSpace(raw)`; if `trimmed == ""` return `("", false)`; otherwise return `(strings.ToLower(trimmed), true)`.
- Use `strings.ToLower` (ASCII + Unicode simple lower-casing) — there is no character whitelist and no length cap in v1, so do not add validation beyond trim + non-empty.
- Export it from package `project` so the per-project set logic (task 1-3) and later grouping phases can reuse the one canonical form. Do not duplicate trim/lower logic at any call site.

**Acceptance Criteria**:
- [ ] `NormaliseTag("  work ")` returns `("work", true)`.
- [ ] `NormaliseTag("Work")`, `NormaliseTag("WORK")`, `NormaliseTag("work")` all return `("work", true)`.
- [ ] `NormaliseTag("")` and `NormaliseTag("   ")` (whitespace-only) return `("", false)`.
- [ ] `NormaliseTag("client a")` returns `("client a", true)` — internal whitespace preserved, only edges trimmed.
- [ ] No other code path re-implements trim/lower-case for tags.

**Tests**:
- `"it trims leading and trailing whitespace"`
- `"it lower-cases mixed and upper case"`
- `"it rejects the empty string"`
- `"it rejects whitespace-only input"`
- `"it preserves internal whitespace"`

**Edge Cases**:
- Leading/trailing whitespace trimmed; whitespace-only rejected; empty string rejected.
- Mixed/upper case collapses to lower.
- Internal whitespace preserved (only edges trimmed).

**Context**:
> Whitespace: leading/trailing whitespace is trimmed on add. `"  work "` is stored as `work`. Empty / whitespace-only: rejected — a no-op. Case: the canonical form is lower-cased; `Work`, `WORK`, and `work` are the same tag. No character whitelist and no hard max length in v1. The same canonical form (trim + lower-case) is used everywhere a tag is compared: per-project dedup, the cross-project union, and By-Tag grouping.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Tag value normalisation & validation)

## session-tagging-and-grouping-1-3 | approved

### Task session-tagging-and-grouping-1-3: Per-project tag set add/remove (normalised, deduped, persisted)

**Problem**: The projects edit modal (Phase 4) needs to add and remove a tag on a known project, but no store method exists to mutate a project's `Tags` as a normalised, deduped set keyed by project path, persisted via the existing atomic-write machinery.

**Solution**: Add `AddTag(path, rawTag string) error` and `RemoveTag(path, rawTag string) error` methods on `*project.Store`. `AddTag` normalises via `NormaliseTag`, rejects empty as a no-op, dedups against the existing canonical set, appends if new, and persists via `Save`. `RemoveTag` normalises and removes the matching canonical entry, persisting only the change.

**Outcome**: Adding `"  Work "` to a project stores `work`; adding `"WORK"` again is a persisted no-op (set unchanged, no duplicate); removing a tag that is present removes it; removing an absent tag is a no-op; adding a blank/whitespace-only value is a no-op; operating on a path that is not a known project returns a clear "not found" signal without writing.

**Do**:
- Add `func (s *Store) AddTag(path, rawTag string) error` and `func (s *Store) RemoveTag(path, rawTag string) error` to `/Users/leeovery/Code/portal/internal/project/store.go` (or the new `tags.go`).
- Both: `Load()` the slice, find the project by exact `Path` match (same match key Upsert/Rename/Remove use). If not found, return a sentinel error (e.g. `var ErrProjectNotFound = errors.New("project not found")`) and do NOT Save.
- `AddTag`: call `NormaliseTag(rawTag)`; if `!ok` return nil (no-op, no Save — pressing Enter on blank input adds nothing). If the canonical tag already exists in the project's `Tags` (compare against each existing entry — entries are already canonical, but normalise defensively), return nil without Save (dedup no-op). Otherwise append the canonical tag and `Save`.
- `RemoveTag`: call `NormaliseTag(rawTag)`; if `!ok` return nil. Remove the canonical entry from `Tags` (e.g. `slices.DeleteFunc`). If nothing changed (tag absent), return nil without Save. Otherwise `Save`.
- Persist via the existing `Save` (which routes through `AtomicWrite`). Do not invent a new write path.
- Keep the existing `Upsert` audit-breadcrumb style optional here — match the surrounding convention if a breadcrumb is warranted, but the spec does not require a new log catalog entry; prefer minimal/no new log surface unless a breadcrumb is the existing norm for mutations (it is — see Upsert/Rename). If adding breadcrumbs, reuse the `projects` component logger and an existing op vocabulary value; do NOT invent new closed-vocabulary attr keys without spec amendment — if uncertain, omit the breadcrumb and note it.

**Acceptance Criteria**:
- [ ] `AddTag(path, "  Work ")` on a known project stores `work` (trimmed, lower-cased) and persists.
- [ ] Calling `AddTag(path, "WORK")` after the project already carries `work` is a no-op: the set is unchanged and no duplicate is written.
- [ ] `AddTag(path, "   ")` (blank/whitespace-only) is a no-op — no tag added, returns nil.
- [ ] `RemoveTag(path, "Work")` removes a present `work` entry and persists.
- [ ] `RemoveTag(path, "absent")` is a no-op (no error, set unchanged).
- [ ] `AddTag`/`RemoveTag` on an unknown path return `ErrProjectNotFound` and do not write the file.
- [ ] The persisted set is deduped and stored in canonical (lower-cased, trimmed) form.

**Tests**:
- `"it adds a normalised tag to a project and persists it"`
- `"it is a no-op when adding a tag the project already carries after normalisation"`
- `"it rejects a blank or whitespace-only add as a no-op"`
- `"it removes a present tag and persists"`
- `"it is a no-op when removing an absent tag"`
- `"it returns ErrProjectNotFound for an unknown path without writing"`

**Edge Cases**:
- Duplicate-after-normalisation add is a no-op (`work` already present, adding `WORK`).
- Removing an absent tag is a no-op.
- Blank/whitespace-only add rejected.
- Project path not found returns a sentinel error without a Save.

**Context**:
> Adding a tag a project already carries (after normalisation) is a no-op — `tags` is a deduped set per project. Pressing Enter on a blank (or whitespace-only) input is a no-op. The projects edit modal lists known projects only; every session creation upserts a project keyed by git-root. Reuses `AtomicWrite` and `configFilePath` — no new store.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Tag value normalisation & validation, § Taggable surface — projects only)

## session-tagging-and-grouping-1-4 | approved

### Task session-tagging-and-grouping-1-4: Canonical directory path key for dir→project lookup

**Problem**: The grouped render looks up a directory's `Project` record (and thus its tags) by path, so the render-time lookup key must match the stored `Project.Path` byte-for-byte. The stamped `@portal-dir` value and the fallback-derived git-root must be normalised to the same canonical form the project store uses, accounting for symlinks, trailing slash, and `~` expansion — a mismatch would silently drop a session out of its group.

**Solution**: Add a canonicalisation helper that produces the lookup key from any directory path, applying `~` expansion, absolute resolution, symlink evaluation, and trailing-slash/clean normalisation, and a lookup that finds a `Project` by canonical-path match. Confirm the same canonical form is what `Project.Path` already holds (git-root from `git rev-parse --show-toplevel`).

**Outcome**: Given a stamped or derived directory path in any reasonable form (`~/code/portal`, `/code/portal/`, a symlinked path), the helper returns a single canonical key; a lookup by that key returns the matching `Project` when one exists and a "not found" result otherwise.

**Do**:
- Add `func CanonicalDirKey(path string) string` (e.g. in `internal/project/tags.go` or a new `internal/project/pathkey.go`). Steps: expand `~` (reuse the logic in `internal/resolver/path.go` `expandTilde`, or factor a shared helper — do NOT duplicate tilde logic if a shared one can be exposed), then `filepath.Abs`, then `filepath.EvalSymlinks` (NOTE: `resolver.NormalisePath` does NOT currently EvalSymlinks — this helper must, to match symlinked paths; if `EvalSymlinks` errors because the path does not exist, fall back to the `filepath.Clean(abs)` value so a missing dir still yields a stable key), then `filepath.Clean` (strips trailing slash, collapses `.`/`..`).
- Add a lookup, e.g. `func MatchProjectByDir(projects []Project, dirPath string) (Project, bool)`, that compares `CanonicalDirKey(dirPath)` against `CanonicalDirKey(p.Path)` for each project (canonicalising the stored path too, so a stored path that was written before symlink-eval still matches).
- CRITICAL build-time verification: write a test that a directory resolved through `ResolveGitRoot` (the source of `Project.Path` at upsert time) produces a `CanonicalDirKey` equal to the key derived from the same directory passed through the stamp/fallback path. This is the spec's "confirm the lookup key matches stored `Project.Path`" requirement — a regression here silently drops sessions from groups.
- A path that is not a known project must return `(Project{}, false)` — the caller routes it to the Unknown/Untagged bucket in Phase 2.

**Acceptance Criteria**:
- [ ] `CanonicalDirKey("~/code/portal")` expands `~` and returns an absolute, symlink-resolved, cleaned path.
- [ ] A path with a trailing slash (`/x/y/`) and the same path without (`/x/y`) produce the identical key.
- [ ] A symlinked path and its real target produce the identical key (when both exist on disk).
- [ ] A relative path is resolved to absolute before keying.
- [ ] `MatchProjectByDir` returns `(_, false)` for a path with no matching project record.
- [ ] A directory's `CanonicalDirKey` equals the canonical key of the `Project.Path` that `ResolveGitRoot` produced for the same directory (the lookup-key-matches-stored-path invariant).

**Tests**:
- `"it expands a leading tilde to the home directory"`
- `"it produces the same key for a path with and without a trailing slash"`
- `"it resolves a symlinked path to the same key as its real target"`
- `"it resolves a relative path to absolute"`
- `"it returns not-found when the path matches no project"`
- `"it produces a lookup key equal to the stored Project.Path canonical form for the same git root"`

**Edge Cases**:
- Symlinked path resolves to the same key as its target (EvalSymlinks).
- Trailing slash normalised away.
- `~` home expansion.
- Path not a known project → not-found (routes to Unknown/Untagged later).
- Relative path resolved to absolute.
- Non-existent path: EvalSymlinks fails — fall back to `Clean(Abs())` so a stable key still results (a deleted-project / dead-dir session must still key consistently).

**Context**:
> The dir→tags lookup keys on a directory path, so the render-time lookup key must match the stored `Project.Path` exactly. Both the stamped `@portal-dir` value and the fallback-derived git-root must be normalised to the same canonical form the project store uses, accounting for: symlinks, trailing slash, `~` expansion. Implementation must confirm the lookup key matches stored `Project.Path` for the same directory; a mismatch would silently drop a session out of its group.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Path-keying canonicalisation (build-time requirement))

## session-tagging-and-grouping-1-5 | approved

### Task session-tagging-and-grouping-1-5: Stamp `@portal-dir` at session creation

**Problem**: Grouping needs to map a live session back to its directory, but a session name (`{project}-{nanoid}`, freely renamed) cannot do this. The fast path is a tmux session user-option `@portal-dir = <resolvedDir>` stamped at creation, which currently does not exist.

**Solution**: After a session is created in the `SessionCreator.CreateFromDir` path, stamp `@portal-dir` to the already-computed `PreparedSession.ResolvedDir` via `SetSessionOption`, best-effort (a failure must not fail session creation). Define `@portal-dir` as a named constant. Handle the `QuickStart` exec-handoff path explicitly as an edge case (it cannot call `SetSessionOption` because `syscall.Exec` replaces the process — the lazy fallback in tasks 1-7/1-8 covers it).

**Outcome**: A session created via `SessionCreator.CreateFromDir` carries `@portal-dir = <resolvedDir>` immediately after creation; the stamp rides the session object (survives rename and pane `cd`); a `SetSessionOption` failure logs/continues without failing creation; the `QuickStart` path is documented as relying on the lazy fallback (no stamp at create).

**Do**:
- Define the option name constant, e.g. `const PortalDirOption = "@portal-dir"`, in the package that owns it (prefer `internal/session` or alongside other portal option names; pick one home and reference it — do NOT hard-code the literal in multiple files).
- Extend the `session.TmuxClient` interface (in `/Users/leeovery/Code/portal/internal/session/create.go`) to add `SetSessionOption(session, name, value string) error` (the concrete `*tmux.Client` already implements it — verify signature in `internal/tmux/tmux.go:366`).
- In `SessionCreator.CreateFromDir` (`create.go`), after the successful `sc.tmux.NewSession(...)`, call `sc.tmux.SetSessionOption(prepared.SessionName, PortalDirOption, prepared.ResolvedDir)`. On error, do NOT return it — swallow/log best-effort and still return the session name (creation succeeded).
- Use `prepared.ResolvedDir` (the git-root already computed in `PrepareSession`) as the stamp value — no new `git rev-parse`.
- For `QuickStart.Run` (`quickstart.go`): it builds `tmux new-session -A` exec args for `syscall.Exec`, so it cannot stamp after creation in-process. Add a doc comment noting the stamp is deliberately omitted here and the lazy stamp-on-render fallback (task 1-7/1-8) covers QuickStart-created sessions on first grouped render. Do NOT attempt to inject `set-option` into the exec args (the `-A` create-or-attach handoff replaces the process).

**Acceptance Criteria**:
- [ ] A constant names the `@portal-dir` option in one place; no bare literal duplicated across files.
- [ ] `SessionCreator.CreateFromDir` calls `SetSessionOption(sessionName, "@portal-dir", resolvedDir)` after `NewSession` succeeds, using the git-root already computed.
- [ ] A `SetSessionOption` failure does not cause `CreateFromDir` to return an error — the session name is still returned.
- [ ] The stamp value is the resolved git-root, not a live pane cwd.
- [ ] `QuickStart.Run` is documented as relying on the lazy fallback (no create-time stamp), with no `set-option` injected into the exec handoff.

**Tests**:
- `"it stamps @portal-dir with the resolved git root after creating a session"`
- `"it returns the session name even when SetSessionOption fails"`
- `"it does not stamp at creation in the QuickStart exec-handoff path"` (assert the QuickStart exec args contain no set-option and document the fallback)
- `"it stamps using the prepared resolved dir, not a re-derived path"`

**Edge Cases**:
- Stamp rides the session object, not its name — survives rename (no re-stamp on rename).
- QuickStart exec-handoff path cannot stamp in-process; covered by the lazy fallback.
- `SetSessionOption` failure is non-fatal — session creation still succeeds.

**Context**:
> At session creation, Portal stamps the tmux session user-option `@portal-dir = <resolvedDir>`, where `<resolvedDir>` is the git-root already computed in `session.PrepareSession`. Survives rename (rides the session object). Survives pane `cd` (stamped once at create). Cheap to read (append `#{@portal-dir}` to the list-sessions format — no `git rev-parse` per render).

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ Session → Directory Resolution → The stamp (fast path))

## session-tagging-and-grouping-1-6 | approved

### Task session-tagging-and-grouping-1-6: Expose `@portal-dir` via ListSessions (Session.Dir)

**Problem**: The grouped render reads each session's stamped directory in the same `list-sessions -F` pass that already fetches names, but `tmux.Session` has no `Dir` field and `ListSessions`'s format string and parser only fetch three fields (`name|windows|attached`).

**Solution**: Add a `Dir string` field to `tmux.Session`, append `#{@portal-dir}` to the `ListSessions` format string, and extend the parser to a 4-field split — handling the empty/absent stamp (parses to empty `Dir`) and being robust to a pipe character that could appear in the directory value.

**Outcome**: `ListSessions` returns `Session{Name, Windows, Attached, Dir}` where `Dir` is the stamped `@portal-dir` (empty string when absent); the format-field count change is reflected in the split logic; a directory value containing `|` does not corrupt the parse.

**Do**:
- In `/Users/leeovery/Code/portal/internal/tmux/tmux.go`: add `Dir string` to the `Session` struct (line ~30).
- Change the `ListSessions` format string (line ~188) from `#{session_name}|#{session_windows}|#{session_attached}` to append the dir field. Because a directory path could theoretically contain `|`, put `@portal-dir` LAST and parse with a bounded split that keeps the tail intact — e.g. `strings.SplitN(line, "|", 4)` with `Dir = parts[3]` (the final field absorbs any embedded pipes only if you instead split the first three from the left; simplest robust approach: keep `#{@portal-dir}` as the trailing field and use `SplitN(line, "|", 4)` so `parts[3]` is everything after the third pipe, preserving embedded pipes in the path). Document this ordering rationale in a comment.
- Update the parser: `len(parts) != 4` is the new malformed guard; `Dir = parts[3]`. An absent/empty `@portal-dir` yields an empty trailing field (`Dir == ""`).
- Update all existing `ListSessions` tests' fixture strings to the 4-field shape (they currently use 3-field `name|windows|attached`). The underscore-prefix filter and existing behaviour stay unchanged.
- Verify no other caller constructs `tmux.Session` literals positionally in a way that breaks with the new field (search for `tmux.Session{`).

**Acceptance Criteria**:
- [ ] `tmux.Session` has a `Dir string` field.
- [ ] `ListSessions` format string includes `#{@portal-dir}` as the trailing field.
- [ ] A session with a stamped `@portal-dir` parses `Dir` to that value.
- [ ] A session with no `@portal-dir` parses `Dir` to `""` (empty trailing field) without error.
- [ ] A directory value containing a `|` round-trips into `Dir` intact (trailing-field split).
- [ ] Existing `ListSessions` tests are updated to the 4-field fixture and still pass (windows/attached/name/filter behaviour unchanged).

**Tests**:
- `"it parses the stamped @portal-dir into Session.Dir"`
- `"it parses an absent @portal-dir to an empty Dir"`
- `"it preserves a pipe character in the directory value"`
- `"it still parses name, windows, attached unchanged with the added field"`
- `"it still filters underscore-prefixed sessions with the 4-field format"`

**Edge Cases**:
- Empty/absent `@portal-dir` parses to empty `Dir`.
- Format-field count change from 3 to 4 (update the malformed-line guard and all fixtures).
- Pipe character in the directory value — keep `@portal-dir` as the trailing field and use a bounded `SplitN(..., 4)` so embedded pipes do not corrupt the parse.

**Context**:
> The grouped render reads `@portal-dir` in the same `list-sessions -F` pass that already fetches session names (append `#{@portal-dir}` to the format string). No `git rev-parse` per session per render. `@portal-dir` is the fast path, not a guarantee — an absent value drives the lazy fallback (task 1-7).

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ The stamp (fast path), § The lazy stamp-on-render fallback)

## session-tagging-and-grouping-1-7 | approved

### Task session-tagging-and-grouping-1-7: Lazy active-pane → git-root directory resolution

**Problem**: `@portal-dir` is absent for post-reboot restored sessions and for sessions already live when the feature first ships. The grouped render must still resolve these sessions' directories — by deriving from the active pane's `current_path` → git-root — for this render, without dropping the session.

**Solution**: Add a resolver that, given a session name, reads the **active** pane's `current_path` from tmux, resolves it to a git-root via `ResolveGitRoot`, and returns the canonical directory key (via task 1-4's `CanonicalDirKey`). It uses the active pane only (not all panes), and tolerates a session killed mid-resolve and a pane with no enclosing git repository.

**Outcome**: Given a live session with no stamp, the resolver returns the canonical git-root directory of its active pane; a pane with no derivable git-root returns a clear "no directory" result (routes to Unknown/Untagged in Phase 2); a session that vanished mid-resolve returns a non-fatal "unresolvable" result rather than erroring the whole render.

**Do**:
- Add a tmux client method to read the active pane's current path, e.g. `func (c *Client) ActivePaneCurrentPath(session string) (string, error)` in `internal/tmux/tmux.go`, using `display-message -p -t <session> -F '#{pane_current_path}'` (targets the session's active pane) OR `list-panes -t <session> -f '#{pane_active}' -F '#{pane_current_path}'`. Prefer the `display-message` form for the active pane — it returns exactly the active pane's value in one call. Use the `=`-exact-match target convention if a name-collision risk applies (see `PaneTargetExact`); a session name here is the live name, so use `-t <session>` directly.
- Add the resolver (e.g. in `internal/session/` or a small new helper consumed by the render layer): given a session name, call `ActivePaneCurrentPath`, then `resolver.ResolveGitRoot(path, runner)`, then `project.CanonicalDirKey(gitRoot)`. Return `(canonicalDir string, ok bool, err error)` or an equivalent three-state shape.
- Active pane ONLY: do not enumerate or pick from all panes — the active pane is the resolution source (spec: "active pane only").
- Session killed mid-resolve: if `ActivePaneCurrentPath` errors with a no-such-session / no-pane class error, return a non-fatal "unresolvable" result (`ok == false`, err nil or a sentinel the caller swallows) — never propagate a fatal error that would abort the whole grouped render.
- Pane with no enclosing git repo: `ResolveGitRoot` returns the original dir unchanged when not a repo (it does not error). The spec wants a pane with NO derivable git-root to yield NO stamp and fall to Unknown/Untagged. Decide the contract: a non-repo pane still has a `current_path` (a real directory), so `ResolveGitRoot` returns that directory — that IS a derivable directory and the session can group By Project under that path if it is a known project, or Unknown if not. A pane with no `current_path` at all (rare; e.g. dead pane) is the true "no directory" case → return `ok == false`. Document this distinction in the resolver so Phase 2's Unknown-bucket routing is unambiguous. (If the spec's intent is that ONLY git-repo directories resolve, note the ambiguity — see Context — but the codebase's `ResolveGitRoot` falling back to the dir itself means a real cwd is always a directory; treat "no derivable git-root" as "no readable current_path".)

**Acceptance Criteria**:
- [ ] A tmux method reads the active pane's `current_path` for a named session in one call.
- [ ] The resolver returns the canonical git-root directory for a session whose active pane sits in a repo.
- [ ] The resolver uses the active pane only — it does not iterate all panes.
- [ ] A session killed mid-resolve returns a non-fatal unresolvable result (no render-aborting error).
- [ ] A pane with no readable `current_path` returns `ok == false` (routes to Unknown/Untagged later).
- [ ] The returned directory is canonicalised via `CanonicalDirKey` (matches stored `Project.Path` keying).

**Tests**:
- `"it resolves the active pane current_path to a canonical git root"`
- `"it reads only the active pane, not all panes"`
- `"it returns an unresolvable result when the session was killed mid-resolve"`
- `"it returns no-directory when the active pane has no readable current_path"`
- `"it canonicalises the derived directory to match stored Project.Path keying"`

**Edge Cases**:
- Pane with no enclosing git repo / no derivable git-root → no stamp, re-attempted each render (cheap, rare).
- Session killed mid-resolve → non-fatal, session simply unresolvable this pass.
- Active pane only (not all panes).

**Context**:
> When the grouped render encounters a session with no `@portal-dir`, it resolves that session's directory from the active pane's current path → git-root. If git-root derivation itself fails (pane has no enclosing git repository), the session is not stamped and falls to the Unknown bucket (By Project) / Untagged (By Tag). It is re-attempted each render (cheap; this is the rare case). The derived value is used for this render, not just cached for the next.
>
> Ambiguity to note: `internal/resolver.ResolveGitRoot` returns the input directory unchanged when it is not a git repo (it does not fail). So a non-repo pane still yields a real directory. The resolver should treat "no derivable git-root" as "no readable current_path" (true no-directory case) and let a real cwd group By Project / fall to Unknown if not a known project, per the Empty-States section. Confirm against the spec's intent during implementation.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ The lazy stamp-on-render fallback, § Empty states → Unresolvable directory)

## session-tagging-and-grouping-1-8 | approved

### Task session-tagging-and-grouping-1-8: Best-effort lazy re-stamp of derived `@portal-dir`

**Problem**: When the lazy fallback (task 1-7) derives a directory for an un-stamped session, that derivation should be cached so subsequent renders are back on the fast path — but the cache write (a `set-session-option`) is a side-effect during a render and must never drop the session if it fails.

**Solution**: After the lazy resolver derives a non-empty canonical directory, best-effort stamp `@portal-dir` for the session via `SetSessionOption`. The derived value is returned and used for the current render regardless of write outcome; a write failure is swallowed and simply re-attempted next render; a failed git-root derivation yields no stamp at all.

**Outcome**: An un-stamped session derives its directory, is used for this render, and is stamped so the next render reads the fast path; if the stamp write fails the session still renders this pass and the stamp is re-attempted next render; if derivation produced no directory, no stamp write is attempted.

**Do**:
- Compose with task 1-7's resolver: in the render-layer entry point (or a small helper the render layer calls), when a session's `Dir` from `ListSessions` is empty, call the lazy resolver; if it returns a non-empty canonical `dir` (`ok == true`), (a) use `dir` for this render, and (b) call `client.SetSessionOption(session, PortalDirOption, dir)` best-effort.
- The stamp write MUST be best-effort: wrap in a non-propagating call — on error, log at most a WARN/DEBUG breadcrumb (reuse an existing component/attr vocabulary; do NOT invent closed-vocabulary keys — if no suitable existing one, swallow silently and note it) and continue. NEVER let the write error remove the session from the rendered set or abort the render.
- The derived value is the render value irrespective of the write — assert this ordering: render uses `dir` first, then attempts the stamp.
- If the resolver returned `ok == false` (no derivable directory), do NOT call `SetSessionOption` — there is nothing to stamp; the session falls to Unknown/Untagged (Phase 2) and is re-attempted next render.
- Keep this purely as the resolution/stamping mechanism; the actual Unknown/Untagged bucketing and rendering are Phase 2 — this task's surface is the derive-use-then-stamp helper and its failure semantics, unit-testable with a mock tmux client.

**Acceptance Criteria**:
- [ ] After deriving a directory for an un-stamped session, `SetSessionOption(session, "@portal-dir", dir)` is called.
- [ ] When `SetSessionOption` returns an error, the helper still yields the derived directory for the current render (no drop) and does not propagate the error.
- [ ] A subsequent invocation (simulating the next render) where the stamp now exists takes the fast path and does not re-derive.
- [ ] When the stamp write failed, the next render re-attempts the derivation + stamp.
- [ ] When git-root derivation yields no directory (`ok == false`), no `SetSessionOption` call is made and the derived value is empty (routes to Unknown/Untagged).

**Tests**:
- `"it stamps the derived directory after a successful lazy resolution"`
- `"it uses the derived value for the current render even when the stamp write fails"`
- `"it swallows a SetSessionOption error without dropping the session"`
- `"it re-attempts the stamp on the next render after a write failure"`
- `"it does not stamp when no git-root is derivable"`

**Edge Cases**:
- `SetSessionOption` failure swallowed and re-attempted next render.
- Git-root derivation failure yields no stamp (no write attempted).
- Derived value used for the current render regardless of write outcome.

**Context**:
> The stamp write is best-effort. If `set-session-option` fails (tmux error, session killed mid-render), the session still renders this pass using the in-memory derived directory; the stamp is simply re-attempted on the next grouped render. A write failure never drops the session from the view. The derived value is used for this render, not just cached for the next — that is what makes "they appear in By Project immediately" true. First-ship cost is a bounded one-time amortisation (N derivations + N stamp writes on the first grouped render); from the second render on, all sessions are on the fast path.

**Spec Reference**: `.workflows/session-tagging-and-grouping/specification/session-tagging-and-grouping/specification.md` (§ The lazy stamp-on-render fallback → Failure & ordering semantics)
