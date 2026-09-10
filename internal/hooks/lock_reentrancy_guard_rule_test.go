package hooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// The rule is driven against synthetic sources so that each of its arms is
// seen to fire: the receiver and method names below are deliberately none of
// the real store's, since binding to those is exactly what the guard must not
// do. A mutation counts among the front doors — it reaches acquireLock through
// its own acquire — which is what makes a mutation calling another mutation a
// violation too.
func TestLockReentrancyGuard_Rule(t *testing.T) {
	const preamble = `package fixture

type Store struct{}

func acquireLock() {}

func (st *Store) acquireMutationLock() { acquireLock() }
func (st *Store) acquireSharedLock()   { acquireLock() }
func (st *Store) Load()                { st.acquireSharedLock() }
func (st *Store) load()                {}
`

	for _, tc := range []struct {
		name           string
		body           string
		wantMutations  int
		wantFrontDoors int
		wantViolations []string
	}{
		{
			name:           "a mutation calling a front door on its receiver is reported",
			body:           `func (st *Store) Put() { st.acquireMutationLock(); st.Load() }`,
			wantMutations:  1,
			wantFrontDoors: 4,
			wantViolations: []string{"Put calls st.Load"},
		},
		{
			name:           "a front door reached through another method is still a front door",
			body:           "func (st *Store) helper() { st.Load() }\nfunc (st *Store) Put() { st.acquireMutationLock(); st.helper() }",
			wantMutations:  1,
			wantFrontDoors: 5,
			wantViolations: []string{"Put calls st.helper"},
		},
		{
			name:           "a mutation reaching the file through the unexported read is clean",
			body:           `func (st *Store) Put() { st.acquireMutationLock(); st.load() }`,
			wantMutations:  1,
			wantFrontDoors: 4,
		},
		{
			name:           "a source with no mutation is judged as nothing, which the guard fatals on",
			body:           ``,
			wantMutations:  0,
			wantFrontDoors: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.go")
			if err := os.WriteFile(path, []byte(preamble+tc.body+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			verdict := judgeLockReentrancy(sourceguardtest.ParseSources(t, []string{path}))

			if verdict.mutations != tc.wantMutations {
				t.Errorf("mutations = %d, want %d", verdict.mutations, tc.wantMutations)
			}
			if verdict.frontDoors != tc.wantFrontDoors {
				t.Errorf("front doors = %d, want %d", verdict.frontDoors, tc.wantFrontDoors)
			}
			if len(verdict.violations) != len(tc.wantViolations) {
				t.Fatalf("violations = %q, want %d", verdict.violations, len(tc.wantViolations))
			}
			for i, want := range tc.wantViolations {
				if !strings.Contains(verdict.violations[i], want) {
					t.Errorf("violation %d = %q, want it to name %q", i, verdict.violations[i], want)
				}
			}
		})
	}
}
