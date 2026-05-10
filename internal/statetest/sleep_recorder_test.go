package statetest_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/statetest"
)

// TestRecordingSleep_FnAppendsEachDuration pins the recording contract: every
// duration handed to the closure returned by Fn() must land in Durations in
// invocation order, with no de-duplication or reordering.
func TestRecordingSleep_FnAppendsEachDuration(t *testing.T) {
	r := &statetest.RecordingSleep{}
	sleep := r.Fn()

	want := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
	}
	for _, d := range want {
		sleep(d)
	}

	if !reflect.DeepEqual(r.Durations, want) {
		t.Errorf("Durations = %v; want %v", r.Durations, want)
	}
}

// TestRecordingSleep_ZeroValueStartsEmpty pins the zero-value contract: a
// freshly-declared RecordingSleep has no recorded durations and produces a
// usable Fn() without explicit initialisation. Callers rely on the zero-value
// to drop the helper into a config struct without ceremony.
func TestRecordingSleep_ZeroValueStartsEmpty(t *testing.T) {
	r := &statetest.RecordingSleep{}
	if len(r.Durations) != 0 {
		t.Errorf("zero-value Durations = %v; want empty slice", r.Durations)
	}
	r.Fn()(5 * time.Millisecond)
	if len(r.Durations) != 1 || r.Durations[0] != 5*time.Millisecond {
		t.Errorf("after one call, Durations = %v; want [5ms]", r.Durations)
	}
}
