package cmd

// Shared session-command test doubles, relocated from the deleted attach_test.go
// when `portal attach` was retired (cli-verb-surface-redesign Phase 5). These are
// consumed across the kill / version-guard / abridged / reattach / open test
// surfaces, so they live in their own file rather than any single _test.go.
//
// Tests consuming these mutate package-level state and MUST NOT use t.Parallel.

// mockSessionConnector records Connect calls for testing. When order is
// non-nil it also appends "connect" to the shared call-order recorder so a
// test can assert the write-strictly-before-connect ordering.
type mockSessionConnector struct {
	connectedTo string
	err         error
	order       *[]string
}

func (m *mockSessionConnector) Connect(name string) error {
	m.connectedTo = name
	if m.order != nil {
		*m.order = append(*m.order, "connect")
	}
	return m.err
}

// mockSessionValidator checks whether a session exists.
type mockSessionValidator struct {
	sessions map[string]bool
}

func (m *mockSessionValidator) HasSession(name string) bool {
	return m.sessions[name]
}

// ackWrite records a single AckWriter.Write(batch, token) call.
type ackWrite struct {
	batch string
	token string
}

// mockAckWriter records Write calls (satisfying spawn.AckWriter) and, when
// order is non-nil, appends "write" to the shared call-order recorder so a
// test can assert the write happens strictly before the connect.
type mockAckWriter struct {
	calls []ackWrite
	err   error
	order *[]string
}

func (m *mockAckWriter) Write(batch, token string) error {
	m.calls = append(m.calls, ackWrite{batch: batch, token: token})
	if m.order != nil {
		*m.order = append(*m.order, "write")
	}
	return m.err
}
