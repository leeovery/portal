package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tmux"
)

func mintPaneToken(t *testing.T) string {
	t.Helper()
	token, err := nanoid.NewPaneTokenGenerator()()
	if err != nil {
		t.Fatalf("mint pane token: %v", err)
	}
	return token
}

func writeTokenBinFile(t *testing.T, stateDir, token string, content []byte) {
	t.Helper()
	path := filepath.Join(stateDir, filepath.FromSlash(state.PendingScrollbackFile(token)))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir scrollback: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func openProductionPreview(t *testing.T, stateDir string, pane tmux.WindowPane) string {
	t.Helper()
	enum := &stubEnumerator{groups: []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{pane}},
	}}
	m, ok := NewPreviewModel("work", enum, NewProductionScrollbackReader(stateDir), nil, 80, 24)
	if !ok {
		t.Fatalf("expected the preview to open")
	}
	return stripTrailingBlanks(m.viewport.View())
}

func TestPreviewWaitingPane_UnmovedRefiledPaneShowsItsTokenFile(t *testing.T) {
	stateDir := t.TempDir()
	token := mintPaneToken(t)
	writeTokenBinFile(t, stateDir, token, []byte("saved transcript\n"))

	got := openProductionPreview(t, stateDir, tmux.WindowPane{Index: 0, Token: token, Pending: true})

	if got != "saved transcript" {
		t.Errorf("viewport content = %q; want the token-named file's bytes %q", got, "saved transcript")
	}
}

func TestPreviewWaitingPane_MovedPaneShowsItsTokenFileNotTheOccupantOfItsAddress(t *testing.T) {
	stateDir := t.TempDir()
	token := mintPaneToken(t)
	writeTokenBinFile(t, stateDir, token, []byte("saved transcript\n"))
	writeBinFile(t, stateDir, state.SanitizePaneKey("work", 0, 0), []byte("another pane's capture\n"))

	got := openProductionPreview(t, stateDir, tmux.WindowPane{Index: 0, Token: token, Pending: true})

	if got != "saved transcript" {
		t.Errorf("viewport content = %q; want the token-named file's bytes %q", got, "saved transcript")
	}
}

func TestPreviewWaitingPane_NotYetRefiledPaneShowsItsPositionalFile(t *testing.T) {
	stateDir := t.TempDir()
	token := mintPaneToken(t)
	writeBinFile(t, stateDir, state.SanitizePaneKey("work", 0, 0), []byte("positional transcript\n"))

	got := openProductionPreview(t, stateDir, tmux.WindowPane{Index: 0, Token: token, Pending: true})

	if got != "positional transcript" {
		t.Errorf("viewport content = %q; want the positional file's bytes %q", got, "positional transcript")
	}
}

// Each case stages a token-named file the pane must not be read from, so
// reading the positional file shows it was not.
func TestPreviewWaitingPane_PanesOutsideTheTokenRuleShowTheirPositionalFile(t *testing.T) {
	token := mintPaneToken(t)
	short := token[:len(token)-1]
	escape := "/../../" + token
	tests := []struct {
		name  string
		pane  tmux.WindowPane
		stage func(t *testing.T, stateDir string)
	}{
		{
			name: "a waiting pane carrying no token",
			pane: tmux.WindowPane{Index: 0, Pending: true},
			stage: func(t *testing.T, stateDir string) {
				writeTokenBinFile(t, stateDir, token, []byte("token transcript\n"))
			},
		},
		{
			name: "a waiting pane whose token is one character short",
			pane: tmux.WindowPane{Index: 0, Token: short, Pending: true},
			stage: func(t *testing.T, stateDir string) {
				writeTokenBinFile(t, stateDir, short, []byte("token transcript\n"))
			},
		},
		{
			name: "a waiting pane whose token climbs out of the scrollback directory",
			pane: tmux.WindowPane{Index: 0, Token: escape, Pending: true},
			stage: func(t *testing.T, stateDir string) {
				writeTokenBinFile(t, stateDir, escape, []byte("token transcript\n"))
				if want := filepath.Join(stateDir, token+".bin"); !fileExists(t, want) {
					t.Fatalf("setup: escaping token did not land at %s", want)
				}
			},
		},
		{
			name: "a pane that is not waiting",
			pane: tmux.WindowPane{Index: 0, Token: token},
			stage: func(t *testing.T, stateDir string) {
				writeTokenBinFile(t, stateDir, token, []byte("token transcript\n"))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			tc.stage(t, stateDir)
			writeBinFile(t, stateDir, state.SanitizePaneKey("work", 0, 0), []byte("positional transcript\n"))

			got := openProductionPreview(t, stateDir, tc.pane)

			if got != "positional transcript" {
				t.Errorf("viewport content = %q; want the positional file's bytes %q", got, "positional transcript")
			}
		})
	}
}

func TestPreviewWaitingPane_UnreadableTokenFileReportsTheErrorRatherThanTheAddressOccupant(t *testing.T) {
	stateDir := t.TempDir()
	token := mintPaneToken(t)
	writeTokenBinFile(t, stateDir, token, []byte("saved transcript\n"))
	if err := themetest.DenyRead(t, filepath.Join(stateDir, filepath.FromSlash(state.PendingScrollbackFile(token)))); err == nil {
		t.Fatalf("setup: token file is still readable")
	}
	writeBinFile(t, stateDir, state.SanitizePaneKey("work", 0, 0), []byte("another pane's capture\n"))

	got := openProductionPreview(t, stateDir, tmux.WindowPane{Index: 0, Token: token, Pending: true})

	if got != previewReadError {
		t.Errorf("viewport content = %q; want %q", got, previewReadError)
	}
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func TestPreviewWaitingPane_ReaderIsHandedTheFocusedPanesTokenOnlyWhileItWaits(t *testing.T) {
	enum := &stubEnumerator{groups: []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{
			{Index: 0, Token: "waitng", Pending: true},
			{Index: 1, Token: "notwai"},
		}},
	}}
	reader := &recordingReader{bytes: []byte("content")}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected the preview to open")
	}
	_, _ = m.Update(nextPaneKey)

	want := []PaneScrollback{
		{PaneKey: state.SanitizePaneKey("work", 0, 0), PendingToken: "waitng"},
		{PaneKey: state.SanitizePaneKey("work", 0, 1)},
	}
	if len(reader.panes) != len(want) {
		t.Fatalf("reader calls = %+v; want %+v", reader.panes, want)
	}
	for i := range want {
		if reader.panes[i] != want[i] {
			t.Errorf("reader call %d = %+v; want %+v", i, reader.panes[i], want[i])
		}
	}
}
