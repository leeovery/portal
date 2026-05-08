AGENT: architecture

STATUS: clean
FINDINGS_COUNT: 0
SUMMARY: Architecture is sound — clean boundaries (state/tui/cmd seams), constructor injection on previewModel, no package-level mutable state in internal/tui, single dispatch point for the (bytes, err) Tail tri-shape, load-bearing ordering of preserveName capture before m.preview zeroing on dismiss is explicit and commented, single-fd invariant preserved in TailScrollback, and resize-vs-read decoupling matches the spec's performance budget.
