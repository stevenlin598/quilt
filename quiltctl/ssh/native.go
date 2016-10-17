package ssh

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// NativeClient is wrapper over Go's SSH client.
type NativeClient struct {
	session *ssh.Session
}

// NewNativeClient creates a new NativeClient.
func NewNativeClient() *NativeClient {
	return &NativeClient{}
}

// Connect establishes an SSH session with reasonable, quilt-specific defaults.
func (c *NativeClient) Connect(host string, keyPath string) error {
	var auth ssh.AuthMethod
	if keyPath == "" {
		if sa, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
			auth = ssh.PublicKeysCallback(agent.NewClient(sa).Signers)
		}
	} else {
		auth = publicKeyFile(keyPath)
	}

	sshConfig := &ssh.ClientConfig{
		User: "quilt",
		Auth: []ssh.AuthMethod{auth},
	}

	conn, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", host), sshConfig)
	if err != nil {
		return err
	}

	c.session, err = conn.NewSession()
	if err != nil {
		return err
	}
	return nil
}

// Run runs an SSH command, erroring if no connection is open.
func (c *NativeClient) Run(command string) error {
	if c.session == nil {
		return errors.New("no open SSH session")
	}
	c.session.Stdout = os.Stdout
	c.session.Stdin = os.Stdin
	c.session.Stderr = os.Stderr

	return c.session.Run(command)
}

// Disconnect closes SSH session, erroring if none open.
func (c *NativeClient) Disconnect() error {
	if c.session == nil {
		return errors.New("no open SSH session")
	}
	return c.session.Close()
}

func publicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}

	return ssh.PublicKeys(key)
}
