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
