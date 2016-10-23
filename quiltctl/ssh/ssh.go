package ssh

// Client is an SSH client used for `quilt` commands.
type Client interface {
	// Connect establishes an SSH connection.
	Connect(string, string) error
	// RequestPTY requests a pseudo-terminal.
	RequestPTY() error
	// Run runs a command over the SSH connection.
	Run(string) error
	// Disconnect closes the SSH connection.
	Disconnect() error
}
