// White-box: the bytes a key sends are unexported.
package attachkeys

import "testing"

// Two keys sending the same bytes could not be told apart, so findClash
// compares keys and the matcher keys its tables on bytes alone.
func TestBindableKeysSendDistinctBytes(t *testing.T) {
	legacy := map[string]Key{}
	csi := map[csiKey]Key{}
	for _, text := range WrittenKeys() {
		k, err := bindable(text)
		if err != nil {
			continue
		}
		if other, ok := legacy[k.legacy()]; ok {
			t.Errorf("%s and %s send the same legacy bytes %q", other, k, k.legacy())
		}
		if other, ok := csi[k.csi()]; ok {
			t.Errorf("%s and %s share the CSI form %+v", other, k, k.csi())
		}
		legacy[k.legacy()], csi[k.csi()] = k, k
	}
	if len(legacy) < 100 {
		t.Fatalf("only %d bindable keys among %d written; the guard is scanning too little", len(legacy), len(WrittenKeys()))
	}
}
