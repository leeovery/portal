package log

import "testing"

func TestResolveRotateSize_DefaultsTo500MWhenUnset(t *testing.T) {
	bytes, source := resolveRotateSize("")

	if bytes != defaultRotateSize {
		t.Errorf("bytes = %d, want %d", bytes, defaultRotateSize)
	}
	if bytes != 524288000 {
		t.Errorf("bytes = %d, want %d (500 MiB)", bytes, 524288000)
	}
	if source != sourceDefault {
		t.Errorf("source = %q, want %q", source, sourceDefault)
	}
}

func TestResolveRotateSize_ParsesSuffixesAndBareBytes(t *testing.T) {
	cases := []struct {
		raw  string
		want int64
	}{
		{"500M", 524288000},
		{"500m", 524288000},
		{"1G", 1073741824},
		{"1g", 1073741824},
		{"512K", 524288},
		{"512k", 524288},
		{"1048576", 1048576},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			bytes, source := resolveRotateSize(tc.raw)

			if bytes != tc.want {
				t.Errorf("bytes = %d, want %d", bytes, tc.want)
			}
			if source != sourceEnv {
				t.Errorf("source = %q, want %q", source, sourceEnv)
			}
		})
	}
}

func TestResolveRotateSize_NormalisesSurroundingWhitespace(t *testing.T) {
	bytes, source := resolveRotateSize("  1G  ")

	if bytes != 1073741824 {
		t.Errorf("bytes = %d, want %d", bytes, 1073741824)
	}
	if source != sourceEnv {
		t.Errorf("source = %q, want %q", source, sourceEnv)
	}
}

func TestResolveRotateSize_FallsBackTo500MForInvalidValue(t *testing.T) {
	cases := []string{"abc", "5X", "-1", "0", "1.5M", "M", "1MM", "500MG"}

	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			bytes, source := resolveRotateSize(raw)

			if bytes != defaultRotateSize {
				t.Errorf("bytes = %d, want %d", bytes, defaultRotateSize)
			}
			if source != sourceFallback {
				t.Errorf("source = %q, want %q", source, sourceFallback)
			}
		})
	}
}

func TestResolveRetentionDays_DefaultsTo30WhenUnset(t *testing.T) {
	days, source, normRaw := resolveRetentionDays("")

	if days != defaultRetentionDays {
		t.Errorf("days = %d, want %d", days, defaultRetentionDays)
	}
	if days != 30 {
		t.Errorf("days = %d, want 30", days)
	}
	if source != sourceDefault {
		t.Errorf("source = %q, want %q", source, sourceDefault)
	}
	if normRaw != "" {
		t.Errorf("raw = %q, want %q", normRaw, "")
	}
}

func TestResolveRetentionDays_AcceptsValidRangeWithSourceEnv(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"0", 0},
		{"7", 7},
		{"30", 30},
		{"365", 365},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			days, source, normRaw := resolveRetentionDays(tc.raw)

			if days != tc.want {
				t.Errorf("days = %d, want %d", days, tc.want)
			}
			if source != sourceEnv {
				t.Errorf("source = %q, want %q", source, sourceEnv)
			}
			if normRaw != tc.raw {
				t.Errorf("raw = %q, want verbatim %q", normRaw, tc.raw)
			}
		})
	}
}

func TestResolveRetentionDays_FallsBackTo30PreservingRaw(t *testing.T) {
	cases := []string{"-1", "366", "400", "abc", "3.5"}

	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			days, source, normRaw := resolveRetentionDays(raw)

			if days != defaultRetentionDays {
				t.Errorf("days = %d, want %d", days, defaultRetentionDays)
			}
			if source != sourceFallback {
				t.Errorf("source = %q, want %q", source, sourceFallback)
			}
			if normRaw != raw {
				t.Errorf("raw = %q, want verbatim %q", normRaw, raw)
			}
		})
	}
}
