package migrate_test

import (
	"bytes"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// How a fixture pane's scrollback was saved.
const (
	byToken    = "token"      // scrollback/pane-<token>.bin, a waiting pane's re-filed transcript
	positional = "positional" // scrollback/<session>__<window>.<pane>.bin
	missing    = "missing"    // named, but gone from the state dir
)

// How a fixture pane's hook was keyed.
const (
	tokenKey   = "token"
	addressKey = "address"
	bothKeys   = "both"
)

type fixturePane struct {
	token, scrollback, hook string
}

// fixtureSession is a tmux session of the realistic fixture: its name, its
// project, and its panes, all in window 0. A project is a repository under
// codeDir, or an absolute directory outside any. A session's first pane works
// in its project's directory, and any other in the project's web directory.
type fixtureSession struct {
	tmux, project string
	panes         []fixturePane
}

// realisticSessions is thirty tmux sessions as the tmux version saved them.
// Every name is invented. One session has two panes; one name is too long for
// zmx; one is a live zmx session's already; one works outside any repository.
var realisticSessions = []fixtureSession{
	{"orchard-pipeline-Hq7wNe · api review", "orchard-pipeline", []fixturePane{{"k3f9qz", byToken, tokenKey}, {"p8Lm2x", byToken, tokenKey}}},
	{"lantern-9fKq2L", "lantern", []fixturePane{{"Tq4n8B", byToken, tokenKey}}},
	{"tidepool-Xa83Lp · flaky test hunt", "tidepool", []fixturePane{{"Rm7c2D", byToken, bothKeys}}},
	{"quarry-Pm4sT1", "quarry", []fixturePane{{"", positional, addressKey}}},
	{"juniper-api-Wd5gH2 · auth refactor", "juniper-api", []fixturePane{{"Hy6v3K", byToken, tokenKey}}},
	{"saltmarsh-Lk2rQ8", "saltmarsh", []fixturePane{{"Nb5x7J", missing, tokenKey}}},
	{"copperleaf-Ze4tY6 · release notes", "copperleaf", []fixturePane{{"Cv8w1M", byToken, tokenKey}}},
	{"driftwood-Fs9pA3", "/tmp/driftwood", []fixturePane{{"", positional, ""}}},
	{"ember-ui-Gu1kB7 · dark mode", "ember-ui", []fixturePane{{"Qe3r6T", byToken, tokenKey}}},
	{"fernwood-Jh5mC9", "fernwood", []fixturePane{{"Ws2y4U", byToken, tokenKey}}},
	{"glacier-ops-Kc7nD1 · terraform plan", "glacier-ops", []fixturePane{{"Ex9i5O", byToken, tokenKey}}},
	{"harbor-Mv3bE5", "harbor", []fixturePane{{"", positional, addressKey}}},
	{"ironbark-Nx6vF8 · migration dry run", "ironbark", []fixturePane{{"Ra1u7P", byToken, tokenKey}}},
	{"kestrel-Pq8cG2", "kestrel", []fixturePane{{"Ty4o9A", byToken, ""}}},
	{"larkspur-Rw1xH4 · onboarding copy", "larkspur", []fixturePane{{"Ui6p2S", byToken, tokenKey}}},
	{"meadowlark-Sy4zJ6", "meadowlark", []fixturePane{{"Io8a3D", missing, tokenKey}}},
	{"nettle-Tz7aK9 · perf profiling", "nettle", []fixturePane{{"Pd5s6F", byToken, tokenKey}}},
	{"oakmoss-Ub2sL3", "oakmoss", []fixturePane{{"As7d1G", byToken, tokenKey}}},
	{"pebble-cli-Vc5dM7 · flag parsing", "pebble-cli", []fixturePane{{"Sf9g4H", byToken, tokenKey}}},
	{"quill-Wd8fN1", "quill", []fixturePane{{"", "", ""}}},
	{"rookery-Xe3gP5 · cache invalidation", "rookery", []fixturePane{{"Dg2h8J", byToken, tokenKey}}},
	{"sorrel-Yf6hQ8", "sorrel", []fixturePane{{"Fh4j5K", byToken, tokenKey}}},
	{"thistle-Zg9jR2 · search ranking", "thistle", []fixturePane{{"Gj6k7L", byToken, tokenKey}}},
	{"umber-Ah1kS4", "umber", []fixturePane{{"Hk8l9Z", byToken, tokenKey}}},
	{"vellum-Bj4lT7 · pdf export", "vellum", []fixturePane{{"Jl1z2X", byToken, tokenKey}}},
	{"willow-docs-Ck7mU9", "willow-docs", []fixturePane{{"Kz3x4C", byToken, tokenKey}}},
	{"yarrow-Dl2nV3 · billing webhooks", "yarrow", []fixturePane{{"Lx5c6V", byToken, tokenKey}}},
	{"zephyr-Em5pW6", "zephyr", []fixturePane{{"Zc7v8B", byToken, tokenKey}}},
	{"basalt-Fn8qX1 · a topic long enough that the label alone would never fit in a zmx socket name", "basalt", []fixturePane{{"Mb9n1N", byToken, tokenKey}}},
	{"cinder-a-session-name-typed-out-by-hand-long-enough-to-overflow-zmx-sockets-Go4rY5", "cinder", []fixturePane{{"Nv2b3M", byToken, tokenKey}}},
}

// realisticTaken are the names already in use: a live zmx session holding a
// fixture session's name, and an unrelated entry.
var realisticTaken = []string{"quarry-Pm4sT1", "dotfiles-Aa1Bb2"}

// realisticUnmapped are hooks.json keys carried by no saved pane, by what each
// was: a token of a pane gone before the last save, a gone session's pane
// address, a session token's pane address, and a tmux pane id.
var realisticUnmapped = map[string]migrate.HookKeyKind{
	"Yy9Ww8":             migrate.UnsavedTokenKey,
	"retired-Qa1sW2:0.0": migrate.PaneAddressKey,
	"Qa1sW2:0.0":         migrate.PaneAddressKey,
	"%12":                migrate.PaneIDKey,
}

// fixtureDir is the directory of project.
func fixtureDir(project string) string {
	if filepath.IsAbs(project) {
		return project
	}
	return codeDir + project
}

// fixtureCwd is the working directory of a pane at index of a session of
// project.
func fixtureCwd(project string, index int) string {
	if index == 0 {
		return fixtureDir(project)
	}
	return fixtureDir(project) + "/web"
}

func (p fixturePane) scrollbackFile(session string, index int) string {
	switch p.scrollback {
	case byToken:
		return "scrollback/pane-" + p.token + ".bin"
	case positional, missing:
		return fmt.Sprintf("scrollback/%s__0.%d.bin", session, index)
	}
	return ""
}

func transcriptOf(session string, index int) string {
	return fmt.Sprintf("\x1b[32m%s\x1b[0m pane %d $ ls\r\nREADME.md  go.mod\r\n", session, index)
}

// commandOf is the hook command of the pane at address, in the string form
// for even panes of the fixture and the object form for odd, one carrying an
// attribute the codec doesn't model.
func commandOf(n int, address string) string {
	command := fmt.Sprintf("claude --resume %08x", n*2654435761)
	switch n % 3 {
	case 1:
		return fmt.Sprintf(`{"command": %q, "resume": "lazy"}`, command)
	case 2:
		return fmt.Sprintf(`{"command": %q, "resume": "eager", "note": %q}`, command, address)
	}
	return fmt.Sprintf("%q", command)
}

// realisticFixture is the tmux version's state dir holding the fixture, and a
// hooks.json body keying its panes' hooks.
func realisticFixture(t *testing.T) (dir, hooksBody string) {
	t.Helper()
	var sessions []v1Session
	files := map[string]string{}
	var entries []string
	n := 0
	for _, s := range realisticSessions {
		var panes []v1Pane
		for i, p := range s.panes {
			file := p.scrollbackFile(s.tmux, i)
			panes = append(panes, pane(i, fixtureCwd(s.project, i)).token(p.token).scrollback(file))
			if p.scrollback == byToken || p.scrollback == positional {
				files[file] = transcriptOf(s.tmux, i)
			}
			address := fmt.Sprintf("%s:0.%d", s.tmux, i)
			if p.hook == tokenKey || p.hook == bothKeys {
				entries = append(entries, fmt.Sprintf(`%q: {"on-resume": %s}`, p.token, commandOf(n, address)))
				n++
			}
			if p.hook == addressKey || p.hook == bothKeys {
				entries = append(entries, fmt.Sprintf(`%q: {"on-resume": %s}`, address, commandOf(n, address)))
				n++
			}
		}
		sessions = append(sessions, session(s.tmux, panes...))
	}
	for _, key := range slices.Sorted(maps.Keys(realisticUnmapped)) {
		entries = append(entries, fmt.Sprintf(`%q: {"on-resume": %s}`, key, commandOf(n, key)))
		n++
	}
	return legacyDir(t, sessions, files), "{\n  " + strings.Join(entries, ",\n  ") + "\n}"
}

func TestRealisticFixture(t *testing.T) {
	legacy, hooksBody := realisticFixture(t)
	backupStore, backup := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody, SidecarAbsent: true})
	store, hooksPath := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody})
	original := rawHooks(t, backup)
	plan := planOf(t, legacy, hookKeys(t, backupStore), naming(t, realisticTaken...))
	byAddress := map[string]migrate.Session{}
	for _, s := range plan.Sessions {
		byAddress[s.From.String()] = s
	}

	t.Run("every pane becomes a session, under a name zmx takes and no other holds", func(t *testing.T) {
		if len(plan.Sessions) != 31 {
			t.Fatalf("%d sessions, want one for each of the 31 panes", len(plan.Sessions))
		}
		validate := zmxBackend(t).ValidateName
		seen := slices.Clone(realisticTaken)
		for _, s := range plan.Sessions {
			if err := validate(s.Name); err != nil {
				t.Errorf("%s: %v", s.Name, err)
			}
			if slices.ContainsFunc(seen, func(other string) bool { return strings.EqualFold(other, s.Name) }) {
				t.Errorf("%s is held twice", s.Name)
			}
			seen = append(seen, s.Name)
		}
	})

	t.Run("each takes its tmux name up to ' · ', labelled with the whole of it", func(t *testing.T) {
		for _, fixture := range realisticSessions {
			got := byAddress[fixture.tmux+":0.0"]
			if got.Changed != nil {
				continue
			}
			given, _, topic := strings.Cut(fixture.tmux, " · ")
			if got.Name != given {
				t.Errorf("%q named %q, want %q", fixture.tmux, got.Name, given)
			}
			wantLabel := ""
			if topic {
				wantLabel = fixture.tmux
			}
			if got.Label != wantLabel || got.LabelRefused != nil {
				t.Errorf("%q labelled %q (refused: %v), want %q", fixture.tmux, got.Label, got.LabelRefused, wantLabel)
			}
		}
	})

	t.Run("the name a live session holds takes a fresh id and the name too long is cut, each labelled with the tmux name", func(t *testing.T) {
		var changed []string
		for _, s := range plan.Sessions {
			if s.Changed != nil {
				changed = append(changed, s.Label+" → "+s.Name+": "+strings.Join(s.Changed, "; "))
			}
		}
		want := []string{
			"quarry-Pm4sT1 → quarry-Gen001: quarry-Pm4sT1 was taken, so it has a new id",
			realisticSessions[29].tmux + " → cinder-a-session-name-typed-out-by-hand-long-enough-to-overflow-zmx-sockets: too long, so it was cut to 75 characters",
		}
		if !slices.Equal(changed, want) {
			t.Errorf("changed %q\nwant %q", changed, want)
		}
	})

	t.Run("the two-pane session becomes <name> and <name>-2", func(t *testing.T) {
		first, second := byAddress[realisticSessions[0].tmux+":0.0"], byAddress[realisticSessions[0].tmux+":0.1"]
		if first.Name != "orchard-pipeline-Hq7wNe" || second.Name != "orchard-pipeline-Hq7wNe-2" {
			t.Errorf("names %q and %q", first.Name, second.Name)
		}
		if second.Label != realisticSessions[0].tmux+"-2" {
			t.Errorf("second label %q", second.Label)
		}
	})

	t.Run("each session's dir is its pane's repository, else its working directory", func(t *testing.T) {
		for _, fixture := range realisticSessions {
			for i := range fixture.panes {
				got := byAddress[fmt.Sprintf("%s:0.%d", fixture.tmux, i)]
				if want := canonical(fixtureDir(fixture.project)); got.Dir != want || got.Cwd != fixtureCwd(fixture.project, i) {
					t.Errorf("%s: dir %q, cwd %q; want %q, %q", got.Name, got.Dir, got.Cwd, want, fixtureCwd(fixture.project, i))
				}
			}
		}
	})

	t.Run("each token key moves to its pane's session, and every other key is removed, saying what it was", func(t *testing.T) {
		wantMoves := map[string]string{}
		var wantRemovals []migrate.HookRemoval
		for _, fixture := range realisticSessions {
			for i, p := range fixture.panes {
				address := fmt.Sprintf("%s:0.%d", fixture.tmux, i)
				if p.hook == tokenKey || p.hook == bothKeys {
					wantMoves[p.token] = byAddress[address].Name
				}
				if p.hook == addressKey || p.hook == bothKeys {
					wantRemovals = append(wantRemovals, migrate.HookRemoval{Key: address, Kind: migrate.PaneAddressKey})
				}
			}
		}
		for key, kind := range realisticUnmapped {
			wantRemovals = append(wantRemovals, migrate.HookRemoval{Key: key, Kind: kind})
		}
		slices.SortFunc(wantRemovals, func(a, b migrate.HookRemoval) int { return strings.Compare(a.Key, b.Key) })

		gotMoves := map[string]string{}
		for _, m := range plan.Moves {
			gotMoves[m.From] = m.To
		}
		if !maps.Equal(gotMoves, wantMoves) {
			t.Errorf("moves = %v\nwant %v", gotMoves, wantMoves)
		}
		if !slices.Equal(plan.Removals, wantRemovals) {
			t.Errorf("removals = %v\nwant %v", plan.Removals, wantRemovals)
		}
	})

	newDir := filepath.Join(t.TempDir(), "state")
	if err := migrate.Build(plan, newDir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := migrate.RewriteHooks(plan, backup, store); err != nil {
		t.Fatalf("RewriteHooks: %v", err)
	}

	t.Run("sessions.json holds every session owed a restore, recording no creation, resuming lazily", func(t *testing.T) {
		idx := readIndex(t, newDir)
		if idx.Len() != len(plan.Sessions) {
			t.Fatalf("%d entries, want %d", idx.Len(), len(plan.Sessions))
		}
		for _, s := range plan.Sessions {
			e, ok := idx.Lookup(s.Name)
			want := sessionindex.Entry{Name: s.Name, Label: s.Label, Dir: s.Dir, Cwd: s.Cwd, ScrollbackFile: s.Transcript.File, Owed: true, ResumeLazy: true}
			if !ok || e != want {
				t.Errorf("entry %+v, want %+v", e, want)
			}
		}
	})

	t.Run("each scrollback file found is copied byte for byte, under a name clean and Forget own", func(t *testing.T) {
		copied := 0
		for _, s := range plan.Sessions {
			if s.Transcript.File == "" {
				continue
			}
			copied++
			path, err := state.ScrollbackPath(newDir, sessionindex.Entry{Name: s.Name, ScrollbackFile: s.Transcript.File})
			if err != nil || !state.IsSessionScrollbackFile(filepath.Base(path)) {
				t.Errorf("%s's copy %q isn't one the zmx version owns: %v", s.Name, s.Transcript.File, err)
				continue
			}
			if got, want := readBytes(t, path), []byte(transcriptOf(s.From.Session, s.From.Pane)); !bytes.Equal(got, want) {
				t.Errorf("%s's copy = %q, want %q", s.Name, got, want)
			}
		}
		if copied != 28 {
			t.Errorf("%d copied, want the 31 panes less the one that named none and the two whose files are gone", copied)
		}
	})

	t.Run("hooks.json holds each moved key's value byte for byte under its new name, and nothing else", func(t *testing.T) {
		want := map[string]string{}
		for _, m := range plan.Moves {
			want[m.To] = compact(t, original[m.From])
		}
		got := map[string]string{}
		for key, value := range rawHooks(t, hooksPath) {
			got[key] = compact(t, value)
		}
		if !maps.Equal(got, want) {
			t.Errorf("hooks.json = %v\nwant %v", got, want)
		}
	})

	t.Run("migrating it again changes nothing", func(t *testing.T) {
		once, hooksFile := dirFiles(t, newDir), readBytes(t, hooksPath)
		if err := migrate.Build(plan, newDir); err != nil {
			t.Fatalf("Build again: %v", err)
		}
		if err := migrate.RewriteHooks(plan, backup, store); err != nil {
			t.Fatalf("RewriteHooks again: %v", err)
		}
		if again := dirFiles(t, newDir); !maps.EqualFunc(again, once, bytes.Equal) {
			t.Error("the state dir changed on a second build")
		}
		hookstest.AssertHooksFileUnchanged(t, hooksPath, hooksFile, "changed by a second rewrite")
	})
}
