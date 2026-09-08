package tmux_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func TestListAllPaneHookKeys_Rows(t *testing.T) {
	t.Run("it returns one row per live pane", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("tokA|alpha:0.0\n|beta:1.2\ntokC|gamma:0.0", "list-panes"))
		client := tmux.NewClient(mock)

		rows, err := client.ListAllPaneHookKeys()
		if err != nil {
			t.Fatalf("ListAllPaneHookKeys: %v", err)
		}

		want := []tmux.PaneHookRow{
			{Token: "tokA", Location: "alpha:0.0"},
			{Token: "", Location: "beta:1.2"},
			{Token: "tokC", Location: "gamma:0.0"},
		}
		assertRowsEqual(t, rows, want)
	})

	t.Run("it keeps an unstamped pane's row rather than dropping it", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Returns("|sess:0.0", "list-panes")))

		rows, err := client.ListAllPaneHookKeys()
		if err != nil {
			t.Fatalf("ListAllPaneHookKeys: %v", err)
		}
		assertRowsEqual(t, rows, []tmux.PaneHookRow{{Token: "", Location: "sess:0.0"}})
	})

	t.Run("it splits on the first separator only", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Returns("tokA|pipe|name:0.0\n|dot.sess:1.3", "list-panes")))

		rows, err := client.ListAllPaneHookKeys()
		if err != nil {
			t.Fatalf("ListAllPaneHookKeys: %v", err)
		}
		assertRowsEqual(t, rows, []tmux.PaneHookRow{
			{Token: "tokA", Location: "pipe|name:0.0"},
			{Token: "", Location: "dot.sess:1.3"},
		})
	})

	t.Run("it errors on a row with no separator", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Returns("tokA|alpha:0.0\nmalformed-row", "list-panes")))

		rows, err := client.ListAllPaneHookKeys()
		if err == nil {
			t.Fatalf("ListAllPaneHookKeys on a separator-less row: want an error, got rows %+v", rows)
		}
		if rows != nil {
			t.Errorf("rows on a parse failure = %+v, want nil (a half-parsed set must not reach a caller)", rows)
		}
		if !strings.Contains(err.Error(), "malformed-row") {
			t.Errorf("error %v does not name the offending line %q", err, "malformed-row")
		}
	})

	t.Run("it returns a non-nil empty slice for empty output", func(t *testing.T) {
		client := tmux.NewClient(commandertest.New(t, commandertest.Returns("", "list-panes")))

		rows, err := client.ListAllPaneHookKeys()
		if err != nil {
			t.Fatalf("ListAllPaneHookKeys on empty output: unexpected error %v", err)
		}
		if rows == nil {
			t.Fatal("ListAllPaneHookKeys returned a nil slice on empty output, want non-nil empty")
		}
		if len(rows) != 0 {
			t.Errorf("rows on empty output = %+v, want empty", rows)
		}
	})

	t.Run("it reads the pane token through the single option constant", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "list-panes"))
		client := tmux.NewClient(mock)

		if _, err := client.ListAllPaneHookKeys(); err != nil {
			t.Fatalf("ListAllPaneHookKeys: %v", err)
		}
		if len(mock.Calls()) != 1 {
			t.Fatalf("tmux calls = %d (%v), want exactly 1 (one read serves every consumer)", len(mock.Calls()), mock.Calls())
		}
		got := strings.Join(mock.Calls()[0], " ")
		if !strings.HasPrefix(got, "list-panes -a -F ") {
			t.Errorf("enumeration argv = %q, want a list-panes -a -F read", got)
		}
		if !strings.Contains(got, "#{"+state.PortalPaneIDOption+"}") {
			t.Errorf("enumeration format %q does not read %q", got, state.PortalPaneIDOption)
		}
	})
}

func assertRowsEqual(t *testing.T, got, want []tmux.PaneHookRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows = %+v (len %d), want %+v (len %d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rows[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
