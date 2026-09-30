package state

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/tmuxerr"
)

// CaptureClient is declared here, over primitive types only, so internal/state
// need not import internal/tmux — which imports it back and would cycle.
type CaptureClient interface {
	ListSessionNames() ([]string, error)
	ListAllPanesWithFormat(format string) (string, error)
	ShowEnvironment(session string) (string, error)
}

// Fields are separated by "|||", which cannot occur in any captured tmux
// value. Columns are consumed by position, so new fields append at the end.
const captureFormat = "#{session_name}|||#{window_index}|||#{window_name}|||#{window_layout}|||#{window_zoomed_flag}|||#{window_active}|||#{pane_index}|||#{pane_current_path}|||#{pane_active}|||#{pane_current_command}|||#{" + PortalPaneIDOption + "}|||#{" + ResumePendingOption + "}"

const captureFieldCount = 12

const internalSessionPrefix = "_"

// CaptureStructure builds a canonical Index of every non-internal tmux
// session's structural topology; it captures no scrollback bytes.
//
// A live pane whose paneKey is in skipSet, and independently of skipSet a pane
// carrying the resume pending marker, keeps the CWD, CurrentCommand and
// ScrollbackFile of the previous record carrying its durable token, whatever its
// address has become. A skipped pane carrying no token keeps the whole previous
// record at its own address, a pending one those three fields of it, unless a
// live pane carries that record's token. A stale marker never resurrects a
// killed pane.
//
// A pane the enumeration lists as waiting whose session missed the capture —
// renamed or killed mid-capture, or failing its environment read — would lose
// its only transcript to the commit's housekeeping pass, so the previous
// session holding its record is carried into the Index whole, under its
// previous name, with any token a fresh record carries removed from it. A carry
// landing on the name of a session the capture reached is an error.
//
// A tmux enumeration failure yields an empty Index and a wrapped error, never a
// partial one. A per-session failure is logged and skipped, unless every
// session failed on something other than vanishing, which errors so the caller
// refuses to commit over a broken read.
//
// The second return holds the pane keys of every live pane carrying the resume
// pending marker, at the pane's live address and only for sessions the capture
// reached. It is non-nil on every return, and nothing about it is persisted.
func CaptureStructure(c CaptureClient, skipSet map[string]struct{}, prev *Index, logger *slog.Logger) (Index, map[string]struct{}, error) {
	captured, err := captureStructure(c, skipSet, prev, logger)
	return captured.index, captured.pending, err
}

type structureCapture struct {
	index   Index
	pending map[string]struct{}
	carried map[string]struct{}
}

// errCarryNameTaken refuses the commit rather than put two sessions under one
// name or drop the waiting pane's only transcript.
var errCarryNameTaken = errors.New("a waiting pane's session missed the capture and a captured session holds its previous name")

func captureStructure(c CaptureClient, skipSet map[string]struct{}, prev *Index, logger *slog.Logger) (structureCapture, error) {
	logger = loggerOrDiscard(logger)
	savedAt := time.Now().UTC()
	empty := structureCapture{
		index:   Index{Version: SchemaVersion, SavedAt: savedAt, Sessions: []Session{}},
		pending: map[string]struct{}{},
		carried: map[string]struct{}{},
	}

	names, err := c.ListSessionNames()
	if err != nil {
		return empty, err
	}

	keep := keepSessionNames(names)

	var grouped map[string][]paneRow
	if len(keep) > 0 {
		raw, err := c.ListAllPanesWithFormat(captureFormat)
		if err != nil {
			return empty, err
		}
		grouped, err = parsePaneRows(raw)
		if err != nil {
			return empty, err
		}
	}

	sessions := make([]Session, 0, len(keep))
	live := map[string]livePane{}
	var anomalousErrs []error
	naturalChurnCount := 0
	for _, name := range sortedKeys(keep) {
		envRaw, err := c.ShowEnvironment(name)
		if err != nil {
			if errors.Is(err, tmuxerr.ErrNoSuchSession) {
				naturalChurnCount++
				logger.Warn("capture skipping vanished session", "session", name, "error", err)
				continue
			}
			anomalousErrs = append(anomalousErrs, err)
			logger.Warn("capture anomalous session error", "session", name, "error", err)
			continue
		}
		sessions = append(sessions, Session{
			Name:        name,
			Environment: parseShowEnvironment(envRaw),
			Windows:     buildWindows(name, grouped[name]),
		})
		addPendingPanes(live, name, grouped[name])
	}

	if len(keep) > 0 && len(sessions) == 0 && len(anomalousErrs) > 0 {
		return empty, fmt.Errorf(
			"capture: all %d sessions failed (%d anomalous, %d natural): %w",
			len(keep), len(anomalousErrs), naturalChurnCount,
			errors.Join(anomalousErrs...))
	}

	idx := Index{Version: SchemaVersion, SavedAt: savedAt, Sessions: sessions}
	liveTokens := liveTokenSet(idx)

	if len(skipSet) > 0 && prev != nil {
		mergeSkippedPanes(&idx, *prev, skipSet, liveTokens)
	}

	if len(live) > 0 && prev != nil {
		mergeFrozenPanes(&idx, *prev, live, liveTokens)
	}

	carried := map[string]struct{}{}
	if prev != nil {
		if carried, err = carryMissedWaitingSessions(&idx, *prev, waitingTokens(grouped)); err != nil {
			return empty, err
		}
	}

	idx.Canonicalize()
	return structureCapture{index: idx, pending: paneKeySet(live), carried: carried}, nil
}

// waitingTokens holds the token of every enumerated pane carrying the resume
// pending marker, whichever session it was listed under.
func waitingTokens(grouped map[string][]paneRow) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, rows := range grouped {
		for _, r := range rows {
			if r.resumePending && r.portalPaneID != "" {
				tokens[r.portalPaneID] = struct{}{}
			}
		}
	}
	return tokens
}

// carryMissedWaitingSessions appends to fresh every previous session whose
// record answers to a waiting token no record of fresh carries, and returns
// the pane keys of what it appended. A token held by more than one previous
// record resolves to the first in canonical order, as the merges resolve it.
func carryMissedWaitingSessions(fresh *Index, prev Index, waiting map[string]struct{}) (map[string]struct{}, error) {
	placed := liveTokenSet(*fresh)
	toCarry := map[string]struct{}{}
	resolved := map[string]struct{}{}
	for _, e := range canonicalPrevPanes(prev) {
		token := e.pane.PortalPaneID
		if _, isWaiting := waiting[token]; !isWaiting {
			continue
		}
		if _, done := resolved[token]; done {
			continue
		}
		resolved[token] = struct{}{}
		if _, onFresh := placed[token]; onFresh {
			continue
		}
		toCarry[e.session] = struct{}{}
	}
	if len(toCarry) == 0 {
		return map[string]struct{}{}, nil
	}

	for _, s := range fresh.Sessions {
		if _, taken := toCarry[s.Name]; taken {
			return nil, fmt.Errorf("carry session %q: %w", s.Name, errCarryNameTaken)
		}
	}

	carried := map[string]struct{}{}
	for _, name := range sortedKeys(toCarry) {
		s := carriedSession(prev, name, placed)
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				carried[SanitizePaneKey(s.Name, w.Index, p.Index)] = struct{}{}
			}
		}
		fresh.Sessions = append(fresh.Sessions, s)
	}
	sort.SliceStable(fresh.Sessions, func(i, j int) bool {
		return fresh.Sessions[i].Name < fresh.Sessions[j].Name
	})
	return carried, nil
}

// carriedSession copies the first previous session named name, so nothing
// done to the carried copy reaches the caller's previous index. A token
// already in placed is removed from the copy, and every token the copy keeps
// is added to placed, so no token lands on a second record.
func carriedSession(prev Index, name string, placed map[string]struct{}) Session {
	var src Session
	for _, s := range prev.Sessions {
		if s.Name == name {
			src = s
			break
		}
	}
	out := Session{Name: src.Name, Environment: maps.Clone(src.Environment), Windows: make([]Window, len(src.Windows))}
	for wi, w := range src.Windows {
		w.Panes = slices.Clone(w.Panes)
		for pi := range w.Panes {
			p := &w.Panes[pi]
			if p.PortalPaneID == "" {
				continue
			}
			if _, taken := placed[p.PortalPaneID]; taken {
				p.PortalPaneID = ""
				continue
			}
			placed[p.PortalPaneID] = struct{}{}
		}
		out.Windows[wi] = w
	}
	return out
}

// livePane holds what the enumeration knows about a waiting pane, so the merge
// reads its token and active flag from the read rather than from the fresh index,
// which the skeleton merge may already have replaced with a previous record.
type livePane struct {
	token  string
	active bool
}

func addPendingPanes(live map[string]livePane, session string, rows []paneRow) {
	for _, r := range rows {
		if !r.resumePending {
			continue
		}
		live[SanitizePaneKey(session, r.windowIdx, r.paneIdx)] = livePane{
			token:  r.portalPaneID,
			active: r.paneActive,
		}
	}
}

func paneKeySet(live map[string]livePane) map[string]struct{} {
	keys := make(map[string]struct{}, len(live))
	for k := range live {
		keys[k] = struct{}{}
	}
	return keys
}

// mergeSkippedPanes matches on the token because restore re-stamps it before
// arming the pane, so a pane restored away from its saved address still finds
// its own record. It mutates only panes the live enumeration returned.
func mergeSkippedPanes(fresh *Index, prev Index, skipSet map[string]struct{}, liveTokens map[string]struct{}) {
	byToken, byAddress := indexPrevPanes(prev, liveTokens)
	for si := range fresh.Sessions {
		s := &fresh.Sessions[si]
		for wi := range s.Windows {
			w := &s.Windows[wi]
			for pi := range w.Panes {
				p := &w.Panes[pi]
				key := SanitizePaneKey(s.Name, w.Index, p.Index)
				if _, skipped := skipSet[key]; !skipped {
					continue
				}
				record, found := takePrevRecord(byToken, byAddress, p.PortalPaneID, key)
				if !found {
					continue
				}
				if p.PortalPaneID == "" {
					*p = record
					continue
				}
				carryPrevContent(p, record)
			}
		}
	}
}

// mergeFrozenPanes carries a waiting pane's previous record onto its live
// address, matched on the pane's durable token (on its address when it carries
// none): the merged record keeps pointing at the scrollback file that already
// holds the pane's bytes, whatever the pane's address has become. It mutates
// only panes the live enumeration returned, so a previous record whose pane is
// gone is never reintroduced.
func mergeFrozenPanes(fresh *Index, prev Index, live map[string]livePane, liveTokens map[string]struct{}) {
	byToken, byAddress := indexPrevPanes(prev, liveTokens)
	for si := range fresh.Sessions {
		s := &fresh.Sessions[si]
		for wi := range s.Windows {
			w := &s.Windows[wi]
			for pi := range w.Panes {
				p := &w.Panes[pi]
				key := SanitizePaneKey(s.Name, w.Index, p.Index)
				waiting, isWaiting := live[key]
				if !isWaiting {
					continue
				}
				p.PortalPaneID = waiting.token
				p.Active = waiting.active
				record, found := takePrevRecord(byToken, byAddress, waiting.token, key)
				if !found {
					continue
				}
				carryPrevContent(p, record)
			}
		}
	}
}

func carryPrevContent(p *Pane, record Pane) {
	p.CWD = record.CWD
	p.CurrentCommand = record.CurrentCommand
	p.ScrollbackFile = record.ScrollbackFile
}

// liveTokenSet holds the token of every record in fresh: before any merge
// replaces a pane with a previous record, that is every live pane's token.
func liveTokenSet(fresh Index) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, s := range fresh.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.PortalPaneID != "" {
					tokens[p.PortalPaneID] = struct{}{}
				}
			}
		}
	}
	return tokens
}

// indexPrevPanes reads prev in canonical order, so a token held by more than
// one record resolves to the first of them. A record whose token a live pane
// carries is left out of byAddress: it belongs to the pane answering to that
// token, so a pane at its old address must never take it — which would commit
// one token on two records.
func indexPrevPanes(prev Index, liveTokens map[string]struct{}) (byToken, byAddress map[string]Pane) {
	byToken = map[string]Pane{}
	byAddress = map[string]Pane{}
	for _, e := range canonicalPrevPanes(prev) {
		if e.pane.PortalPaneID != "" {
			if _, taken := byToken[e.pane.PortalPaneID]; !taken {
				byToken[e.pane.PortalPaneID] = e.pane
			}
			if _, owned := liveTokens[e.pane.PortalPaneID]; owned {
				continue
			}
		}
		byAddress[e.key] = e.pane
	}
	return byToken, byAddress
}

type prevPaneEntry struct {
	session string
	window  int
	key     string
	pane    Pane
}

func canonicalPrevPanes(prev Index) []prevPaneEntry {
	var entries []prevPaneEntry
	for _, s := range prev.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				entries = append(entries, prevPaneEntry{
					session: s.Name,
					window:  w.Index,
					key:     SanitizePaneKey(s.Name, w.Index, p.Index),
					pane:    p,
				})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.session != b.session {
			return a.session < b.session
		}
		if a.window != b.window {
			return a.window < b.window
		}
		return a.pane.Index < b.pane.Index
	})
	return entries
}

// takePrevRecord consumes the entry it returns, so no two live panes matched
// the same way take one previous record.
func takePrevRecord(byToken, byAddress map[string]Pane, token, key string) (Pane, bool) {
	if token != "" {
		record, found := byToken[token]
		if found {
			delete(byToken, token)
		}
		return record, found
	}
	record, found := byAddress[key]
	if found {
		delete(byAddress, key)
	}
	return record, found
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func keepSessionNames(names []string) map[string]struct{} {
	keep := make(map[string]struct{}, len(names))
	for _, name := range names {
		if isInternalSession(name) {
			continue
		}
		keep[name] = struct{}{}
	}
	return keep
}

func isInternalSession(name string) bool {
	return strings.HasPrefix(name, internalSessionPrefix)
}

type paneRow struct {
	session        string
	windowIdx      int
	windowName     string
	layout         string
	zoomed         bool
	windowActive   bool
	paneIdx        int
	cwd            string
	paneActive     bool
	currentCommand string
	portalPaneID   string
	resumePending  bool
}

// parsePaneRows groups every non-internal row by the session it was listed
// under, including sessions the session-name read did not return.
func parsePaneRows(raw string) (map[string][]paneRow, error) {
	out := map[string][]paneRow{}
	if raw == "" {
		return out, nil
	}
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		row, err := parsePaneRow(line)
		if err != nil {
			return nil, err
		}
		if isInternalSession(row.session) {
			continue
		}
		out[row.session] = append(out[row.session], row)
	}
	return out, nil
}

func parsePaneRow(line string) (paneRow, error) {
	parts := strings.Split(line, "|||")
	if len(parts) != captureFieldCount {
		return paneRow{}, fmt.Errorf("unexpected pane row field count %d in %q", len(parts), line)
	}
	windowIdx, err := strconv.Atoi(parts[1])
	if err != nil {
		return paneRow{}, fmt.Errorf("invalid window index %q: %w", parts[1], err)
	}
	paneIdx, err := strconv.Atoi(parts[6])
	if err != nil {
		return paneRow{}, fmt.Errorf("invalid pane index %q: %w", parts[6], err)
	}
	return paneRow{
		session:        parts[0],
		windowIdx:      windowIdx,
		windowName:     parts[2],
		layout:         parts[3],
		zoomed:         parseTmuxBool(parts[4]),
		windowActive:   parseTmuxBool(parts[5]),
		paneIdx:        paneIdx,
		cwd:            parts[7],
		paneActive:     parseTmuxBool(parts[8]),
		currentCommand: parts[9],
		portalPaneID:   parts[10],
		resumePending:  ResumePendingSet(parts[11]),
	}, nil
}

func parseTmuxBool(s string) bool {
	return s == "1"
}

func buildWindows(session string, rows []paneRow) []Window {
	byWindow := make(map[int][]paneRow)
	for _, r := range rows {
		byWindow[r.windowIdx] = append(byWindow[r.windowIdx], r)
	}

	indices := make([]int, 0, len(byWindow))
	for i := range byWindow {
		indices = append(indices, i)
	}
	sort.Ints(indices)

	windows := make([]Window, 0, len(indices))
	for _, wi := range indices {
		group := byWindow[wi]
		// Window-level fields repeat on every pane row of the window.
		head := group[0]
		windows = append(windows, Window{
			Index:  head.windowIdx,
			Name:   head.windowName,
			Layout: head.layout,
			Zoomed: head.zoomed,
			Active: head.windowActive,
			Panes:  buildPanes(session, head.windowIdx, group),
		})
	}
	return windows
}

func buildPanes(session string, windowIdx int, rows []paneRow) []Pane {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].paneIdx < rows[j].paneIdx
	})
	panes := make([]Pane, 0, len(rows))
	for _, r := range rows {
		key := SanitizePaneKey(session, windowIdx, r.paneIdx)
		panes = append(panes, Pane{
			Index:          r.paneIdx,
			CWD:            r.cwd,
			Active:         r.paneActive,
			CurrentCommand: r.currentCommand,
			ScrollbackFile: positionalScrollbackFile(key),
			PortalPaneID:   r.portalPaneID,
		})
	}
	return panes
}

func parseShowEnvironment(raw string) map[string]string {
	env := map[string]string{}
	if raw == "" {
		return env
	}
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "-") {
			continue
		}
		before, after, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		env[before] = after
	}
	return env
}
