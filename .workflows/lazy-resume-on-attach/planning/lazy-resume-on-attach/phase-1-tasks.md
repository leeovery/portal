# Phase 1: Resume mode — storage, override and read-back — 7 tasks

## lazy-resume-on-attach-1-1

### Task 1.1: Resume mode vocabulary and resolution

**Problem**: Every restored registration must resolve to exactly one of two modes — eager (fire the hook as restore finishes, today's behaviour) or lazy (hold it behind a waiting panel) — from an install-wide default plus a three-state per-registration override where "nothing" means inherit. Two packages need that vocabulary: `internal/hooks` (which will carry the stored `resume` attribute) and `internal/prefs` (which will carry the install-wide `resume_mode` key). Both are guarded leaves whose own leaf-guard tests forbid them from importing each other, so neither can own the type. Without a shared home, the two would each spell out their own eager/lazy strings and their own recogniser, and the two spellings would drift.

**Solution**: A new stdlib-only leaf package, `internal/resumemode`, holding the closed `Mode` vocabulary, its on-disk spellings, the single strict recogniser both the reader and the CLI flag parse through, the shipped default, and the one resolution function that answers eager-or-lazy from a registration's mode and the install's. This is the same situation `internal/nanoid` already exists to answer, and it takes the same shape: a leaf with its own dependency guard.

**Outcome**: `resumemode.Resolve` answers `Eager` or `Lazy` for any pair of inputs, a value nothing recognises carries no mode rather than being coerced into one, and a package-dependency guard pins the new package to the standard library alone across both build lanes.

**Do**:
- Create `internal/resumemode/resumemode.go` declaring `type Mode int` with `Unset Mode = iota` (names no mode — the zero value), `Eager` and `Lazy`; the unexported on-disk spellings `"eager"` and `"lazy"`; and `const Default = Lazy`, the install-wide default the feature ships with.
- Give `Mode` a `String() string` returning the on-disk spelling, and the empty string for `Unset` or any value outside the vocabulary — an unset mode renders as absent rather than impersonating one of the two.
- Add `Parse(s string) (Mode, bool)` — the single recogniser, matching the two spellings exactly with no trimming, no case folding and no prefix matching; every other input, the empty string included, answers `(Unset, false)`. Its two callers apply opposite policies to that `false` (a stored value tolerates it and carries no mode; the CLI flag refuses it), so `Parse` itself decides nothing beyond recognition.
- Add `Resolve(registration, install Mode) Mode`: a registration that names a mode wins outright; otherwise the install's mode if it names one; otherwise `Default`. It is total — it never answers `Unset`.
- Add `internal/resumemode/leaf_guard_test.go` modelled on `internal/nanoid/leaf_guard_test.go`: `sourceguardtest.AssertDepsWithin(t, resumeModePkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)` over every lane in `sourceguardtest.Lanes()`.
- Add a `resumemode` row to CLAUDE.md's package table, beside the other leaves: the closed eager/lazy vocabulary — the `Mode` kind with its `Unset` zero meaning "names no mode", the two on-disk spellings, the single strict `Parse` that recognises those two words and nothing else, the shipped `Default` (lazy), and the `Resolve` that answers a registration's mode against the install's — stdlib-only, so `internal/hooks` and `internal/prefs` can each reach it without an edge between them, pinned by its own dependency guard across both lanes.

**Acceptance Criteria**:
- [ ] `resumemode.Parse` recognises `"eager"` and `"lazy"` and nothing else; `"Lazy"`, `"LAZY"`, `" lazy"`, `"lazy "`, `"laz"`, `"lazyy"` and `""` all answer `(Unset, false)`.
- [ ] `Mode.String()` round-trips `Eager` and `Lazy` through `Parse`, and returns `""` for `Unset` and for an out-of-vocabulary `Mode` value.
- [ ] `Resolve` returns the registration's mode whenever the registration names one, whatever the install says — including a registration pinned eager under a lazy install and the reverse.
- [ ] `Resolve` returns the install's mode when the registration names none, and returns `Lazy` when neither names one.
- [ ] The package's transitive dependency set is the standard library alone, in both the default and the integration lane, and the guard fails on any added edge.

**Tests**:
- `"it recognises the two on-disk spellings and nothing else"` (table over `eager`, `lazy`, `Lazy`, `LAZY`, ` lazy`, `lazy `, `laz`, `lazyy`, `""`, `"1"`)
- `"it renders an unset mode as the empty string"`
- `"it round-trips each named mode through String and Parse"`
- `"it lets a registration's mode win over the install's"`
- `"it falls back to the install's mode when the registration names none"`
- `"it answers lazy when neither the registration nor the install names a mode"`
- `"it depends on the standard library alone"` (leaf guard, both lanes)

**Edge Cases**:
- An unrecognised stored value carries no mode — `Parse` answers `(Unset, false)` and the caller inherits, rather than the value being coerced to the nearest match.
- An empty value carries no mode, and is not distinguished from an absent one by this package.
- Case and whitespace variants are unrecognised rather than coerced: nothing is trimmed and nothing is lowercased, so `" lazy"` from a hand edit is a value nothing recognises.
- The strict parse refuses every value but the two words — the CLI flag (a later task) turns that refusal into a non-zero exit, so `Parse` must not quietly accept a near miss.
- `Resolve` is called with an `install` of `Unset` only where no preference could be read at all; it must still answer `Lazy` rather than a zero `Mode`.

**Context**:
> The three-state override is the product decision, taken rather than bet against: the store holds arbitrary user-authored commands, and a thing the user walks up to behaves very differently under a panel that waits forever than a thing that needs to be up whether or not anyone looks at it — a dev server, a tunnel, a watcher. So a registration carries eager, lazy, or nothing, where nothing means inherit and changes with the install.
>
> The install-wide default is lazy: the feature ships on rather than waiting to be discovered, and an install that upgrades and reboots meets panels rather than processes.
>
> `Mode` is an int kind rather than a string kind — unlike `hooks.Event`, which is a string kind because its value *is* the map index on disk. A mode is rendered and recognised at boundaries rather than used as a key, and the three-state model needs a representable "names no mode" that no on-disk string spells. The int kind also stops a caller passing an invented literal that compiles.
>
> Both consumers are guarded leaves: `internal/hooks` may import only `fileutil`, `log`, `nanoid` and `storelog`; `internal/prefs` may import only `fileutil`. Each of those allowlists widens by this one entry in the task that adds the import — not here.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §2, §2.1, §3.1, §3.2

## lazy-resume-on-attach-1-2

### Task 1.2: hooks.json accepts the object form and preserves what it did not write

**Problem**: `hooks.json` is `map[hook_key]map[event]command` — strings all the way down, with no slot for an attribute that is not a command (`internal/hooks/store.go`, the `Snapshot` type). A registration now has to be able to carry a setting beside its command, and the store rewrites the whole file on every mutation — so any entry it cannot model on the way in is erased on the way out. The external Claude Code `SessionStart` hook fires `portal hook set` on every session start, so "every mutation rewrites every entry" is an hourly event, not a rare one: a reader that drops what it does not understand would quietly strip a hand-pinned mode, or a future attribute, off forty other entries the call never named.

**Solution**: Make an event's stored value a `Registration` — decoded from either a JSON string (the command alone) or a JSON object (`command` plus `resume`, and whatever else is in there) — whose codec retains the exact bytes it was decoded from and re-emits them verbatim for any entry a mutation did not replace. A value that is neither string nor object decodes to a registration carrying nothing and still round-trips, so one junk entry never fails the load for its neighbours.

**Outcome**: Both stored shapes load, an entry the writer did not name comes back out of a rewrite carrying every attribute and every unrecognised value it went in with, and no shape of stored value can fail the file for another entry.

**Do**:
- Add `internal/hooks/registration.go` declaring `type Registration struct { Command string; Resume resumemode.Mode; raw json.RawMessage }`, and add `github.com/leeovery/portal/internal/resumemode` to `hooksMayImport` in `internal/hooks/leaf_guard_test.go`.
- Implement `UnmarshalJSON` so it never returns an error and always retains the raw bytes: a JSON string sets `Command` and leaves `Resume` unset; a JSON object reads `command` (a non-string or absent key leaves `Command` empty) and `resume` through `resumemode.Parse` (absent, empty, non-string or unrecognised leaves `Resume` unset); any other JSON value sets neither.
- Implement `MarshalJSON` on the value receiver: emit the retained raw bytes when they are present (an entry nobody replaced), otherwise the plain string form when `Resume` is unset and the `{"command":…,"resume":…}` object form when it is not. A registration a mutation builds carries no raw, so it is written in the shape its own content chooses.
- Change `Snapshot` to `map[string]map[string]Registration`, and carry the change through the store: `Set` keeps its current `command string` parameter here and stores `Registration{Command: command}`; `classifySet` compares the registration that will be written against the stored one on both command and mode, so rewriting a mode-carrying entry with a bare command is a `modify` rather than a `set-noop`; `removedValue` renders each event's `Command`.
- Update the `hooks.Snapshot` literals in `internal/hooksweep/sweep_test.go` and `internal/hooksweep/decline_error_test.go` to the new value type, and confirm `StaleKeys`, `narrowToSnapshot`, `hooksweep` and `cmd/doctor.go`'s `checkStaleHooks` still compile and pass untouched — they read keys and lengths only.
- Edit the `hooks` row of CLAUDE.md's package table where it says the store holds per-pane on-resume commands: an event's stored value is a `Registration` decoded from either a JSON string (the command alone) or a JSON object (`command` plus `resume`), both shapes permanently valid with no migration between them; an entry a mutation did not name is re-emitted from the bytes it was decoded from, so an attribute the reader does not model and a `resume` value it cannot make sense of both survive a sibling's rewrite; and a value that is neither string nor object never fails the load for its neighbours.

**Acceptance Criteria**:
- [ ] A string value loads as a registration carrying that command and no mode; an object carrying `command` and `resume` loads as that command and that mode.
- [ ] After `Set` rewrites one entry, every other entry's value is byte-equivalent to what it held — an attribute the reader does not model (e.g. `"nickname"`), a `resume` value it could not make sense of (e.g. `"LAZY"`), and a value that is neither string nor object alike. Key order inside a preserved object is unchanged; file-level indentation is the store's and is not part of the contract.
- [ ] A value that is neither string nor object (number, array, boolean, `null`) does not fail the load: the other entries load normally and that entry round-trips untouched.
- [ ] A registration built by a mutation and carrying no mode is written as a plain JSON string; one carrying a mode is written as an object holding `command` and `resume`.
- [ ] `clean-stale`'s recoverable `value` breadcrumb carries the command out of an object-form entry, exactly as it does out of a string-form one, and a key holding several events still renders every `event=command` pair in event order.
- [ ] `doctor`'s stale-hook count, the daemon's sweep and `StaleKeys` behave exactly as before across the existing suites.

**Tests**:
- `"it loads a string value as a command carrying no mode"`
- `"it loads an object value as its command and its mode"`
- `"it preserves an unmodelled attribute on an entry a rewrite did not name"`
- `"it preserves an unrecognised resume value on an entry a rewrite did not name"`
- `"it loads the remaining entries when one value is neither a string nor an object"` (table over `42`, `["x"]`, `true`, `null`)
- `"it round-trips a value it cannot model through a rewrite of its sibling"`
- `"it marshals a registration carrying no mode as a plain string"`
- `"it marshals a registration carrying a mode as an object"`
- `"it renders the clean-stale value breadcrumb out of an object entry"`
- `"it renders every event=command pair for a key holding several events"` (existing behaviour, re-pinned over the new value type)

**Edge Cases**:
- An unmodelled attribute survives a sibling rewrite — the object is re-emitted from its retained bytes rather than re-marshalled from the two fields the reader models.
- An unrecognised `resume` value survives a sibling rewrite: it carries no mode to the reader and is still written back as the user typed it. Nothing is rewritten to correct it.
- A value that is neither string nor object never fails the load for other entries; it carries no command, so it is a lookup miss, and it is left on disk as it was found.
- An object carrying no `command` key, or an empty one, decodes to an empty `Command` — the miss rule that follows from it belongs to the lookup task.
- A malformed *file* is unchanged in behaviour: the read doors still degrade it to an empty map after one DEBUG record, and a mutation still refuses it with `ErrMalformed` rather than writing a one-entry map over unreadable registrations.

**Context**:
> Neither shape is legacy and neither deprecates the other. The string form is the primary shape for a registration that is only a command; the object form is the primary shape for one that carries configuration. Both are permanently valid, there is no migration, and there is no future pass that converts one into the other.
>
> That rule is load-bearing rather than cosmetic: the external `SessionStart` hook fires `portal hook set` on every session start and each call rewrites the whole file, so a writer that always emitted the object form would convert every entry on the install to the verbose shape within a day. Emitting the string form when there is nothing to carry keeps a hand-edited file looking as it does today, with the occasional expanded entry where something has been pinned.
>
> A rewrite of one registration leaves every other entry exactly as it found it. Only the entry being written is written whole; what that call is handed is all that entry keeps.
>
> The out-of-repo consumers were checked before the shape was chosen: `~/.claude/hooks/portal-resume-hook.sh` reads `portal hook list` and filters on the event column rather than parsing the file, and that output does not change shape. The one script that does parse the file with `jq` looks entries up by the pre-token `session:window.pane` key, already matches nothing on the live install, and is confirmed no longer used.
>
> The rejected alternative was a second entry beside `on-resume` in the same inner map: it keeps a `jq` reader working, but it puts a non-event in the event namespace and adds a row to `hook list`, which is a machine interface an external script parses.
>
> Fixtures for the object form are staged through `hookstest.StageStore(t, hookstest.Staging{Seed: …})`, which takes a raw body — no new `hookstest` surface is needed for this task.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §3.2, §2.2

## lazy-resume-on-attach-1-3

### Task 1.3: hook set writes the registration whole in the shape its content chooses

**Problem**: `Store.Set` takes a bare command string, so there is no way to write a registration that carries a mode, and no way to express the rule that governs one: a registration replaces its predecessor completely. Nothing survives a re-registration it was not given. Left as it is, either the mode would have to arrive through a second mutation (a write whose result depends on state the caller never saw) or a rewrite would carry the old entry's pin forward — the behaviour nobody can account for when it surprises them later.

**Solution**: Give `Set` the whole registration to write. The value it is handed is the entirety of what that entry keeps: a mode it carries is written in the object form, a mode it does not carry is written in the string form, and everything the replaced entry held — a pin, an attribute the reader does not model — goes with it. The no-op classification widens to the whole value, so a mode-only change is a real change.

**Outcome**: One call writes a registration whole in the shape its content chooses, an identical rewrite is still a no-op that touches no file, and a rewrite that differs only in its mode is a `modify` that lands.

**Do**:
- Change the signature to `Set(key string, event Event, registration Registration, via Via) error`, storing the value it was handed directly into `h[key][event.String()]` — the passed registration carries no retained raw bytes, so the entry is re-encoded from its own content.
- Generalise `classifySet` to compare the registration to be written against the stored one on command and mode together: unchanged on both is `set-noop` (DEBUG, no write, file untouched), an absent key or event is `set`, anything else is `modify`.
- Keep the breadcrumb vocabulary exactly as it is — same ops, same `hook_key`, same `via`, and `value` carrying the command — so no new log attr or op is introduced by this task.
- Update the two non-test call sites: `cmd/hooks.go`'s `hooksSetCmd` (`hooks.Registration{Command: command}` — no mode passes through the CLI until the flag task) and `internal/hookstest/hooks.go`'s `SeedHooksJSON`; then the `internal/hooks` and `cmd` suites that call `Set` directly.
- Add object-form coverage to `internal/hooks/store_test.go` (or a sibling file) staging fixtures through `hookstest.StageStore` with a raw `Seed`, asserting both the resulting on-disk shape and the emitted breadcrumb via `logtest.Install` + `logtest.AssertRecord`.

**Acceptance Criteria**:
- [ ] Setting a registration carrying a mode writes the object form holding `command` and `resume`; setting one carrying no mode writes the plain string form.
- [ ] Rewriting an entry with a registration identical in command and mode is a `set-noop`: no write, the file byte-unchanged, one DEBUG record, no INFO.
- [ ] Rewriting an entry with the same command and a different mode — in either direction, including to no mode — is a `modify` that writes.
- [ ] An object-form predecessor rewritten with a registration carrying no mode comes back as a plain string and keeps none of its unmodelled attributes.
- [ ] A rewrite still leaves every other entry in the file exactly as it found it.
- [ ] The `set` / `modify` / `set-noop` breadcrumbs carry the same message, `op`, `hook_key`, `via` and `value` (the command) as before, out of either stored shape, and a failed save still carries `error` and `error_class` and no `value`-less shape change.

**Tests**:
- `"it writes the object form for a registration carrying a mode"`
- `"it writes the string form for a registration carrying no mode"`
- `"it treats an identical object-form rewrite as a no-op and leaves the file untouched"`
- `"it treats a mode-only change as a modify"` (both directions: none→lazy, eager→none)
- `"it drops an object-form predecessor's unmodelled attributes when the rewrite carries no mode"`
- `"it leaves every other entry as it found it when writing one"`
- `"it carries the command as the value attr out of an object-form write"`
- `"it reports a failed save with its error class and writes nothing"` (existing behaviour, re-pinned over the new signature — stage with `hookstest.Staging{WritesDenied: true}`)

**Edge Cases**:
- A mode-only change is a `modify`, not a `set-noop` — classification compares the whole value, so a pin added or removed without touching the command still lands on disk.
- An identical object-form rewrite is still a no-op: the file is not rewritten, so an entry's unmodelled attributes survive a caller that re-registers the same thing (which the external `SessionStart` hook does constantly).
- An object-form predecessor rewritten with no mode returns to the string form and loses its unmodelled attributes — this is the contract, not a defect.
- An empty command is written as today, not refused: `Set` persists what it is handed, and the emptiness is the lookup's business.
- A registration built in a test literal (`hooks.Registration{Command: …, Resume: …}`) carries no raw bytes, so it always encodes canonically.

**Context**:
> Nothing survives a re-registration it was not given. `portal hook set` writes exactly what it is handed. A mode the caller does not pass is not a mode — no attribute from the entry being replaced is carried forward, and the store's existing wholesale overwrite of an event's value is the correct behaviour rather than something to work around.
>
> The alternative — that a `hook set` carrying no mode flag should leave an existing mode alone — was rejected on the model rather than the mechanics: inheriting configuration from a dead registration means a writer's output depends on state it never saw.
>
> The consequence is stated rather than mitigated: a pin lives on the registration, not on the pane. Anything that re-registers without the flag drops it. The practical exposure is small — the external Claude Code `SessionStart` hook writes only for panes running a session, its `SessionEnd` counterpart usually removes the entry first, and the one collision resolves by re-passing the flag.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §2.2, §3.2, §3.3

## lazy-resume-on-attach-1-4

### Task 1.4: The store's reads report the registration's mode

**Problem**: The store can now hold a mode, but nothing can read one back out. `LookupOnResume` answers `(command, found, error)` — the shape the hydrate helper consumes — and `List` builds `Hook{Key, Event, Command}`, so a stored `resume` attribute is invisible to every consumer. The helper will shortly have to decide whether a pane waits or fires, and it reads the command and the mode at the same moment, from the same read.

**Solution**: Give the lookup a small result struct carrying the command, the mode and whether anything was found, and give `Hook` the mode alongside the command. Both readers apply the same rule for what is *not* a registration: an entry carrying no command — a string entry that is empty, an object with no `command` key or an empty one, or a value that is neither — is a miss, exactly as an absent key is.

**Outcome**: One read reports the command and the mode together, a stored value carrying no command reports a miss rather than a registration with nothing to run, and an unreadable store still degrades to no hook.

**Do**:
- Add `type OnResume struct { Command string; Mode resumemode.Mode; Found bool }` in `internal/hooks/lookup.go` and change the signature to `LookupOnResume(hookKey string, via Via) (OnResume, error)`, returning the zero value alongside any error and alongside every miss.
- Keep every existing miss rule intact and route the new shapes through the same one: an empty `hookKey` is refused before the file is read; an absent key, an absent `on-resume` event, and an entry whose `Command` is empty all answer `OnResume{}` with a nil error.
- On a hit, populate `Command` and `Mode` from the loaded `Registration` and set `Found` — the lookup reports what is stored and resolves nothing against the install default.
- Add `Resume resumemode.Mode` to the `Hook` struct and populate it in `List`, leaving the existing sort by key then event untouched.
- Update the call sites: `cmd/state_hydrate.go`'s `execShellOrHookAndExit` (reading `.Found` and `.Command`; its existing DEBUG `hook lookup` hit/miss/error records and its WARN are unchanged), `cmd/bootstrap/transient_listpanes_helpers_integration_test.go`, and the `internal/hooks` suites that call the lookup (`lookup_test.go`, `read_lock_test.go`, `event_test.go`).

**Acceptance Criteria**:
- [ ] A hit on a string-form entry reports the command, `Found` true and no mode; a hit on an object-form entry carrying `resume` reports the command, `Found` true and that mode.
- [ ] An object whose `resume` is absent, empty, unrecognised or not a string reports the command with no mode — the entry inherits, and the lookup says nothing about the install default.
- [ ] An object with no `command` key, an object whose `command` is empty, and a value that is neither string nor object each report the zero result with a nil error.
- [ ] An empty `hookKey`, an absent key and an absent `on-resume` event each report the zero result with a nil error, and the empty key still reads no file.
- [ ] A genuine read failure returns the zero result alongside the error, and the hydrate helper still execs a bare shell on it; a malformed `hooks.json` still degrades to a miss after one DEBUG record rather than erroring.
- [ ] `List` carries each entry's mode beside its command, in the existing key-then-event order, and a string-form entry carries none.

**Tests**:
- `"it reports the command and no mode for a string-form entry"`
- `"it reports the command and the mode for an object-form entry"`
- `"it reports no mode when the stored resume is absent, empty or unrecognised"` (table)
- `"it reports a miss for an object carrying no command key"`
- `"it reports a miss for an object whose command is empty"`
- `"it reports a miss for a value that is neither a string nor an object"`
- `"it reports a miss for an empty hook key without reading the file"` (existing, re-pinned over the new signature)
- `"it returns the zero result alongside a read error"`
- `"it degrades to a bare shell when the store is unreadable"` (`cmd`, existing coverage re-pinned)
- `"it carries each entry's mode through List in key then event order"`

**Edge Cases**:
- An absent, empty or unrecognised `resume` reads as no mode — never as eager, never as lazy, and never as an error.
- An object with no command key, or an empty one, is not a registration: the pane falls through to a plain shell exactly as an unregistered pane does, a path the helper already has.
- A string-form entry reads as no mode; the two shapes differ in what they can carry, not in how a command is reported.
- An unreadable store still degrades to no hook at the call site: the lookup returns the error, and the helper's existing behaviour of giving the pane a bare shell so it stays usable is unchanged.
- A whitespace-only hook key is still looked up literally — nothing is trimmed.

**Context**:
> A stored value the reader cannot make sense of never fails a pane. An object whose `resume` attribute is absent, empty, or holds anything other than `eager` or `lazy` carries no mode — the registration inherits the install-wide default exactly as a string-form entry does. An object carrying no command, or an empty one, is not a registration: the pane falls through to a plain shell as an unregistered pane does. Nothing is rewritten to correct either case; the file stays as the user left it.
>
> The result struct is deliberate rather than a fourth return value: the consumer that matters reads the command and the mode together, at the moment the pane is decided.
>
> The lookup reports what is stored. Combining it with the install-wide default is the resolution function's job, at the call site that knows both.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §3.2, §3.4, §6.1

## lazy-resume-on-attach-1-5

### Task 1.5: prefs.json carries resume_mode

**Problem**: The per-registration override only covers the exceptions; the install-wide default governs everything else, and it has nowhere to live. `prefs.json` is where the install's preferences already live, but its record is a fixed struct — any key it does not declare is dropped on re-encode, and every writer re-encodes the whole file. So a hand-set `resume_mode` would be erased by the user's next `s` keypress or theme commit, invisible until they went looking for it.

**Solution**: Declare `resume_mode` on the prefs record — decoded tolerantly and independently of every other key, like every field there, and `omitempty` on write so an install that never set it carries no key — and expose one accessor that resolves it to `eager` or `lazy`, falling back to the shipped default for anything it cannot make sense of.

**Outcome**: A hand-set `resume_mode` is readable as a mode, survives every other write the prefs store makes, and a missing, empty, corrupt or unrecognised value answers lazy.

**Do**:
- Declare a tolerant named string type for the field in `internal/prefs/store.go`, modelled on `migrationMarker`: an `UnmarshalJSON` that takes a JSON string as the value and absorbs any other JSON value as the empty string without erroring, so a wrong-typed value cannot zero the tolerant load's whole record.
- Add `ResumeMode <that type> \`json:"resume_mode,omitempty"\`` to `prefsFile`, so the key round-trips through every existing mutation (`Save`, `SaveTheme`, `SaveThemeSlot`, `SaveMigrationMarker`, `SaveTranslation`) untouched.
- Add `LoadResumeMode() (resumemode.Mode, error)` over the tolerant `readFile`, returning `resumemode.Parse`'s answer when it recognises the stored string and `resumemode.Default` otherwise; a non-`ErrNotExist` read error propagates alongside `resumemode.Default`, as `Load` does with `ModeFlat`.
- Add `github.com/leeovery/portal/internal/resumemode` to `prefsMayImport` in `internal/prefs/leaf_guard_test.go`; add no writer — there is no surface for changing this setting and nothing in Portal writes it.
- Update the `prefs.json` row of the README's config table to name `resume_mode` alongside the grouping mode and the theme keys, stating that it holds `eager` or `lazy` and is the install-wide default a registration that names no mode of its own inherits.
- Edit the `prefs` row of CLAUDE.md's package table: add the `resume_mode` key to the literal key shape it enumerates, state that the field decodes tolerantly and independently like every other one there — missing, empty, corrupt, unrecognised or a file that cannot be read at all all give the shipped default, lazy — and that nothing in Portal writes it; and widen the leaf clause from stdlib plus `fileutil` only to stdlib plus `fileutil` plus `internal/resumemode` only, leaving the no-`internal/log` rule and its reasoning exactly as they are.

**Acceptance Criteria**:
- [ ] `LoadResumeMode` returns `Eager` for `"eager"` and `Lazy` for `"lazy"`.
- [ ] It returns `Lazy` for an absent file, an absent key, an empty value, an unrecognised value (`"LAZY"`, `" lazy"`, `"off"`), a non-string value, and a wholly corrupt file.
- [ ] A `prefs.json` that exists but cannot be read at all answers `Lazy` too — the value is the shipped default alongside whatever error is propagated, so a caller that takes the value meets panels rather than processes.
- [ ] A wrong-typed `resume_mode` does not zero the rest of the tolerant record: the theme keys and the grouping mode beside it still load.
- [ ] A hand-set `resume_mode` is still on disk, with its value unchanged, after `Save(ModeByTag)` and after `SaveTheme`/`SaveThemeSlot`.
- [ ] An install that never set the key has no `resume_mode` in the file after any of those writes.
- [ ] The strict write-path decode is unchanged: a malformed `prefs.json` still aborts a write rather than merging over it, and the file is left as it was.
- [ ] `internal/prefs` still depends on `internal/fileutil` and `internal/resumemode` alone, across both lanes.

**Tests**:
- `"it reads eager and lazy from the persisted key"`
- `"it answers lazy when the key is missing, empty, unrecognised or wrong-typed"` (table)
- `"it answers lazy for an absent prefs.json and for a corrupt one"`
- `"it answers lazy for a prefs.json that cannot be read at all"` (file staged unreadable, as `themetest.DenyRead` stages one)
- `"it keeps the rest of the record when resume_mode is wrong-typed"`
- `"it preserves a hand-set resume_mode across a grouping-mode toggle"`
- `"it preserves a hand-set resume_mode across a theme commit"`
- `"it omits resume_mode from a file that never set it"`
- `"it still aborts a write on a malformed prefs.json"` (existing behaviour, re-pinned with the new field declared)
- `"it stays a leaf over fileutil and resumemode"` (leaf guard, both lanes)

**Edge Cases**:
- Missing, empty, corrupt or unrecognised all give lazy — the shipped default — with no error and no repair of the file.
- A file that cannot be read at all gives lazy as well, and by the same route: the accessor answers the default beside the error rather than leaving the mode undecided, so no caller has to invent a rule for the case. An install whose preferences file has gone unreadable meets panels rather than processes, which is the safe direction — a panel is answered in a keystroke and a resume the user did not want cannot be taken back.
- A wrong-typed value (a number, an object) does not zero the rest of the tolerant record, because the field's own decoder absorbs it. It carries no mode, and — unlike an unrecognised *string*, which round-trips verbatim — it is not preserved through the next write. That asymmetry is this task's call: the spec requires tolerance and preservation of what the user typed, and a non-string value is not something any version of Portal reads.
- The strict write-path decode still aborts on a malformed file; the new field must not soften it into a merge.
- There is no writer: nothing in Portal sets this key, so no test should assert one exists.

**Context**:
> `prefs.json` holds the install's UI preferences — the theme and the session-list grouping mode — and is where this one lives, as the key `resume_mode`, holding `eager` or `lazy`. It decodes tolerantly and independently like every other field there, and the key is `omitempty` on write, so an install that never set it carries no key.
>
> **Corrigendum 2026-09-19**: a file that cannot be read at all resolves the same way — the shipped default arriving by the ordinary route rather than a second rule. The reason is stated with it: a panel can be answered in a keystroke, while a resume the user did not want cannot be taken back, so an unreadable preferences file lands the install on the safe side of the setting rather than the convenient one.
>
> There is no surface for changing it. The theme picker is the only preference with a UI, so until a settings screen exists this one is changed by hand-editing the file. That raises the stakes on the default rather than changing where it lives; a settings screen is parked on the roadmap as `preferences-ui`.
>
> The field must stay declared for the same reason `appearance` does: `prefs.json` decodes into a plain struct, so any undeclared key is dropped on re-encode, and every writer re-encodes the whole file.
>
> `internal/prefs` is deliberately a leaf — stdlib plus `internal/fileutil` — so `internal/tui` can import it without a cycle, which is why the mode vocabulary had to be a leaf of its own rather than living in `internal/hooks`.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §3.1, §2.1, §10

## lazy-resume-on-attach-1-6

### Task 1.6: portal hook set --resume-mode

**Problem**: A registration's mode can be stored and read, but nothing can set one. The override has to be reachable from the thing that actually writes registrations — the external `SessionStart` hook is a shell script, not a person at a screen — so the route has to be a flag a script can pass alongside the command it is registering. Until it exists, every registration on the install inherits and the three-state model has only two reachable states.

**Solution**: `--resume-mode eager|lazy` on `portal hook set`, beside `--on-resume`. The value is recognised strictly and refused at the top of the command body, before anything reads tmux or touches a file, and a recognised one rides into the registration the store writes whole.

**Outcome**: `portal hook set --on-resume "<cmd>" --resume-mode lazy` writes an object-form registration under the pane's token; a mistyped mode fails where it was typed with nothing written and nothing stamped; and a call that passes no mode writes a registration carrying none, whatever its predecessor carried.

**Do**:
- Register the flag in `cmd/hooks.go`'s `init()`: `hooksSetCmd.Flags().String("resume-mode", "", …)`. Leave `MarkFlagRequired("on-resume")` exactly as it is — cobra already refuses a `--resume-mode` passed on its own, before `RunE` runs, so this task adds no second refusal for that case and pins the existing one with a test instead.
- In `hooksSetCmd`'s `RunE`, read both flags and resolve the mode as the first thing the body does, ahead of `resolveCurrentPaneKey()`: an unpassed flag (`cmd.Flags().Changed("resume-mode")` false) gives `resumemode.Unset`; a passed one must be recognised by `resumemode.Parse`, and a value it rejects — the empty string included — returns an error naming the two accepted values, which cobra renders and exits non-zero on.
- Pass the result through as `hooks.Registration{Command: command, Resume: mode}` to `store.Set`, leaving the pane-key resolution, the lazy token mint and stamp, and the `save.requested` touch exactly as they are.
- Extend `runHookSet` in `cmd/testhelpers_test.go` with variadic extra args (as `runHookRm` already has) and change `readHooksJSON` to decode into `hooks.Snapshot` so an object-form entry can be asserted, updating its existing callers to compare `.Command`.
- Add the flag to the README's `hook` section — one line in the example block and a sentence stating that a registration carries eager, lazy or nothing, that nothing means it follows the install-wide setting in `prefs.json`, and that a call which does not pass the flag writes a registration carrying no mode whatever its predecessor carried.

**Acceptance Criteria**:
- [ ] `hook set --on-resume "<cmd>" --resume-mode eager` and `… lazy` each write the object form under the pane's hook key, carrying that command and that mode.
- [ ] `hook set --on-resume "<cmd>"` with no mode flag writes the string form, and does so over an object-form predecessor — the stored entry keeps no mode and no unmodelled attribute.
- [ ] `hook set --resume-mode lazy` with no `--on-resume` exits non-zero, writes nothing, and performs no tmux read.
- [ ] `--resume-mode` with an unrecognised value, and with an explicitly empty value, each exit non-zero and write nothing — with no pane-key read, no token mint, no `set-option -p` stamp and no `save.requested` touch.
- [ ] A refusal names both accepted values in its message; the wording is one line and goes to the command's error stream.
- [ ] A recognised mode leaves the rest of the command unchanged: an unstamped pane is still stamped before the entry is written, a stamped one is reused with no `set-option`, and `save.requested` is still touched after a successful write.

**Tests**:
- `"it writes the object form when the flag names a mode"` (subtests for `eager` and `lazy`)
- `"it writes a registration carrying no mode when the flag is not passed"`
- `"it drops an object-form predecessor's mode when the flag is not passed"`
- `"it refuses --resume-mode with no --on-resume and writes nothing"`
- `"it refuses an unrecognised mode before any tmux read"` (assert the key resolver's call count is zero, the stamper is never called, the hooks file is unchanged, and `save.requested` is absent)
- `"it refuses an explicitly empty mode"`
- `"it names both accepted values in the refusal"`
- `"it still stamps a freshly minted token before writing a pinned registration"`

**Edge Cases**:
- `--resume-mode` with no `--on-resume` exits non-zero writing nothing — refused by the existing required-flag machinery, before the body runs.
- An unrecognised or empty value exits non-zero before any tmux read, token mint, pane stamp or `save.requested` touch: the validation is the first thing in the body, so a mistyped pin fails where it was typed rather than landing on disk as a mode nothing reads.
- An unpassed flag writes a registration carrying no mode whatever its predecessor carried — the flag's zero value is "names no mode", not "keep what was there".
- Pinning an existing registration means re-passing its command alongside the flag; there is no mode-only write.
- The refusal is the writer's side only — a value that reaches `hooks.json` by hand edit still carries no mode and still never fails a pane.
- Tests drive the command body, so every tmux-touching seam must be injected through `withHooksDeps` — `cmd`'s `TestMain` poisons `TMUX` package-wide and a missed injection dials a dead socket.

**Context**:
> The override is set where the registration is made: `portal hook set`, as the flag `--resume-mode eager|lazy`, alongside `--on-resume`. This follows from who writes registrations — the external `SessionStart` hook is a shell script, not a person at a screen, so the route has to be something a script can pass.
>
> The waiting panel is not the place for it: a panel offering "always resume this one without asking" would be setting a durable preference from a surface whose whole job is answering one instance of a question.
>
> A mode is always passed with the command it belongs to. `--resume-mode` on its own is refused — a registration is written whole and both stored shapes carry a command, so there is no entry a mode could attach to by itself.
>
> A mode the command cannot recognise is refused with it: `--resume-mode` takes `eager` or `lazy` and nothing else, so a mistyped pin fails where it was typed rather than landing on disk as a mode nothing reads.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §3.3, §2.2, §3.2

## lazy-resume-on-attach-1-7

### Task 1.7: portal hook list shows the mode in a fifth column

**Problem**: A pin can now be set and stored, and the only way to see one is to open `hooks.json` — configuration you can set and cannot check. `portal hook list` is the machine interface that reports what is registered, and it reports four columns: key, event, command, location.

**Solution**: Append the mode as a fifth tab-separated column, taken from the store read the listing already performs. The cell holds `eager` or `lazy` when the registration carries one and is empty when it does not — matching how the location column already reads when it has nothing to say. The install-wide default stays out of the listing entirely.

**Outcome**: `portal hook list` reports each registration's mode without disturbing the first four columns or adding a tmux read, so an existing positional parser is untouched and an empty fifth cell means the entry follows the install.

**Do**:
- Extend the row print in `cmd/hooks.go`'s `hooksListCmd` to five tab-separated fields, the fifth being the `Hook`'s mode rendered through its `String()` — which is empty for a registration carrying none.
- Take the mode from the `store.List` result that already produced the row: the listing keeps its single `ListAllPaneHookKeys` call for the location column, and none at all when there are no entries.
- Print no header, footer or summary line: the install-wide default is not part of this output.
- Update the expected-output strings in `cmd/hooks_test.go` (`TestHooksListCommand`, `TestHooksListLocationColumn`) for the extra field, and stage an object-form fixture through `hooksFileInTempDir`'s underlying `hookstest.StageStore` raw `Seed` for the new cases.
- Update the `hook list` line in the README's `hook` example block to describe the fifth column and what an empty cell means.
- Edit CLAUDE.md's **Resume-hook command** paragraph where it describes the location column as "a fourth tab-separated column after key/event/command": the location column is followed by a fifth holding the registration's mode — `eager`, `lazy`, or empty when it carries none — taken from the store read the listing already performs, so no second tmux read is added and the first four columns stay byte-identical for a positional parser.

**Acceptance Criteria**:
- [ ] Each row is five tab-separated fields ending in a newline, and the first four are byte-identical to today's output for the same store and the same live panes.
- [ ] An object-form registration carrying `eager` or `lazy` renders that word in the fifth field; a string-form registration renders an empty fifth field.
- [ ] An object-form registration whose stored `resume` is unrecognised, empty or absent renders an empty fifth field.
- [ ] The listing performs exactly one `ListAllPaneHookKeys` call when it has entries, and none when it has none — no second tmux read is added for the mode.
- [ ] Output holds one line per registration and nothing else: no header, no footer, no install-wide default.
- [ ] The existing key-then-event sort, the empty-output cases (no entries, absent file), and the empty location cell for a token no live pane carries are all unchanged.

**Tests**:
- `"it appends the registration's mode as a fifth column"`
- `"it renders an empty fifth column for a registration carrying no mode"`
- `"it renders an empty fifth column for an unrecognised stored resume value"`
- `"it leaves the first four columns unchanged for a positional parser"`
- `"it adds no second tmux read"` (one `ListAllPaneHookKeys` call with entries; a loud lister proves zero with none)
- `"it prints no line for the install-wide default"`
- `"it keeps the sort by key then event"` (existing, re-pinned over the five-column row)

**Edge Cases**:
- An unrecognised stored `resume` renders empty rather than echoing the unrecognised word — the column reports the mode the reader resolved, and the reader resolved none.
- The first four columns stay byte-identical, including the empty location cell, so a script filtering on the event column or reading `$3` is untouched.
- A run with no entries still prints nothing and makes no tmux read at all.
- The command is bootstrap-exempt and starts no tmux server, so a machine with no server still lists successfully with every location cell empty — unchanged by this task.

**Context**:
> A pinned registration is readable from `portal hook list`, as a fifth column. A mode that could only be seen by opening `hooks.json` would be configuration you can set and cannot check.
>
> The mode is appended as a fifth rather than inserted, so anything reading the first four positionally is untouched; this is how the location column itself arrived. The cell holds `eager` or `lazy` when the registration carries one and is empty when it does not, matching how the location column already reads when it has nothing to say. So the column reports what is stored, and an empty cell means the entry follows the install.
>
> The install-wide default is deliberately not in that listing: it is one value for the whole install rather than a property of any row, and the only place to put it is a header or footer line — which breaks naive parsers of a machine interface for a fact that does not vary between rows. It is reported by `portal doctor` instead, on an informational line, which is a later phase's work.
>
> The out-of-repo consumer that reads this output filters on the event column rather than parsing `hooks.json`, and the command lands in the same column out of either stored shape.

**Spec Reference**: `.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md` §3.4, §3.2
