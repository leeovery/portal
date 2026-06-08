package project

import "testing"

func TestNormaliseTag(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantTag string
		wantOk  bool
	}{
		{
			name:    "it trims leading and trailing whitespace",
			raw:     "  work ",
			wantTag: "work",
			wantOk:  true,
		},
		{
			name:    "it lower-cases mixed case",
			raw:     "Work",
			wantTag: "work",
			wantOk:  true,
		},
		{
			name:    "it lower-cases upper case",
			raw:     "WORK",
			wantTag: "work",
			wantOk:  true,
		},
		{
			name:    "it leaves already-lower-case unchanged",
			raw:     "work",
			wantTag: "work",
			wantOk:  true,
		},
		{
			name:    "it rejects the empty string",
			raw:     "",
			wantTag: "",
			wantOk:  false,
		},
		{
			name:    "it rejects whitespace-only input",
			raw:     "   ",
			wantTag: "",
			wantOk:  false,
		},
		{
			name:    "it preserves internal whitespace",
			raw:     "client a",
			wantTag: "client a",
			wantOk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTag, gotOk := NormaliseTag(tt.raw)
			if gotTag != tt.wantTag {
				t.Errorf("NormaliseTag(%q) tag = %q, want %q", tt.raw, gotTag, tt.wantTag)
			}
			if gotOk != tt.wantOk {
				t.Errorf("NormaliseTag(%q) ok = %v, want %v", tt.raw, gotOk, tt.wantOk)
			}
		})
	}
}

func TestFindByPath(t *testing.T) {
	projects := []Project{
		{Path: "/code/alpha", Name: "alpha"},
		{Path: "/code/beta", Name: "beta"},
		{Path: "/code/gamma", Name: "gamma"},
	}

	tests := []struct {
		name    string
		path    string
		wantIdx int
		wantOk  bool
	}{
		{name: "it finds the first project", path: "/code/alpha", wantIdx: 0, wantOk: true},
		{name: "it finds a middle project", path: "/code/beta", wantIdx: 1, wantOk: true},
		{name: "it finds the last project", path: "/code/gamma", wantIdx: 2, wantOk: true},
		{name: "it reports not-found for an unknown path", path: "/code/absent", wantIdx: -1, wantOk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIdx, gotOk := findByPath(projects, tt.path)
			if gotIdx != tt.wantIdx {
				t.Errorf("findByPath(%q) idx = %d, want %d", tt.path, gotIdx, tt.wantIdx)
			}
			if gotOk != tt.wantOk {
				t.Errorf("findByPath(%q) ok = %v, want %v", tt.path, gotOk, tt.wantOk)
			}
		})
	}
}
