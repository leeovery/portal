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

The picker's own filter does not. The sessions list filters through `charm.land/bubbles/v2/list.DefaultFilter`, which is `fuzzy.Find` from `sahilm/fuzzy` — subsequence matching, anywhere, rank-sorted — and `internal/tui` installs no filter of its own, so that default stands (`grep -rn 'SetFilterFunc\|DefaultFilter' internal/tui/*.go | grep -v _test` → no matches). Under that rule a session at `~/Projects/rust-tools` satisfies `port`: **p** in `projects`, **o** in `projects`, **r** in `rust`, **t** in `tools`, with the four letters never appearing together.

That is harmless inside the picker, where the user reads the results and skips the nonsense rows. It is not harmless for the sigil, because two decisions compound: matching directories gives the matcher a long absolute path to find scattered letters in, and eager resolution (§3.2) means a lone match is acted on without ever being shown. Together they can attach the user to a session they never saw and would not have chosen. `/port` finding nothing is a better failure than `/port` attaching `rust-tools`.

Portal's own `internal/fuzzy` package is not what the picker uses and has no production consumer (`grep -rln 'internal/fuzzy' . --include='*.go' | grep -v '^./internal/fuzzy/'` → `internal/fuzzy/match_test.go`).

#### 4.4 The rule diverges by entry point; the fields do not

| Entry point | Matched fields | Matching rule |
|---|---|---|
| Sigil — the count deciding §3.2, and the picker's first displayed set | name + recorded directory | containment |
| `-f/--filter` | name + recorded directory | the picker's own fuzzy |
| `/` typed by hand in the picker | name + recorded directory | the picker's own fuzzy |

**The picker's own filter is untouched by this work.** Once the sigil has opened the picker, the moment the user edits the filter by hand the picker's rule applies and the row set can widen. The divergence is therefore visible only by rows appearing, never by rows the user expected going missing.

The distinguishing question is whether a path can act without showing the user anything: the picker shows its results and can afford a loose rule; the shell form cannot.

#### 4.5 Accepted cost and confidence

`/port` is less precise than it would be on names alone — sessions the user would never think of as "port" appear because their path happens to contain it. Taken knowingly, as something use rather than argument will settle. Narrowing the match domain back to names alone is a cheap reversal if the false-positive rate proves intolerable.

---

### 5. Argv Composition

#### 5.1 The rule

**`/term` composes with nothing. Any other argument on the line is a usage error.**

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

#### 5.5 `-f` is unchanged

`-f/--filter` keeps its existing behaviour in full, including its Projects redirect under a pending command. The two forms are deliberately not symmetrical — the sigil takes no command exception — which is one more line of help text (§9) and no behaviour change to a shipped flag.

---

### 6. Search Result Display

#### 6.1 The requirement

**A row in a sigil-opened list shows the directory it matched on, beside the session name.**

Matching on the recorded directory (§4.1) means `/port` can return a row the user cannot account for: a session named `api-work` appears because it lives in the Portal checkout, while the row shows only a name, a window count and an attached marker. In Flat mode — the default, and where the sigil's narrowed list lands — the user is looking at a name with no visible relationship to what they typed.

That is the seed's own complaint returning by the back door. Names are precisely what the user said they cannot recognise sessions by; the feature answers that by matching the directory instead; displaying the name alone sends them back to previewing each candidate, which is the ceremony being removed.

#### 6.2 Placement and weight

Alongside the name rather than on its own line, rendered in the muted rung of the text ramp — the role paths, counts and subtitles already take elsewhere in the picker. This introduces no new colour token and no new convention; the selected row's own treatment one step brighter is the established pattern the sigil row reuses.

The sibling `theming-system` specification fixes the token vocabulary at nineteen closed semantic roles and forbids raw colour at call sites. This section references an existing role rather than proposing a new one, so nothing in that vocabulary changes.

Exact column treatment is presentation detail for implementation. The row already flexes the name against a fixed count slot, a fixed attached slot and a right margin, and already truncates (`grep -n 'ansi.Truncate' internal/tui/session_item.go`), so a long path in a narrow terminal is handled by the mechanism that is already there.

#### 6.3 Scope

Scoped to the sigil's own list. How Sessions rows render when the picker is reached any other way is untouched, consistent with the matching rule's own divergence (§4.4).

#### 6.4 Accepted cost

The row carries more text.

---

## Working Notes
