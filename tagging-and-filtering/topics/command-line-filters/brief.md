# Discovery Brief — Command Line Filters

Drawn from discovery session 001.

## Soft decisions

- In scope, accepted by the user: a command-line way to reach sessions by tag (and by project), e.g. `x --tag work`. It picks up a thread explicitly deferred when tagging first shipped.
- Tags live on projects only; a session carries its project's tags.

## Rejected paths

(none)

## Open questions

- Whether `x --tag work` opens the picker already narrowed, or opens every matching session at once — the latter going through the existing open-several-windows path.
- How it sits beside the existing `-f/--filter` flag (opens the picker pre-filtered with text) and the `x /term` search.
- Shell completion for tag names.
- Whether the any/all and exclude choices from the picker's filtering carry over to the command line, and whether a project can be named the same way.
