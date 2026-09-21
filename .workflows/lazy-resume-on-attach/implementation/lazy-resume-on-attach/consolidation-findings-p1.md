# Consolidation Findings: lazy-resume-on-attach (Phase 1)

## Findings

### F1: `Store.Set` honours a registration's retained raw bytes, so a read-modify-write writes the pre-edit value under a breadcrumb claiming it wrote the new one
- **Class**: behaviour
- **Failure**: A caller that loads a registration, adjusts it and hands it back to `Set` writes the bytes it loaded, not the bytes it built. The classification and the audit trail both report the edit as landed (`sameRegistration` compares `Command` and `Resume` only, so a mode-only change classifies `modify`, the file is rewritten, and an INFO `modify` line is emitted carrying the command), while the entry on disk is byte-identical to what it was. Noticed by the user as a pin that silently does not take — a pane that comes back eager after being set lazy — with a log line saying it did; or as a `hook list` fifth column that never changes. Nothing in the tree fails: no test covers a raw-carrying registration reaching `Set`, and nothing can compile-refuse one, because `raw` is unexported and a plain struct copy of a loaded `Registration` carries it.
- **Evidence**:
  - `internal/hooks/registration.go:17-22` — the unexported `raw json.RawMessage`, populated on every decode.
  - `internal/hooks/registration.go:30-31` — `UnmarshalJSON` sets `raw` for **every** stored shape, the string form included, so every value in a loaded `Snapshot` carries it.
  - `internal/hooks/registration.go:52-54` — `MarshalJSON` short-circuits to `r.raw` whenever it is non-empty, ahead of the two content-derived shapes.
  - `internal/hooks/store.go:160` — `h[key][event.String()] = registration` stores the handed value directly; nothing re-projects it.
  - `internal/hooks/store.go:171-179` — `classifySet` / `sameRegistration` judge on `Command` + `Resume` alone, so the verdict and the breadcrumb describe the intended write while `save` emits the retained bytes.
  - `internal/hooks/store.go:40-45` (`Load`) — the exported route that hands out raw-carrying values; `internal/hooks/store.go:389-407` (`removedValue`) and `internal/hooks/lookup.go:41` already read such values field-wise, so a caller has every reason to believe the struct is a plain value type.
  - Both current non-test callers construct fresh: `cmd/hooks.go:222` (`hooks.Registration{Command: command, Resume: mode}`) and `internal/hookstest/hooks.go:79`. There is no live read-modify-write today — this is a latent trap the phase created, not a reproducible defect.
- **Proposed shape**: Re-project inside `Set` before storing, so the method's own contract — "nothing the replaced value held survives a rewrite it was not given" (`internal/hooks/store.go:129-133`) — is enforced by the store rather than assumed of every caller: `h[key][event.String()] = Registration{Command: registration.Command, Resume: registration.Resume}`. Behaviour for every existing caller is unchanged (they pass no raw), and the preservation property is untouched — untouched siblings still come out of `h` carrying their own raw. A `NewRegistration` constructor is the weaker alternative: it does not foreclose the struct-copy path, which is the one a caller reaches by accident. In the same edit, replace the claim about callers in the `raw` field comment (`internal/hooks/registration.go:19-21`, "A registration a mutation builds carries none, so it is written in the shape its own content chooses") with the fact the code then enforces — a mutation's value is re-projected before it is stored — since as written it asserts a property of distant code that nothing checks.
- **Bank**: `lazy-resume-on-attach-1-3` (reviewer) — "Store.Set accepts a Registration that may still carry its decoded raw bytes, so a read-modify-write caller silently writes the pre-edit value." Confirmed against the final state: `Set` still stores the handed value directly and `MarshalJSON` still prefers `raw`; tasks 1-4 through 1-7 added no re-projection. Its suggested fix (re-project inside `Set`) is the shape proposed above.

## Comment Corrections

- `internal/hooks/registration.go:27-29` — the `UnmarshalJSON` doc states one admission rule for both object attributes; `command` is admitted for any JSON string (`stringAttribute(object["command"])`, line 44), and only `resume` is checked against the vocabulary (line 45).
  OLD:
  ```
  // the file for its neighbours. A JSON string is the command alone; a JSON
  // object's command and resume are read when each is a string the vocabulary
  // admits, and anything else leaves that field unset.
  ```
  NEW:
  ```
  // the file for its neighbours. A JSON string is the command alone; a JSON
  // object's command is read when it is a string and its resume when it is one
  // the vocabulary admits, and anything else leaves that field unset.
  ```
