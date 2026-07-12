package spawn

import "testing"

func TestResolveAdapter(t *testing.T) {
	tests := []struct {
		name           string
		id             Identity
		wantResolution Resolution
		wantGhostty    bool
	}{
		{
			name:           "it resolves a Ghostty bundle id to the native adapter",
			id:             NewIdentity("com.mitchellh.ghostty", "Ghostty"),
			wantResolution: ResolutionNative,
			wantGhostty:    true,
		},
		{
			name:           "it resolves a channel-suffixed Ghostty bundle id via family match",
			id:             NewIdentity("com.mitchellh.ghostty.debug", ""),
			wantResolution: ResolutionNative,
			wantGhostty:    true,
		},
		{
			name:           "it returns unsupported for a NULL identity",
			id:             NewIdentity("", ""),
			wantResolution: ResolutionUnsupported,
			wantGhostty:    false,
		},
		{
			name:           "it returns unsupported for a known terminal with no native adapter",
			id:             NewIdentity("com.apple.Terminal", "Apple Terminal"),
			wantResolution: ResolutionUnsupported,
			wantGhostty:    false,
		},
		{
			name:           "it returns unsupported for a passthrough unknown identity",
			id:             NewIdentity("com.example.MyTerm", ""),
			wantResolution: ResolutionUnsupported,
			wantGhostty:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, resolution := ResolveAdapter(tt.id)

			if resolution != tt.wantResolution {
				t.Errorf("resolution = %q, want %q", resolution, tt.wantResolution)
			}

			// Invariants that must hold for every resolution: the resolver
			// never returns the Phase-4 config tier, and an unsupported
			// resolution always carries a nil adapter.
			if resolution == ResolutionConfig {
				t.Errorf("resolution = %q, want never ResolutionConfig in Phase 2", resolution)
			}
			if resolution == ResolutionUnsupported && adapter != nil {
				t.Errorf("adapter = %T with ResolutionUnsupported, want nil", adapter)
			}

			if tt.wantGhostty {
				if adapter == nil {
					t.Fatalf("adapter = nil, want non-nil *ghosttyAdapter")
				}
				if _, ok := adapter.(*ghosttyAdapter); !ok {
					t.Errorf("adapter = %T, want *ghosttyAdapter", adapter)
				}
				return
			}
			if adapter != nil {
				t.Errorf("adapter = %T, want nil", adapter)
			}
		})
	}
}
