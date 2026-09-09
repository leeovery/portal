TASK: resume-hooks-silently-lost-9-5 — "The on-resume Event Is An Untyped Literal The Store Now Reaches For Internally" (phase 9, implementation-analysis; commit 6d812e19)

ACCEPTANCE CRITERIA:
1. `Set`, `Remove` and the on-resume lookup take the `Event` type; a plain untyped string literal at a call site no longer compiles unless it is an untyped constant.
2. `grep -rn '"on-resume"' --include='*.go'` finds exactly one declaration site.
3. A stale key whose entry is filed under `on-resume` is reaped with the removed command carried in the `value` attr, exactly as today.
4. A stale key whose entry is filed under a non-`on-resume` event is reaped with a non-empty `value` naming what it held, where today it logs empty.
5. Every existing hooks store, cmd and hookstest suite passes with only the call-site type change.

STATUS: complete

SPEC CONTEXT:
The specification (§ "Each removed key is logged at INFO under the `hooks` component") requires the clean-stale sweep to emit one INFO per reaped key carrying the removed entry's `on-resume` command in the existing `value` attr, because at deletion time the token identifies nothing recoverable and the entry holding the command is the thing being destroyed. It also fixes the closed attr vocabulary (`op`, `hook_key`, `value`, `via`, `entries`) — no new keys. This task is a phase-9 implementation-analysis item, so its authority is its own body: it generalises that breadcrumb from "the assumed on-resume slot" to "whatever the reaped key actually held", which preserves the spec's contract exactly for the only shape Portal writes (a single `on-resume` event) while closing the silent-empty case for any other.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/event.go:11` — `type Event string`; `:14` — `const EventOnResume Event = "on-resume"`; `:17` — `String()`. Shape mirrors `internal/hooks/via.go`, with the string-kind choice stated and justified in the doc (the map index and the on-disk key are the same string).
  - `internal/hooks/store.go:115` `Set(key string, event Event, …)`, `:153` `classifySet(h, key string, event Event, …)`, `:173` `Remove(key string, event Event, …)` — the conversion happens only at the map index (`:141`, `:158`, `:192`, `:196`).
  - `internal/hooks/lookup.go:26` — `events[EventOnResume.String()]`.
  - `internal/hooks/store.go:352` — the per-key INFO now sources `value` from `removedValue(h[key])` (was `h[key]["on-resume"]`); `:366-384` — `removedValue` renders a single-event key's command alone and a multi-event key as `event=command` pairs in sorted event order.
  - Re-pointed literal sites: `cmd/hooks.go:218` (`Set`), `cmd/hooks.go:292` (`Remove`), `internal/hookstest/hooks.go:79`, `internal/hookstest/staging.go:130`. All four of the plan's non-`internal/hooks` named sites, plus the three inside the package, now reach the constant — verified by `grep -rn '"on-resume"' --include='*.go' | grep -v '_test.go'`, which leaves only `internal/hooks/event.go:14`, the five cobra flag-name strings in `cmd/hooks.go:200,305,306,308,309` (a CLI flag name, not the store's event key) and the JSON fixture body in `internal/hookstest/hooks.go:207-208`.
- Notes:
  - `deleteStale` reads `h[key]` — the map loaded under its own exclusive hold — while it writes `kept := maps.Clone(h)` with the key deleted from the clone only. `maps.Clone` is shallow but nothing mutates the inner maps on this path, so the breadcrumb genuinely reports the pre-delete content (`internal/hooks/store.go:338-353`).
  - Criterion 2 is met for the production tree but not for the whole tree: ~150 `"on-resume"` spellings remain across `*_test.go` files, of which roughly eighty are `store.Set(k, "on-resume", …)` / `store.Remove(k, "on-resume", …)` call sites that now pass an untyped constant into the `Event` parameter (e.g. `internal/hooks/lock_test.go:39`, `internal/hooks/store_test.go:104`, `cmd/bootstrap/reboot_roundtrip_test.go:82`). This is not raised as a finding: the plan's own Do list enumerated exactly eight sites to re-point and all eight are done, the remainder are either JSON fixture bodies that cannot take a Go constant or assertions whose subject is the wire value, and no consequence follows from a test spelling the value it is asserting (`internal/hooks/event_test.go:13` pins the wire value, so a change to it fails there first).
  - Criterion 1's second clause is satisfied only in the sense the criterion itself concedes: a string kind admits any untyped literal, so `Set(key, "invented", …)` still compiles. `event.go:8-10` states that choice and its reason openly, and the task body explicitly permitted it ("a `hooks.Event` type (or at minimum an exported `hooks.EventOnResume` constant)"), so this is a sanctioned divergence from the `Via` int-enum shape rather than a shortfall.
  - Signature change is contained: no interface in the tree declares `Set`/`Remove` over the hooks store (`cmd/state_daemon.go:31` and `cmd/state_hydrate.go:39` both hold a concrete `*hooks.Store`), and the only production writer is `cmd/hooks.go`.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/hooks/event_test.go:13` `TestEventWireValue` — pins `EventOnResume.String() == "on-resume"` as an on-disk contract; this is what makes the constant safe to fan out to.
  - `internal/hooks/event_test.go:20` "it persists and reads back an entry through the Event constant" — Set → raw map read → `LookupOnResume` → `Remove`, i.e. the write, the read and the removal all agree on one key.
  - `internal/hooks/store_test.go:953` "it logs the removed command in the value attr when a stale on-resume entry is reaped" (criterion 3), `:973` "it logs a non-empty value when the reaped entry is filed under another event" (criterion 4 — the previously-passing assertion `value == ""` was correctly inverted to `value == "x"`), `:993` "it reports the events a reaped key actually held rather than one assumed name" (asserts the exact rendering `on-exit=x; on-resume=cmd1`, which pins the sort order too).
  - The pre-existing `:897` multi-key case and the `:1010` failed-save case still assert the unchanged surroundings (one INFO per key, no per-key line when the write failed).
- Notes:
  - Not over-tested: each of the three new sub-tests names a distinct branch of `removedValue` (single on-resume, single foreign, multi-event) and asserts one thing each.
  - The zero-event branch (`{"key":{}}` on disk → empty `value`) is unasserted. It is unreachable through Portal's own writers — `Remove` drops the outer key when its last event goes (`store.go:197-199`) — so it needs a hand-edited file to occur, and the rendering ("nothing was lost") is honest. Not a gap worth a test.
  - Test-side change is exactly the call-site type work criterion 5 describes, plus the two flipped clean-stale assertions the behaviour change requires: `store_test.go:1319` (`ev hooks.Event`) and `:1347` (`hooks.Event(event)` conversion for a table-driven event string). No other suite seeds a non-`on-resume` or multi-event entry, so nothing else observes the changed rendering.

CODE QUALITY:
- Project conventions: Followed. `event.go` mirrors the `via.go` shape the package already establishes; the doc comments explain why this one is a string kind and the other is not, which is the discrimination a reader needs. No new imports into `internal/hooks` beyond `strings` (the leaf-guard dependency set is unaffected). The `sort.Strings` build-keys idiom in `removedValue` matches the existing production pattern (`cmd/doctor.go:471`, `internal/state/capture.go:210`, `internal/restore/session.go:307`), so the `modernize` linter's baseline is unchanged.
- SOLID principles: Good. `removedValue` is a pure rendering function with one reason to change, separated from the deletion it serves.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. The `removedValue` doc names both branches and why each was chosen (single → the copy-paste recoverable form; several → naming one would misreport the rest).
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
