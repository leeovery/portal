package spawn

import (
	"slices"
	"testing"
)

func TestPreflightMissing(t *testing.T) {
	t.Run("it returns nil when every session is present", func(t *testing.T) {
		exists := func(string) bool { return true }
		if got := PreflightMissing([]string{"s1", "s2", "s3"}, exists); got != nil {
			t.Errorf("PreflightMissing = %#v, want nil when all present", got)
		}
	})

	t.Run("it collects the single gone session", func(t *testing.T) {
		exists := func(name string) bool { return name != "s2" }
		got := PreflightMissing([]string{"s1", "s2", "s3"}, exists)
		if !slices.Equal(got, []string{"s2"}) {
			t.Errorf("PreflightMissing = %#v, want [s2]", got)
		}
	})

	t.Run("it collects every gone session preserving input order", func(t *testing.T) {
		// s3 appears before s1 in the arg list; the result must follow input
		// order (not be sorted or reordered).
		gone := map[string]bool{"s3": true, "s1": true}
		exists := func(name string) bool { return !gone[name] }
		got := PreflightMissing([]string{"s3", "s2", "s1", "s0"}, exists)
		if !slices.Equal(got, []string{"s3", "s1"}) {
			t.Errorf("PreflightMissing = %#v, want [s3 s1] in list order", got)
		}
	})

	t.Run("it is pure — it probes each session exactly once, in order, via the injected exists", func(t *testing.T) {
		var probed []string
		exists := func(name string) bool { probed = append(probed, name); return true }
		PreflightMissing([]string{"a", "b", "c"}, exists)
		if !slices.Equal(probed, []string{"a", "b", "c"}) {
			t.Errorf("probed = %#v, want each session probed once in order", probed)
		}
	})
}
