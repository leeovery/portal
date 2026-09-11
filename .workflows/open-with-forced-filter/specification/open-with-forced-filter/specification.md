# Specification: Open With Forced Filter

## Specification

### 1. Purpose and Scope

#### 1.1 The gap this fills

`portal open`'s argument surface reaches a live session by an exact session name (bare or pinned with `-s`), or by `-s <glob>`. The glob form answers ambiguity by opening a window per match rather than by narrowing — one match degenerates to a direct open, two or more run the multi-window burst (`sed -n '90p' cmd/open_burst.go` → `if len(surfaces) == 1 {`) — and it demands quoted glob syntax, because an unquoted `x -s port*` is consumed by zsh's `nomatch` before Portal sees it. Every other route on the surface mints: a bare path, alias or zoxide hit, and the `-p` / `-a` / `-z` pins.

Nothing on the surface searches live sessions and then lets the user choose among the matches. This feature adds that route.

#### 1.2 What it adds

A positional beginning with `/` — `x /port` — declares session-search intent. The term after the slash is forced into the sessions list as filter text and never enters the resolution chain. Exactly one matching session attaches outright; two or more open the picker with the list already narrowed and the cursor on the first row; zero is a hard failure.

The flow it collapses is the user's dominant one today: `x`, wait for the picker, `/`, type three or four characters, `Enter`, `Space` through the two or three survivors to see which is which, `Enter` to attach. After this feature the leading five steps are one keystroke sequence: `x /port`.

#### 1.3 Scope

This specification covers the sigil's shape and recognition rule (§2), its resolution semantics (§3), what the search matches and by what rule (§4), what else may share its command line (§5), how a matched row is displayed (§6), its cold-start classification (§7), tab completion for the form together with the pre-existing defect that stops the `x` function completing at all (§8), and the help-text and README changes the form obliges (§9).

What this feature deliberately leaves untouched is stated in §10; the correction it owes a sibling specification is §11.

---

### 2. The Sigil Form

#### 2.1 Shape

The form is a positional argument beginning with `/`, with a space between the command and the argument: `x /port`, `portal open /port`. The character after the `/` begins the search term.

The seed's no-space form `x/port` is ruled out permanently. It is not a function call in any shell — it is a path, a command named `port` inside a directory named `x` — and no function definition can claim that shape. The only mechanisms that could intercept it are a global unknown-command hook, which would put Portal in the path of every mistyped command on the machine, or one function per search term. Neither is built.

#### 2.2 The recognition rule

**A positional argument is a sigil when it begins with `/` and contains no further `/`.**

A leading `/` today makes an argument a path unconditionally (`sed -n '14p' internal/resolver/path.go` → `return strings.Contains(arg, "/") || arg[0] == '.' || arg[0] == '~'`), so this rule narrows the path test for one shape and leaves every other path argument exactly as it is.

| Argument | Reading |
|---|---|
| `/port` | sigil — session search on `port` |
| `/Users/leeovery/Code/portal` | path — mints, unchanged |
| `/tmp/` | path — mints, unchanged; the trailing slash is the second one |
| `./port`, `~/port`, `port` | unchanged — the rule tests a leading `/` only |

The rule survives ordinary use because the two forms are produced differently: shell completion appends a trailing slash to a directory, so `x /tm<TAB>` yields `/tmp/` and stays a path, while `/port` typed by hand is a sigil.

#### 2.3 Recognition is positional-independent

The **shape** is what makes a positional a sigil, wherever it sits on the command line. `portal open ~/Code/api /tmp` is therefore a usage error under §5 rather than a two-mint burst — `/tmp` is a sigil, and a sigil with another target on the line is refused.

The alternative — recognising the shape only as the sole positional — was rejected because it makes the outcome depend on argument order: `portal open /tmp ~/Code/api` would error while `portal open ~/Code/api /tmp` succeeded, with the same two arguments. No error message can explain that rule.

#### 2.4 Accepted cost

Single-segment absolute directories typed *without* a trailing slash — `x /tmp`, `x /opt`, `x /srv` — stop minting and start filtering. The escape is `-p`, the pin that exists for exactly this: `portal open -p /tmp` mints there, and `portal open -p ~/Code/api -p /tmp` bursts two mints unchanged.

#### 2.5 The degenerate form

`x /` — a bare slash with no term — is a usage error, following `-f`'s existing answer to an empty value (`sed -n '161,163p' cmd/open.go` → the `-f/--filter value must not be empty` guard). It does not mean "mint at root".

---

## Working Notes
