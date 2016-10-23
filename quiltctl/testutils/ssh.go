package testutils

import "github.com/stretchr/testify/mock"

// MockSSHClient mocks quiltctl/ssh.Client.
type MockSSHClient struct {
	mock.Mock
}

// Connect mocks ssh.Client.Connect().
func (m *MockSSHClient) Connect(host string, key string) error {
	args := m.Called(host, key)
	return args.Error(0)
}

// Run mocks ssh.Client.Run().
func (m *MockSSHClient) Run(command string) error {
	args := m.Called(command)
	return args.Error(0)
}

// RequestPTY mocks ssh.Client.RequestPTY()
func (m *MockSSHClient) RequestPTY() error {
	args := m.Called()
	return args.Error(0)
}

// Disconnect mocks ssh.Client.Disconnect().
func (m *MockSSHClient) Disconnect() error {
	args := m.Called()
	return args.Error(0)
}
