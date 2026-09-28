package cmd

import (
	"errors"
	"maps"
	"testing"
)

type serverOptionsStub struct {
	out string
	err error
}

func (s serverOptionsStub) ShowAllServerOptions() (string, error) { return s.out, s.err }

func TestSkeletonMarkerReader_ReadsTheSkeletonMarkersTheServerCarries(t *testing.T) {
	r := skeletonMarkerReader{lister: serverOptionsStub{out: "@portal-skeleton-abc 1\n@portal-restoring 1\n@portal-skeleton-def 1\n"}}

	got, err := r.ListSkeletonMarkers()

	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}
	want := map[string]struct{}{"abc": {}, "def": {}}
	if !maps.Equal(got, want) {
		t.Errorf("markers = %v, want %v", got, want)
	}
}

func TestSkeletonMarkerReader_HandsBackTheReadFailure(t *testing.T) {
	readErr := errors.New("no server running")
	r := skeletonMarkerReader{lister: serverOptionsStub{err: readErr}}

	got, err := r.ListSkeletonMarkers()

	if !errors.Is(err, readErr) || got != nil {
		t.Errorf("ListSkeletonMarkers = (%v, %v), want (nil, %v)", got, err, readErr)
	}
}
