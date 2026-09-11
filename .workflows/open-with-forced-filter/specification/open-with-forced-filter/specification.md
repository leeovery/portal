# Specification: Open With Forced Filter

## Specification

### 1. Purpose and Scope

#### 1.1 The gap this fills

`portal open`'s argument surface reaches a live session by an exact session name (bare or pinned with `-s`), or by a session glob — bare or pinned, which resolve identically (`sed -n '31,41p' cmd/open_surfaces.go` → both `ResolveBareAll` and `ResolveSessionPinAll` dispatch a glob to `expandSessionGlobAll`). The glob form answers ambiguity by opening a window per match rather than by narrowing — one match degenerates to a direct open, two or more run the multi-window burst (`sed -n '90p' cmd/open_burst.go` → `if len(surfaces) == 1 {`) — and it demands quoted glob syntax, because an unquoted `x port*` is consumed by zsh's `nomatch` before Portal sees it. Every other route on the surface mints: a bare path, alias or zoxide hit, and the `-p` / `-a` / `-z` pins.

Nothing on the surface searches live sessions and then lets the user choose among the matches. This feature adds that route.

#### 1.2 What it adds

A positional beginning with `/` — `x /port` — declares session-search intent. The term after the slash is forced into the sessions list as filter text and never enters the resolution chain, and the number of live sessions it matches decides what happens (§3.2).

The flow it collapses is the user's dominant one today: `x`, wait for the picker, `/`, type three or four characters, `Enter`, `Space` through the two or three survivors to see which is which, `Enter` to attach. After this feature the leading five steps are one keystroke sequence: `x /port`.

#### 1.3 Scope

This specification covers the sigil's shape and recognition rule (§2), its resolution semantics (§3), what the search matches and by what rule (§4), what else may share its command line (§5), how a matched row is displayed (§6), its cold-start classification (§7), tab completion for the form together with the pre-existing defect that stops the `x` function completing at all (§8), and the help-text and README changes the form obliges (§9).

What this feature deliberately leaves untouched is stated in §10; the correction it owes a sibling specification is §11.

---

### 2. The Sigil Form

#### 2.1 Shape

The form is a positional argument beginning with `/`, with a space between the command and the argument: `x /port`, `portal open /port`. The character after the `/` begins the search term.

The seed's no-space form `x/port` is ruled out permanently — not because the shape is unclaimable, but because claiming it buys nothing. Both shells will define a function whose literal name is `x/port` and dispatch that word to it (`bash --noprofile --norc -c 'x/port() { echo RAN-FUNC; }; x/port'` → `RAN-FUNC`, GNU bash 5.3.15; `zsh -f -c 'x/port() { echo RAN-FUNC }; x/port'` → `RAN-FUNC`, zsh 5.9) — but the name must be the search term, so it would take a function per term anyone might ever type. Without one the word falls through to a path, a command named `port` inside a directory named `x`. The only other interception is a global unknown-command hook, which would put Portal in the path of every mistyped command on the machine. Neither is built.

#### 2.2 The recognition rule

**A positional argument is a sigil when it begins with `/` and contains no further `/`.**

A leading `/` today makes an argument a path unconditionally (`sed -n '14p' internal/resolver/path.go` → `return strings.Contains(arg, "/") || arg[0] == '.' || arg[0] == '~'`), so this rule narrows the path test for one shape and leaves every other path argument exactly as it is.

| Argument | Reading |
|---|---|
| `/port` | sigil — session search on `port` |
| `/Users/leeovery/Code/portal` | path — mints, unchanged |
| `/tmp/` | path — mints, unchanged; the trailing slash is the second one |
| `./port`, `~/port`, `port` | unchanged — the rule tests a leading `/` only |

Completion offers no help telling the two apart. Portal returns no candidates for a slash-leading word and switches the shell's filename fallback off (`portal __complete open /tm` → no candidates, `ShellCompDirectiveNoFileComp`; `portal completion bash | grep -n 'compopt +o default'`), so `x /tm<TAB>` completes to nothing rather than to `/tmp/`. The trailing slash that keeps an argument a path is therefore one the user types, and a single-segment absolute directory typed without it is a sigil however it was reached.

#### 2.3 Recognition is positional-independent

The **shape** is what makes a positional a sigil, wherever it sits on the command line. `portal open ~/Code/api /tmp` is therefore a usage error under §5 rather than a two-mint burst — `/tmp` is a sigil, and a sigil with another target on the line is refused.

The alternative — recognising the shape only as the sole positional — was rejected because it makes the outcome depend on argument order: `portal open /tmp ~/Code/api` would error while `portal open ~/Code/api /tmp` succeeded, with the same two arguments. No error message can explain that rule.

**The command payload is not searched for the shape.** Recognition applies to the arguments `open` parses as targets, and stops at a `--` separator: the words after it are the trailing command's own, passed to that command untouched, so a `/word` among them is that command's argument and never a sigil. `portal open ~/Code/api -- ls /tmp` is unaffected by this feature. The rule loses nothing by stopping there, because a sigil line carries no command at all (§5.1) — a line holding both is a usage error on the target it names. A command carried as a flag value is a value rather than a positional and was never in reach of the rule.

#### 2.4 Accepted cost

Single-segment absolute directories typed *without* a trailing slash — `x /tmp`, `x /opt`, `x /srv` — stop minting and start filtering. The escape is `-p`, the pin that exists for exactly this: `portal open -p /tmp` mints there, and `portal open -p ~/Code/api -p /tmp` bursts two mints unchanged.

#### 2.5 The degenerate form

`x /` — a bare slash with no term — is a usage error, following `-f`'s existing answer to an empty value (`sed -n '161,163p' cmd/open.go` → the `-f/--filter value must not be empty` guard). It does not mean "mint at root".

---

### 3. Resolution

#### 3.1 The sigil is session-domain by declaration

A sigil argument never enters the bare-positional resolution chain. It reaches neither the path, alias nor zoxide domain, and it can never mint a session. Its term is a search over live sessions and nothing else.

This is what makes eager resolution safe. The sibling `cli-verb-surface-redesign` specification rejected project-prefix session matching (`api` resolving to the sole live `api-*` session) because it reintroduces attach-versus-create guessing with a cliff at the second match. The sigil has the same cliff at the same place and is nevertheless sound, because both sides of its cliff are session-domain: the worst case is a picker the user was already heading for, not a stray session minted where an attach was wanted.

#### 3.2 Outcomes by match count

Let K be the number of live sessions matching the term under §4.

| K | Outcome |
|---|---|
| 0 | Hard failure. Nothing opens, nothing mints, and the picker does not appear. |
| 1 | The matching session is attached directly. No picker. |
| >= 2 | The picker opens on the Sessions page with the term applied as a committed filter and the cursor on the first matching row. |

Eager attach on a single session-search match is already shipped behaviour rather than a new one: `-s <glob>` matching exactly one session attaches it outright (§1.1).

#### 3.3 The committed-filter landing

On K >= 2 the picker lands exactly as `-f` already lands it: filter text set and filter state committed rather than focused, cursor re-anchored onto the post-filter visible set, so arrows, `Space` and `Enter` work immediately on the narrowed list (`sed -n '1261,1273p' internal/tui/model.go`). The user is not left inside a live filter input.

#### 3.4 When the count is taken

K is evaluated against the live session set once the tmux server is ready to answer for it. On a warm server that is immediately. On a cold server the sigil takes the picker's concurrent-bootstrap path (§7), so the count is taken after restore has reconstructed the saved sessions — the loading page appears first and is replaced by the attach when K turns out to be 1.

#### 3.5 Accepted cost

The user cannot predict before typing whether they land in a session or in the picker. That was taken knowingly: the cost of the wrong side of the cliff is a screen they were already opening.

#### 3.6 No `resolve` log line

The sigil emits no `resolve` component line. That component records one INFO line per bare positional resolved through the guessing chain; deterministic targets — globs and domain pins — emit none, and a sigil declares its domain in the same way a pin does.

*Derived, not decided in discussion: the source material settles the sigil's domain but never names its logging. The derivation is the `resolve` component's own stated rule.*

#### 3.7 The zero-match failure, and how an attach is performed

**The K = 0 message names the search that found nothing.** It must say that no live session matched the term, and it must not point the user at `-f` — the existing bare-positional miss message does (`grep -n 'try -f' cmd/open_burst.go`), which is right for a miss in the guessing chain and wrong here: `-f` opens a picker filtered by the same term, which would list nothing either. Exact wording follows the house convention for `open`'s user-facing errors.

The usage errors of §5.1 are ordinary usage errors, carrying the same shape as `-f`'s own mutual-exclusion refusal.

**On a cold boot the miss arrives after the loading page and reads identically.** A sigil invocation takes the loading page before K can be taken (§7), so a K = 0 on a cold server is discovered with the TUI already on screen. The TUI closes, the same message is written to the terminal, and the exit status is the same non-zero one the warm path returns. This is not a bootstrap fatal and does not take the in-TUI error frame; the picker never appears, exactly as §3.2 requires.

**An attach under K = 1 uses the connector the invocation already selects** — `syscall.Exec` into `tmux attach-session` outside tmux, `switch-client` inside it. The sigil introduces no third connection mode.

*Derived, not decided in discussion: the sources settle that zero fails honestly and that one match attaches, but name neither the message nor the connection mode. The derivations are the existing miss-message's purpose and `open`'s existing connector selection.*

---

### 4. Match Domain and Matching Rule

#### 4.1 The matched fields

**A session matches on its name and on its recorded directory.** Project records and tags stay out of the match domain.

The recorded directory is the `@portal-dir` tmux session user-option, stamped at creation and returned with the session list at no extra cost — `ListSessions` appends `#{@portal-dir}` to its `list-sessions -F` format and parses it into `Session.Dir` (`grep -n '@portal-dir' internal/tmux/tmux.go`). Matching on it therefore costs no additional tmux round-trip.

**Never a derived directory.** The picker can derive a missing directory by asking a session's pane where it is, but only in the grouped views, and it caches the answer — so a session would be findable or not depending on which view the user last left the picker in. The shell form has it worse: K (§3.2) is taken before any picker exists, against a session list carrying names only. Matching the recorded value alone makes the answer identical everywhere — the count and the list, the shell and the picker — and keeps the sigil path free of a per-session pane read on a path whose whole point is to feel instant.

**Known limitation, unconditional and self-correcting:** a session created before the directory stamp shipped carries no recorded directory and matches on name alone, in every view. Such sessions age out as they are killed and replaced. Deriving the missing value everywhere was rejected for its cost — one pane read per unrecorded session on every `/term`.

#### 4.2 The matched fields are shared across all three filter entry points

`-f/--filter`, the sigil, and typing `/` by hand inside the picker all narrow the same list through the same filter value. Widening the matched fields to include the directory widens them for all three. That is deliberate: the same list narrowing on different *fields* depending on how the user arrived at it is harder to predict than one wider rule.

#### 4.3 The matching rule

**The sigil matches by containment** — the typed characters appearing as a run in the matched text — case-folded.

The two fields are tested separately: a session matches when the term appears as a run in its name, or as a run in its recorded directory. They are never joined into a single string to be searched — a term matching across the join would return a session that neither field contains, and no row could account for it.

The picker's own filter does not. The sessions list filters through `charm.land/bubbles/v2/list.DefaultFilter`, which is `fuzzy.Find` from `sahilm/fuzzy` — subsequence matching, anywhere, case-folded and rank-sorted — and `internal/tui` installs no filter of its own, so that default stands (`grep -rn 'SetFilterFunc\|DefaultFilter' internal/tui/*.go | grep -v _test` → no matches). Case-folding is the one property the sigil keeps from it: the two rules diverge on subsequence against containment and on nothing else. Under that rule a session at `~/Projects/rust-tools` satisfies `port`: **p** in `projects`, **o** in `projects`, **r** in `rust`, **t** in `tools`, with the four letters never appearing together.

That is harmless inside the picker, where the user reads the results and skips the nonsense rows. It is not harmless for the sigil, because two decisions compound: matching directories gives the matcher a long absolute path to find scattered letters in, and eager resolution (§3.2) means a lone match is acted on without ever being shown. Together they can attach the user to a session they never saw and would not have chosen. `/port` finding nothing is a better failure than `/port` attaching `rust-tools`.

Portal's own `internal/fuzzy` package is not what the picker uses and has no production consumer (`grep -rln 'internal/fuzzy' . --include='*.go' | grep -v '^./internal/fuzzy/'` → `internal/fuzzy/match_test.go`).

#### 4.4 The rule diverges by entry point; the fields do not

| Entry point | Matched fields | Matching rule |
|---|---|---|
| Sigil — the count deciding §3.2, and its list for as long as its filter text stands untouched | name + recorded directory | containment |
| `-f/--filter` | name + recorded directory | the picker's own fuzzy |
| `/` typed by hand in the picker | name + recorded directory | the picker's own fuzzy |

**The containment set holds for as long as the sigil's filter text stands untouched.** The narrowed list does not sit still — a `Space` preview and back, an `s` regroup, a refresh after a session is killed elsewhere all re-render it — and every one of those reproduces the containment set. The list the user is choosing from is the list they were handed. Only a hand edit of the filter text returns the list to the picker's own rule, and the test is the text rather than the act: while the committed filter value is character-identical to the term the sigil supplied, containment stands — opening the filter input and leaving it as it was, or editing back to the same characters, keeps it. Any other value, a cleared filter included, is the picker's own rule.

**Containment narrows the list; it does not reorder it.** Rows keep the order the sessions list gives them in whatever grouping mode is current, with non-matching rows removed and nothing re-ranked — the picker's rank-sorting is a property of its fuzzy rule, which the sigil does not use. The first matching row (§3.2) is the first surviving row of that existing order.

**The picker's own filter is untouched by this work.** Once the user edits that text the picker's rule applies and the row set can widen. The divergence is therefore visible only by rows appearing, never by rows the user expected going missing.

The distinguishing question is whether a path can act without showing the user anything: the picker shows its results and can afford a loose rule; the shell form cannot.

#### 4.5 Accepted cost and confidence

`/port` is less precise than it would be on names alone — sessions the user would never think of as "port" appear because their path happens to contain it. Taken knowingly, as something use rather than argument will settle. Narrowing the match domain back to names alone is a cheap reversal if the false-positive rate proves intolerable.

---

### 5. Argv Composition

#### 5.1 The rule

**`/term` composes with nothing. Any other target, any trailing command, and any of `open`'s own flags on the same line is a usage error.**

Flags that answer before the command body runs are outside the rule: `portal open /term --help` prints help, and root-level persistent flags apply as they do to any other invocation.

The sigil is not a target that sits in the grammar alongside other targets — it is a whole-invocation mode, the way `-f` is. `portal open /term` is a complete invocation, and nothing else belongs on the line.

| Line | Outcome |
|---|---|
| `portal open /term` | the sigil, per §3 |
| `portal open /term -e <cmd>` | usage error |
| `portal open /term -- <cmd>` | usage error |
| `portal open /term <other-target>` | usage error, at every arity |
| `portal open <other-target> /term` | usage error — recognition is positional-independent (§2.3) |
| `portal open /term -s|-p|-a|-z <value>` | usage error |
| `portal open /term -f <text>` | usage error |
| `portal open /term --ack <batch>:<token>` | usage error |

#### 5.2 Why a command is refused rather than redirected

A trailing command declares mint: it can only run in a session about to be created. `-f` answers that by routing its term to the **Projects** page instead of the sessions list, which is correct for `-f` — it never promised session-domain (`sed -n '1265,1271p' internal/tui/model.go` → the `commandPending` branch sends the filter to the project list).

Inheriting that would mean typing a glyph whose entire job is to declare "search my live sessions, never mint" and landing on the mint page — the sigil contradicting itself. Refusing is honest where a silent page switch is not.

#### 5.3 Why a second target is refused

The sigil never participates in the multi-window burst. A case was floated for allowing it — wanting an existing Portal session *and* a fresh session in another project, in two windows, from one line — and declined.

#### 5.4 The domain pins and the hidden `--ack`

These are settled by derivation rather than by discussion. The two rulings above make the sigil a whole-invocation form, and `-f`'s existing contract already rejects every domain pin (`sed -n '156,160p' cmd/open.go`); the sigil is stricter than `-f` in every other respect, so it cannot be laxer here. `--ack` is the burst's internal handshake flag and the sigil never bursts, so it falls under the same rule.

*Derivation recorded: `-f`'s mutual-exclusion contract plus §5.1's whole-invocation ruling.*

#### 5.5 What `-f` keeps

`-f/--filter` keeps its argument contract, its fuzzy matching rule and its Projects redirect under a pending command. Its matched fields widen with everything else's (§4.2): after this feature `-f` narrows on session name and recorded directory, exactly as the sigil and the hand-typed filter do. The two forms are deliberately not symmetrical — the sigil takes no command exception — which is one more line of help text (§9).

---

### 6. Search Result Display

#### 6.1 The requirement

**Every row in a sigil-opened list shows its recorded directory beside the session name** — the rows found by their directory and the rows found by their name alike, so the column is uniform rather than a signal in itself.

Matching on the recorded directory (§4.1) means `/port` can return a row the user cannot account for: a session named `api-work` appears because it lives in the Portal checkout, while the row shows only a name, a window count and an attached marker.

**In every grouping mode**, not only Flat. The picker reopens in whichever session-list grouping mode the user last left it in — persisted and re-applied at construction, with Flat only the fallback for a user who has never pressed `s` (`sed -n '612,615p' cmd/open.go`) — so the sigil's narrowed list lands in Flat, By Project or By Tag depending on that history. By Project does not carry the information by another route: the grouping survives a committed filter but the headings do not, because a header row's filter value is empty (`sed -n '93p' internal/tui/session_item.go` → `func (HeaderItem) FilterValue() string { return "" }`). A filtered By-Project list therefore shows grouped-indented names with neither heading nor directory.

**Only the recorded directory is displayed.** A session carrying no recorded directory (§4.1) shows none beside its name — the slot is simply empty. The displayed value is never derived from a pane read, for the same reason the match is not: the row must show what the search actually matched against, and the sigil path pays for no per-session pane read.

That is the seed's own complaint returning by the back door. Names are precisely what the user said they cannot recognise sessions by; the feature answers that by matching the directory instead; displaying the name alone sends them back to previewing each candidate, which is the ceremony being removed.

#### 6.2 Placement and weight

Alongside the name rather than on its own line, taking the same colour token as the row's window count — the muted rung of the text ramp, the role paths, counts and subtitles already take elsewhere in the picker. This introduces no new colour token and no new convention; the selected row's own treatment one step brighter is the established pattern the sigil row reuses.

The sibling `theming-system` specification fixes the token vocabulary at nineteen closed semantic roles and forbids raw colour at call sites. This section references an existing role rather than proposing a new one, so nothing in that vocabulary changes.

**When the row is too narrow for both, the directory gives way and the session name never does.** The directory is shortened **from the left**, so its tail survives — `…/Code/portal`, not `/Users/leeovery/Cod…` — because the tail is the segment a human recognises a checkout by, and the whole premise of showing it is that the directory is recognisable where the name is not. A path under the user's home is always displayed abbreviated to `~/`, at any width — the abbreviation is how the value is rendered rather than a rung of the truncation ladder, and it commonly reclaims enough width that no truncation is needed at all.

The row already flexes the name against a fixed count slot, a fixed attached slot and a right margin, and already truncates (`grep -n 'ansi.Truncate' internal/tui/session_item.go`); the directory column takes its width from what remains and applies the left-truncation above.

**Below a floor the directory is dropped rather than truncated to noise.** When the remaining width cannot hold an ellipsis plus one whole path segment, the row shows the session name alone — a row ending in `…l` says nothing and reads as damage, where a bare name at least reads as a name.

#### 6.3 Scope

Scoped to the picker session a sigil opened — across every grouping mode that list can be in (§6.1), and for as long as that picker is open. A hand edit of the filter text returns the matching rule to the picker's own (§4.4) but does not take the column with it: the rows can still be present on the strength of their directory, so the accounting is still owed. How Sessions rows render when the picker is reached any other way is untouched.

#### 6.4 Accepted cost

The row carries more text.

---

### 7. Cold-Path Classification

#### 7.1 The requirement

**A sigil invocation is classified as a picker invocation.** It takes the concurrent bootstrap and the honest loading page, and its soft bootstrap warnings take the in-TUI route rather than stderr.

#### 7.2 What the classification decides

On a cold boot — no tmux server running — Portal must start the server, register hooks, restore every saved session and replay its scrollback before it can list a single session. Which invocations get the loading page for that is decided from the command line before anything is resolved, and anything positional is classified as not-heading-for-the-picker (`sed -n '168,170p' cmd/root.go` → `return cmd.Name() == "open" && len(args) == 0 && !anyOpenDomainPin(cmd)`).

The same verdict decides where soft bootstrap warnings go: on the non-picker classification they are written straight to the terminal the picker is about to take over with its alternate screen, which is the corruption the classification exists to prevent.

Without this section, `x /port` on a cold boot would give a blank terminal for the whole bootstrap and could spray a warning into the frame the picker is about to claim, while `x -f port` on the same boot showed the loading page throughout.

#### 7.3 Why the awkwardness is real

`/term` does not know whether it is heading for the picker — that depends on K (§3.2), and on a cold server there are no sessions to match until restore has finished. The classification is therefore taken on the form rather than on the outcome.

A brief loading page on the way to a direct attach costs a flicker. A silent multi-second blank terminal on every cold boot reads as a hang, and it is the first impression the feature makes after every reboot. The warnings argument points the same way — they need somewhere safe to land, and the picker path already has one.

#### 7.4 Accepted cost

When K turns out to be 1, the loading page appears and is replaced by the attach, so the user sees a brief screen they did not need.

#### 7.5 A sigil that resolves to a direct attach still delivers its warnings

On K = 1 the TUI tears down before the connector runs, so the notice band never surfaces. The accumulated soft warnings are written to the terminal at that point instead — after teardown, before the attach — which is where a warm-server sigil attach already puts them. The alternate screen is gone by then, so the corruption this classification prevents cannot occur.

#### 7.6 The concurrent path itself is unchanged

Nothing about how the concurrent bootstrap behaves changes. This section adds the sigil to the set of invocations that take it.

---

### 8. Tab Completion

#### 8.1 Completion looks past the sigil

**`/po<TAB>` completes the term after — and excluding — the `/`, against live session names, leaving the sigil in place.** `/po` completing to `/portal-a1b2` leaves exactly one match, which under §3.2 attaches outright, so `/po<TAB><Enter>` becomes the whole interaction.

Offered words carry the sigil — `/po` completes to `/portal-a1b2`, never to `portal-a1b2`, which would replace the whole word and drop the slash the user typed. The words offered are the live session names the typed term prefixes: the shell discards any candidate that is not an extension of the word being completed, so completion is prefix-shaped even though the form itself matches by containment (§4.3). `/ort<TAB>` therefore offers nothing, while `/ort` still finds `portal-a1b2` on Enter.

Directories are deliberately excluded from what is *offered*, even though they count for *matching* (§4.1): a completed `/Users/leeovery/Code/portal` reads as a path and trips the no-second-slash rule (§2.2) straight back into path territory.

Today the sigil form completes to nothing — the slash is part of the word being completed, no session name begins with one, and Portal suppresses the shell's filename fallback, so Tab is silently inert rather than misleading (`portal __complete open /po` → no candidates, `ShellCompDirectiveNoFileComp`).

#### 8.2 The `x` function's completion is registered one level too high

This is a pre-existing defect, not one this feature introduces, and it is **folded into this feature** rather than spun out.

`portal init` emits a function that runs `portal open`, but registers Portal's completer against `portal` — so the shell completes everything typed after `x` as though the user had typed `portal`. All three emitted shells share the mistake (`grep -n 'complete -o default -F __start_portal\|compdef _portal\|complete -c .* -w portal' cmd/init.go`).

Measured against the built binary:

| Probe | Result |
|---|---|
| `portal __complete open ""` | live session names — correct |
| `portal __complete open -s ""` | live session names — correct |
| `portal __complete ""` | the subcommand list (`alias`, `doctor`, `hook`, `init`, `kill`, `list`, `open`, `theme`, …) — what `x <TAB>` actually offers |
| `portal __complete -s ""` | `unknown shorthand flag: 's'`, ending in `ShellCompDirectiveDefault` — why `x -s <TAB>` falls through to filenames |

So every `x` completion is off by one level, not just the flag: `x <TAB>` offers subcommand names where it should offer session names, and has done for as long as the function has existed.

#### 8.3 The correction

**`portal init` is corrected so the session-opening function completes as `portal open` rather than as `portal`, across bash, zsh and fish.** `xctl` keeps completing at the root level, which is already correct — it genuinely does map to `portal`.

The correction applies to the **configured** function name, not the literal `x`: `portal init --cmd <name>` renames both emitted functions, and the completion registration must follow whatever name was chosen.

#### 8.4 Why it is folded in rather than spun out

This feature's deliverable is not a parsing rule, it is `x /term` becoming muscle memory. A completion decision that does not reach that form has not been delivered — without the fix, §8.1 is reachable only by typing `portal open /term` in full.

#### 8.5 Rollout consequence

The fix lands in the output of `portal init`, which users evaluate in their shell startup file. An existing install does not pick it up until the shell is restarted (or `portal init` is re-evaluated), even though the binary is new. This affects completion behaviour only — the `/term` form itself is parsed by Portal and works the moment the new binary is in place.

---

### 9. Documentation

#### 9.1 Why documentation is a deliverable here

After this feature the surface reaches a live session by a bare exact session name, a quoted bare glob, `-s <name>`, `-s <glob>`, and `/term`, and opens the picker pre-filtered by `-f <text>` and `/term`. None of them is a duplicate, but the closest pair reads like one when described by input shape.

`-f` documented as "open the picker pre-filtered" invites a reader to assume `/term` is shorthand for it. They differ exactly where it matters: with one match, `/term` attaches and `-f` shows a list of one. **The pair must be described by outcome — one always shows the list, one takes you there when there is only one place to go.**

The outcome carries a use with it, and the documentation says which to reach for: `-f` is the form for a script or a keybinding, which needs to land in the same place every time; `/term` is the interactive form.

This is wording work, not behaviour change.

#### 9.2 What the documentation must carry

- The sigil form and its recognition rule, including the single-segment absolute-directory cost and the `-p` escape (§2.2, §2.4).
- The three outcomes by match count (§3.2).
- What the search matches, and that it matches by containment while the picker's own filter stays fuzzy (§4).
- That the form composes with nothing (§5.1).
- `-f` and `/term` distinguished by outcome rather than by input shape, and which of the two to reach for (§9.1).

Both `portal open --help` (`cmd/open.go`'s `Long` and the `-f` flag description) and the README's `x (open)` section carry it. The README's resolution table is where the sigil row belongs, alongside the domain pins and `-f` (`grep -n -- '-f, --filter' README.md` → the pin table row).

#### 9.3 CHANGELOG

No CHANGELOG entry is written as part of this work — the release process owns that file.

---

### 10. Out of Scope and Unchanged

Each entry below was reached as a decision, not left out by omission.

#### 10.1 The bare-positional resolution chain

**Unchanged.** Its ordering, its domains, and the sibling `cli-verb-surface-redesign` specification's Axiom 2 (no find-or-create) stand exactly as that specification set them. Project-prefix session matching stays rejected, on that specification's own grounds — `api` resolving to the sole live `api-*` session reintroduces attach-versus-create guessing with an ambiguity cliff the moment a second `api-*` session exists.

The complaint that opened this work — that a bare argument walks branches whose order nobody remembers — was measured and does not hold as stated. A plain word can reach neither the path domain nor the glob branch (§2.2 for the path test; a glob needs one of `*?[`, `grep -n 'globMeta =' internal/resolver/glob.go`), so its live set is exact session name → alias → zoxide, and the last two both mint at a directory. The outcome of a bare word is invariant as long as no live session is named exactly that word.

The residual variation is accepted: which directory a bare word mints at is zoxide's frecency call, and a session renamed to exactly that word changes the word's outcome for as long as it lives. That is the exact-session branch working as specified, and the domain pins are the deliberate route to the other outcome.

A search branch on the bare positional — `x port` attaching when exactly one session matches — was considered and rejected. Under it the two sides of the cliff are *attach an existing session* and *mint a brand-new one*, which is the guess the sibling specification refused. The sigil avoids it by declaring its domain first (§3.1).

#### 10.2 A `+` mint sigil

**Not built.** "Add new" is already the bare form's behaviour. The only gap such a sigil could fill — forcing a mint when a live session is named exactly the term — is already covered by `-p`, `-a` and `-z`. A fourth mint route is the surface accretion this work set out not to cause.

#### 10.3 A configurable sigil

**Not built.** A keybinding and an argv token are different kinds of thing: a keystroke lives in one user's session, a command string is a shared artifact that goes into a README, a script, or a bug report. Under a configurable sigil `x /tmp` mints on one machine and filters on another, and neither user can read the other's command. It would also be the first Portal setting to change *parsing* rather than appearance — every field `prefs.json` carries today is UI state (`grep -n 'json:"' internal/prefs/store.go`).

#### 10.4 Session naming

**Unchanged.** `{project}-{nanoid}` stays exactly as it is, and no part of this feature touches session creation, the naming scheme, or renaming. The feature's own answer removes whatever pressure existed: searching directories as well as names means a session is found by the place it was opened in, with no need to recognise `portal-c3d4` at all.

Out of scope and not deferred — there is no open thread here to pick up later.

#### 10.5 The rest of the argument surface

**Nothing is retired, renamed or deprecated.** `-f`, all four domain pins, and the bare positional chain keep their current prominence, and the domain pins and the bare positional chain keep their current behaviour exactly. Two changes are owed to the existing surface: `-f`'s widened match domain (§4.2), and how the `-f` / `/term` pair is described (§9.1).

#### 10.6 The picker reached any other way

**Unchanged.** The picker's own filter keeps its fuzzy, rank-sorted behaviour (§4.4), and Sessions rows reached by any route other than the sigil render as they do today (§6.3).

#### 10.7 Match-domain widening beyond name and directory

Project records and tags stay out (§4.1). Nothing in the source material asked for them, and each added field widens the false-positive set further for a case nobody has hit.

---

### 11. Correction Owed to `cli-verb-surface-redesign`

That specification's target-resolution section defines a path argument by the leading `/` / `.` / `~` test and places the path domain second in the bare-positional chain. §2.2 narrows that test for one shape: a single-segment leading-`/` argument is session-search text rather than a path, and never reaches the chain at all.

That is a change to shipped behaviour the sibling specification describes, and this specification is the superseding source for it. The correction lands in the sibling's `## Corrigenda` section — its live body edited so the path test carries the exception, with one corrigendum entry naming this work unit as the source.

Nothing else in that specification is contradicted. Axiom 2, the accepted consequence that bare project shorthand does not reattach, the rejection of project-prefix session matching, `-f` as a non-composing flag, and the tab-completion principle are all ratified rather than changed (§10.1, §3.1, §5.5, §8.1).

**The pinned-domain contract is adjacent rather than breached.** That contract holds that every domain pin hard-fails on an unresolvable target and never falls back to the TUI picker. The sigil is not a domain pin: reaching the picker is its purpose (§3.2) rather than a fallback from a failure, and on its own unresolvable case — K = 0 — it hard-fails exactly as a pin does.

---

## Working Notes
