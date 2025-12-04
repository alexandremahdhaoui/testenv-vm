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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testProvider is a simple ClientProvider implementation for testing.
type testClientProvider struct {
	vms map[string]*VMInfo
}

func newTestProvider() *testClientProvider {
	return &testClientProvider{
		vms: make(map[string]*VMInfo),
	}
}

func (p *testClientProvider) AddVM(name string, info *VMInfo) {
	p.vms[name] = info
}

func (p *testClientProvider) GetVMInfo(vmName string) (*VMInfo, error) {
	info, ok := p.vms[vmName]
	if !ok {
		return nil, errors.New("VM not found")
	}
	return info, nil
}

// validVMInfo returns a valid VMInfo for testing.
func validVMInfo() *VMInfo {
	return &VMInfo{
		Host:       "192.168.1.100",
		Port:       "22",
		User:       "testuser",
		PrivateKey: []byte("-----BEGIN OPENSSH PRIVATE KEY-----\ntest-key\n-----END OPENSSH PRIVATE KEY-----"),
	}
}

// TestNewClientCreatesClientWithDefaults verifies NewClient creates client with defaults.
func TestNewClientCreatesClientWithDefaults(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	client, err := NewClient(provider, "test-vm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.provider != provider {
		t.Error("expected provider to be set")
	}

	if client.vmName != "test-vm" {
		t.Errorf("expected vmName 'test-vm', got %q", client.vmName)
	}

	if client.sshRunner == nil {
		t.Error("expected default sshRunner to be set")
	}

	if client.defaultExecCtx == nil {
		t.Error("expected default execution context to be set")
	}
}

// TestNewClientRequiresProvider verifies NewClient returns error when provider is nil.
func TestNewClientRequiresProvider(t *testing.T) {
	_, err := NewClient(nil, "test-vm")
	if err == nil {
		t.Fatal("expected error for nil provider")
	}

	if !strings.Contains(err.Error(), "provider is required") {
		t.Errorf("expected error to contain 'provider is required', got: %v", err)
	}
}

// TestNewClientRequiresVMName verifies NewClient returns error when vmName is empty.
func TestNewClientRequiresVMName(t *testing.T) {
	provider := newTestProvider()

	_, err := NewClient(provider, "")
	if err == nil {
		t.Fatal("expected error for empty vmName")
	}

	if !strings.Contains(err.Error(), "vmName is required") {
		t.Errorf("expected error to contain 'vmName is required', got: %v", err)
	}
}

// TestWithSSHRunnerSetsCustomRunner verifies WithSSHRunner sets custom runner.
func TestWithSSHRunnerSetsCustomRunner(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.sshRunner != mockRunner {
		t.Error("expected custom SSH runner to be set")
	}
}

// TestWithDefaultExecutionContextSetsContext verifies WithDefaultExecutionContext sets context.
func TestWithDefaultExecutionContextSetsContext(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	execCtx := NewExecutionContext().WithEnv("TEST", "value")

	client, err := NewClient(provider, "test-vm", WithDefaultExecutionContext(execCtx))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.defaultExecCtx != execCtx {
		t.Error("expected custom execution context to be set")
	}
}

// TestRunFormatsCommandCorrectly verifies Run formats command correctly.
func TestRunFormatsCommandCorrectly(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = "ok"

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	_, _, err = client.Run(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	commands := mockRunner.GetCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	// The command should be formatted with FormatCmd
	if !strings.Contains(commands[0], "echo") || !strings.Contains(commands[0], "hello") {
		t.Errorf("expected command to contain 'echo' and 'hello', got %q", commands[0])
	}
}

// TestRunWithContextUsesCustomContext verifies RunWithContext uses custom context.
func TestRunWithContextUsesCustomContext(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = "ok"

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create context with privilege escalation
	execCtx := NewExecutionContext().WithPrivilegeEscalation(PrivilegeEscalationSudo())

	ctx := context.Background()
	_, _, err = client.RunWithContext(ctx, execCtx, "whoami")
	if err != nil {
		t.Fatalf("RunWithContext failed: %v", err)
	}

	commands := mockRunner.GetCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	// Command should include sudo
	if !strings.Contains(commands[0], "sudo") {
		t.Errorf("expected command to contain 'sudo', got %q", commands[0])
	}
}

// TestRunWithContextWithEnvs verifies RunWithContext applies environment variables.
func TestRunWithContextWithEnvs(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = "ok"

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	execCtx := NewExecutionContext().WithEnv("MY_VAR", "my_value")

	ctx := context.Background()
	_, _, err = client.RunWithContext(ctx, execCtx, "echo", "$MY_VAR")
	if err != nil {
		t.Fatalf("RunWithContext failed: %v", err)
	}

	commands := mockRunner.GetCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	// Command should include environment variable
	if !strings.Contains(commands[0], "MY_VAR") {
		t.Errorf("expected command to contain 'MY_VAR', got %q", commands[0])
	}
}

// TestCopyToReadsFileAndGeneratesCorrectCommand verifies CopyTo reads file and generates correct command.
func TestCopyToReadsFileAndGeneratesCorrectCommand(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "test.txt")
	content := "test content for copy"
	if err := os.WriteFile(localFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = ""

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	err = client.CopyTo(ctx, localFile, "/tmp/remote.txt")
	if err != nil {
		t.Fatalf("CopyTo failed: %v", err)
	}

	commands := mockRunner.GetCommands()
	// Should have at least 2 commands: mkdir -p and the copy
	if len(commands) < 2 {
		t.Fatalf("expected at least 2 commands, got %d", len(commands))
	}

	// First command should be mkdir
	if !strings.Contains(commands[0], "mkdir -p") {
		t.Errorf("expected first command to contain 'mkdir -p', got %q", commands[0])
	}

	// Second command should contain base64
	if !strings.Contains(commands[1], "base64") {
		t.Errorf("expected second command to contain 'base64', got %q", commands[1])
	}
}

// TestCopyToReturnsErrorForMissingFile verifies CopyTo returns error if local file doesn't exist.
func TestCopyToReturnsErrorForMissingFile(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	err = client.CopyTo(ctx, "/nonexistent/file.txt", "/tmp/remote.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}

	if !strings.Contains(err.Error(), "failed to read local file") {
		t.Errorf("expected error to mention 'failed to read local file', got: %v", err)
	}
}

// TestCopyFromGeneratesCorrectCommandAndDecodesOutput verifies CopyFrom works correctly.
func TestCopyFromGeneratesCorrectCommandAndDecodesOutput(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	// Base64 encoded "test content"
	mockRunner.DefaultStdout = "dGVzdCBjb250ZW50"

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "downloaded.txt")

	ctx := context.Background()
	err = client.CopyFrom(ctx, "/tmp/remote.txt", localFile)
	if err != nil {
		t.Fatalf("CopyFrom failed: %v", err)
	}

	// Verify the command was correct
	commands := mockRunner.GetCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	if !strings.Contains(commands[0], "base64") {
		t.Errorf("expected command to contain 'base64', got %q", commands[0])
	}

	// Verify the file was written
	data, err := os.ReadFile(localFile)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(data) != "test content" {
		t.Errorf("expected content 'test content', got %q", string(data))
	}
}

// TestFileExistsReturnsTrueCorrectly verifies FileExists returns true for existing file.
func TestFileExistsReturnsTrueCorrectly(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = "exists"

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	exists, err := client.FileExists(ctx, "/tmp/test.txt")
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}

	if !exists {
		t.Error("expected FileExists to return true")
	}
}

// TestFileExistsReturnsFalseCorrectly verifies FileExists returns false for non-existing file.
func TestFileExistsReturnsFalseCorrectly(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = "not_exists"

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	exists, err := client.FileExists(ctx, "/tmp/nonexistent.txt")
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}

	if exists {
		t.Error("expected FileExists to return false")
	}
}

// TestMkdirAllGeneratesCorrectCommand verifies MkdirAll generates correct command.
func TestMkdirAllGeneratesCorrectCommand(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = ""

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	err = client.MkdirAll(ctx, "/tmp/nested/directory")
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	commands := mockRunner.GetCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	if !strings.Contains(commands[0], "mkdir -p") {
		t.Errorf("expected command to contain 'mkdir -p', got %q", commands[0])
	}
}

// TestChmodGeneratesCorrectCommand verifies Chmod generates correct command.
func TestChmodGeneratesCorrectCommand(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = ""

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	err = client.Chmod(ctx, "/tmp/test.sh", "755")
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	commands := mockRunner.GetCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	if !strings.Contains(commands[0], "chmod") || !strings.Contains(commands[0], "755") {
		t.Errorf("expected command to contain 'chmod' and '755', got %q", commands[0])
	}
}

// TestWaitReadyPollsAndSucceeds verifies WaitReady polls and succeeds.
func TestWaitReadyPollsAndSucceeds(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = ""

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	err = client.WaitReady(ctx, 30*time.Second)
	if err != nil {
		t.Fatalf("WaitReady failed: %v", err)
	}

	// Should have at least 2 commands: echo ready (SSH check) and cloud-init check
	commands := mockRunner.GetCommands()
	if len(commands) < 2 {
		t.Fatalf("expected at least 2 commands, got %d", len(commands))
	}

	// First command should be echo ready
	if !strings.Contains(commands[0], "echo ready") {
		t.Errorf("expected first command to be 'echo ready', got %q", commands[0])
	}

	// Second command should be cloud-init check
	if !strings.Contains(commands[1], "cloud-init") {
		t.Errorf("expected second command to contain 'cloud-init', got %q", commands[1])
	}
}

// TestWaitReadyTimesOutCorrectly verifies WaitReady times out correctly.
func TestWaitReadyTimesOutCorrectly(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	// Always return an error to simulate SSH not ready
	mockRunner.DefaultErr = errors.New("connection refused")

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	// Use a very short timeout
	err = client.WaitReady(ctx, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected error to contain 'timeout', got: %v", err)
	}
}

// TestWaitReadyRespectsContextCancellation verifies WaitReady respects context cancellation.
func TestWaitReadyRespectsContextCancellation(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	// Always return an error to force polling
	mockRunner.DefaultErr = errors.New("connection refused")

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately
	cancel()

	err = client.WaitReady(ctx, 30*time.Second)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}

	if !strings.Contains(err.Error(), "context cancelled") {
		t.Errorf("expected error to contain 'context cancelled', got: %v", err)
	}
}

// TestCloseIsIdempotent verifies Close is idempotent.
func TestCloseIsIdempotent(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	client, err := NewClient(provider, "test-vm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close multiple times should not panic or return error
	if err := client.Close(); err != nil {
		t.Errorf("first Close returned error: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Errorf("second Close returned error: %v", err)
	}
}

// TestGetVMInfoCachesResult verifies getVMInfo caches result.
func TestGetVMInfoCachesResult(t *testing.T) {
	provider := newTestProvider()
	vmInfo := validVMInfo()
	provider.AddVM("test-vm", vmInfo)

	mockRunner := NewMockSSHRunner()

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Run a command (this will call getVMInfo)
	ctx := context.Background()
	_, _, _ = client.Run(ctx, "echo", "test")

	// Verify info is cached
	if client.cachedVMInfo == nil {
		t.Fatal("expected VM info to be cached")
	}

	// Modify the provider to return different info
	provider.AddVM("test-vm", &VMInfo{
		Host:       "different-host",
		Port:       "22",
		User:       "different-user",
		PrivateKey: []byte("different-key"),
	})

	// Run another command - should use cached info
	_, _, _ = client.Run(ctx, "echo", "test2")

	// Cached info should still be the original
	if client.cachedVMInfo.Host != vmInfo.Host {
		t.Errorf("expected cached host %q, got %q", vmInfo.Host, client.cachedVMInfo.Host)
	}
}

// TestRunReturnsErrorWhenVMNotFound verifies Run returns error when VM is not found.
func TestRunReturnsErrorWhenVMNotFound(t *testing.T) {
	provider := newTestProvider()
	// Don't add any VMs

	client, err := NewClient(provider, "nonexistent-vm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	_, _, err = client.Run(ctx, "echo", "test")
	if err == nil {
		t.Fatal("expected error for nonexistent VM")
	}

	if !strings.Contains(err.Error(), "failed to get VM info") {
		t.Errorf("expected error to contain 'failed to get VM info', got: %v", err)
	}
}

// TestRunReturnsSSHError verifies Run returns SSH error.
func TestRunReturnsSSHError(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultErr = errors.New("SSH connection failed")
	mockRunner.DefaultStderr = "error details"

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	_, stderr, err := client.Run(ctx, "echo", "test")

	if err == nil {
		t.Fatal("expected SSH error")
	}

	if stderr != "error details" {
		t.Errorf("expected stderr 'error details', got %q", stderr)
	}
}

// TestCopyFromReturnsErrorWhenDecodeFailsverifies CopyFrom returns error on decode failure.
func TestCopyFromReturnsErrorWhenDecodeFails(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	// Invalid base64
	mockRunner.DefaultStdout = "not valid base64!!!"

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "downloaded.txt")

	ctx := context.Background()
	err = client.CopyFrom(ctx, "/tmp/remote.txt", localFile)
	if err == nil {
		t.Fatal("expected decode error")
	}

	if !strings.Contains(err.Error(), "failed to decode base64") {
		t.Errorf("expected error to contain 'failed to decode base64', got: %v", err)
	}
}

// TestClientWithMultipleOptions verifies client works with multiple options.
func TestClientWithMultipleOptions(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	mockRunner.DefaultStdout = "ok"

	execCtx := NewExecutionContext().
		WithEnv("TEST", "value").
		WithPrivilegeEscalation(PrivilegeEscalationSudo())

	client, err := NewClient(
		provider,
		"test-vm",
		WithSSHRunner(mockRunner),
		WithDefaultExecutionContext(execCtx),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	_, _, err = client.Run(ctx, "whoami")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	commands := mockRunner.GetCommands()
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}

	// Command should include both env and sudo
	if !strings.Contains(commands[0], "TEST") {
		t.Errorf("expected command to contain 'TEST', got %q", commands[0])
	}
	if !strings.Contains(commands[0], "sudo") {
		t.Errorf("expected command to contain 'sudo', got %q", commands[0])
	}
}

// TestWaitReadyCloudInitFails verifies WaitReady returns error when cloud-init fails.
func TestWaitReadyCloudInitFails(t *testing.T) {
	provider := newTestProvider()
	provider.AddVM("test-vm", validVMInfo())

	mockRunner := NewMockSSHRunner()
	// SSH check succeeds
	mockRunner.AddResponse("echo ready", MockResponse{Stdout: "ready"})
	// Cloud-init check fails
	mockRunner.DefaultErr = errors.New("cloud-init failed")
	mockRunner.DefaultStderr = "cloud-init error"

	client, err := NewClient(provider, "test-vm", WithSSHRunner(mockRunner))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	err = client.WaitReady(ctx, 30*time.Second)
	if err == nil {
		t.Fatal("expected cloud-init error")
	}

	if !strings.Contains(err.Error(), "cloud-init did not complete") {
		t.Errorf("expected error to contain 'cloud-init did not complete', got: %v", err)
	}
}

// TestVMInfoValidationFailure verifies error when VMInfo validation fails.
func TestVMInfoValidationFailure(t *testing.T) {
	provider := newTestProvider()
	// Add VM with invalid info (empty Host)
	provider.AddVM("test-vm", &VMInfo{
		Host:       "", // Invalid
		Port:       "22",
		User:       "testuser",
		PrivateKey: []byte("key"),
	})

	client, err := NewClient(provider, "test-vm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	_, _, err = client.Run(ctx, "echo", "test")
	if err == nil {
		t.Fatal("expected validation error")
	}

	if !strings.Contains(err.Error(), "invalid VM info") {
		t.Errorf("expected error to contain 'invalid VM info', got: %v", err)
	}
}
