package spawn

import "testing"

func TestResultOutcomes_AllFourDistinct(t *testing.T) {
	// "it distinguishes all four outcomes with distinct enum values"
	seen := map[Outcome]bool{}
	for _, r := range []Result{
		Success("s"),
		Unsupported("u"),
		SpawnFailed("f"),
		PermissionRequired("p", "g"),
	} {
		seen[r.Outcome] = true
	}
	if len(seen) != 4 {
		t.Fatalf("expected 4 distinct Outcome values, got %d: %v", len(seen), seen)
	}

	// Each constructor stamps its designated Outcome constant.
	if got := Success("").Outcome; got != OutcomeSuccess {
		t.Errorf("Success().Outcome = %v, want OutcomeSuccess", got)
	}
	if got := Unsupported("").Outcome; got != OutcomeUnsupported {
		t.Errorf("Unsupported().Outcome = %v, want OutcomeUnsupported", got)
	}
	if got := SpawnFailed("").Outcome; got != OutcomeSpawnFailed {
		t.Errorf("SpawnFailed().Outcome = %v, want OutcomeSpawnFailed", got)
	}
	if got := PermissionRequired("", "").Outcome; got != OutcomePermissionRequired {
		t.Errorf("PermissionRequired().Outcome = %v, want OutcomePermissionRequired", got)
	}
}

func TestResultOK_TrueOnlyForSuccess(t *testing.T) {
	// "it reports OK only for the success outcome"
	if !Success("ok").OK() {
		t.Errorf("Success(...).OK() = false, want true")
	}
	for _, r := range []Result{
		Unsupported("u"),
		SpawnFailed("f"),
		PermissionRequired("p", "g"),
	} {
		if r.OK() {
			t.Errorf("Result{Outcome: %v}.OK() = true, want false", r.Outcome)
		}
	}
}

func TestResult_RoundTripsDetailAndGuidance(t *testing.T) {
	// "it round-trips opaque detail and guidance without interpretation"
	r := PermissionRequired("evt -1743", "grant Automation for Ghostty")
	if r.Detail != "evt -1743" {
		t.Errorf("Detail = %q, want %q", r.Detail, "evt -1743")
	}
	if r.Guidance != "grant Automation for Ghostty" {
		t.Errorf("Guidance = %q, want %q", r.Guidance, "grant Automation for Ghostty")
	}

	// Every non-permission constructor carries Detail verbatim and leaves
	// Guidance empty (Guidance is populated only by the permission path).
	for _, tc := range []struct {
		name   string
		got    Result
		detail string
	}{
		{"Success", Success("clean exit 0"), "clean exit 0"},
		{"SpawnFailed", SpawnFailed("AppleScript error body"), "AppleScript error body"},
		{"Unsupported", Unsupported("no native adapter"), "no native adapter"},
	} {
		if tc.got.Detail != tc.detail {
			t.Errorf("%s Detail = %q, want %q", tc.name, tc.got.Detail, tc.detail)
		}
		if tc.got.Guidance != "" {
			t.Errorf("%s Guidance = %q, want empty", tc.name, tc.got.Guidance)
		}
	}
}
