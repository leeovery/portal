package cmd

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
