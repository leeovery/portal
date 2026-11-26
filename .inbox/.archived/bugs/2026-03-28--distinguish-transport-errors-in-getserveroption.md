# Distinguish Transport Errors From Missing Option in GetServerOption

`GetServerOption` in `internal/tmux/tmux.go:304` still maps every Commander error to the sentinel `ErrOptionNotFound`. A genuine transport or connectivity failure (tmux server crashed mid-call, permission error, etc.) is indistinguishable from the option simply not existing.

## Updated state (as of 2026-05-12)

The hook-executor "two-condition check" that motivated the original note no longer exists — hook firing was moved into the hydrate helper's exec chain (`portal state hydrate`). The original scenario is gone.

However, the conflation has propagated into a new shape:

**`TryGetServerOption` (tmux.go:317-326) has a misleading contract.** Its docstring claims it distinguishes "absence from a real tmux failure (which surfaces as a non-nil error)." It does not — and cannot, given `GetServerOption` already swallows the distinction:

```go
func (c *Client) TryGetServerOption(name string) (string, bool, error) {
    val, err := c.GetServerOption(name)
    if errors.Is(err, ErrOptionNotFound) {
        return "", false, nil
    }
    if err != nil {           // ← unreachable
        return "", false, err
    }
    return val, true, nil
}
```

The `if err != nil` branch at line 322 is dead. Every error from `GetServerOption` matches `ErrOptionNotFound` because that's the only error it returns.

**New affected caller:** `internal/state/markers.go:140` uses `TryGetServerOption(RestoringMarkerName)` and relies on the (false) promise. A transport blip during a `@portal-restoring` lookup would be read as "marker absent" — silently flipping the daemon out of restoration-window suppression at the wrong moment. The package docstring at markers.go:34-35 makes this dependency explicit and assumes the wrapper delivers what it claims.

## Suggested approach

Inspect the Commander error output for tmux's "unknown option" / "invalid option" message and only return `ErrOptionNotFound` for that case; propagate other errors as-is. Then `TryGetServerOption`'s dead branch becomes live, and `markers.go` can treat transport failure as "skip this pane / abort capture cycle" rather than "marker absent."

## Why this is bug-shaped now, not just an idea

- `TryGetServerOption`'s docstring documents behaviour the code does not deliver — a contract violation, not a future improvement.
- The dead `if err != nil` branch is misleading on read and obscures the conflation from anyone auditing the code.
- The marker-read consumer at markers.go:140 silently inherits the wrong behaviour at a load-bearing site (restoration-window detection).

Practical risk surface remains low — tmux commands are local and fast — but the architectural mismatch is now documented in two places and depended on by a third.

Relevant files: `internal/tmux/tmux.go` (GetServerOption, TryGetServerOption), `internal/state/markers.go` (caller).
