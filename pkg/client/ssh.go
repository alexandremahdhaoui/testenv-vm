// Copyright 2025 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package client

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHRunner executes commands on a remote VM via SSH.
type SSHRunner interface {
	// Run executes a command on the remote VM.
	// cmd is the already-formatted command string (from FormatCmd).
	Run(ctx context.Context, vmInfo *VMInfo, cmd string) (stdout, stderr string, err error)
}

// MockResponse holds a mock response for a command.
type MockResponse struct {
	Stdout string
	Stderr string
	Err    error
}

// MockSSHRunner is a mock implementation of SSHRunner for testing.
type MockSSHRunner struct {
	mu            sync.Mutex
	Commands      []string                // Records all commands executed
	Responses     map[string]MockResponse // Maps command patterns to responses
	DefaultStdout string
	DefaultStderr string
	DefaultErr    error
}

// NewMockSSHRunner creates a new MockSSHRunner with initialized maps.
func NewMockSSHRunner() *MockSSHRunner {
	return &MockSSHRunner{
		Commands:  make([]string, 0),
		Responses: make(map[string]MockResponse),
	}
}

// Run records the command and returns the configured response.
func (m *MockSSHRunner) Run(ctx context.Context, vmInfo *VMInfo, cmd string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Record the command
	m.Commands = append(m.Commands, cmd)

	// Check for a matching response
	if response, ok := m.Responses[cmd]; ok {
		return response.Stdout, response.Stderr, response.Err
	}

	// Return default response
	return m.DefaultStdout, m.DefaultStderr, m.DefaultErr
}

// AddResponse adds a response for a specific command pattern.
func (m *MockSSHRunner) AddResponse(cmdPattern string, response MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses[cmdPattern] = response
}

// GetCommands returns all commands that were executed.
func (m *MockSSHRunner) GetCommands() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to prevent external modification
	commands := make([]string, len(m.Commands))
	copy(commands, m.Commands)
	return commands
}

// Reset clears recorded commands and responses.
func (m *MockSSHRunner) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Commands = make([]string, 0)
	m.Responses = make(map[string]MockResponse)
	m.DefaultStdout = ""
	m.DefaultStderr = ""
	m.DefaultErr = nil
}

// sshRunner is the default SSHRunner implementation using golang.org/x/crypto/ssh.
type sshRunner struct {
	timeout time.Duration // Connection timeout (default 10s)
}

// NewSSHRunner creates a new SSH runner with the given timeout.
// If timeout is 0, defaults to 10 seconds.
func NewSSHRunner(timeout time.Duration) SSHRunner {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &sshRunner{
		timeout: timeout,
	}
}

// Run executes a command on the remote VM via SSH.
// Note: Context is used for timeout only, not per-command cancellation.
// The SSH session will run to completion or until the context deadline.
func (r *sshRunner) Run(ctx context.Context, vmInfo *VMInfo, cmd string) (string, string, error) {
	// 1. Parse private key
	signer, err := ssh.ParsePrivateKey(vmInfo.PrivateKey)
	if err != nil {
		return "", "", fmt.Errorf("unable to parse private key: %w", err)
	}

	// 2. Create SSH client config
	config := &ssh.ClientConfig{
		User: vmInfo.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For testing
		Timeout:         r.timeout,
	}

	// 3. Dial TCP connection
	addr := net.JoinHostPort(vmInfo.Host, vmInfo.Port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return "", "", fmt.Errorf("unable to connect to %s: %w", addr, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	// 4. Create session
	session, err := conn.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("unable to create SSH session: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()

	// 5. Capture stdout/stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	// 6. Run command
	if err := session.Run(cmd); err != nil {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("remote command failed: %w", err)
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}
