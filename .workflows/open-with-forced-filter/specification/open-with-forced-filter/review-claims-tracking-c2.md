# Review Tracking: Open With Forced Filter - Claims Verification

## Findings

### 1. Tab completion never produces the trailing-slash directory form the recognition rule leans on

**Source**: Tree measurement — `portal __complete open /tm`
**Category**: Source defect
**Move**: route
**Affects**: §2.2 (the recognition rule and why it survives ordinary use); bears on §2.4 (accepted cost) and §8.1 (completion suppresses the filename fallback)

**Problem**:
The record assures the reader that reaching an absolute directory the ordinary way stays safe: press Tab after `x /tm` and the shell hands back `/tmp/`, whose second slash keeps it a path, so only a hand-typed single-segment directory is caught by the new search form. Measured, that route does not exist. Portal's completion returns no candidates for a leading-slash word and explicitly switches the shell's filename fallback off, so Tab after `x /tm` completes to nothing in bash and reports "No matching completions" in zsh — the same today (where the function completes at the `portal` level) and after the completion correction (where it completes as `portal open`). There is no way for completion to produce `/tmp/`, so a user reaching for `/tmp` gets the search form whichever way they type it. The cost of the recognition rule is therefore wider than the record states, and the argument that ordinary use keeps the two forms apart does not hold.

**Evidence**:
Claim (specification §2.2, verbatim): "The rule survives ordinary use because the two forms are produced differently: shell completion appends a trailing slash to a directory, so `x /tm<TAB>` yields `/tmp/` and stays a path, while `/port` typed by hand is a sigil."

```
$ portal __complete open /tm
:4
Completion ended with directive: ShellCompDirectiveNoFileComp

$ portal __complete /tm          # what the `x` function routes to today
:4
Completion ended with directive: ShellCompDirectiveNoFileComp

$ portal __complete open '~/Co'
:4
Completion ended with directive: ShellCompDirectiveNoFileComp
```

No candidate lines precede the `:4` directive in any of the three — the completion offers nothing at all for a path-shaped word.

The filename fallback is then suppressed by the generated scripts, so no shell-side directory completion (and no appended trailing slash) can happen either:

```
$ portal completion bash | grep -n 'shellCompDirectiveNoFileComp' -A 3
96:        if (((directive & shellCompDirectiveNoFileComp) != 0)); then
97-            if [[ $(type -t compopt) == builtin ]]; then
99-                compopt +o default

$ portal completion zsh | grep -n 'shellCompDirectiveNoFileComp'
129:        if [ ${#completions} -ne 0 ] || [ $((directive & shellCompDirectiveNoFileComp)) -eq 0 ]; then
189:            if [ $((directive & shellCompDirectiveNoFileComp)) -ne 0 ]; then
```

The completion function itself offers session names only, filtered by prefix, and never paths:

```
$ grep -n 'func completeSessionNames' -A 8 cmd/completion.go
24:func completeSessionNames(toComplete string) ([]string, cobra.ShellCompDirective) {
25-	var matches []string
26-	for _, name := range completionSessionNames() {
27-		if strings.HasPrefix(name, toComplete) {
28-			matches = append(matches, name)
29-		}
30-	}
31-	return matches, cobra.ShellCompDirectiveNoFileComp
32-}
```

Source carrying the claim: `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md`, `## filter-shortcut-form` → `### Decision` (lines 418–420): "The rule survives ordinary use because the two forms are produced differently: shell completion appends a trailing slash to a directory, so `x /tm<TAB>` yields `/tmp/` and stays a path, while `/port` typed by hand is a filter."

**Proposed Text**:

**Resolution**: Pending
**Notes**:

---

### 2. A shell function can be named `x/port` — the no-space form is not mechanically impossible

**Source**: Tree measurement — `bash --noprofile --norc -c 'x/port() { echo RAN-FUNC; }; x/port'`
**Category**: Source defect
**Move**: route
**Affects**: §2.1 (ruling out the no-space `x/port` form)

**Problem**:
The record rules the no-space form out as impossible rather than undesirable: it states that `x/port` is a path in every shell, that no function definition can claim that shape, and that the only mechanisms left are a machine-wide unknown-command hook or one function per search term. Measured, the first part is false — bash 5.3 and zsh 5.9 both define a function whose literal name is `x/port` and dispatch that word to it. The remaining objection is real and untouched (a function would be needed for every search term anyone ever types, which is the per-term mechanism already named), so the form stays out; but the stated ground for it — that no shell can be made to recognise the shape — does not survive measurement, and a reader checking the reasoning finds it wrong.

**Evidence**:
Claim (specification §2.1, verbatim): "It is not a function call in any shell — it is a path, a command named `port` inside a directory named `x` — and no function definition can claim that shape."

```
$ cd <empty scratch dir> && ls -a
.  ..

$ bash --noprofile --norc -c 'x/port() { echo RAN-FUNC; }; x/port; echo "exit=$?"'
RAN-FUNC
exit=0

$ zsh -f -c 'x/port() { echo RAN-FUNC }; x/port; echo "exit=$?"'
RAN-FUNC
exit=0

$ bash -c 'x/port() { echo RAN-FUNC; }; type x/port'
x/port is a function
x/port ()
{
    echo RAN-FUNC
}

$ zsh -f -c 'x/port() { echo RAN-FUNC }; whence -w x/port'
x/port: function

$ bash --version | head -1
GNU bash, version 5.3.15(1)-release (aarch64-apple-darwin25.4.0)
$ zsh --version
zsh 5.9 (arm64-apple-darwin25.0)
```

No file or directory named `x` existed in the working directory, so nothing on disk could have satisfied the invocation.

Source carrying the claim: `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md`, `## filter-shortcut-form` → `### Options Considered`, Option E (lines 366–369): "Rejected as impossible rather than undesirable. `x/port` is not a function call in any shell: it is a path — a command named `port` inside a directory named `x` — and no function definition can claim that shape."

**Proposed Text**:

**Resolution**: Pending
**Notes**:
