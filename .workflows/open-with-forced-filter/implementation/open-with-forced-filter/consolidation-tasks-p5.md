# Consolidation Tasks: open-with-forced-filter (Phase 5)

## Task 1: Keep the user's cursor position through the bash completion shim
placement: phase 5
severity: behaviour

**Problem**: `bashOpenCompletionShim` (`cmd/init.go:54-61`) rewrites the word array to `portal open …`, then replaces the command line with the re-joined array and pins the cursor offset to its end:

```
COMP_LINE="${COMP_WORDS[*]}"
COMP_POINT=${#COMP_LINE}
```

`COMP_POINT` is bash's byte offset of the cursor *into* `COMP_LINE`, and the shell's completion machinery derives the current word from that pair, not from `COMP_CWORD`: cobra's generated function calls `_init_completion` (falling back to its own `__portal_init_completion`), and both route through bash-completion's `_get_comp_words_by_ref` → `__get_cword_at_cursor_by_ref`, which truncates the line at `COMP_POINT` to produce `cur`; cobra then feeds that `cur` to `compgen -W "${completions[*]}" -- "$cur"` (verified in cobra v1.10.2: `bash_completionsV2.go:44-50`, `:311-317`, `:432-438`). Pinning the offset to end-of-line is only true when the user's cursor was already there. Move the cursor back to fix an earlier word on an `x` line and press Tab, and `cur` becomes the whole tail of the line from that word onward, so no candidate matches it: Tab then yields nothing where the shell honoured `ShellCompDirectiveNoFileComp` (cobra gates `compopt +o default` on `compopt` being a builtin, `bash_completionsV2.go:132-138`, and runs it *before* the `compgen` filter — so the fallback is already off by the time the match fails), or filenames where it could not and the standing `complete -o default` supplied them — while `xctl` on the same line in the same shell completes correctly, because it is registered on `__start_portal` directly. Cobra supports the moved-cursor case deliberately (`bash_completionsV2.go:444-448`); the shim defeats half of that support — `cword` stays right, `cur` goes wrong. The same rewrite also normalises the user's spacing, since `"${COMP_WORDS[*]}"` re-joins with a single space, so `x   api` is reported as a line the user never typed. The zsh shim (`cmd/init.go:63-68`) has no equivalent defect — it adjusts `words`/`CURRENT` only and zsh derives `PREFIX`/`SUFFIX` from the real cursor — and fish's `-w 'portal open'` wrap never touches the line. This is one shell of the three, and the one the phase hand-wrote the most machinery for.

The defect's reach is the bash that has the `bash-completion` package: cobra's completion function calls `_init_completion`, falling back to its own thin wrapper around `_get_comp_words_by_ref` — both from that package — so a bash without it returns early (`bash_completionsV2.go:432-438`) and Tab after the function falls through to filenames for a different reason entirely, defect or no defect. That is the same condition the specification now carries for the filename-fallback switch-off (Corrigendum 2026-09-14, §2.2 and §8.3): the configurations where Portal answers a bash Tab at all are exactly the ones where this defect is visible.

**Solution**: Keep the `COMP_LINE` rewrite — it is load-bearing, because the cursor walk matches the line against the rewritten word array and a stale line yields an empty `cur` — but shift the offset rather than pinning it, and substitute the prefix into the line the user typed rather than re-joining the array, so their spacing survives: capture the typed function name, replace it with `portal open` in `COMP_LINE`, and add the length difference to the existing `COMP_POINT`. State the treatment of a line with leading whitespace, where a plain prefix strip removes nothing — handle it or state the limit in a comment beside the shim.

Two obligations the edit carries with it:

1. **Measure before changing.** This machine has no `bash-completion` installed, so the derivation above is read off cobra's and bash-completion's sources rather than observed. Establish the behaviour directly — on a machine or container with the package present — before the shim is touched, and record what was measured. If the measurement contradicts the derivation, the finding is void and the task closes with that recorded.
2. **Give it a guard that can fail.** `cmd/init_completion_shell_test.go:32-38`'s `_init_completion` stand-in derives `cur` from `COMP_WORDS[COMP_CWORD]` alone and `:44-45` sets the driver's own `COMP_POINT` to end-of-line, so the existing suite models away precisely the half of bash-completion the shim disagrees with and structurally cannot fail for this reason. Teach the stand-in the cursor-aware derivation (walk `COMP_LINE` to `COMP_POINT` as `__get_cword_at_cursor_by_ref` does) and drive one mid-line case. The shim reaching this state unnoticed is what an unguarded fix would repeat.

**Outcome**: Tab on an `x` line completes from the word under the cursor wherever the cursor is, matching what `portal open` and `xctl` already do, and the emitted line preserves the user's own spacing — with a suite case that fails against the shim as it stands today, and the measurement behind the change on the record.

**Do**:

- **Measure first, and record it.** On a bash that has the `bash-completion` package (a container, or a local install — this machine has none: no `bash_completion` under `/usr/share` or Homebrew's `etc`), stage what `cmd/init_completion_shell_test.go` already stages minus the `_init_completion` stand-in — the emitted `portal init bash` script sourced over the recording `portal` stub on `PATH`, `BASH_COMP_DEBUG_FILE` set — declare **no** `_init_completion` so the real package supplies it, set `COMP_WORDS=(x /po extra)` / `COMP_CWORD=1` / `COMP_LINE='x /po extra'` with `COMP_POINT` at the end of `/po`, call `__start_portal_open`, and read back the `cur` cobra traced, the request the stub recorded and `COMPREPLY`. No portal binary is built or run and no tmux server is contacted. If the measured `cur` is the word under the cursor rather than the tail of the line, the finding is void: leave `cmd/init.go` untouched and close the task with the measurement recorded.
- **Teach the guard the derivation it models away.** In `cmd/init_completion_shell_test.go`, replace the `_init_completion` stand-in's `cur=${COMP_WORDS[COMP_CWORD]}` with the cursor-aware one — locate `COMP_WORDS[COMP_CWORD]`'s offset in `COMP_LINE` by scanning the words in order, then take `cur` as the slice of `COMP_LINE` from that offset up to `COMP_POINT` — and give `driveCompletion` a way to state where the cursor sits, defaulting to end-of-line so every existing call site is unchanged; the bash driver derives `COMP_POINT` and `COMP_CWORD` from it instead of pinning both to the last word.
- **Add the mid-line case** (bash only — zsh and fish have no equivalent defect): the line `x /po extra` with the cursor at the end of `/po`, asserting both that the request is `__complete|open|/po` and that the reply offers the stub candidate. Confirm it fails against the shim as it stands before editing the shim.
- **Shift the offset instead of pinning it** in `bashOpenCompletionShim` (`cmd/init.go:54-61`): capture the typed function name from `COMP_WORDS[0]` before the array is rewritten, substitute `portal open` for it in `COMP_LINE` rather than re-joining the word array, and add the length difference to the existing `COMP_POINT`. Handle a line with leading whitespace, where a plain prefix strip removes nothing, or state that limit in one comment line beside the shim.
- **Move the shim's verbatim expectation with it** — `cmd/init_test.go:146-149` holds the emitted text byte-for-byte — and add an emitted-text assertion that `COMP_POINT` is shifted rather than assigned.

**Acceptance Criteria**:

- [ ] The measurement is recorded in the task's implementation record: the bash and `bash-completion` versions, the `cur` cobra saw for a mid-line Tab before the edit, the request the stub recorded, `COMPREPLY`, and whether file completion supplied filenames in its place.
- [ ] A measurement contradicting the derivation closes the task with `cmd/init.go` unedited and that recorded; no further criterion applies in that case.
- [ ] The driver's `_init_completion` stand-in derives `cur` from `COMP_LINE`/`COMP_POINT`, and every existing end-of-line case in `cmd/init_completion_shell_test.go` still passes unchanged.
- [ ] The mid-line case fails against the pre-edit shim and passes after it, with both runs recorded.
- [ ] The emitted bash script carries no `COMP_POINT=${#COMP_LINE}`: the offset is the user's own plus the length difference between `portal open` and the typed name.
- [ ] The rewritten `COMP_LINE` preserves the user's spacing — `x   api` becomes `portal open   api`, not `portal open api`.
- [ ] A line with leading whitespace either completes correctly or has its limit stated in one comment line beside the shim.
- [ ] `zshOpenCompletionShim`, the fish emission, and both `__start_portal` registrations (`portal`, `<name>ctl`) are byte-unchanged.
- [ ] `go test ./cmd` is green, including the verbatim shim expectation in `cmd/init_test.go`.

**Tests**:

- `"it completes the word under a mid-line cursor on an x line"`
- `"it asks portal for the word under the cursor, not the tail of the line"`
- `"it leaves an end-of-line completion unchanged"`
- `"it preserves the user's own spacing in the rewritten line"`
- `"it shifts COMP_POINT rather than pinning it to the end of the line"`
- `"it completes the word under the cursor on a line with leading whitespace"` (only where that case is handled rather than stated as a limit)

## Task 2: Corrections
placement: phase 5
severity: corrections

**Problem**: The README's `x (open)` section carries two claims that ordinary change falsifies silently, both authored by this phase and both recorded by the 5-4 reviewer without being raised, because the task's acceptance criteria were met as written.

**Solution**: One prose pass over `README.md`, no code:

- `README.md:175` — re-cast the paragraph's opening as a standing statement rather than a release note. "**This release corrects `portal init`** … **until now** the request it emitted was answered by the shell instead" cannot be read two versions later: a reader cannot tell which release is meant, whether "until now" describes their binary, or whether the re-evaluation instruction is an upgrade step they still owe — so they either re-run the eval needlessly or treat the whole paragraph as history and skip the part that is permanently true. The durable fact must stay: completion reaches a shell only once that shell has evaluated the current `portal init` output (start a new shell, or re-run the `eval`). State it as a property of `portal init`, dropping "This release", "until now" and the before/after framing. Everything downstream in the paragraph — what `x <TAB>` and `x /po<TAB>` offer, the deliberate filename-fallback loss, the bash-version parenthetical — is already timeless and stays as it is.
- `README.md:161` — replace "The last three rows are not pins" with the rows named: `-f`, `/<term>` and `-e`/`--` are not pins. The positional form is correct only for the table's present seven rows; adding a pin, or another non-pin row anywhere but the tail, makes the sentence name the wrong rows and a reader then mis-reads which forms hard-fail on a miss. No plausible guard covers a positional claim, which is why the wording carries it instead.

**Outcome**: The `x (open)` section reads the same at any version — the Tab-completion paragraph states what `portal init` emits and what a shell must have evaluated for it to apply, and the domain-pin sentence names its non-pin rows rather than counting them — with behaviour, the table's seven rows and every test untouched.

**Do**:

- Re-cast the opening of the **Tab completion.** paragraph (`README.md:175`) as a standing statement: Tab after the session-opening function asks Portal for `open`'s completions, and a shell has that behaviour only once it has evaluated the current `portal init` output — start a new shell, or re-run the `eval` in your profile. Drop "This release", "until now", and the before/after framing around them.
- Keep the rest of that paragraph's substance as it is — what `x <TAB>` and `x /po<TAB>` offer, the deliberate filename-fallback loss, the bash-version parenthetical — dropping only the residual "now" and "the new binary" that belong to the framing above; the `/term` form's independence from the shell integration is durable and stays, stated as the binary's own parsing rather than as a version's.
- At `README.md:161`, name the rows: `-f`, `/<term>` and `-e`/`--` are not pins. Leave the table itself and every other sentence in the section byte-unchanged.

**Acceptance Criteria**:

- [ ] The Tab-completion paragraph carries no "This release", no "until now", and no sentence that reads as an upgrade step owed against a particular version.
- [ ] The durable rollout fact survives as a property of `portal init`: completion reaches a shell only once that shell has evaluated its current output (new shell, or re-run the `eval`).
- [ ] The paragraph still states what `x <TAB>` and `x /po<TAB>` offer (the slash staying in place), the deliberate loss of filename fallback after the function, and the shell condition on switching that fallback off (bash 4+ with `bash-completion`, zsh, or fish; macOS's stock `/bin/bash` 3.2 cannot).
- [ ] `README.md:161` names `-f`, `/<term>` and `-e`/`--`, and no positional claim about the table's rows remains anywhere in the section.
- [ ] `README.md` is the only file changed: no source, test or specification edit, and no behaviour change.
- [ ] The section still carries the tokens its guard reads — `portal init`, `` `/<term>` ``, `-p /tmp`, `-f, --filter` — and the commented `x /port` and `x /` example lines.

**Tests**:

- No new or changed tests: prose only, behaviour unchanged.
- `"it documents the search form"` — the existing `TestReadmeDocumentsSearchForm` (`cmd/open_docs_test.go`) stays green over the re-worded section, unmodified.
