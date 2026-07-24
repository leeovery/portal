# Plan: Custom-Terminal Docs And Clickable See-Docs

## Phase 1: Apply Change

Create the dedicated `terminals.json` custom-terminal setup docs page and make the
picker's named-unsupported banner `see docs` hint a clickable OSC 8 link to it.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| custom-terminal-docs-and-clickable-see-docs-1-1 | Create custom-terminal docs page + trim README to a pointer | Keep the `terminals.json` Configuration-table row; leave other intra-README `#configuration` links as-is |
| custom-terminal-docs-and-clickable-see-docs-1-2 | Make the banner `see docs` hint a clickable OSC 8 link + update test/fixtures | OSC 8 emitted unconditionally (rides `NO_COLOR`); zero-width escape must not perturb width/one-row/degrade; never print a URL/path |
