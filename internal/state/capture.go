package state

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CaptureClient is the narrow interface CaptureStructure needs from a tmux
// client. Defining it in this package — and using only primitive return types
// — means internal/state has no import-time dependency on internal/tmux,
// which would otherwise close a cycle (internal/tmux already imports
// internal/state for daemon-state plumbing).
//
// *tmux.Client satisfies this interface implicitly: ListSessionNames is a
// thin wrapper provided in internal/tmux, while ListAllPanesWithFormat and
// ShowEnvironment match the existing tmux methods exactly.
type CaptureClient interface {
	ListSessionNames() ([]string, error)
	ListAllPanesWithFormat(format string) (string, error)
	ShowEnvironment(session string) (string, error)
}

// captureFormat is the tmux -F format string used to read the structural
// topology of every pane across every session in a single list-panes -a call.
// Fields are separated by "|||" — three characters that cannot occur in any
// of the captured tmux variables.
const captureFormat = "#{session_name}|||#{window_index}|||#{window_name}|||#{window_layout}|||#{window_zoomed_flag}|||#{window_active}|||#{pane_index}|||#{pane_current_path}|||#{pane_active}|||#{pane_current_command}"

const captureFieldCount = 10

// internalSessionPrefix marks tmux sessions that Portal owns and which must
// not appear in the captured structural index. See specification → Session
// Visibility and Filtering.
const internalSessionPrefix = "_"

// CaptureStructure builds an in-memory Index of every non-internal tmux
// session's structural topology. It does not capture scrollback bytes; that
// is the daemon's responsibility (see specification → Atomic Commit
// Discipline). Callers receive a fully canonical Index with sessions sorted
// alphabetically and windows/panes sorted by index ascending — the result is
// stable for downstream encoding.
//
// skipSet contains paneKeys (as produced by SanitizePaneKey) of skeleton-
// restored panes whose pre-boot state must not be overwritten by a fresh
// capture. When non-empty and prev is non-nil, the corresponding pane entries
// from prev are merged into the result — sessions and windows are
// reintroduced if absent, and existing panes at matching paths are
// overwritten with the prev data. See specification → Marker Coordination →
// `@portal-skeleton-<paneKey>`.
//
// When skipSet is empty or prev is nil, the merge is a no-op and the result
// is exactly the fresh capture.
//
// On any tmux error, CaptureStructure returns an Index with an empty Sessions
// slice and a wrapped error — never a partial index. This matches the spec's
// "all reads run to completion before any writes" discipline: a downstream
// writer keying off the returned error will not commit a half-built state.
func CaptureStructure(c CaptureClient, skipSet map[string]struct{}, prev *Index) (Index, error) {
	savedAt := time.Now().UTC()
	empty := Index{Version: SchemaVersion, SavedAt: savedAt, Sessions: []Session{}}

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
		grouped, err = parsePaneRows(raw, keep)
		if err != nil {
			return empty, err
		}
	}

	sessions := make([]Session, 0, len(keep))
	for _, name := range sortedKeys(keep) {
		envRaw, err := c.ShowEnvironment(name)
		if err != nil {
			return empty, err
		}
		sessions = append(sessions, Session{
			Name:        name,
			Environment: parseShowEnvironment(envRaw),
			Windows:     buildWindows(name, grouped[name]),
		})
	}

	idx := Index{Version: SchemaVersion, SavedAt: savedAt, Sessions: sessions}

	if len(skipSet) > 0 && prev != nil {
		mergeSkippedPanes(&idx, *prev, skipSet)
	}

	idx.Canonicalize()
	return idx, nil
}

// mergeSkippedPanes reintroduces or overrides panes in fresh whose paneKey is
// in skipSet, taking authoritative state from prev. Existing panes at matching
// (window, pane) coordinates are replaced. The result is re-sorted so the
// canonical ordering survives the merge.
//
// A skeleton marker is no longer treated as authoritative on its own: the
// merge proceeds for a given prev pane only when its session, window index,
// AND pane index are all still present in the freshly-captured index. This
// rejects stale markers that point at killed sessions, killed windows, or
// killed panes — see specification → Fix Component A → Filtering Levels.
//
// Matching is by structural identity — session name, window index, pane
// index — derived via SanitizePaneKey. That is the same paneKey used to set
// the skeleton marker, so prev and skipSet always agree on which panes count.
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

// buildLiveStructure projects fresh's Sessions/Windows/Panes into a nested
// lookup map keyed by session name → window index → pane index. The map is
// the live-tmux truth at the call site of mergeSkippedPanes and is used to
// gate prev-pane merges so stale skeleton markers cannot resurrect killed
// sessions, windows, or panes. Window/pane levels are populated now to keep
// the helper's shape stable as additional filtering levels land in subsequent
// tasks.
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

// mergePane integrates a single (session, window, pane) triple from prev into
// fresh. The session and window are created if they do not already exist;
// the pane replaces any existing entry at the same index, otherwise it is
// appended. Environment is taken from prev only when creating a new session
// — pre-existing fresh sessions keep their environment.
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

// findOrAppendSession returns the index in fresh.Sessions of the session with
// ps's name. If absent, a shallow copy of ps (with no windows) is appended
// and the new index is returned. The caller is responsible for populating
// the session's windows via subsequent merges.
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

// findOrAppendWindow returns the index in s.Windows of the window with pw's
// index. If absent, a shallow copy of pw (with no panes) is appended and the
// new index is returned. Panes are populated by the caller.
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

// resortIndex restores the canonical ordering of an Index after merge:
// sessions ascending by name, windows ascending by index, panes ascending by
// index. Canonicalize is responsible for nil-slice/map normalisation; this
// helper only sorts.
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

// sortedKeys returns the keys of set in ascending lexicographic order. Used
// to produce a deterministic per-session iteration order for the captured
// index.
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// keepSessionNames returns the set of session names that are eligible for
// capture: those that do not begin with the internal-session prefix.
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

// paneRow holds one row of structural pane state parsed from list-panes -a.
// Window-level fields (windowName, layout, zoomed, windowActive) are repeated
// for every pane in the same window; the consumer takes them from the first
// row encountered for that window.
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
}

// parsePaneRows splits raw list-panes -a output into rows grouped by session
// name. Rows whose session is not in keep are silently skipped.
func parsePaneRows(raw string, keep map[string]struct{}) (map[string][]paneRow, error) {
	out := make(map[string][]paneRow, len(keep))
	if raw == "" {
		return out, nil
	}
	for _, line := range strings.Split(raw, "\n") {
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

// parsePaneRow parses a single pane line of the format produced by
// captureFormat. Returns an error for malformed rows so the caller can abort
// rather than silently producing an inconsistent index.
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
	}, nil
}

// parseTmuxBool maps tmux's "1"/"0" string-form boolean to Go's bool. Any
// value other than "1" — including empty string — maps to false.
func parseTmuxBool(s string) bool {
	return s == "1"
}

// buildWindows groups pane rows by window and produces a sorted []Window for
// the named session. Windows are sorted by index ascending; panes within each
// window are likewise sorted by index ascending.
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
		// Window-level fields are repeated per pane row; first row is canonical.
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

// buildPanes converts the rows belonging to a single window into a sorted
// []Pane with each pane's ScrollbackFile set to the canonical relative path.
func buildPanes(session string, windowIdx int, rows []paneRow) []Pane {
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].paneIdx < rows[j].paneIdx
	})
	panes := make([]Pane, 0, len(rows))
	for _, r := range rows {
		key := SanitizePaneKey(session, windowIdx, r.paneIdx)
		// filepath.ToSlash normalises to forward slashes so the on-disk schema
		// is identical across platforms (the daemon may run on Windows in
		// future; the spec stores forward-slash relative paths).
		path := filepath.ToSlash(filepath.Join("scrollback", key+".bin"))
		panes = append(panes, Pane{
			Index:          r.paneIdx,
			CWD:            r.cwd,
			Active:         r.paneActive,
			CurrentCommand: r.currentCommand,
			ScrollbackFile: path,
		})
	}
	return panes
}

// parseShowEnvironment parses raw "tmux show-environment -t <session>" output
// into a map[string]string. Lines starting with "-" represent variables marked
// as removed from the session environment and are skipped. Lines without an
// "=" are skipped silently. Values may themselves contain "=" — only the first
// occurrence is treated as the separator.
//
// The returned map is always non-nil; an empty input yields an empty map so
// callers do not need to guard against a nil receiver.
func parseShowEnvironment(raw string) map[string]string {
	env := map[string]string{}
	if raw == "" {
		return env
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "-") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		env[line[:eq]] = line[eq+1:]
	}
	return env
}
