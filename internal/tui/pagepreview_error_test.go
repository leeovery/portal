package tui

import (
	"errors"
	"reflect"
	"strings"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// nilErrReader returns (nil, err) on every Tail call, simulating an OS-level
// read failure (EACCES, EIO, etc.) per § Read-Failure Handling > Placeholder
// > Error string. It records every paneKey passed so the "refocus retries"
// contract can be asserted on call counts.
type nilErrReader struct {
	err   error
	calls []string
}

func (r *nilErrReader) Tail(paneKey string) ([]byte, error) {
	r.calls = append(r.calls, paneKey)
	return nil, r.err
}

// keyedReader returns a per-paneKey (bytes, err) outcome so tests that
// straddle one successful and one failing pane (Tab away / Tab back) can
// drive distinct outcomes from a single reader instance.
type keyedReader struct {
	outcomes map[string]struct {
		bytes []byte
		err   error
	}
	calls []string
}

func (r *keyedReader) Tail(paneKey string) ([]byte, error) {
	r.calls = append(r.calls, paneKey)
	o := r.outcomes[paneKey]
	return o.bytes, o.err
}

// sequenceReader returns a different (bytes, err) outcome on each successive
// call to Tail for the same paneKey — the "transient error → success" shape:
// first call fails, second call succeeds.
type sequenceReader struct {
	outcomes []struct {
		bytes []byte
		err   error
	}
	calls []string
	idx   int
}

func (r *sequenceReader) Tail(paneKey string) ([]byte, error) {
	r.calls = append(r.calls, paneKey)
	o := r.outcomes[r.idx]
	if r.idx < len(r.outcomes)-1 {
		r.idx++
	}
	return o.bytes, o.err
}

func TestPreviewError_RendersAtInitialOpenWhenTailReturnsNilErr(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &nilErrReader{err: errors.New("EACCES")}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)

	if !ok {
		t.Fatalf("expected ok=true on (nil, err) initial open, got false")
	}
	got := stripTrailingBlanks(m.viewport.View())
	if got != previewReadError {
		t.Errorf("viewport content = %q; want %q", got, previewReadError)
	}
}

func TestPreviewError_StringIsUniformAcrossErrnoTypes(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}

	errs := []error{
		syscall.EACCES,
		syscall.EIO,
		errors.New("custom error"),
	}

	views := make([]string, len(errs))
	for i, e := range errs {
		enum := &stubEnumerator{groups: groups}
		reader := &nilErrReader{err: e}
		m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
		if !ok {
			t.Fatalf("err %d (%v): expected ok=true, got false", i, e)
		}
		views[i] = m.viewport.View()
	}

	for i := 1; i < len(views); i++ {
		if views[i] != views[0] {
			t.Errorf("err %d viewport.View() differs from err 0:\n[0]=%q\n[%d]=%q", i, views[0], i, views[i])
		}
	}
}

func TestPreviewError_StringDiffersFromPlaceholder(t *testing.T) {
	if previewReadError == previewPlaceholder {
		t.Errorf("previewReadError must differ from previewPlaceholder; both = %q", previewReadError)
	}
}

func TestPreviewError_StringIsCanonicalWordingUnableToReadScrollback(t *testing.T) {
	if previewReadError != "(unable to read scrollback)" {
		t.Errorf("previewReadError = %q; want %q", previewReadError, "(unable to read scrollback)")
	}
}

func TestPreviewError_RefocusAfterErrorIssuesFreshTailViaTab(t *testing.T) {
	// Two panes in one window. Pane 0 errors; pane 1 succeeds. Tab to pane 1
	// (success), Tab back to pane 0 — paneKey for pane 0 must appear twice
	// in reader.calls (initial via the constructor + retry on refocus).
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
	}
	pane0Key := state.SanitizePaneKey("work", 0, 0)
	pane1Key := state.SanitizePaneKey("work", 0, 1)
	reader := &keyedReader{
		outcomes: map[string]struct {
			bytes []byte
			err   error
		}{
			pane0Key: {bytes: nil, err: syscall.EACCES},
			pane1Key: {bytes: []byte("ok"), err: nil},
		},
	}

	enum := &stubEnumerator{groups: groups}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("constructor: expected ok=true, got false")
	}
	// Tab to pane 1, then Tab back to pane 0.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	pane0Calls := 0
	for _, c := range reader.calls {
		if c == pane0Key {
			pane0Calls++
		}
	}
	if pane0Calls != 2 {
		t.Errorf("expected 2 Tail calls for pane0 (initial + retry on refocus), got %d (all calls=%v)", pane0Calls, reader.calls)
	}
	if m.paneIdx != 0 {
		t.Fatalf("expected paneIdx=0 after Tab cycle, got %d", m.paneIdx)
	}
	got := stripTrailingBlanks(m.viewport.View())
	if got != previewReadError {
		t.Errorf("viewport content after refocus = %q; want %q", got, previewReadError)
	}
}

func TestPreviewError_RefocusAfterErrorIssuesFreshTailViaBracket(t *testing.T) {
	// Two windows, one pane each. Window 0 / pane 0 errors; window 1 / pane 0
	// succeeds. ] to window 1, ] back to window 0 (wraps).
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0}},
	}
	w0Key := state.SanitizePaneKey("work", 0, 0)
	w1Key := state.SanitizePaneKey("work", 1, 0)
	reader := &keyedReader{
		outcomes: map[string]struct {
			bytes []byte
			err   error
		}{
			w0Key: {bytes: nil, err: syscall.EIO},
			w1Key: {bytes: []byte("ok"), err: nil},
		},
	}

	enum := &stubEnumerator{groups: groups}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("constructor: expected ok=true, got false")
	}
	// ] to window 1, ] again wraps to window 0.
	m, _ = m.Update(nextWindowKey)
	m, _ = m.Update(nextWindowKey)

	w0Calls := 0
	for _, c := range reader.calls {
		if c == w0Key {
			w0Calls++
		}
	}
	if w0Calls != 2 {
		t.Errorf("expected 2 Tail calls for window0/pane0 (initial + retry on refocus), got %d (all calls=%v)", w0Calls, reader.calls)
	}
	if m.windowIdx != 0 {
		t.Fatalf("expected windowIdx=0 after wrap, got %d", m.windowIdx)
	}
	got := stripTrailingBlanks(m.viewport.View())
	if got != previewReadError {
		t.Errorf("viewport content after refocus = %q; want %q", got, previewReadError)
	}
}

func TestPreviewError_SecondTailCallAfterErrorSeesNewOutcome(t *testing.T) {
	// Tab away and back to the same pane: the second Tail call sees a fresh
	// outcome (success), and the viewport renders bytes — not the error
	// string. This nails down "no per-pane error cache".
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
	}
	pane0Key := state.SanitizePaneKey("work", 0, 0)

	// Sequence per call to Tail:
	//   1. constructor reads pane 0 → error
	//   2. Tab to pane 1 reads pane 1 → success ("other")
	//   3. Tab back to pane 0 reads pane 0 → success ("recovered")
	reader := &sequenceReader{
		outcomes: []struct {
			bytes []byte
			err   error
		}{
			{bytes: nil, err: syscall.EACCES},
			{bytes: []byte("other"), err: nil},
			{bytes: []byte("recovered"), err: nil},
		},
	}

	enum := &stubEnumerator{groups: groups}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("constructor: expected ok=true, got false")
	}
	// Initial open shows the error string.
	if got := stripTrailingBlanks(m.viewport.View()); got != previewReadError {
		t.Fatalf("initial open: viewport = %q; want %q", got, previewReadError)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if m.paneIdx != 0 {
		t.Fatalf("expected paneIdx=0 after Tab cycle, got %d", m.paneIdx)
	}
	got := stripTrailingBlanks(m.viewport.View())
	if got == previewReadError {
		t.Errorf("expected viewport to render new bytes after recovery, still got error string")
	}
	if !strings.Contains(m.viewport.View(), "recovered") {
		t.Errorf("expected viewport to contain %q after recovery, got %q", "recovered", m.viewport.View())
	}

	// And pane0 was Tail'd twice.
	pane0Calls := 0
	for _, c := range reader.calls {
		if c == pane0Key {
			pane0Calls++
		}
	}
	if pane0Calls != 2 {
		t.Errorf("expected 2 Tail calls for pane0 (initial + retry), got %d", pane0Calls)
	}
}

func TestPreviewError_NoPerPaneErrorStateOnPreviewModel(t *testing.T) {
	// Code-inspection-style guard: enumerate previewModel's fields and assert
	// no error-cache-shaped field exists (any name containing "error" or
	// "errByPaneKey" or shaped like map[string]error / per-pane string cache).
	// This pins the "no per-pane error cache" decision in the type itself
	// rather than relying solely on behavioural tests.
	tp := reflect.TypeOf(previewModel{})
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		name := strings.ToLower(f.Name)
		if strings.Contains(name, "error") || strings.Contains(name, "errcache") || strings.Contains(name, "errby") {
			t.Errorf("previewModel has field %q (%s) — per-pane error cache state forbidden by spec", f.Name, f.Type)
		}
		// Any map keyed by string with error or string values would shape an
		// error-by-paneKey cache; flag them.
		if f.Type.Kind() == reflect.Map && f.Type.Key().Kind() == reflect.String {
			elem := f.Type.Elem()
			if elem.Kind() == reflect.String || elem.Implements(reflect.TypeOf((*error)(nil)).Elem()) {
				t.Errorf("previewModel has map field %q (%s) — error-by-paneKey cache shape forbidden by spec", f.Name, f.Type)
			}
		}
	}
}

func TestPreviewError_ChromeCountsUnaffectedByErrorBranch(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "other", PaneIndices: []int{0}},
	}
	enum := &stubEnumerator{groups: groups}
	reader := &nilErrReader{err: syscall.EACCES}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	// chromeLine() under the error branch must be byte-identical to the
	// chromeLine() of an equivalent model whose reader returned bytes — chrome
	// is a pure function of cached groups + windowIdx + paneIdx.
	got := stripANSI(chromeLineForTest(m))
	expected := stripANSI(chromeLineForTest(newPreviewModelForHelpers("work", groups, 0, 0)))
	if got != expected {
		t.Errorf("chromeLine() under error = %q; want %q (identical to non-error shape)", got, expected)
	}
}
