package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestFitSessionDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name  string
		dir   string
		width int
		want  string
	}{
		{
			name:  "it returns the home-abbreviated value when it fits",
			dir:   home + "/Code/portal",
			width: 40,
			want:  "~/Code/portal",
		},
		{
			name:  "it abbreviates even at a width the raw path would fit",
			dir:   home + "/Code/portal",
			width: lipgloss.Width(home+"/Code/portal") + 1,
			want:  "~/Code/portal",
		},
		{
			name:  "it abbreviates at the narrowest width that fits",
			dir:   home + "/Code/portal",
			width: lipgloss.Width("~/Code/portal"),
			want:  "~/Code/portal",
		},
		{
			name:  "it shortens from the left at a separator so the tail survives whole",
			dir:   home + "/Code/portal/internal/tui",
			width: lipgloss.Width("…/internal/tui"),
			want:  "…/internal/tui",
		},
		{
			name:  "it returns the longest tail the width allows at a narrow width",
			dir:   home + "/Code/portal/internal/tui",
			width: lipgloss.Width("…/portal/internal/tui") - 1,
			want:  "…/internal/tui",
		},
		{
			name:  "it returns the longest tail the width allows at a wider width",
			dir:   home + "/Code/portal/internal/tui",
			width: lipgloss.Width("…/portal/internal/tui"),
			want:  "…/portal/internal/tui",
		},
		{
			name:  "it drops the directory when the last segment cannot fit beside the ellipsis",
			dir:   home + "/Code/portal",
			width: lipgloss.Width("…/portal") - 1,
			want:  "",
		},
		{
			name:  "it returns a value carrying no separator whole when it fits",
			dir:   "scratchpad",
			width: lipgloss.Width("scratchpad"),
			want:  "scratchpad",
		},
		{
			name:  "it returns nothing for a value carrying no separator that does not fit",
			dir:   "scratchpad",
			width: lipgloss.Width("scratchpad") - 1,
			want:  "",
		},
		{
			name:  "it never emits a separators-only tail",
			dir:   home + "/Code/",
			width: lipgloss.Width("~/Code/") - 1,
			want:  "",
		},
		{
			name:  "it returns a bare tilde for the home directory itself",
			dir:   home,
			width: 40,
			want:  "~",
		},
		{
			name:  "it leaves a directory outside the home directory unabbreviated",
			dir:   "/opt/tools",
			width: 40,
			want:  "/opt/tools",
		},
		{
			name:  "it returns the empty string for an empty recorded directory",
			dir:   "",
			width: 40,
			want:  "",
		},
		{
			name:  "it returns the empty string for a zero width",
			dir:   home + "/Code/portal",
			width: 0,
			want:  "",
		},
		{
			name:  "it returns the empty string for a negative width",
			dir:   home + "/Code/portal",
			width: -3,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitSessionDir(tt.dir, tt.width); got != tt.want {
				t.Errorf("fitSessionDir(%q, %d) = %q, want %q", tt.dir, tt.width, got, tt.want)
			}
		})
	}
}

func TestFitSessionDirNeverCutsInsideASegment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := home + "/Code/portal/internal/tui"
	value := "~/Code/portal/internal/tui"

	for width := 1; width <= lipgloss.Width(value); width++ {
		got := fitSessionDir(dir, width)
		if got == "" || got == value {
			continue
		}

		tail, ok := strings.CutPrefix(got, dirTruncationPrefix)
		if !ok {
			t.Fatalf("fitSessionDir(%q, %d) = %q, want it prefixed by %q", dir, width, got, dirTruncationPrefix)
		}
		if !strings.HasPrefix(tail, "/") || !strings.HasSuffix(value, tail) {
			t.Fatalf("fitSessionDir(%q, %d) = %q, want a whole separator-delimited tail of %q", dir, width, got, value)
		}
		if lipgloss.Width(got) > width {
			t.Fatalf("fitSessionDir(%q, %d) = %q, which is %d cells wide", dir, width, got, lipgloss.Width(got))
		}
	}
}

func TestFitSessionDirMeasuresDisplayWidth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// 15 cells across 18 bytes — a byte-length measurement would drop it.
	dir := "/opt/日本語/tui"

	if got := fitSessionDir(dir, 15); got != dir {
		t.Errorf("fitSessionDir(%q, 15) = %q, want it whole", dir, got)
	}
	if got := fitSessionDir(dir, 14); got != "…/日本語/tui" {
		t.Errorf("fitSessionDir(%q, 14) = %q, want %q", dir, got, "…/日本語/tui")
	}
}

func TestFitSessionDirWithoutAResolvableHome(t *testing.T) {
	t.Setenv("HOME", "")

	dir := "/Users/someone/Code/portal"

	if got := fitSessionDir(dir, 40); got != dir {
		t.Errorf("fitSessionDir(%q, 40) = %q, want the raw path", dir, got)
	}
}
