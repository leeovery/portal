package spawn

import "testing"

func TestSurfaceKindString(t *testing.T) {
	tests := []struct {
		name string
		kind SurfaceKind
		want string
	}{
		{"attach", SurfaceAttach, "attach"},
		{"mint", SurfaceMint, "mint"},
		{"unknown sentinel renders readably", SurfaceKind(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("SurfaceKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestSurfaceKindIota(t *testing.T) {
	// The engine and its tests depend on the zero value being SurfaceAttach.
	if SurfaceAttach != 0 {
		t.Errorf("SurfaceAttach = %d, want 0 (iota zero value)", SurfaceAttach)
	}
	if SurfaceMint != 1 {
		t.Errorf("SurfaceMint = %d, want 1", SurfaceMint)
	}
}

func TestSurfaceStruct(t *testing.T) {
	t.Run("attach carries the session name in Value", func(t *testing.T) {
		s := Surface{Kind: SurfaceAttach, Value: "api-x7Kd9a"}
		if s.Kind != SurfaceAttach {
			t.Errorf("Kind = %v, want SurfaceAttach", s.Kind)
		}
		if s.Value != "api-x7Kd9a" {
			t.Errorf("Value = %q, want %q", s.Value, "api-x7Kd9a")
		}
	})

	t.Run("mint carries the literal dir in Value", func(t *testing.T) {
		s := Surface{Kind: SurfaceMint, Value: "/Users/lee/Code/blog"}
		if s.Kind != SurfaceMint {
			t.Errorf("Kind = %v, want SurfaceMint", s.Kind)
		}
		if s.Value != "/Users/lee/Code/blog" {
			t.Errorf("Value = %q, want %q", s.Value, "/Users/lee/Code/blog")
		}
	})
}
