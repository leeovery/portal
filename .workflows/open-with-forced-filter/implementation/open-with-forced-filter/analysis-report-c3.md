# Analysis Report: Open With Forced Filter (Cycle 3)

## Stats

- Total findings: 5
- Deduplicated findings: 5
- Proposed tasks: 4

## Summary

Three agents ran a full pass; standards found no conformance drift at all, duplication found one un-tied lockstep pair, and architecture found four seam-shape issues. Every finding is low severity, and they cluster: three of the four architecture findings are the same shape — a rule the `cmd` layer now holds that the type or the model it sits beside could hold instead — and the duplication finding is a lockstep pair of string literals in `cmd/init.go`. One architecture finding is discarded as a reversal of cycle 2's settled direction on the completer; the two comment corrections are collected verbatim below.

## Comment Corrections

- internal/tui/model.go:550 — `WithSearchForm`'s doc claims every search form lands on a committed filter, which is false for the term-less form the same option serves (it lands focused and empty)
  OLD: // which lands as the committed sessions filter on the Sessions page.
  NEW: // which lands on the Sessions page as the committed sessions filter — or, for
       // an empty term, as that filter opened focused and empty.

- internal/resolver/path.go:89-92 — `AbbreviateHome`'s doc calls the function "a pure string test", which overclaims: the result depends on `$HOME` (read through `os.UserHomeDir`), which is not an argument, and the consumer comment at internal/tui/session_dir_column.go:20-22 says the opposite
  OLD: // directory is rewritten to its `~/…` form and every other path is returned
  // unchanged. It is a pure string test — it never touches the filesystem — so a
  // path that does not exist abbreviates exactly as one that does, and a home
  // directory that cannot be resolved degrades to the path as given.
  NEW: // directory is rewritten to its `~/…` form and every other path is returned
  // unchanged. It never touches the filesystem, so a path that does not exist
  // abbreviates exactly as one that does; it does read the environment's home
  // directory, so one value abbreviates differently under a different $HOME, and
  // a home directory that cannot be resolved degrades to the path as given.

## Discarded Findings

- **The completer restates the searched-set rule instead of composing through PickerSessions** (architecture, low) — reverses a settled direction without grounds. Cycle 2's approved Task 1 weighed this exact route and settled against it in terms: the completer "works over a `[]string` of names rather than `[]tmux.Session`, so it cannot call `tui.PickerSessions` without changing `completionSessionNames`' shape and its test seam — settled as a skip rather than a reshaping, because what the findings show must not be re-authored is the *read* … not a compile-checked `name == current` equality." The finding recommends precisely that reshaping (enumerate sessions on the completion path and route them through `tui.PickerSessions`) and brings no new ground: no measurement, no specification entry and no project rule shows the settled direction wrong — its evidence is the same drift hypothetical the settled task considered and answered. Confirmed against the tree: `cmd/completion.go`'s skip is the `current != "" && name == current` equality cycle 2 described, reached through the single `currentPickerSession` read that task installed.
