# Review Tracking: Open With Forced Filter - Claims Verification

## Findings

### 1. A quoted bare glob already reaches a live session, so the route count is one short

**Source**: Tree measurement — `sed -n '33,44p' cmd/open_burst.go`, `sed -n '126,152p' internal/resolver/query.go`
**Category**: Source defect
**Move**: route
**Affects**: §1.1 (The gap this fills), §9.1 (Why documentation is a deliverable here)

**Problem**:
Portal already reaches a live session five ways, not four. Alongside an exact session name, `-s <name>`, `-s <glob>` and the new `/term`, a **quoted bare glob positional** — `x 'port*'`, with no `-s` — is session-domain by construction today: it expands over the live session set, attaches outright on one match and bursts on two or more. The specification's account of the surface omits it, both where it states what the surface can reach today and where it counts the routes the user-facing documentation must keep distinct. The documentation deliverable is written from that count, so the shipped bare-glob route would go undescribed while the specification claims the surface has been reconciled in full — and a reader comparing `/term` against "the glob form" would be told the glob form requires `-s`, which it does not.

**Evidence**:
Claim (§1.1): "`portal open`'s argument surface reaches a live session by an exact session name (bare or pinned with `-s`), or by `-s <glob>`."

Claim (§9.1): "After this feature there are four ways to reach a live session — a bare exact session name, `-s <name>`, `-s <glob>`, and `/term` — plus two ways to open the picker pre-filtered, `-f <text>` and `/term`."

Measurement 1 — a single bare positional carrying glob metacharacters is routed to the burst dispatcher, and the bare domain is glob-expandable:

```
$ sed -n '33,44p' cmd/open_burst.go
// A single glob-expandable target counts as multi because it may expand to K≥2
// surfaces.
func isMultiTarget(ordered []Target) bool {
	if len(ordered) >= 2 {
		return true
	}
	if len(ordered) == 1 {
		t := ordered[0]
		return globExpandableDomain(t.Domain) && resolver.HasGlobMeta(t.Value)
	}
	return false
}

$ sed -n '46,53p' cmd/open_burst.go
// A domain glob-expands only over a finite Portal-owned namespace: -p is a
// literal path and -z a zoxide subsequence query, so neither qualifies.
func globExpandableDomain(domain resolver.Domain) bool {
	switch domain {
	case resolver.DomainBare, resolver.DomainSession, resolver.DomainAlias:
		return true
	default:
		return false
	}
}
```

Measurement 2 — the bare-target resolver expands that glob over live session names into session-domain results:

```
$ sed -n '126,152p' internal/resolver/query.go
func expandSessionGlobAll(pattern string, names []string) []QueryResult {
	matches := MatchGlob(pattern, names)
	if len(matches) == 0 {
		return []QueryResult{&MissResult{Target: pattern}}
	}
	results := make([]QueryResult, 0, len(matches))
	for _, m := range matches {
		results = append(results, &SessionResult{Name: m, Domain: DomainGlob})
	}
	return results
}

// ResolveBareAll is the multi-target Resolve: a glob expands to one result per
// matching session and every failure is collected as a *MissResult so pre-flight
// reports all of them at once. The returned error is always nil.
func (qr *QueryResolver) ResolveBareAll(query string) ([]QueryResult, error) {
	if HasGlobMeta(query) {
		names, _ := qr.sessions.ListSessionNames()
		return expandSessionGlobAll(query, names), nil
	}
	...
}
```

Measurement 3 — one expanded match attaches directly rather than bursting:

```
$ sed -n '90p' cmd/open_burst.go
	if len(surfaces) == 1 {
```

The sibling `cli-verb-surface-redesign` specification records the same branch as shipped design (`sed -n '52p' .workflows/cli-verb-surface-redesign/specification/cli-verb-surface-redesign/specification.md` → "**Glob pre-check.** If the target contains glob metacharacters (`*`, `?`, `[…]`), it is **session-domain by construction**"), and this specification's own §10.1 acknowledges the bare glob branch exists.

Source carrying the claim: `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md`, `## surface-reconciliation` → Context (lines 826–829):

```
$ sed -n '826,829p' .workflows/open-with-forced-filter/discussion/open-with-forced-filter.md
has accumulated ways to do things, and adding another risks making it worse. After
this feature there are four ways to reach a live session — a bare exact session
name, `-s <name>`, `-s <glob>`, and `/term` — plus two ways to open the picker
pre-filtered, `-f <text>` and `/term`.
```

**Resolution**: Routed
**Notes**: Routed to `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md` (`## surface-reconciliation`, Context + Journey) and repaired there: the count is five, the quoted bare glob named alongside `-s <glob>`. A further measurement taken during the repair showed the two glob forms resolve identically — both dispatch through `expandSessionGlobAll` — so the Journey no longer claims a behavioural difference between them. Specification §1.1 and §9.1 re-aligned to the corrected source.

---

### 2. The sigil's narrowed list opens in the user's persisted grouping mode, not Flat

**Source**: Tree measurement — `sed -n '612,614p' cmd/open.go`, `sed -n '1190,1200p' internal/tui/model.go`
**Category**: Source defect
**Move**: route
**Affects**: §6.1 (Search result display — the requirement)

**Problem**:
The picker opens in whichever session-list grouping mode the user last left it in — that mode is persisted and re-applied at construction — so a user who last used By Project or By Tag gets the sigil's narrowed list in that mode, not in Flat. Flat is only the starting default, for a user who has never pressed `s`. The specification's case for showing the matched directory beside the name is pinned to Flat being where the sigil lands, and it offers the By-Project view's group heading as the place the information is already reachable. Under a committed filter that heading is not there: group heading rows carry an empty filter value and drop out the moment a filter is applied, so a sigil-opened By-Project list shows grouped-indented session names with no heading and no directory — the same unaccountable row the requirement exists to fix, in the mode the specification treats as already covered. An implementer reading it as "the sigil list is Flat" could scope the new directory column to flat rows and leave the grouped modes bare.

**Evidence**:
Claim (§6.1): "In Flat mode — the default, and where the sigil's narrowed list lands — the user is looking at a name with no visible relationship to what they typed."

Measurement 1 — the picker's initial grouping mode is loaded from persisted prefs, with Flat only as the fallback:

```
$ sed -n '612,614p' cmd/open.go
	initialMode := prefs.ModeFlat
	if prefsStore != nil {
		initialMode, _ = prefsStore.Load()

$ grep -n 'InitialMode' cmd/open.go internal/tui/build.go
cmd/open.go:537:		InitialMode:      cfg.initialMode,
internal/tui/build.go:45:	InitialMode prefs.SessionListMode
internal/tui/build.go:117:	opts = append(opts, WithInitialMode(deps.InitialMode))
```

Measurement 2 — the single list-rebuild chokepoint groups by that mode with no filter-state condition, so an applied filter does not return the list to Flat:

```
$ sed -n '1190,1200p' internal/tui/model.go
	var items []list.Item
	switch {
	case m.byTagSignpost:
		items = ToListItems(filtered)
	case m.sessionListMode == prefs.ModeByProject:
		items = buildByProject(m.resolveSessionDirs(filtered), m.projectIndex)
	case m.sessionListMode == prefs.ModeByTag:
		items = buildByTag(m.resolveSessionDirs(filtered), m.projectIndex)
	default:
		items = ToListItems(filtered)
	}
```

Measurement 3 — under a committed filter the group headings vanish while the grouped rows remain, so the By-Project "information is reachable there by another route" does not hold for a filtered list:

```
$ sed -n '93p' internal/tui/session_item.go
func (HeaderItem) FilterValue() string { return "" }

$ sed -n '215,217p' internal/tui/session_item.go
	indent := ""
	if it.GroupKey != "" {
		indent = groupRowIndent
```

Source carrying the claim: `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md`, `## search-result-display` (lines 928–931):

```
$ sed -n '928,931p' .workflows/open-with-forced-filter/discussion/open-with-forced-filter.md
In Flat mode, which is the default and where the sigil's narrowed list lands, the
user is looking at a name with no visible relationship to what they typed. The
By-Project view carries the project as a group heading, so the information is
reachable there by another route; Flat has none.
```

**Resolution**: Routed
**Notes**: Routed to `.workflows/open-with-forced-filter/discussion/open-with-forced-filter.md` (`## search-result-display`). The Context prose was repaired in place and the Decision block took a dated timeline revision (2026-09-11), the failed measurements as its trigger: the requirement re-lands unchanged in kind and broader in reach — the directory column applies in every grouping mode the sigil's list can be in, since none of them displays the matched directory under a committed filter. Specification §6.1 and §6.3 re-aligned to the revised decision.
