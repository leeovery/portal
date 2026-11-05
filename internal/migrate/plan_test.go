package migrate_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

const orchard = codeDir + "orchard-pipeline"

// The reasons a name changed, as the plan says them.
const (
	slashReplaced = "'/' isn't allowed in a session name, so it became '-'"
	leadDropped   = "a session name can't start with '-', so it was dropped"
	nothingLeft   = "no part of it is allowed in a session name, so it has a generated name"
)

func takenWhy(name string) string { return name + " was taken, so it has a new id" }

func cutWhy(runes int) string { return fmt.Sprintf("too long, so it was cut to %d characters", runes) }

// named is a session's planned name, and how it changed and why.
type named struct {
	name    string
	changed []string
}

// assertNamed fails unless plan names its sessions as want, each changed as
// it says.
func assertNamed(t *testing.T, plan migrate.Plan, want ...named) {
	t.Helper()
	var got []named
	for _, s := range plan.Sessions {
		got = append(got, named{s.Name, s.Changed})
	}
	if !slices.EqualFunc(got, want, func(a, b named) bool { return a.name == b.name && slices.Equal(a.changed, b.changed) }) {
		t.Errorf("named %q\nwant %q", got, want)
	}
}

func TestNewPlan_Names(t *testing.T) {
	t.Run("a session takes its tmux name up to the first ' · ', trimmed, unchanged", func(t *testing.T) {
		for tmuxName, want := range map[string]string{
			"orchard-pipeline-Hq7wNe · api review": "orchard-pipeline-Hq7wNe",
			"lantern":                              "lantern",
			"tidepool · flaky test · round two":    "tidepool",
			"no·spaces·around":                     "no·spaces·around",
			"trail  · two spaces before":           "trail",
			"  padded · leading spaces":            "padded",
			"a--b · a run of '-' it was given":     "a--b",
		} {
			assertNamed(t, planSessions(t, session(tmuxName, pane(0, orchard))), named{name: want})
		}
	})

	t.Run("each character zmx refuses becomes '-', a run that joins collapsing, saying so", func(t *testing.T) {
		for own, want := range map[string]named{
			"flow-v1-api-fv1-45/27": {"flow-v1-api-fv1-45-27", []string{slashReplaced}},
			"a//b":                  {"a-b", []string{slashReplaced}},
			"a-/-b":                 {"a-b", []string{slashReplaced}},
			"a--/b":                 {"a-b", []string{slashReplaced}},
			"a--b/c":                {"a--b-c", []string{slashReplaced}},
			"trail/":                {"trail-", []string{slashReplaced}},
			"tab\there":             {"tab-here", []string{"'\t' isn't allowed in a session name, so it became '-'"}},
			"a/b\tc/d":              {"a-b-c-d", []string{"'/' and '\t' aren't allowed in a session name, so each became '-'"}},
			"a/b\tc\nd":             {"a-b-c-d", []string{"'/', '\t' and '\n' aren't allowed in a session name, so each became '-'"}},
		} {
			assertNamed(t, planSessions(t, session(own+" · topic", pane(0, orchard))), want)
		}
	})

	t.Run("a '-' or space it would start with is dropped, saying so", func(t *testing.T) {
		for own, want := range map[string]named{
			"-flag":    {"flag", []string{leadDropped}},
			"--help":   {"help", []string{leadDropped}},
			"/lead":    {"lead", []string{slashReplaced, leadDropped}},
			"/ spaced": {"spaced", []string{slashReplaced, leadDropped}},
		} {
			assertNamed(t, planSessions(t, session(own+" · topic", pane(0, orchard))), want)
		}
	})

	t.Run("a name too long is cut to fit, on a character boundary, never ending in '-'", func(t *testing.T) {
		limit := zmxBackend(t).MaxNameLen()
		for own, want := range map[string]named{
			strings.Repeat("x", limit+9):              {strings.Repeat("x", limit), []string{cutWhy(limit)}},
			strings.Repeat("é", limit):                {strings.Repeat("é", limit/2), []string{cutWhy(limit / 2)}},
			strings.Repeat("x", limit-1) + "-tail":    {strings.Repeat("x", limit-1), []string{cutWhy(limit - 1)}},
			"a/" + strings.Repeat("x", limit):         {"a-" + strings.Repeat("x", limit-2), []string{slashReplaced, cutWhy(limit)}},
			strings.Repeat("x", limit) + " · a topic": {strings.Repeat("x", limit), nil},
		} {
			assertNamed(t, planSessions(t, session(own+" · topic", pane(0, orchard))), want)
		}
	})

	for own, why := range map[string]string{
		"///":   nothingLeft,
		"-":     nothingLeft,
		"":      nothingLeft,
		"logs":  `invalid session name: "logs" is zmx's log directory, so it has a generated name`,
		"LOGS":  `invalid session name: "LOGS" is zmx's log directory, so it has a generated name`,
		"/logs": `invalid session name: "logs" is zmx's log directory, so it has a generated name`,
		"star*": `invalid session name: "star*" ends with '*', which zmx kill reads as a prefix match, so it has a generated name`,
		"..":    `invalid session name: ".." names a directory entry, so it has a generated name`,
	} {
		t.Run(fmt.Sprintf("a name with nothing left, or zmx still refuses, %q, is generated by the minting rule", own), func(t *testing.T) {
			got := planSessions(t, session(own+" · topic", pane(0, orchard)))

			assertNamed(t, got, named{"orchard-pipeline-Gen001", []string{why}})
		})
	}

	t.Run("a name an earlier session took keeps its stem, with a fresh id", func(t *testing.T) {
		plan := planSessions(t, session("api · user search", pane(0, orchard)), session("api · billing", pane(0, orchard)))

		assertNamed(t, plan, named{name: "api"}, named{"api-Gen001", []string{takenWhy("api")}})
	})

	for _, tc := range []struct {
		own, taken string
		want       named
	}{
		{"folio-bGDWc0", "folio-bGDWc0", named{"folio-Gen001", []string{takenWhy("folio-bGDWc0")}}},
		{"kit", "kit", named{"kit-Gen001", []string{takenWhy("kit")}}},
		{"web-v2", "web-v2", named{"web-v2-Gen001", []string{takenWhy("web-v2")}}},
		{"short-Ab12C", "short-Ab12C", named{"short-Ab12C-Gen001", []string{takenWhy("short-Ab12C")}}},
		{"long-Ab12Cd3", "long-Ab12Cd3", named{"long-Ab12Cd3-Gen001", []string{takenWhy("long-Ab12Cd3")}}},
		{"a/b", "a-b", named{"a-b-Gen001", []string{slashReplaced, takenWhy("a-b")}}},
		{"trail/", "trail-", named{"trail-Gen001", []string{slashReplaced, takenWhy("trail-")}}},
	} {
		t.Run("a name a live session or an entry holds, "+tc.taken+", drops an id it ends in and takes a fresh one", func(t *testing.T) {
			dir := legacyDir(t, []v1Session{session(tc.own+" · topic", pane(0, orchard))}, nil)

			assertNamed(t, planOf(t, dir, nil, naming(t, "other", tc.taken)), tc.want)
		})
	}

	t.Run("a name a name folding to it holds takes a fresh id", func(t *testing.T) {
		dir := legacyDir(t, []v1Session{session("lantern", pane(0, orchard))}, nil)

		assertNamed(t, planOf(t, dir, nil, naming(t, "LANTERN")), named{"lantern-Gen001", []string{takenWhy("lantern")}})
	})

	t.Run("a fresh id taken already, folding case, draws another", func(t *testing.T) {
		dir := legacyDir(t, []v1Session{session("kit", pane(0, orchard))}, nil)

		assertNamed(t, planOf(t, dir, nil, naming(t, "kit", "KIT-gen001")), named{"kit-Gen002", []string{takenWhy("kit")}})
	})

	t.Run("a fresh id on a stem too long cuts the stem to fit", func(t *testing.T) {
		longest := strings.Repeat("x", zmxBackend(t).MaxNameLen())
		dir := legacyDir(t, []v1Session{session(longest, pane(0, orchard))}, nil)

		got := planOf(t, dir, nil, naming(t, longest)).Sessions[0].Name

		if want := strings.Repeat("x", len(longest)-len("-Gen001")) + "-Gen001"; got != want {
			t.Errorf("name = %q, want %q", got, want)
		}
	})

	t.Run("no free fresh id fails the plan", func(t *testing.T) {
		dir := legacyDir(t, []v1Session{session("kit", pane(0, orchard))}, nil)
		n := naming(t, "kit", "kit-Gen001")
		n.Mint = func() (string, error) { return "Gen001", nil }

		if _, err := migrate.NewPlan(readLegacy(t, dir), nil, n); err == nil {
			t.Error("NewPlan succeeded with every fresh id taken")
		}
	})

	t.Run("a generated name taken already draws another nanoid", func(t *testing.T) {
		dir := legacyDir(t, []v1Session{session("star*", pane(0, orchard))}, nil)
		got := planOf(t, dir, nil, naming(t, "orchard-pipeline-gen001")).Sessions[0]

		if got.Name != "orchard-pipeline-Gen002" {
			t.Errorf("name = %q, want orchard-pipeline-Gen002", got.Name)
		}
	})

	t.Run("a generated name is reserved, so a mint drawing the same nanoid again draws another", func(t *testing.T) {
		dir := legacyDir(t, []v1Session{session("star*", pane(0, orchard)), session("logs", pane(0, orchard))}, nil)
		n := naming(t)
		draws := []string{"Same01", "Same01", "Next02"}
		n.Mint = func() (string, error) {
			id := draws[0]
			draws = draws[1:]
			return id, nil
		}

		if got := namesOf(planOf(t, dir, nil, n)); !slices.Equal(got, []string{"orchard-pipeline-Same01", "orchard-pipeline-Next02"}) {
			t.Errorf("names = %v, want two names", got)
		}
	})

	t.Run("a generated name's project is the last part of the pane's dir, as a mint's", func(t *testing.T) {
		long := codeDir + strings.Repeat("p", 120)
		for cwd, want := range map[string]string{
			orchard + "/web/src":       "orchard-pipeline-Gen001",
			"/tmp/scratch":             "scratch-Gen001",
			goneDir + "old-project":    "old-project-Gen001",
			codeDir + "My Project/doc": "My Project-Gen001",
			codeDir + "_hidden":        "hidden-Gen001",
			"":                         "session-Gen001",
			"/":                        "session-Gen001",
			long:                       strings.Repeat("p", zmxBackend(t).MaxNameLen()-len("-Gen001")) + "-Gen001",
		} {
			if got := planSessions(t, session("star*", pane(0, cwd))).Sessions[0].Name; got != want {
				t.Errorf("cwd %q gave %q, want %q", cwd, got, want)
			}
		}
	})

	t.Run("no free name to generate fails the plan", func(t *testing.T) {
		dir := legacyDir(t, []v1Session{session("star*", pane(0, orchard))}, nil)
		n := naming(t, "orchard-pipeline-Gen001")
		n.Mint = func() (string, error) { return "Gen001", nil }

		if _, err := migrate.NewPlan(readLegacy(t, dir), nil, n); err == nil {
			t.Error("NewPlan succeeded with every generated name taken")
		}
	})

	for name, own := range map[string]string{"generated": "star*", "given a fresh id": "kit"} {
		t.Run("a mint that fails fails the plan, for a name "+name, func(t *testing.T) {
			dir := legacyDir(t, []v1Session{session(own, pane(0, orchard))}, nil)
			n := naming(t, "kit")
			entropy := errors.New("no entropy")
			n.Mint = func() (string, error) { return "", entropy }

			if _, err := migrate.NewPlan(readLegacy(t, dir), nil, n); !errors.Is(err, entropy) {
				t.Errorf("NewPlan = %v, want the mint's failure", err)
			}
		})
	}
}

func TestNewPlan_Labels(t *testing.T) {
	for tmuxName, want := range map[string]string{
		"orchard-pipeline-Hq7wNe · api review": "orchard-pipeline-Hq7wNe · api review",
		"lantern":                              "",
		"star* · topic":                        "star* · topic",
		"flow/27 · topic":                      "flow/27 · topic",
		"flow/27":                              "flow/27",
		"trail  · x":                           "trail  · x",
	} {
		got := planSessions(t, session(tmuxName, pane(0, orchard))).Sessions[0]
		if got.Label != want || got.LabelRefused != nil {
			t.Errorf("%q labelled %q (refused: %v), want %q", tmuxName, got.Label, got.LabelRefused, want)
		}
	}

	t.Run("a label the index refuses is omitted, saying why", func(t *testing.T) {
		got := planSessions(t, session("abc\u202edef · topic", pane(0, orchard))).Sessions[0]

		if got.Label != "" || !errors.Is(got.LabelRefused, sessionindex.ErrLabelUnprintable) {
			t.Errorf("label %q (refused: %v), want none, refused as unprintable", got.Label, got.LabelRefused)
		}
	})
}

func TestNewPlan_Panes(t *testing.T) {
	t.Run("a two-pane session becomes <name> and <name>-2, each labelled", func(t *testing.T) {
		plan := planSessions(t, session("orchard-pipeline-Hq7wNe · api review", pane(0, orchard), pane(1, orchard)))

		if got := namesOf(plan); !slices.Equal(got, []string{"orchard-pipeline-Hq7wNe", "orchard-pipeline-Hq7wNe-2"}) {
			t.Errorf("names = %v", got)
		}
		if got := labelsOf(plan); !slices.Equal(got, []string{"orchard-pipeline-Hq7wNe · api review", "orchard-pipeline-Hq7wNe · api review-2"}) {
			t.Errorf("labels = %v", got)
		}
	})

	t.Run("a pane whose label is its name records none", func(t *testing.T) {
		plan := planSessions(t, session("lantern", pane(0, orchard), pane(1, orchard)))

		if got := labelsOf(plan); !slices.Equal(got, []string{"", ""}) {
			t.Errorf("labels = %v, want none", got)
		}
	})

	t.Run("panes are named in window order, then pane order", func(t *testing.T) {
		plan := planSessions(t, sessionOf("lantern",
			window(2, pane(0, "/c")),
			window(0, pane(1, "/b"), pane(0, "/a")),
			window(1, pane(3, "/d"))))

		if got := namesOf(plan); !slices.Equal(got, []string{"lantern", "lantern-2", "lantern-3", "lantern-4"}) {
			t.Errorf("names = %v", got)
		}
		var from []string
		for _, s := range plan.Sessions {
			from = append(from, s.From.String()+" "+s.Cwd)
		}
		if want := []string{"lantern:0.0 /a", "lantern:0.1 /b", "lantern:1.3 /d", "lantern:2.0 /c"}; !slices.Equal(from, want) {
			t.Errorf("panes = %v, want %v", from, want)
		}
	})

	t.Run("a pane whose number is taken takes the next free one, labelled with it, unchanged", func(t *testing.T) {
		dir := legacyDir(t, []v1Session{session("api · user search", pane(0, orchard), pane(1, orchard), pane(2, orchard))}, nil)
		plan := planOf(t, dir, nil, naming(t, "api-2"))

		assertNamed(t, plan, named{name: "api"}, named{name: "api-3"}, named{name: "api-4"})
		if got := labelsOf(plan); !slices.Equal(got, []string{"api · user search", "api · user search-3", "api · user search-4"}) {
			t.Errorf("labels = %v", got)
		}
	})

	t.Run("every session's own name is reserved before any pane's number", func(t *testing.T) {
		plan := planSessions(t, session("work", pane(0, orchard), pane(1, orchard)), session("work-2", pane(0, orchard)))

		if got := namesOf(plan); !slices.Equal(got, []string{"work", "work-3", "work-2"}) {
			t.Errorf("names = %v, want the real work-2 keeping its name", got)
		}
		if got := labelsOf(plan); !slices.Equal(got, []string{"", "", ""}) {
			t.Errorf("labels = %v, want each its name", got)
		}
	})

	t.Run("a pane whose numbered name is too long takes a fresh id on the first's stem, labelled with its number", func(t *testing.T) {
		limit := zmxBackend(t).MaxNameLen()
		for first, stem := range map[string]string{
			strings.Repeat("x", limit):                            strings.Repeat("x", limit-len("-Gen001")),
			strings.Repeat("f", limit-len("-bGDWc0")) + "-bGDWc0": strings.Repeat("f", limit-len("-bGDWc0")),
		} {
			plan := planSessions(t, session(first+" · topic", pane(0, orchard), pane(1, orchard+"/web"), pane(2, orchard)))

			assertNamed(t, plan,
				named{name: first},
				named{stem + "-Gen001", []string{first + "-2 is too long, so it has a new id"}},
				named{stem + "-Gen002", []string{first + "-3 is too long, so it has a new id"}})
			if got := labelsOf(plan)[1:]; !slices.Equal(got, []string{first + " · topic-2", first + " · topic-3"}) {
				t.Errorf("labels = %q, want the tmux name with each pane's number", got)
			}
		}
	})

	t.Run("the other panes are numbered on a changed first name, carrying its change", func(t *testing.T) {
		for first, want := range map[string][]named{
			"a/b": {{"a-b", []string{slashReplaced}}, {"a-b-2", []string{slashReplaced}}},
			"kit": {{"kit-Gen001", []string{takenWhy("kit")}}, {"kit-Gen001-2", []string{takenWhy("kit")}}},
		} {
			dir := legacyDir(t, []v1Session{session(first, pane(0, orchard), pane(1, "/elsewhere"))}, nil)

			assertNamed(t, planOf(t, dir, nil, naming(t, "kit")), want...)
		}
	})

	t.Run("a generated first name gives the other panes theirs", func(t *testing.T) {
		plan := planSessions(t, session("star*", pane(0, orchard), pane(1, "/elsewhere")))

		why := []string{`invalid session name: "star*" ends with '*', which zmx kill reads as a prefix match, so it has a generated name`}
		assertNamed(t, plan, named{"orchard-pipeline-Gen001", why}, named{"orchard-pipeline-Gen001-2", why})
	})
}

func TestNewPlan_DirAndCwd(t *testing.T) {
	t.Run("the dir is the pane's grouping dir, else its cwd; the cwd is the pane's", func(t *testing.T) {
		plan := planSessions(t, session("lantern",
			pane(0, orchard), pane(1, orchard+"/web/src"), pane(2, "/tmp/scratch"), pane(3, "/home/user/notes"), pane(4, goneDir+"old"), pane(5, "")))

		for i, want := range []struct{ dir, cwd string }{
			{orchard, orchard},
			{orchard, orchard + "/web/src"},
			{"/private/tmp/scratch", "/tmp/scratch"},
			{"/home/user/notes", "/home/user/notes"},
			{goneDir + "old", goneDir + "old"},
			{"", ""},
		} {
			if got := plan.Sessions[i]; got.Dir != want.dir || got.Cwd != want.cwd {
				t.Errorf("%s dir %q, cwd %q; want %q, %q", got.Name, got.Dir, got.Cwd, want.dir, want.cwd)
			}
		}
	})

	t.Run("with no grouping dir to find, the dir is the cwd", func(t *testing.T) {
		n := naming(t)
		n.GroupingDir = nil
		got := planOf(t, legacyDir(t, []v1Session{session("lantern", pane(0, orchard+"/web"))}, nil), nil, n).Sessions[0]

		if got.Dir != orchard+"/web" {
			t.Errorf("dir = %q, want the cwd", got.Dir)
		}
	})
}

func TestNewPlan_Skipped(t *testing.T) {
	plan := planSessions(t,
		v1Session{Name: "windowless"},
		sessionOf("paneless", window(0), window(1)),
		session("lantern", pane(0, orchard)))

	want := []migrate.SkippedSession{{Name: "windowless", Why: migrate.ErrNoWindows}, {Name: "paneless", Why: migrate.ErrNoPanes}}
	if !slices.Equal(plan.Skipped, want) {
		t.Errorf("skipped = %v, want %v", plan.Skipped, want)
	}
	if got := namesOf(plan); !slices.Equal(got, []string{"lantern"}) {
		t.Errorf("names = %v, want lantern alone", got)
	}
}

func TestNewPlan_Hooks(t *testing.T) {
	sessions := []v1Session{
		session("lantern", pane(0, orchard).token("tokLan"), pane(1, orchard).token("tokLa2")),
		sessionOf("tidepool · flaky test", window(0, pane(0, orchard)), window(1, pane(0, orchard), pane(2, orchard).token("tokTd2"))),
		session("orchard", pane(0, orchard).token("tokDup")),
		session("quarry", pane(0, orchard).token("tokDup")),
	}
	dir := legacyDir(t, sessions, nil)
	keys := []string{
		"tokLan", "tokTd2", "tokDup", // token keys, tokDup carried by two panes
		"tokGon",                                        // a token of no saved pane
		"lantern:0.1",                                   // pane addresses: of a pane carrying a token,
		"tidepool · flaky test:1.0",                     // of one carrying none,
		"gone:0.0",                                      // and of no saved pane
		"Zq8xW4:1.2",                                    // a session token's
		"%12",                                           // a tmux pane id
		"seed-other", "", "a:b.c", "a:1.", ":0.0", "%x", // anything else
	}
	plan := planOf(t, dir, keys, naming(t))

	wantMoves := []migrate.HookMove{
		{From: "tokDup", To: "orchard"},
		{From: "tokLan", To: "lantern"},
		{From: "tokTd2", To: "tidepool-3"},
	}
	if !slices.Equal(plan.Moves, wantMoves) {
		t.Errorf("moves = %v\nwant %v", plan.Moves, wantMoves)
	}
	wantRemovals := []migrate.HookRemoval{
		{Key: "", Kind: migrate.OtherKey},
		{Key: "%12", Kind: migrate.PaneIDKey},
		{Key: "%x", Kind: migrate.OtherKey},
		{Key: ":0.0", Kind: migrate.OtherKey},
		{Key: "Zq8xW4:1.2", Kind: migrate.PaneAddressKey},
		{Key: "a:1.", Kind: migrate.OtherKey},
		{Key: "a:b.c", Kind: migrate.OtherKey},
		{Key: "gone:0.0", Kind: migrate.PaneAddressKey},
		{Key: "lantern:0.1", Kind: migrate.PaneAddressKey},
		{Key: "seed-other", Kind: migrate.OtherKey},
		{Key: "tidepool · flaky test:1.0", Kind: migrate.PaneAddressKey},
		{Key: "tokGon", Kind: migrate.UnsavedTokenKey},
	}
	if !slices.Equal(plan.Removals, wantRemovals) {
		t.Errorf("removals = %v\nwant %v", plan.Removals, wantRemovals)
	}
}

func TestNewPlan_Transcripts(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "escaped.bin"), "not the state dir's")
	sessions := []v1Session{session("lantern",
		pane(0, orchard).scrollback("scrollback/lantern__0.0.bin"),
		pane(1, orchard),
		pane(2, orchard).scrollback("scrollback/gone.bin"),
		pane(3, orchard).scrollback("../outside.bin"),
		pane(4, orchard).scrollback("/etc/absolute.bin"),
		pane(5, orchard).scrollback("scrollback/a-dir"),
		pane(6, orchard).scrollback("scrollback/link.bin"),
		pane(7, orchard).scrollback("linked/escaped.bin"),
		pane(8, orchard).scrollback("scrollback/lantern__0.0.bin/below-a-file.bin"),
		pane(9, orchard).scrollback("scrollback/nul\x00.bin"),
	)}
	dir := legacyDir(t, sessions, map[string]string{"scrollback/lantern__0.0.bin": "saved\n", "scrollback/a-dir/inside": "x"})
	if err := os.Symlink(filepath.Join(dir, "scrollback", "lantern__0.0.bin"), filepath.Join(dir, "scrollback", "link.bin")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	plan := planOf(t, dir, nil, naming(t))

	t.Run("a file version 1 named is copied to the one the zmx version names for the session", func(t *testing.T) {
		want, err := state.SessionScrollbackFile("lantern", savedAt, 0)
		if err != nil {
			t.Fatalf("SessionScrollbackFile: %v", err)
		}
		got := plan.Sessions[0].Transcript
		if got.Named != "scrollback/lantern__0.0.bin" || got.File != want || got.Missing != nil {
			t.Errorf("transcript = %+v, want %s copied to %s", got, "scrollback/lantern__0.0.bin", want)
		}
	})

	t.Run("a pane naming none copies none", func(t *testing.T) {
		if got := plan.Sessions[1].Transcript; got != (migrate.Transcript{}) {
			t.Errorf("transcript = %+v, want none", got)
		}
	})

	for i, tc := range map[int]struct {
		what string
		want error
	}{
		2: {"an absent file", fs.ErrNotExist},
		3: {"a file outside the state dir", migrate.ErrOutsideState},
		4: {"an absolute path", migrate.ErrOutsideState},
		5: {"a directory", migrate.ErrNotRegular},
		6: {"a symlink", migrate.ErrNotRegular},
	} {
		t.Run(tc.what+" copies none, saying why", func(t *testing.T) {
			got := plan.Sessions[i].Transcript
			if got.File != "" || !errors.Is(got.Missing, tc.want) {
				t.Errorf("transcript = %+v, want none copied, missing for %v", got, tc.want)
			}
		})
	}

	for i, what := range map[int]string{
		7: "a file below a directory link leading out of the state dir",
		8: "a path through a file",
		9: "a name holding NUL",
	} {
		t.Run(what+" copies none, carrying the lookup's error", func(t *testing.T) {
			got := plan.Sessions[i].Transcript
			if got.File != "" || got.Missing == nil {
				t.Errorf("transcript = %+v, want none copied, missing with the lookup's error", got)
			}
		})
	}
}
