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

// Package provider implements MCP client communication with provider processes.
package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// Default timeout for MCP operations.
const DefaultTimeout = 30 * time.Second

// jsonrpcRequest represents a JSON-RPC 2.0 request.
type jsonrpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

// jsonrpcResponse represents a JSON-RPC 2.0 response.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// jsonrpcError represents a JSON-RPC 2.0 error.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// mcpInitializeParams are the parameters for the initialize request.
type mcpInitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ClientInfo      mcpImplementation  `json:"clientInfo"`
	Capabilities    mcpCapabilities    `json:"capabilities"`
}

// mcpImplementation describes a client or server implementation.
type mcpImplementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// mcpCapabilities describes client capabilities.
type mcpCapabilities struct {
	// Empty for now - we don't need any special capabilities
}

// mcpInitializeResult is the response from initialize.
type mcpInitializeResult struct {
	ProtocolVersion string            `json:"protocolVersion"`
	ServerInfo      mcpImplementation `json:"serverInfo"`
	Capabilities    any               `json:"capabilities"`
}

// mcpToolCallParams are the parameters for a tools/call request.
type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// mcpToolCallResult is the response from tools/call.
type mcpToolCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
	Meta    any          `json:"_meta,omitempty"`
}

// mcpContent represents content in a tool call result.
type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Client is an MCP client wrapper for provider communication.
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	scanner *bufio.Scanner
	encoder *json.Encoder

	requestID   atomic.Int64
	initialized bool
	mu          sync.Mutex

	timeout time.Duration
}

// NewClient creates a new MCP client from a command.
// The command should be set up but not started yet.
// NewClient will start the process and perform the MCP initialization handshake.
func NewClient(cmd *exec.Cmd) (*Client, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("failed to start provider process: %w", err)
	}

	client := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		scanner: bufio.NewScanner(stdout),
		encoder: json.NewEncoder(stdin),
		timeout: DefaultTimeout,
	}

	// Perform MCP initialization handshake
	if err := client.initialize(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("MCP initialization failed: %w", err)
	}

	return client, nil
}

// SetTimeout sets the timeout for MCP operations.
func (c *Client) SetTimeout(timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timeout = timeout
}

// initialize performs the MCP initialization handshake.
func (c *Client) initialize() error {
	// Send initialize request
	params := mcpInitializeParams{
		ProtocolVersion: "2024-11-05",
		ClientInfo: mcpImplementation{
			Name:    "testenv-vm",
			Version: "1.0.0",
		},
		Capabilities: mcpCapabilities{},
	}

	var result mcpInitializeResult
	if err := c.call("initialize", params, &result); err != nil {
		return fmt.Errorf("initialize request failed: %w", err)
	}

	// Send initialized notification (no response expected)
	if err := c.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("initialized notification failed: %w", err)
	}

	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()
	return nil
}

// call sends a JSON-RPC request and waits for a response.
func (c *Client) call(method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := int(c.requestID.Add(1))

	// Convert params to map[string]any
	var paramsMap map[string]any
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
		if err := json.Unmarshal(data, &paramsMap); err != nil {
			return fmt.Errorf("failed to convert params to map: %w", err)
		}
	}

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsMap,
	}

	// Send request
	if err := c.encoder.Encode(req); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Read response synchronously while holding the mutex.
	// This avoids the race condition where a timeout could cause the mutex
	// to be released while a goroutine is still reading from the scanner.
	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
		return fmt.Errorf("unexpected end of response stream")
	}

	// Make a copy of the bytes since scanner reuses the buffer
	data := make([]byte, len(c.scanner.Bytes()))
	copy(data, c.scanner.Bytes())

	var resp jsonrpcResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Verify response ID matches request ID
	if resp.ID != id {
		return fmt.Errorf("response ID mismatch: expected %d, got %d", id, resp.ID)
	}

	// Check for JSON-RPC error
	if resp.Error != nil {
		return fmt.Errorf("JSON-RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	// Unmarshal result if provided
	if result != nil && resp.Result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params any) error {
	// Notifications have no ID
	var paramsMap map[string]any
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
		if err := json.Unmarshal(data, &paramsMap); err != nil {
			return fmt.Errorf("failed to convert params to map: %w", err)
		}
	}

	req := struct {
		JSONRPC string         `json:"jsonrpc"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsMap,
	}

	if err := c.encoder.Encode(req); err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}

	return nil
}

// Call invokes an MCP tool and returns the operation result.
func (c *Client) Call(tool string, input any) (*providerv1.OperationResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	// Convert input to map[string]any
	var args map[string]any
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal input: %w", err)
		}
		if err := json.Unmarshal(data, &args); err != nil {
			return nil, fmt.Errorf("failed to convert input to map: %w", err)
		}
	}

	params := mcpToolCallParams{
		Name:      tool,
		Arguments: args,
	}

	var result mcpToolCallResult
	if err := c.call("tools/call", params, &result); err != nil {
		return nil, err
	}

	// Check if the tool call returned an error
	if result.IsError {
		errMsg := "unknown error"
		if len(result.Content) > 0 {
			errMsg = result.Content[0].Text
		}
		return providerv1.ErrorResult(providerv1.NewProviderError(errMsg, false)), nil
	}

	// Parse the result content as OperationResult
	// Providers return OperationResult as JSON in the text content
	if len(result.Content) == 0 {
		return providerv1.ErrorResult(providerv1.NewProviderError("empty response from provider", false)), nil
	}

	var opResult providerv1.OperationResult
	if err := json.Unmarshal([]byte(result.Content[0].Text), &opResult); err != nil {
		// If the content is not a valid OperationResult, wrap it
		return providerv1.SuccessResult(result.Content[0].Text), nil
	}

	return &opResult, nil
}

// CallWithContext invokes an MCP tool with a context for cancellation/timeout.
func (c *Client) CallWithContext(ctx context.Context, tool string, input any) (*providerv1.OperationResult, error) {
	// Create a channel for the result
	type callResult struct {
		result *providerv1.OperationResult
		err    error
	}
	resultCh := make(chan callResult, 1)

	go func() {
		result, err := c.Call(tool, input)
		resultCh <- callResult{result: result, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-resultCh:
		return r.result, r.err
	}
}

// Capabilities retrieves the provider's capabilities.
func (c *Client) Capabilities() (*providerv1.CapabilitiesResponse, error) {
	result, err := c.Call("provider_capabilities", nil)
	if err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, fmt.Errorf("capabilities call failed: %s", result.Error.Message)
	}

	// The resource field should contain the capabilities
	data, err := json.Marshal(result.Resource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal capabilities: %w", err)
	}

	var caps providerv1.CapabilitiesResponse
	if err := json.Unmarshal(data, &caps); err != nil {
		return nil, fmt.Errorf("failed to unmarshal capabilities: %w", err)
	}

	return &caps, nil
}

// Close terminates the provider process and cleans up resources.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error

	// Close stdin to signal the process to exit
	if c.stdin != nil {
		if err := c.stdin.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close stdin: %w", err))
		}
	}

	// Close stdout
	if c.stdout != nil {
		if err := c.stdout.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close stdout: %w", err))
		}
	}

	// Wait for the process to exit (with timeout)
	if c.cmd != nil && c.cmd.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- c.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited cleanly
		case <-time.After(5 * time.Second):
			// Force kill if it doesn't exit in time
			if err := c.cmd.Process.Kill(); err != nil {
				errs = append(errs, fmt.Errorf("failed to kill process: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// IsRunning checks if the provider process is still running.
func (c *Client) IsRunning() bool {
	if c.cmd == nil || c.cmd.Process == nil {
		return false
	}
	// ProcessState is nil if the process hasn't exited
	return c.cmd.ProcessState == nil
}
