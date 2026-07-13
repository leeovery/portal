package spawn

import "testing"

func TestMatchConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     TerminalsConfig
		id      Identity
		wantKey string
		wantOK  bool
	}{
		{
			name: "it picks the exact raw bundle id over a .app name, alias, and glob",
			cfg: TerminalsConfig{
				"com.mitchellh.ghostty": {},
				"Ghostty":               {},
				"ghostty":               {},
				"com.mitchellh.*":       {},
				"*":                     {},
			},
			id:      NewIdentity("com.mitchellh.ghostty", "Ghostty"),
			wantKey: "com.mitchellh.ghostty",
			wantOK:  true,
		},
		{
			name: "it matches a friendly-alias key through the bundle-id family",
			cfg: TerminalsConfig{
				"ghostty": {},
				"*":       {},
			},
			id:      NewIdentity("com.mitchellh.ghostty", "Ghostty"),
			wantKey: "ghostty",
			wantOK:  true,
		},
		{
			name: "it prefers a longer glob over a broader glob and both over a bare catch-all",
			cfg: TerminalsConfig{
				"com.mitchellh.*": {},
				"com.*":           {},
				"*":               {},
			},
			id:      NewIdentity("com.mitchellh.ghostty", "Ghostty"),
			wantKey: "com.mitchellh.*",
			wantOK:  true,
		},
		{
			name: "it selects the bare * catch-all only when nothing more specific matches",
			cfg: TerminalsConfig{
				"com.mitchellh.*": {},
				"dev.warp.Warp-*": {},
				"*":               {},
			},
			id:      NewIdentity("com.unknown.Term", "Term"),
			wantKey: "*",
			wantOK:  true,
		},
		{
			name: "it returns no match for an identity absent from the config",
			cfg: TerminalsConfig{
				"com.mitchellh.ghostty": {},
				"dev.warp.Warp-*":       {},
			},
			id:     NewIdentity("com.unknown.Term", "Term"),
			wantOK: false,
		},
		{
			name: "it prefers a named .app name over a glob",
			cfg: TerminalsConfig{
				"Term":  {},
				"com.*": {},
			},
			id:      NewIdentity("com.unknown.Term", "Term"),
			wantKey: "Term",
			wantOK:  true,
		},
		{
			name: "it matches a friendly-alias warp key through the bundle-id family",
			cfg: TerminalsConfig{
				"warp": {},
				"*":    {},
			},
			id:      NewIdentity("dev.warp.Warp-Stable", "Warp"),
			wantKey: "warp",
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, _, gotOK := matchConfig(tt.cfg, tt.id)
			if gotOK != tt.wantOK {
				t.Fatalf("matchConfig ok = %v, want %v (key=%q)", gotOK, tt.wantOK, gotKey)
			}
			if gotOK && gotKey != tt.wantKey {
				t.Errorf("matchConfig key = %q, want %q", gotKey, tt.wantKey)
			}
		})
	}
}

func TestMatchConfig_ReturnsWinningEntry(t *testing.T) {
	// The winning key's entry must be the one returned — prove the entry travels
	// with the key, not just the key string.
	want := TerminalEntry{Commands: Capabilities{Open: &Recipe{Argv: []string{"ghostty", "{command}"}}}}
	cfg := TerminalsConfig{
		"com.mitchellh.ghostty": want,
		"*":                     {},
	}

	gotKey, gotEntry, ok := matchConfig(cfg, NewIdentity("com.mitchellh.ghostty", "Ghostty"))

	if !ok {
		t.Fatal("matchConfig ok = false, want true")
	}
	if gotKey != "com.mitchellh.ghostty" {
		t.Fatalf("matchConfig key = %q, want %q", gotKey, "com.mitchellh.ghostty")
	}
	if gotEntry.Commands.Open == nil {
		t.Fatal("winning entry Commands.Open is nil, want the ghostty recipe")
	}
	if got := gotEntry.Commands.Open.Argv; !equalStrings(got, want.Commands.Open.Argv) {
		t.Errorf("winning entry argv = %v, want %v", got, want.Commands.Open.Argv)
	}
}

func TestMatchConfig_DeterministicTieBreak(t *testing.T) {
	// Two distinct globs that score EXACTLY equal (same tier, same literal
	// count) and both match the SAME identity. The winner must be the
	// lexicographically-smaller key, reproducibly, regardless of Go's
	// randomised map iteration order.
	//
	// bundle id "a.b.c" matches both "a.b.*" (literals ".a.b." → 4) and
	// "*.b.c" (literals ".b.c" → 4); "*.b.c" < "a.b.*" ('*' 0x2A < 'a' 0x61).
	cfg := TerminalsConfig{
		"a.b.*": {},
		"*.b.c": {},
	}
	id := NewIdentity("a.b.c", "")

	const wantKey = "*.b.c"
	for i := range 200 {
		gotKey, _, ok := matchConfig(cfg, id)
		if !ok {
			t.Fatalf("iteration %d: matchConfig ok = false, want true", i)
		}
		if gotKey != wantKey {
			t.Fatalf("iteration %d: matchConfig key = %q, want %q (nondeterministic tie-break)", i, gotKey, wantKey)
		}
	}
}

func TestCountLiterals(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want int
	}{
		{name: "it counts every non-star rune of a specific glob", key: "com.mitchellh.*", want: 14},
		{name: "it counts a broader glob as fewer literals", key: "com.*", want: 4},
		{name: "it counts a bare catch-all as zero literals", key: "*", want: 0},
		{name: "it counts multiple stars as still zero for a bare pattern", key: "**", want: 0},
		{name: "it counts an all-literal key in full", key: "com.apple.Terminal", want: 18},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countLiterals(tt.key); got != tt.want {
				t.Errorf("countLiterals(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}
