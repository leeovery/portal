package tui

import (
	"errors"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

type nilErrReader struct {
	err   error
	calls []string
}

func (r *nilErrReader) Tail(pane PaneScrollback) ([]byte, error) {
	paneKey := pane.PaneKey
	r.calls = append(r.calls, paneKey)
	return nil, r.err
}

type keyedReader struct {
	outcomes map[string]struct {
		bytes []byte
		err   error
	}
	calls []string
}

func (r *keyedReader) Tail(pane PaneScrollback) ([]byte, error) {
	paneKey := pane.PaneKey
	r.calls = append(r.calls, paneKey)
	o := r.outcomes[paneKey]
	return o.bytes, o.err
}

type sequenceReader struct {
	outcomes []struct {
		bytes []byte
		err   error
	}
	calls []string
	idx   int
}

func (r *sequenceReader) Tail(pane PaneScrollback) ([]byte, error) {
	paneKey := pane.PaneKey
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
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
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
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
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
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
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
	m, _ = m.Update(nextPaneKey)
	m, _ = m.Update(nextPaneKey)

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
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}}},
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
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
	}
	pane0Key := state.SanitizePaneKey("work", 0, 0)

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
	if got := stripTrailingBlanks(m.viewport.View()); got != previewReadError {
		t.Fatalf("initial open: viewport = %q; want %q", got, previewReadError)
	}

	m, _ = m.Update(nextPaneKey)
	m, _ = m.Update(nextPaneKey)

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
	tp := reflect.TypeFor[previewModel]()
	for f := range tp.Fields() {
		name := strings.ToLower(f.Name)
		if strings.Contains(name, "error") || strings.Contains(name, "errcache") || strings.Contains(name, "errby") {
			t.Errorf("previewModel has field %q (%s) — per-pane error cache state forbidden by spec", f.Name, f.Type)
		}
		if f.Type.Kind() == reflect.Map && f.Type.Key().Kind() == reflect.String {
			elem := f.Type.Elem()
			if elem.Kind() == reflect.String || elem.Implements(reflect.TypeFor[error]()) {
				t.Errorf("previewModel has map field %q (%s) — error-by-paneKey cache shape forbidden by spec", f.Name, f.Type)
			}
		}
	}
}

func TestPreviewError_ChromeCountsUnaffectedByErrorBranch(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "other", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	enum := &stubEnumerator{groups: groups}
	reader := &nilErrReader{err: syscall.EACCES}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	got := stripANSI(chromeLineForTest(m))
	expected := stripANSI(chromeLineForTest(newPreviewModelForHelpers(t, "work", groups, 0, 0)))
	if got != expected {
		t.Errorf("chromeLine() under error = %q; want %q (identical to non-error shape)", got, expected)
	}
}
