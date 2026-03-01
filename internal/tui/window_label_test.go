package tui

import "testing"

func TestWindowLabel(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  string
	}{
		{name: "singular", count: 1, want: "1 window"},
		{name: "plural", count: 3, want: "3 windows"},
		{name: "zero", count: 0, want: "0 windows"},
		{name: "large count", count: 100, want: "100 windows"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := windowLabel(tt.count)
			if got != tt.want {
				t.Errorf("windowLabel(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}
