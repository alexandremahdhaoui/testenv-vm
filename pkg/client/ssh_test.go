//go:build unit

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
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestMockSSHRunnerImplementsInterface verifies MockSSHRunner implements SSHRunner interface.
func TestMockSSHRunnerImplementsInterface(t *testing.T) {
	var _ SSHRunner = (*MockSSHRunner)(nil)
}

// TestMockSSHRunnerNewCreatesInstance verifies NewMockSSHRunner creates a valid instance.
func TestMockSSHRunnerNewCreatesInstance(t *testing.T) {
	mock := NewMockSSHRunner()

	if mock == nil {
		t.Fatal("expected non-nil MockSSHRunner")
	}
	if mock.Commands == nil {
		t.Error("expected Commands slice to be initialized")
	}
	if mock.Responses == nil {
		t.Error("expected Responses map to be initialized")
	}
}

// TestMockSSHRunnerRunRecordsCommand verifies Run records commands in Commands slice.
func TestMockSSHRunnerRunRecordsCommand(t *testing.T) {
	mock := NewMockSSHRunner()
	ctx := context.Background()
	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("test-key"),
	}

	cmd := "echo hello"
	_, _, err := mock.Run(ctx, vmInfo, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commands := mock.GetCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0] != cmd {
		t.Errorf("expected command %q, got %q", cmd, commands[0])
	}
}

// TestMockSSHRunnerRunReturnsConfiguredResponse verifies Run returns specific response when pattern matches.
func TestMockSSHRunnerRunReturnsConfiguredResponse(t *testing.T) {
	mock := NewMockSSHRunner()
	ctx := context.Background()
	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("test-key"),
	}

	cmd := "whoami"
	expectedResponse := MockResponse{
		Stdout: "root",
		Stderr: "",
		Err:    nil,
	}
	mock.AddResponse(cmd, expectedResponse)

	stdout, stderr, err := mock.Run(ctx, vmInfo, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != expectedResponse.Stdout {
		t.Errorf("expected stdout %q, got %q", expectedResponse.Stdout, stdout)
	}
	if stderr != expectedResponse.Stderr {
		t.Errorf("expected stderr %q, got %q", expectedResponse.Stderr, stderr)
	}
}

// TestMockSSHRunnerRunReturnsDefaultResponse verifies Run returns default response when no match.
func TestMockSSHRunnerRunReturnsDefaultResponse(t *testing.T) {
	mock := NewMockSSHRunner()
	mock.DefaultStdout = "default output"
	mock.DefaultStderr = "default error"
	mock.DefaultErr = errors.New("default error")

	ctx := context.Background()
	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("test-key"),
	}

	stdout, stderr, err := mock.Run(ctx, vmInfo, "unknown command")

	if stdout != mock.DefaultStdout {
		t.Errorf("expected stdout %q, got %q", mock.DefaultStdout, stdout)
	}
	if stderr != mock.DefaultStderr {
		t.Errorf("expected stderr %q, got %q", mock.DefaultStderr, stderr)
	}
	if err == nil || err.Error() != mock.DefaultErr.Error() {
		t.Errorf("expected error %v, got %v", mock.DefaultErr, err)
	}
}

// TestMockSSHRunnerAddResponse verifies AddResponse adds response to map.
func TestMockSSHRunnerAddResponse(t *testing.T) {
	mock := NewMockSSHRunner()
	cmd := "test command"
	response := MockResponse{
		Stdout: "test output",
		Stderr: "test error",
		Err:    errors.New("test error"),
	}

	mock.AddResponse(cmd, response)

	ctx := context.Background()
	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("test-key"),
	}

	stdout, stderr, err := mock.Run(ctx, vmInfo, cmd)
	if stdout != response.Stdout {
		t.Errorf("expected stdout %q, got %q", response.Stdout, stdout)
	}
	if stderr != response.Stderr {
		t.Errorf("expected stderr %q, got %q", response.Stderr, stderr)
	}
	if err == nil || err.Error() != response.Err.Error() {
		t.Errorf("expected error %v, got %v", response.Err, err)
	}
}

// TestMockSSHRunnerGetCommands verifies GetCommands returns copy of commands.
func TestMockSSHRunnerGetCommands(t *testing.T) {
	mock := NewMockSSHRunner()
	ctx := context.Background()
	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("test-key"),
	}

	// Run multiple commands
	commands := []string{"cmd1", "cmd2", "cmd3"}
	for _, cmd := range commands {
		_, _, err := mock.Run(ctx, vmInfo, cmd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	retrieved := mock.GetCommands()
	if len(retrieved) != len(commands) {
		t.Fatalf("expected %d commands, got %d", len(commands), len(retrieved))
	}

	for i, cmd := range commands {
		if retrieved[i] != cmd {
			t.Errorf("command %d: expected %q, got %q", i, cmd, retrieved[i])
		}
	}

	// Verify it's a copy (modifying returned slice doesn't affect internal state)
	retrieved[0] = "modified"
	retrieved2 := mock.GetCommands()
	if retrieved2[0] == "modified" {
		t.Error("GetCommands should return a copy, not the original slice")
	}
}

// TestMockSSHRunnerReset verifies Reset clears commands and responses.
func TestMockSSHRunnerReset(t *testing.T) {
	mock := NewMockSSHRunner()
	ctx := context.Background()
	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("test-key"),
	}

	// Add some commands and responses
	mock.AddResponse("cmd1", MockResponse{Stdout: "output1"})
	mock.DefaultStdout = "default"
	_, _, _ = mock.Run(ctx, vmInfo, "cmd1")
	_, _, _ = mock.Run(ctx, vmInfo, "cmd2")

	if len(mock.GetCommands()) != 2 {
		t.Fatalf("expected 2 commands before reset, got %d", len(mock.GetCommands()))
	}

	// Reset
	mock.Reset()

	// Verify commands cleared
	if len(mock.GetCommands()) != 0 {
		t.Errorf("expected 0 commands after reset, got %d", len(mock.GetCommands()))
	}

	// Verify responses cleared
	stdout, _, _ := mock.Run(ctx, vmInfo, "cmd1")
	if stdout == "output1" {
		t.Error("expected responses to be cleared after reset")
	}

	// Verify defaults cleared
	if mock.DefaultStdout != "" {
		t.Errorf("expected DefaultStdout to be cleared, got %q", mock.DefaultStdout)
	}
}

// TestMockSSHRunnerConcurrentAccess verifies thread safety with concurrent Run calls.
func TestMockSSHRunnerConcurrentAccess(t *testing.T) {
	mock := NewMockSSHRunner()
	mock.DefaultStdout = "ok"

	ctx := context.Background()
	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("test-key"),
	}

	// Run multiple goroutines concurrently
	const goroutines = 10
	const commandsPerGoroutine = 10
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < commandsPerGoroutine; j++ {
				cmd := "echo test"
				_, _, err := mock.Run(ctx, vmInfo, cmd)
				if err != nil {
					t.Errorf("goroutine %d: unexpected error: %v", id, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all commands were recorded
	commands := mock.GetCommands()
	expectedCount := goroutines * commandsPerGoroutine
	if len(commands) != expectedCount {
		t.Errorf("expected %d commands, got %d", expectedCount, len(commands))
	}
}

// TestMockSSHRunnerConcurrentAddResponse verifies thread safety with concurrent AddResponse calls.
func TestMockSSHRunnerConcurrentAddResponse(t *testing.T) {
	mock := NewMockSSHRunner()

	const goroutines = 10
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			mock.AddResponse(
				"cmd",
				MockResponse{Stdout: "output"},
			)
		}(i)
	}

	wg.Wait()

	// Should not panic (race detector would catch issues)
}

// TestMockSSHRunnerMultipleResponsesForDifferentCommands verifies multiple responses can be configured.
func TestMockSSHRunnerMultipleResponsesForDifferentCommands(t *testing.T) {
	mock := NewMockSSHRunner()
	ctx := context.Background()
	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("test-key"),
	}

	// Add multiple responses
	mock.AddResponse("whoami", MockResponse{Stdout: "root"})
	mock.AddResponse("hostname", MockResponse{Stdout: "testhost"})
	mock.AddResponse("pwd", MockResponse{Stdout: "/home/user"})

	// Verify each command returns its configured response
	tests := []struct {
		cmd            string
		expectedStdout string
	}{
		{"whoami", "root"},
		{"hostname", "testhost"},
		{"pwd", "/home/user"},
	}

	for _, tt := range tests {
		stdout, _, err := mock.Run(ctx, vmInfo, tt.cmd)
		if err != nil {
			t.Errorf("command %q: unexpected error: %v", tt.cmd, err)
		}
		if stdout != tt.expectedStdout {
			t.Errorf("command %q: expected stdout %q, got %q", tt.cmd, tt.expectedStdout, stdout)
		}
	}
}

// TestMockSSHRunnerErrorResponse verifies error responses are handled correctly.
func TestMockSSHRunnerErrorResponse(t *testing.T) {
	mock := NewMockSSHRunner()
	ctx := context.Background()
	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("test-key"),
	}

	expectedErr := errors.New("command failed")
	mock.AddResponse("failing-cmd", MockResponse{
		Stdout: "",
		Stderr: "error message",
		Err:    expectedErr,
	})

	stdout, stderr, err := mock.Run(ctx, vmInfo, "failing-cmd")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != expectedErr.Error() {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if stderr != "error message" {
		t.Errorf("expected stderr %q, got %q", "error message", stderr)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

// TestSSHRunnerImplementsInterface verifies sshRunner implements SSHRunner interface.
func TestSSHRunnerImplementsInterface(t *testing.T) {
	var _ SSHRunner = (*sshRunner)(nil)
}

// TestNewSSHRunnerDefaultTimeout verifies NewSSHRunner with default timeout (0 -> 10s).
func TestNewSSHRunnerDefaultTimeout(t *testing.T) {
	runner := NewSSHRunner(0)
	if runner == nil {
		t.Fatal("expected non-nil runner")
	}

	// Type assertion to check timeout
	sshR, ok := runner.(*sshRunner)
	if !ok {
		t.Fatal("expected runner to be *sshRunner")
	}

	expectedTimeout := 10 * time.Second
	if sshR.timeout != expectedTimeout {
		t.Errorf("expected timeout %v, got %v", expectedTimeout, sshR.timeout)
	}
}

// TestNewSSHRunnerCustomTimeout verifies NewSSHRunner with custom timeout.
func TestNewSSHRunnerCustomTimeout(t *testing.T) {
	customTimeout := 30 * time.Second
	runner := NewSSHRunner(customTimeout)
	if runner == nil {
		t.Fatal("expected non-nil runner")
	}

	// Type assertion to check timeout
	sshR, ok := runner.(*sshRunner)
	if !ok {
		t.Fatal("expected runner to be *sshRunner")
	}

	if sshR.timeout != customTimeout {
		t.Errorf("expected timeout %v, got %v", customTimeout, sshR.timeout)
	}
}

// TestSSHRunnerInvalidKeyFormat verifies invalid key format returns parse error.
func TestSSHRunnerInvalidKeyFormat(t *testing.T) {
	runner := NewSSHRunner(0)
	ctx := context.Background()

	vmInfo := &VMInfo{
		Host:       "127.0.0.1",
		Port:       "22",
		User:       "test",
		PrivateKey: []byte("invalid-key-format"),
	}

	_, _, err := runner.Run(ctx, vmInfo, "echo test")
	if err == nil {
		t.Fatal("expected error for invalid key format, got nil")
	}

	if !contains(err.Error(), "unable to parse private key") {
		t.Errorf("expected error to contain 'unable to parse private key', got: %v", err)
	}
}

// contains is a helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
