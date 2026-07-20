---
topic: cli-verb-surface-redesign
cycle: 4
total_proposed: 1
---
# Analysis Tasks: CLI Verb Surface Redesign (Cycle 4)

## Task 1: Refresh the stale `open` command help text to describe the redesigned verb
status: pending
severity: low
sources: standards

**Problem**: `cmd/open.go`'s top-level `openCmd.Use` (`"open [-e cmd] [destination] [-- cmd args...]"`) and `Short` (`"Open the interactive session picker or start a session at a path"`) still read like the pre-redesign single-destination command. They omit the entire attach / session-name capability, the domain pins (`-s`/`-p`/`-z`/`-a`/`-f`), and the multi-target absorb/net-N burst that absorbed `attach` + `spawn`. The redesign's governing principle is "the public surface names what happens," and bare `portal` now prints help/usage (spec § Bare `portal`), so the help surface has elevated importance — yet a user running `portal open --help` or `portal --help` gets a misleading picture of the flagship redesigned verb. The individual pin-flag descriptions (open.go:976-979) are already accurate; only the top-level `Use` line and `Short` summary are stale.

**Solution**: Update `Use` and `Short` to accurately name what the verb does now, and add a `Long` description covering the full surface, staying consistent with the accurate per-flag descriptions and the spec §405 Command Surface Summary row for `open` ("single public session verb; no-args → picker; flags -s/-p/-z/-a/-f, -e/--; absorb/net-N; hidden --ack"). Purely a help-metadata change — no behavioral change.

**Outcome**: `portal open --help` and `portal --help` describe the redesigned verb faithfully: bare/session-name attach, the domain pins, `-f`/`--filter`, `-e`/`--` command scoping, and multi-target opening — no longer implying a single path destination. Nothing about dispatch, resolution, or flag behavior changes.

**Do**:
1. In `cmd/open.go` (the `openCmd = &cobra.Command{...}` literal, ~line 151), change `Use` from `"open [-e cmd] [destination] [-- cmd args...]"` to reflect the target-set shape, e.g. `"open [targets…] [-- cmd args…]"`.
2. Change `Short` to name the current behavior, e.g. `"Open portals to one or more targets, or launch the interactive picker"` — wording that covers both attach and mint and the no-args picker.
3. Add a `Long` field to the command literal describing: no-args → interactive picker; bare positional target(s) resolved through the precedence chain (session name → path → alias → zoxide); the `-s`/`-p`/`-z`/`-a` domain pins and `-f`/`--filter` (skips resolution, opens the picker pre-filtered); `-e`/`--` command scoping (mint-only); and multi-target absorb/net-N (open N portals to N targets). Keep the copy consistent with the accurate per-flag strings at open.go:976-979 and the spec §405 row. The spec does not dictate exact copy, so match intent, not a golden string.
4. Do NOT touch `RunE`, `Args`, flag registration, dispatch, or any resolution logic — this task changes help-metadata strings only.

**Acceptance Criteria**:
- `portal open --help` output describes session-name attach, the domain pins (`-s`/`-p`/`-z`/`-a`), `-f`/`--filter`, `-e`/`--` command scoping, and multi-target opening; it no longer implies a single path `destination`.
- `portal --help` (bare `portal` → help) reflects the same accurate summary for `open`.
- `Use`/`Short`/`Long` copy is consistent with the accurate per-flag descriptions (open.go:976-979) and the spec §405 Command Surface Summary row.
- No behavioral, dispatch, or resolution change — only help-metadata strings differ; the full existing test suite (unit + integration) still passes.

**Tests**:
- Add or update a small `cmd`-package unit assertion that `openCmd.Short` (and, if present, `openCmd.Long`) mentions the redesigned capabilities — e.g. references to session targets and the domain pins — as a cheap guard against the help re-staling. If any existing test asserts on the literal `Use`/`Short` strings, update it to match the new copy.
- Manual verification: run `portal open --help` and `portal --help` and confirm the summary reads faithfully for the redesigned verb.
