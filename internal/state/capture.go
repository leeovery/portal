package state

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
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
// Panes whose paneKey is in skipSet keep their prev state, but only where
// session, window and pane are all still live — a stale marker must not
// resurrect a killed pane. Independently of skipSet, a pane carrying the resume
// pending marker keeps its prev record's CWD, CurrentCommand and
// ScrollbackFile, matched on the pane's durable token rather than its address.
// A tmux enumeration failure yields an empty Index and a wrapped error, never a
// partial one. A per-session failure is logged and skipped, unless every
// session failed on something other than vanishing, which errors so the caller
// refuses to commit over a broken read.
//
// The second return holds the pane keys of every live pane carrying the resume
// pending marker, at the pane's live address and only for sessions that reached
// the Index. It is non-nil on every return, and nothing about it is persisted.
func CaptureStructure(c CaptureClient, skipSet map[string]struct{}, prev *Index, logger *slog.Logger) (Index, map[string]struct{}, error) {
	logger = loggerOrDiscard(logger)
	savedAt := time.Now().UTC()
	empty := Index{Version: SchemaVersion, SavedAt: savedAt, Sessions: []Session{}}
	emptyPending := map[string]struct{}{}

	names, err := c.ListSessionNames()
	if err != nil {
		return empty, emptyPending, err
	}

	keep := keepSessionNames(names)

	var grouped map[string][]paneRow
	if len(keep) > 0 {
		raw, err := c.ListAllPanesWithFormat(captureFormat)
		if err != nil {
			return empty, emptyPending, err
		}
		grouped, err = parsePaneRows(raw, keep)
		if err != nil {
			return empty, emptyPending, err
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
		return empty, emptyPending, fmt.Errorf(
			"capture: all %d sessions failed (%d anomalous, %d natural): %w",
			len(keep), len(anomalousErrs), naturalChurnCount,
			errors.Join(anomalousErrs...))
	}

	idx := Index{Version: SchemaVersion, SavedAt: savedAt, Sessions: sessions}

	if len(skipSet) > 0 && prev != nil {
		mergeSkippedPanes(&idx, *prev, skipSet)
	}

	if len(live) > 0 && prev != nil {
		mergeFrozenPanes(&idx, *prev, live)
	}

	idx.Canonicalize()
	return idx, paneKeySet(live), nil
}

// livePane holds what the enumeration knows about a waiting pane, so the merge
// reads its token and liveness from the read rather than from the fresh index,
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

func mergeSkippedPanes(fresh *Index, prev Index, skipSet map[string]struct{}) {
	live := buildLiveStructure(*fresh)
	for _, ps := range prev.Sessions {
		liveWindows, sessionLive := live[ps.Name]
		if !sessionLive {
			continue
		}
		for _, pw := range ps.Windows {
			livePanes, windowLive := liveWindows[pw.Index]
			if !windowLive {
				continue
			}
			for _, pp := range pw.Panes {
				if _, paneLive := livePanes[pp.Index]; !paneLive {
					continue
				}
				key := SanitizePaneKey(ps.Name, pw.Index, pp.Index)
				if _, skipped := skipSet[key]; !skipped {
					continue
				}
				mergePane(fresh, ps, pw, pp)
			}
		}
	}
	resortIndex(fresh)
}

// mergeFrozenPanes carries a waiting pane's previous record onto its live
// address, matched on the pane's durable token: the merged record keeps
// pointing at the scrollback file that already holds the pane's bytes, whatever
// the pane's address has become. It mutates only panes the live enumeration
// returned, so a previous record whose pane is gone is never reintroduced.
func mergeFrozenPanes(fresh *Index, prev Index, live map[string]livePane) {
	byToken, byAddress := indexPrevPanes(prev)
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
				p.CWD = record.CWD
				p.CurrentCommand = record.CurrentCommand
				p.ScrollbackFile = record.ScrollbackFile
			}
		}
	}
}

// indexPrevPanes reads prev in canonical order, so a token held by more than
// one record resolves to the first of them.
func indexPrevPanes(prev Index) (byToken, byAddress map[string]Pane) {
	byToken = map[string]Pane{}
	byAddress = map[string]Pane{}
	for _, e := range canonicalPrevPanes(prev) {
		if e.pane.PortalPaneID != "" {
			if _, taken := byToken[e.pane.PortalPaneID]; !taken {
				byToken[e.pane.PortalPaneID] = e.pane
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

// takePrevRecord consumes the entry it returns, so one previous record serves
// at most one live pane.
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

func buildLiveStructure(idx Index) map[string]map[int]map[int]struct{} {
	live := make(map[string]map[int]map[int]struct{}, len(idx.Sessions))
	for _, s := range idx.Sessions {
		windows := make(map[int]map[int]struct{}, len(s.Windows))
		for _, w := range s.Windows {
			panes := make(map[int]struct{}, len(w.Panes))
			for _, p := range w.Panes {
				panes[p.Index] = struct{}{}
			}
			windows[w.Index] = panes
		}
		live[s.Name] = windows
	}
	return live
}

func mergePane(fresh *Index, ps Session, pw Window, pp Pane) {
	si := findOrAppendSession(fresh, ps)
	wi := findOrAppendWindow(&fresh.Sessions[si], pw)
	w := &fresh.Sessions[si].Windows[wi]
	for i := range w.Panes {
		if w.Panes[i].Index == pp.Index {
			w.Panes[i] = pp
			return
		}
	}
	w.Panes = append(w.Panes, pp)
}

func findOrAppendSession(fresh *Index, ps Session) int {
	for i := range fresh.Sessions {
		if fresh.Sessions[i].Name == ps.Name {
			return i
		}
	}
	fresh.Sessions = append(fresh.Sessions, Session{
		Name:        ps.Name,
		Environment: ps.Environment,
		Windows:     []Window{},
	})
	return len(fresh.Sessions) - 1
}

func findOrAppendWindow(s *Session, pw Window) int {
	for i := range s.Windows {
		if s.Windows[i].Index == pw.Index {
			return i
		}
	}
	s.Windows = append(s.Windows, Window{
		Index:  pw.Index,
		Name:   pw.Name,
		Layout: pw.Layout,
		Zoomed: pw.Zoomed,
		Active: pw.Active,
		Panes:  []Pane{},
	})
	return len(s.Windows) - 1
}

func resortIndex(idx *Index) {
	sort.Slice(idx.Sessions, func(i, j int) bool {
		return idx.Sessions[i].Name < idx.Sessions[j].Name
	})
	for si := range idx.Sessions {
		ws := idx.Sessions[si].Windows
		sort.Slice(ws, func(i, j int) bool { return ws[i].Index < ws[j].Index })
		for wi := range ws {
			ps := ws[wi].Panes
			sort.Slice(ps, func(i, j int) bool { return ps[i].Index < ps[j].Index })
		}
	}
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
		if strings.HasPrefix(name, internalSessionPrefix) {
			continue
		}
		keep[name] = struct{}{}
	}
	return keep
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

func parsePaneRows(raw string, keep map[string]struct{}) (map[string][]paneRow, error) {
	out := make(map[string][]paneRow, len(keep))
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
		if _, ok := keep[row.session]; !ok {
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
		// The on-disk schema stores forward slashes on every platform.
		path := filepath.ToSlash(filepath.Join("scrollback", key+".bin"))
		panes = append(panes, Pane{
			Index:          r.paneIdx,
			CWD:            r.cwd,
			Active:         r.paneActive,
			CurrentCommand: r.currentCommand,
			ScrollbackFile: path,
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
