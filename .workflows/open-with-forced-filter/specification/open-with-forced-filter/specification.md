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

## Working Notes
