TASK: open-with-forced-filter-5-6 (tick-a33475) — "Corrections": one prose pass over README.md's `x (open)` section — re-cast the Tab-completion paragraph's release framing as a standing statement about `portal init`, and replace the positional "The last three rows are not pins" with the rows named.

ACCEPTANCE CRITERIA:
1. The Tab-completion paragraph carries no "This release", no "until now", and no sentence that reads as an upgrade step owed against a particular version.
2. The durable rollout fact survives as a property of `portal init`: completion reaches a shell only once that shell has evaluated its current output (new shell, or re-run the `eval`).
3. The paragraph still states what `x <TAB>` and `x /po<TAB>` offer (the slash staying in place), the deliberate loss of filename fallback after the function, and the shell condition on switching that fallback off (bash 4+ with `bash-completion`, zsh, or fish; macOS's stock `/bin/bash` 3.2 cannot).
4. `README.md:161` names `-f`, `/<term>` and `-e`/`--`, and no positional claim about the table's rows remains anywhere in the section.
5. `README.md` is the only file changed: no source, test or specification edit, and no behaviour change.
6. The section still carries the tokens its guard reads — `portal init`, `` `/<term>` ``, `-p /tmp`, `-f, --filter` — and the commented `x /port` and `x /` example lines.

STATUS: complete

SPEC CONTEXT: Specification §9.2 lists what the documentation must carry, including "That the corrected completion reaches an existing install only once the output of `portal init` is re-evaluated — a new shell, or re-running `portal init` (§8.3, §8.5)", and §9.2's closing paragraph assigns the completion correction and its rollout consequence to the README's `x (open)` section because they belong to `portal init` rather than to `open` and have nowhere to live in a command's help. §9.3 forbids a CHANGELOG entry. The spec states the rollout fact in release-relative wording ("an existing install", "the corrected completion"); this task deliberately re-states the same fact version-independently, which keeps the substance §9.2 requires while removing the framing that decays. That is a sound divergence in wording only — the durable obligation on the reader is unchanged.

IMPLEMENTATION:
- Status: Implemented
- Location: single commit `287c6c51e`, two lines of `README.md`:
  - `README.md:161` — "**Domain pins** skip the chain and force one domain — a pin that misses hard-fails without popping the picker. `-f`, `/<term>` and `-e`/`--` are not pins:" (was "The last three rows are not pins").
  - `README.md:175` — "**Tab completion.** Tab after the session-opening function asks Portal for `open`'s completions: … **A shell has that behaviour only once it has evaluated the current output of `portal init`** — start a new shell, or re-run the `eval` in your profile; the `/term` form itself is `portal open`'s own parsing, independent of the shell integration. The deliberate trade beside it: a path argument after the function does not fall through to filename completion …".
- Notes: criterion-by-criterion —
  1. Met. `grep -nEi "this release|until now|the new binary|no longer falls|picks the correction" README.md` returns nothing, whole file. The rollout sentence is now a standing property ("A shell has that behaviour only once …"), not an owed upgrade step.
  2. Met, verbatim in substance: the new-shell / re-run-the-`eval` pair survives attached to `portal init`.
  3. Met. `x <TAB>` → live session names; `x /po<TAB>` → the term after the slash, "leaving the slash in place"; the filename-fallback trade and the bash 4+/zsh/fish vs `/bin/bash` 3.2 parenthetical are byte-unchanged from the pre-task text apart from "no longer falls" → "does not fall" and "loss" → "trade", both of which are the de-versioning the task's Do list prescribed.
  4. Met. Line 161 names all three forms. Scanning the whole section (README.md:120-177) for positional language finds only "first match wins" (the resolution chain, README.md:137), "cursor on the first match" (README.md:148) and "this terminal becomes the first surface" (README.md:173) — none is a claim about the table's rows. `README.md:134`'s "(see below)" points at the Multi-window bursts paragraph, not a row.
  5. Met. `git show 287c6c51e --stat` = `README.md | 4 ++--`, one file, two insertions, two deletions. No source, test, spec or testdata file touched; no behaviour change is possible from a README edit.
  6. Met. `portal init` (README.md:175), `` `/<term>` `` (README.md:161 and the table row README.md:170), `-p /tmp` (README.md:155), `-f, --filter` (README.md:169); the fenced block carries `x /port  # search live sessions for "port"` and `x /  # open the picker with the filter ready to type`.
- The table itself (README.md:163-171) and every other sentence in the section are byte-unchanged, as the task required — confirmed by the diff having exactly two changed lines.
- One later commit (`936f540cb`, task 8-2) re-worded the middle of README.md:175 to state the search completer's offered set ("the sessions a search can reach — every live session but the one you are attached to"). That is a separate task's authorised change and leaves this task's three properties (no release framing, the `portal init` standing statement, the filename-fallback trade with its shell condition) intact in the current file.
- The retained claims still hold against the code: `completeOpenPositional` (`cmd/completion.go:104`) returns `ShellCompDirectiveNoFileComp` via `completeSessionNames`/`completeSearchTerm` (`cmd/completion.go:39`, `cmd/completion.go:82`) for a pre-separator positional, which is the "does not fall through to filename completion" the paragraph claims; the bash shim emits `complete -o default -F __start_portal_open` (`cmd/init.go:105`), so turning the fallback off at runtime needs cobra's `compopt`, which is exactly the bash-3.2 carve-out the parenthetical states; and `/term` recognition is `resolver.IsSearchSigil` inside the binary, so "independent of the shell integration" is true.

TESTS:
- Status: Adequate (none owed — prose only, behaviour unchanged, and the task's Tests section explicitly requires no new or changed test)
- Coverage: `TestReadmeDocumentsSearchForm` (`cmd/open_docs_test.go:68`) is the standing guard over this section. Read against the current README it still passes: `readmeOpenSection` finds the `### `x` (open)` heading and cuts at the next `\n### ` (`### `xctl list``, README.md:179), so the whole edited region is in scope; `hasCommentedExample` matches the `x /port` and `x /` comment lines in the fenced block; and all four tokens in its map (`` `/<term>` ``, `-p /tmp`, `portal init`, `-f, --filter`) are present at the lines cited above. The file is untouched by this commit, as the task required.
- Notes: no over-testing — the guard asserts token presence rather than prose wording, which is precisely what lets a re-wording task like this one land without editing a test. Nothing about a prose re-cast is testable beyond that.

CODE QUALITY:
- Project conventions: Followed. No code changed. CLAUDE.md's documentation conventions are respected (no CHANGELOG entry — spec §9.3 and the project's own rule both forbid one; no version or release references left in user-facing copy).
- SOLID principles: N/A (documentation-only change)
- Complexity: N/A
- Modern idioms: N/A
- Readability: Good. Both replacements read as standing statements: the domain-pin sentence now names the three non-pin forms exactly as the table spells them (`-f`, `/<term>`, `-e`/`--`), so adding a pin row or reordering the table cannot make the sentence name the wrong rows; the completion paragraph opens on what Tab does rather than on what a release changed.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None. Every criterion is settleable by reading: five are properties of the README text, and the sixth (the guard's tokens) is a pure string-containment assertion whose inputs — `cmd/open_docs_test.go` and `README.md` — were both read in full.
