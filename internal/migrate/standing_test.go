package migrate_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/sessionindex"
)

// standingConfig is a state dir holding index, a sessions.json of the zmx
// version's, beside hooks.json holding hooks, and zmx holding live.
func standingConfig(t *testing.T, index, hooks string, live ...string) migrate.Config {
	t.Helper()
	root := t.TempDir()
	cfg := migrate.Config{
		StateDir:  filepath.Join(root, "state"),
		HooksFile: filepath.Join(root, "hooks.json"),
		Live:      func(context.Context) ([]string, error) { return live, nil },
	}
	if index != "" {
		writeFile(t, sessionindex.Path(cfg.StateDir), `{"version": 2, "saved_at": "2026-11-05T09:00:00Z", "sessions": [`+index+`]}`)
	}
	if hooks != "" {
		writeFile(t, cfg.HooksFile, hooks)
	}
	return cfg
}

const (
	lazyLantern  = `{"name": "lantern", "owed": true, "resume_lazy": true}`
	lanternHook  = `{"lantern": {"on-resume": "claude --resume lantern"}}`
	owedQuarry   = `{"name": "quarry", "owed": true}`
	restoredBell = `{"name": "bell"}`
)

func TestWaits(t *testing.T) {
	for name, tc := range map[string]struct {
		index, hooks string
		want         bool
	}{
		"an entry migrate made, with a resume hook":    {lazyLantern, lanternHook, true},
		"an entry migrate made, with no hook":          {lazyLantern, `{"quarry": {"on-resume": "make"}}`, false},
		"an entry migrate made, with no hooks.json":    {lazyLantern, "", false},
		"an entry migrate made, its hook no command":   {lazyLantern, `{"lantern": {"on-resume": ""}}`, false},
		"an entry restored, its hook's mode its own":   {`{"name": "lantern"}`, lanternHook, false},
		"an entry migrate made, hooks.json unparsable": {lazyLantern, `{"lantern": `, false},
		"no index": {"", lanternHook, false},
	} {
		t.Run("it reads "+name, func(t *testing.T) {
			if got := migrate.Waits(standingConfig(t, tc.index, tc.hooks)); got != tc.want {
				t.Errorf("Waits = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLeft(t *testing.T) {
	t.Run("it names each owed entry zmx doesn't hold, in the index's order", func(t *testing.T) {
		cfg := standingConfig(t, owedQuarry+", "+restoredBell+", "+lazyLantern+`, {"name": "tidepool", "owed": true}`, "", "tidepool", "_portal-saver")

		left, err := migrate.Left(context.Background(), cfg)

		if want := []string{"quarry", "lantern"}; err != nil || !slices.Equal(left, want) {
			t.Errorf("Left = %v, %v; want %v: tidepool is live, its helper finishing, and bell owed nothing", left, err, want)
		}
	})

	t.Run("it fails on an index it can't read", func(t *testing.T) {
		cfg := standingConfig(t, "", "")
		writeFile(t, sessionindex.Path(cfg.StateDir), `{"version": 2, "sessions": [`)

		if left, err := migrate.Left(context.Background(), cfg); err == nil {
			t.Errorf("Left = %v, nil; want an error", left)
		}
	})

	t.Run("it fails on zmx's sessions unlisted", func(t *testing.T) {
		cfg := standingConfig(t, owedQuarry, "")
		refused := errors.New("zmx: refused")
		cfg.Live = func(context.Context) ([]string, error) { return nil, refused }

		if left, err := migrate.Left(context.Background(), cfg); !errors.Is(err, refused) {
			t.Errorf("Left = %v, %v; want the listing's error", left, err)
		}
	})
}
