# Investigation: Distinguish Transport Errors From Missing Option in GetServerOption

## Symptoms

### Problem Description

**Expected behavior:**
- `GetServerOption` should distinguish "option does not exist" from "tmux command failed for any other reason" (transport failure, permission error, server crash mid-call, etc.).
- `TryGetServerOption`'s `(value, found, err)` contract should match its docstring: option-absent returns `("", false, nil)`; any other failure returns `("", false, non-nil-err)`.
- Callers reading volatile markers (e.g. `@portal-restoring`) should be able to tell "marker absent" apart from "tmux call failed," so transport blips never get silently converted into a marker-state flip.

**Actual behavior:**
- `GetServerOption` (`internal/tmux/tmux.go:304-310`) collapses every `c.cmd.Run("show-option", "-sv", name)` error to the sentinel `ErrOptionNotFound`. The original error and its message are discarded.
- `TryGetServerOption` (`internal/tmux/tmux.go:317-326`) delegates to `GetServerOption`, then checks `errors.Is(err, ErrOptionNotFound)` first. Because that branch always matches for any non-nil error from `GetServerOption`, the subsequent `if err != nil { return "", false, err }` at line 322 is unreachable.
- Net effect: every transport/transient tmux failure is reported to callers as `("", false, nil)` — i.e., "option absent, no error."
- The package-level docstring at `internal/state/markers.go:34-35` documents the wrapper as "absence vs. real failure is distinguishable" — a claim the implementation cannot satisfy.

### Manifestation

This bug is *latent* — no user-visible incident has been reported. Manifestation is structural:

- **Misleading API contract:** `TryGetServerOption`'s docstring says it distinguishes absence from real failure; it does not.
- **Dead-code branch:** `internal/tmux/tmux.go:322-324` is unreachable, masking the conflation from readers auditing the code.
- **Silent caller misbehaviour potential:** `internal/state/markers.go:140` reads `@portal-restoring` via `TryGetServerOption` to gate daemon capture suppression during the restoration window. A transport blip during this lookup would be read as "marker absent," potentially flipping the daemon out of restoration-window suppression mid-bootstrap.

### Reproduction Steps

Direct reproduction in production is not realistic — local tmux socket failures are vanishingly rare. Reproduction is via inspection / fault injection in tests:

1. Construct a `Commander` mock that returns a non-tmux-unknown-option error from `show-option -sv @some-marker` (e.g., simulate `exit 1` with stderr `lost server`).
2. Call `client.TryGetServerOption("@portal-restoring")`.
3. Observed result: `("", false, nil)`.
4. Expected result: `("", _, non-nil-error)` so the caller can choose to abort the operation rather than treat the marker as absent.

**Reproducibility:** Always, given the fault-injection harness above. The bug is deterministic given the current code structure.

### Environment

- **Affected environments:** All (Darwin / Linux). The conflation is in pure Go logic with no platform branching.
- **Trigger conditions:** Any `cmd.Run` failure from `show-option -sv <name>` — either the option being absent (legitimate) or any other failure (the bug case).

### Impact

- **Severity:** Low (latent — no observed user impact).
- **Scope:** API surface in `internal/tmux` and one known consumer in `internal/state/markers.go`. Additional consumers may surface during code analysis.
- **Architectural impact:** Medium. A documented-but-unmet contract is depended on by load-bearing daemon code; silent contract drift increases the cost of future correctness audits.

### References

- Original inbox entry (now archived): `.workflows/.inbox/.archived/bugs/2026-03-28--distinguish-transport-errors-in-getserveroption.md`
- Source files: `internal/tmux/tmux.go` (GetServerOption, TryGetServerOption, ErrOptionNotFound), `internal/state/markers.go` (RestoringMarker reader and the package-level docstring asserting distinguishability).

---

## Analysis

*To be filled during Step 5.*

---

## Fix Direction

*To be filled during Steps 6–8.*

---

## Notes

- The original motivation (a hook-executor "two-condition check") no longer exists — hook firing migrated into the hydrate helper's exec chain. The original symptom site is gone; the architectural concern moved to marker-state reads in the daemon's restoration-window logic.
- **Investigation scope (user-confirmed):** wide. Verify the dead-branch claim, confirm `markers.go:140` consumer impact, enumerate *every* caller of `GetServerOption` / `TryGetServerOption` and assess each, and audit adjacent error-mapping patterns in `internal/tmux` (e.g., `ShowAllServerOptions`, `ListSkeletonMarkers`) for the same shape of bug.
