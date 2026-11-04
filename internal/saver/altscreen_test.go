package saver

import "testing"

func TestAlternateScreen(t *testing.T) {
	for name, tc := range map[string]struct {
		vt   string
		want bool
	}{
		"vim, led by its modes":                 {"\x1b[?1h\x1b[?66h\x1b[?1049halt line\r\n\x1b[2;1H\x1b[0m", true},
		"1049 alone":                            {"\x1b[?1049halt line\x1b[2;6H\x1b[0m", true},
		"1047":                                  {"\x1b[?1047halt line", true},
		"47":                                    {"\x1b[?47halt line", true},
		"after modes zmx orders before it":      {"\x1b[?25l\x1b[?1049h\x1b[?2004halt line", true},
		"set with other modes in one sequence":  {"\x1b[?1;1049halt line", true},
		"the main screen":                       {"main line\r\nready\x1b[2;6H\x1b[0m", false},
		"the main screen led by its modes":      {"\x1b[?1h\x1b[?25l\x1b[?2004hmain line\r\n", false},
		"a mode sequence after the first cell":  {"main line\x1b[?1049halt", false},
		"a mode sequence after a cursor move":   {"\x1b[2;6H\x1b[?1049halt", false},
		"the alternate screen reset":            {"\x1b[?1049lmain line", false},
		"a mode that only ends in 1049":         {"\x1b[?11049hmain line", false},
		"a mode that only starts with 1049":     {"\x1b[?10490hmain line", false},
		"an ANSI mode, not a DEC private one":   {"\x1b[1049hmain line", false},
		"a sequence cut short":                  {"\x1b[?1049", false},
		"a sequence with no final mode byte":    {"\x1b[?1049mmain line", false},
		"the text ?1049h with no escape before": {"?1049h main line", false},
		"nothing":                               {"", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := alternateScreen([]byte(tc.vt)); got != tc.want {
				t.Errorf("alternateScreen(%q) = %v, want %v", tc.vt, got, tc.want)
			}
		})
	}
}
