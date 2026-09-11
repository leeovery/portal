# Review Tracking: Open With Forced Filter - Claims Verification

## Findings

### 1. Tab after the `x` function does not ask Portal's completer anything — it runs a real `portal open` command

**Source**: Tree measurement — `portal init bash | grep -n 'requestComp='` and a stubbed drive of the emitted `__start_portal` function
**Category**: Source defect
**Move**: route
**Affects**: §8.2 (the pre-existing completion defect and its measured probe table)

**Problem**:
The specification tells the reader that pressing Tab after the `x` shell function today offers Portal's top-level subcommand names — `alias`, `doctor`, `hook`, `init`, `kill`, `list`, `open`, `theme` — and that `x -s <TAB>` falls through to filenames because Portal's root completer rejects `-s`. Neither is what the shipped shell integration does.

In bash and zsh the emitted completion script asks the *typed word* for its completions: it builds the request as `<typed-word> __complete <args>`. The typed word is the `x` function, and `x` expands to `portal open`. So Tab after `x` executes `portal open __complete …` — an actual `open` invocation with `__complete` as a positional target — rather than asking Portal's completer anything. Portal's root completer is never consulted on that path, so the subcommand list it would return never reaches the user, and the filename fallback the reader is told about arrives because the request produced no completion output at all, not because the root completer rejected `-s`.

The same mechanism is what makes the sibling `xctl` function complete correctly — `xctl` expands to bare `portal`, so its request really is `portal __complete …`. The specification's account cannot distinguish the two: both functions carry the identical registration line, so "the completer is registered against `portal`" predicts identical behaviour for a pair that measurably differs.

What the reader loses is both the severity and the shape of the fix. The severity: today every Tab press after `x` runs an `open` invocation against the live tmux server, not a completion query. The shape: correcting this has to change *what command the emitted script asks for completions*, not only which completer name is registered.

**Evidence**:

Claim (§8.2), verbatim:

> `portal init` emits a function that runs `portal open`, but registers Portal's completer against `portal` — so the shell completes everything typed after `x` as though the user had typed `portal`.

> | `portal __complete ""` | the subcommand list (`alias`, `doctor`, `hook`, `init`, `kill`, `list`, `open`, `theme`, …) — what `x <TAB>` actually offers |
> | `portal __complete -s ""` | `unknown shorthand flag: 's'`, ending in `ShellCompDirectiveDefault` — why `x -s <TAB>` falls through to filenames |

> So every `x` completion is off by one level, not just the flag: `x <TAB>` offers subcommand names where it should offer session names, and has done for as long as the function has existed.

The four `portal __complete …` probe results themselves all reproduce. What fails is the attribution of two of them to `x`.

Measurement 1 — what the emitted bash script asks for completions:

```
$ portal init bash | grep -n 'requestComp='
29:    requestComp="${words[0]} __complete ${args[*]}"
39:        requestComp="${requestComp} ''"

$ portal init bash | head -2
x() { portal open "$@"; }
xctl() { portal "$@"; }

$ portal init bash | grep -n '^complete '
431:complete -o default -F __start_portal x
432:complete -o default -F __start_portal xctl
```

`${words[0]}` is the word the user typed, so the request is `x __complete …`, and `x` is `portal open`. Both functions carry the same `complete -o default -F __start_portal` registration.

Measurement 2 — driving the emitted script with `x`/`xctl` stubbed to record the argv they are handed (no real `portal open` run):

```
$ bash --noprofile --norc -c '
    source init.bash
    _init_completion() { cur="${COMP_WORDS[COMP_CWORD]}"; prev="${COMP_WORDS[COMP_CWORD-1]}"; \
                         words=("${COMP_WORDS[@]}"); cword=$COMP_CWORD; return 0; }
    x()    { printf "x    -> portal open %s\n" "$*" >> calls.log; }
    xctl() { printf "xctl -> portal %s\n"      "$*" >> calls.log; }
    run() { COMP_WORDS=("$@"); COMP_CWORD=$(( $# - 1 )); COMP_LINE="$*"; \
            COMP_POINT=${#COMP_LINE}; __start_portal >/dev/null 2>&1; }
    run x ""; run x -s ""; run x /tm; run xctl ""
  '
$ cat calls.log
x    -> portal open __complete
x    -> portal open __complete -s
x    -> portal open __complete /tm
xctl -> portal __complete
```

Measurement 3 — zsh's emitted script builds the request the same way:

```
$ portal init zsh | grep -n 'requestComp='
52:    requestComp="${words[1]} __complete ${words[2,-1]}"

$ portal init zsh | grep -n '^compdef'
5:compdef _portal portal
217:compdef _portal x
218:compdef _portal xctl
```

Source carrying the claim: `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md`, section `## completion` → `### Journey`:

```
$ sed -n '781,790p' .workflows/open-with-forced-filter/discussion/open-with-forced-filter.md
The `x` function maps to `portal open`, but `portal init` registers
Portal's completer against `portal` — so the shell completes `x …` as though the
user had typed `portal …`, one command level too high. Measured:

- `portal __complete open ""` → live session names (correct)
- `portal __complete open -s ""` → live session names (correct)
- `portal __complete ""` → the subcommand list — … — which is what `x <TAB>` actually offers
- `portal __complete -s ""` → `unknown shorthand flag: 's'`, ending in the default directive, which is why `x -s <TAB>` falls through to filenames

So every `x` completion is off by one level, not just the flag the user noticed:
`x <TAB>` offers subcommand names where it should offer session names.
```

The specification's clause "and has done for as long as the function has existed" is spec-only; the assertion it extends is the source's.

Binary measured: `portal version` → `0.11.1` (`/opt/homebrew/bin/portal`); emitted scripts come from cobra v1.10.2's `GenBashCompletionV2` / `GenZshCompletion` per `cmd/init.go:64,118`.

**Resolution**: Pending
**Notes**:

---
