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
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Client provides high-level operations on a VM via SSH.
type Client struct {
	provider       ClientProvider
	vmName         string
	sshRunner      SSHRunner
	defaultExecCtx *ExecutionContext
	cachedVMInfo   *VMInfo
}

// ClientOption configures the Client.
type ClientOption func(*Client)

// WithSSHRunner sets a custom SSH runner (e.g., MockSSHRunner for testing).
func WithSSHRunner(runner SSHRunner) ClientOption {
	return func(c *Client) {
		c.sshRunner = runner
	}
}

// WithDefaultExecutionContext sets the default execution context.
func WithDefaultExecutionContext(ctx *ExecutionContext) ClientOption {
	return func(c *Client) {
		c.defaultExecCtx = ctx
	}
}

// NewClient creates a new Client for the given VM.
// Note: provider is the ClientProvider interface defined in this package.
func NewClient(provider ClientProvider, vmName string, opts ...ClientOption) (*Client, error) {
	if provider == nil {
		return nil, fmt.Errorf("client: provider is required")
	}
	if vmName == "" {
		return nil, fmt.Errorf("client: vmName is required")
	}

	c := &Client{
		provider:       provider,
		vmName:         vmName,
		sshRunner:      NewSSHRunner(10 * time.Second),
		defaultExecCtx: NewExecutionContext(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Run executes a command on the VM using the default execution context.
func (c *Client) Run(ctx context.Context, cmd ...string) (stdout, stderr string, err error) {
	return c.RunWithContext(ctx, c.defaultExecCtx, cmd...)
}

// RunWithContext executes a command with a custom execution context.
func (c *Client) RunWithContext(ctx context.Context, execCtx *ExecutionContext, cmd ...string) (stdout, stderr string, err error) {
	vmInfo, err := c.getVMInfo()
	if err != nil {
		return "", "", fmt.Errorf("client: failed to get VM info: %w", err)
	}

	formattedCmd := FormatCmd(execCtx, cmd...)
	return c.sshRunner.Run(ctx, vmInfo, formattedCmd)
}

// CopyTo copies a local file to the VM.
func (c *Client) CopyTo(ctx context.Context, localPath, remotePath string) error {
	vmInfo, err := c.getVMInfo()
	if err != nil {
		return fmt.Errorf("client: failed to get VM info: %w", err)
	}

	// Read local file
	content, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("client: failed to read local file %s: %w", localPath, err)
	}

	// Create parent directory on remote
	parentDir := filepath.Dir(remotePath)
	mkdirCommand := mkdirCmd(parentDir)
	formattedMkdir := FormatCmd(c.defaultExecCtx, mkdirCommand)
	_, stderr, err := c.sshRunner.Run(ctx, vmInfo, formattedMkdir)
	if err != nil {
		return fmt.Errorf("client: failed to create remote directory %s: %w (stderr: %s)", parentDir, err, stderr)
	}

	// Copy file content using base64
	copyCommand := copyToCmd(content, remotePath)
	formattedCopy := FormatCmd(c.defaultExecCtx, copyCommand)
	_, stderr, err = c.sshRunner.Run(ctx, vmInfo, formattedCopy)
	if err != nil {
		return fmt.Errorf("client: failed to copy file to %s: %w (stderr: %s)", remotePath, err, stderr)
	}

	return nil
}

// CopyFrom copies a remote file from the VM to local.
func (c *Client) CopyFrom(ctx context.Context, remotePath, localPath string) error {
	vmInfo, err := c.getVMInfo()
	if err != nil {
		return fmt.Errorf("client: failed to get VM info: %w", err)
	}

	// Get file content from remote via base64
	copyCommand := copyFromCmd(remotePath)
	formattedCmd := FormatCmd(c.defaultExecCtx, copyCommand)
	stdout, stderr, err := c.sshRunner.Run(ctx, vmInfo, formattedCmd)
	if err != nil {
		return fmt.Errorf("client: failed to read remote file %s: %w (stderr: %s)", remotePath, err, stderr)
	}

	// Decode base64 content
	content, err := decodeBase64(stdout)
	if err != nil {
		return fmt.Errorf("client: failed to decode base64 content: %w", err)
	}

	// Create local parent directory if needed
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("client: failed to create local directory %s: %w", localDir, err)
	}

	// Write to local file
	if err := os.WriteFile(localPath, content, 0644); err != nil {
		return fmt.Errorf("client: failed to write local file %s: %w", localPath, err)
	}

	return nil
}

// FileExists checks if a file exists on the VM.
func (c *Client) FileExists(ctx context.Context, path string) (bool, error) {
	vmInfo, err := c.getVMInfo()
	if err != nil {
		return false, fmt.Errorf("client: failed to get VM info: %w", err)
	}

	existsCommand := fileExistsCmd(path)
	formattedCmd := FormatCmd(c.defaultExecCtx, existsCommand)
	stdout, _, err := c.sshRunner.Run(ctx, vmInfo, formattedCmd)
	if err != nil {
		return false, fmt.Errorf("client: failed to check file existence: %w", err)
	}

	return parseFileExists(stdout), nil
}

// MkdirAll creates a directory on the VM (with parents).
func (c *Client) MkdirAll(ctx context.Context, path string) error {
	vmInfo, err := c.getVMInfo()
	if err != nil {
		return fmt.Errorf("client: failed to get VM info: %w", err)
	}

	mkdirCommand := mkdirCmd(path)
	formattedCmd := FormatCmd(c.defaultExecCtx, mkdirCommand)
	_, stderr, err := c.sshRunner.Run(ctx, vmInfo, formattedCmd)
	if err != nil {
		return fmt.Errorf("client: failed to create directory %s: %w (stderr: %s)", path, err, stderr)
	}

	return nil
}

// Chmod sets file permissions on the VM.
func (c *Client) Chmod(ctx context.Context, path, mode string) error {
	vmInfo, err := c.getVMInfo()
	if err != nil {
		return fmt.Errorf("client: failed to get VM info: %w", err)
	}

	chmodCommand := chmodCmd(path, mode)
	formattedCmd := FormatCmd(c.defaultExecCtx, chmodCommand)
	_, stderr, err := c.sshRunner.Run(ctx, vmInfo, formattedCmd)
	if err != nil {
		return fmt.Errorf("client: failed to chmod %s %s: %w (stderr: %s)", mode, path, err, stderr)
	}

	return nil
}

// WaitReady waits for the VM to be ready (SSH + cloud-init).
// Phase 1: Poll for VM IP and SSH until connection succeeds (every 5s).
// Phase 2: Run `timeout 60 cloud-init status --wait || test -f /var/lib/cloud/instance/boot-finished`
func (c *Client) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 5 * time.Second

	var vmInfo *VMInfo
	var lastErr error

	// Phase 1: Poll for VM info and SSH until connection succeeds
	for {
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("client: timeout waiting for VM %s to be ready: %w", c.vmName, lastErr)
			}
			return fmt.Errorf("client: timeout waiting for SSH on VM %s", c.vmName)
		}

		// Try to get VM info (IP may not be available immediately)
		// Clear cached info to force re-query from provider
		c.cachedVMInfo = nil
		vmInfo, lastErr = c.getVMInfo()
		if lastErr != nil {
			// VM info not available yet, wait and retry
			select {
			case <-ctx.Done():
				return fmt.Errorf("client: context cancelled while waiting for VM info: %w", ctx.Err())
			case <-time.After(pollInterval):
				continue
			}
		}

		// Try a simple command to verify SSH connectivity
		_, _, lastErr = c.sshRunner.Run(ctx, vmInfo, "echo ready")
		if lastErr == nil {
			break // SSH is ready
		}

		// Check if context is done
		select {
		case <-ctx.Done():
			return fmt.Errorf("client: context cancelled while waiting for SSH: %w", ctx.Err())
		case <-time.After(pollInterval):
			// Continue polling
		}
	}

	// Phase 2: Wait for cloud-init completion
	// Use `timeout 60` to prevent hangs if cloud-init never completes
	// Fallback to checking boot-finished file for systems without cloud-init status command
	cloudInitCmd := "timeout 60 cloud-init status --wait || test -f /var/lib/cloud/instance/boot-finished"
	_, stderr, err := c.sshRunner.Run(ctx, vmInfo, cloudInitCmd)
	if err != nil {
		return fmt.Errorf("client: cloud-init did not complete: %w (stderr: %s)", err, stderr)
	}

	return nil
}

// Close cleans up any resources.
func (c *Client) Close() error {
	// Currently no resources to clean up.
	// This method exists for future-proofing and interface compliance.
	return nil
}

// getVMInfo returns cached VM info or fetches it from provider.
func (c *Client) getVMInfo() (*VMInfo, error) {
	if c.cachedVMInfo != nil {
		return c.cachedVMInfo, nil
	}

	vmInfo, err := c.provider.GetVMInfo(c.vmName)
	if err != nil {
		return nil, err
	}

	// Validate the VM info
	if err := vmInfo.Validate(); err != nil {
		return nil, fmt.Errorf("client: invalid VM info: %w", err)
	}

	c.cachedVMInfo = vmInfo
	return c.cachedVMInfo, nil
}
