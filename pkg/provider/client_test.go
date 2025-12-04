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

// Package provider implements MCP client communication with provider processes.
package provider

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// mockProcess simulates an MCP provider process for testing.
type mockProcess struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser

	// For simulating the process
	stdinReader  io.ReadCloser
	stdoutWriter io.WriteCloser

	responses []string
	respIndex int
	mu        sync.Mutex

	closed bool
}

// newMockProcess creates a mock process with predefined responses.
func newMockProcess(responses []string) *mockProcess {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	mp := &mockProcess{
		stdin:        stdinWriter,
		stdout:       stdoutReader,
		stdinReader:  stdinReader,
		stdoutWriter: stdoutWriter,
		responses:    responses,
	}

	// Start a goroutine to handle requests
	go mp.handleRequests()

	return mp
}

func (mp *mockProcess) handleRequests() {
	scanner := bufio.NewScanner(mp.stdinReader)
	for scanner.Scan() {
		mp.mu.Lock()
		if mp.respIndex < len(mp.responses) {
			resp := mp.responses[mp.respIndex]
			mp.respIndex++
			mp.mu.Unlock()
			mp.stdoutWriter.Write([]byte(resp + "\n"))
		} else {
			mp.mu.Unlock()
			// No more responses
			break
		}
	}
}

func (mp *mockProcess) Close() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if mp.closed {
		return nil
	}
	mp.closed = true
	mp.stdin.Close()
	mp.stdout.Close()
	mp.stdinReader.Close()
	mp.stdoutWriter.Close()
	return nil
}

// TestCallWithValidResponse tests that call() properly handles valid responses.
func TestCallWithValidResponse(t *testing.T) {
	// Create a response for the call
	response := `{"jsonrpc":"2.0","id":1,"result":{"key":"value"}}`

	mp := newMockProcess([]string{response})
	defer mp.Close()

	client := &Client{
		stdin:   mp.stdin,
		stdout:  mp.stdout,
		scanner: bufio.NewScanner(mp.stdout),
		encoder: json.NewEncoder(mp.stdin),
		timeout: 5 * time.Second,
	}

	var result map[string]string
	err := client.call("test_method", nil, &result)
	if err != nil {
		t.Fatalf("call() returned error: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("expected result[key] = value, got %v", result["key"])
	}
}

// TestCallWithErrorResponse tests that call() properly handles error responses.
func TestCallWithErrorResponse(t *testing.T) {
	response := `{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid Request"}}`

	mp := newMockProcess([]string{response})
	defer mp.Close()

	client := &Client{
		stdin:   mp.stdin,
		stdout:  mp.stdout,
		scanner: bufio.NewScanner(mp.stdout),
		encoder: json.NewEncoder(mp.stdin),
		timeout: 5 * time.Second,
	}

	err := client.call("test_method", nil, nil)
	if err == nil {
		t.Fatal("expected error from call()")
	}

	if !strings.Contains(err.Error(), "Invalid Request") {
		t.Errorf("expected error to contain 'Invalid Request', got: %v", err)
	}
}

// TestCallWithIDMismatch tests that call() detects response ID mismatches.
func TestCallWithIDMismatch(t *testing.T) {
	// Response with wrong ID (2 instead of 1)
	response := `{"jsonrpc":"2.0","id":2,"result":{}}`

	mp := newMockProcess([]string{response})
	defer mp.Close()

	client := &Client{
		stdin:   mp.stdin,
		stdout:  mp.stdout,
		scanner: bufio.NewScanner(mp.stdout),
		encoder: json.NewEncoder(mp.stdin),
		timeout: 5 * time.Second,
	}

	err := client.call("test_method", nil, nil)
	if err == nil {
		t.Fatal("expected error for ID mismatch")
	}

	if !strings.Contains(err.Error(), "response ID mismatch") {
		t.Errorf("expected error about ID mismatch, got: %v", err)
	}
}

// TestCallWithTimeout tests that call() times out correctly.
func TestCallWithTimeout(t *testing.T) {
	// Create a mock process that never responds
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	defer stdinReader.Close()
	defer stdinWriter.Close()
	defer stdoutReader.Close()
	defer stdoutWriter.Close()

	client := &Client{
		stdin:   stdinWriter,
		stdout:  stdoutReader,
		scanner: bufio.NewScanner(stdoutReader),
		encoder: json.NewEncoder(stdinWriter),
		timeout: 100 * time.Millisecond, // Short timeout for testing
	}

	// Drain stdin to prevent blocking
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := stdinReader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	start := time.Now()
	err := client.call("test_method", nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got: %v", err)
	}

	// Verify timeout occurred approximately at the right time
	if elapsed < 100*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Errorf("timeout occurred at unexpected time: %v", elapsed)
	}
}

// TestCallConcurrentAccess tests that call() is safe for concurrent access.
func TestCallConcurrentAccess(t *testing.T) {
	// Create responses for multiple concurrent calls
	responses := make([]string, 10)
	for i := range responses {
		responses[i] = `{"jsonrpc":"2.0","id":` + string(rune('1'+i)) + `,"result":{}}`
	}

	// Use a channel-based approach for concurrent testing
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	client := &Client{
		stdin:   stdinWriter,
		stdout:  stdoutReader,
		scanner: bufio.NewScanner(stdoutReader),
		encoder: json.NewEncoder(stdinWriter),
		timeout: 5 * time.Second,
	}

	// Start a goroutine to respond to requests
	go func() {
		scanner := bufio.NewScanner(stdinReader)
		for scanner.Scan() {
			var req jsonrpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				continue
			}
			resp := jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{}`),
			}
			data, _ := json.Marshal(resp)
			stdoutWriter.Write(data)
			stdoutWriter.Write([]byte("\n"))
		}
	}()

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Launch 10 concurrent calls - but they will be serialized by the mutex
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := client.call("test_method", nil, nil)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	stdinWriter.Close()
	stdoutWriter.Close()

	for err := range errors {
		t.Errorf("concurrent call failed: %v", err)
	}
}

// TestSetTimeout tests that SetTimeout correctly updates the timeout value.
func TestSetTimeout(t *testing.T) {
	client := &Client{
		timeout: DefaultTimeout,
	}

	newTimeout := 10 * time.Second
	client.SetTimeout(newTimeout)

	client.mu.Lock()
	defer client.mu.Unlock()

	if client.timeout != newTimeout {
		t.Errorf("expected timeout %v, got %v", newTimeout, client.timeout)
	}
}

// TestCallPublicMethod tests the public Call method that wraps internal call.
func TestCallPublicMethod(t *testing.T) {
	// Create a response that looks like an OperationResult
	opResult := providerv1.OperationResult{
		Success: true,
		Resource: map[string]any{
			"name": "test-resource",
		},
	}
	opResultJSON, _ := json.Marshal(opResult)

	mcpResult := mcpToolCallResult{
		Content: []mcpContent{
			{Type: "text", Text: string(opResultJSON)},
		},
	}
	mcpResultJSON, _ := json.Marshal(mcpResult)

	// For tools/call - ID will be 1 since requestID starts at 0 and adds 1
	callResponse := `{"jsonrpc":"2.0","id":1,"result":` + string(mcpResultJSON) + `}`

	mp := newMockProcess([]string{callResponse})
	defer mp.Close()

	client := &Client{
		stdin:       mp.stdin,
		stdout:      mp.stdout,
		scanner:     bufio.NewScanner(mp.stdout),
		encoder:     json.NewEncoder(mp.stdin),
		timeout:     5 * time.Second,
		initialized: true, // Skip initialization for this test
	}

	result, err := client.Call("test_tool", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Call() returned error: %v", err)
	}

	if !result.Success {
		t.Error("expected success=true")
	}
}

// TestCallNotInitialized tests that Call returns error when not initialized.
func TestCallNotInitialized(t *testing.T) {
	client := &Client{
		initialized: false,
	}

	_, err := client.Call("test_tool", nil)
	if err == nil {
		t.Fatal("expected error when not initialized")
	}

	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("expected 'not initialized' error, got: %v", err)
	}
}

// TestInitializeRaceCondition tests that initialize() sets initialized safely.
func TestInitializeRaceCondition(t *testing.T) {
	// Initialize response
	initResponse := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","serverInfo":{"name":"test","version":"1.0.0"}}}`

	mp := newMockProcess([]string{initResponse})
	defer mp.Close()

	client := &Client{
		stdin:   mp.stdin,
		stdout:  mp.stdout,
		scanner: bufio.NewScanner(mp.stdout),
		encoder: json.NewEncoder(mp.stdin),
		timeout: 5 * time.Second,
	}

	// Run initialize and check that initialized is set correctly
	err := client.initialize()
	if err != nil {
		t.Fatalf("initialize() returned error: %v", err)
	}

	// Access initialized with proper synchronization
	client.mu.Lock()
	initialized := client.initialized
	client.mu.Unlock()

	if !initialized {
		t.Error("expected initialized=true after initialize()")
	}
}

// TestIsRunning tests the IsRunning method.
func TestIsRunning(t *testing.T) {
	// Test with nil cmd
	client := &Client{
		cmd: nil,
	}
	if client.IsRunning() {
		t.Error("expected IsRunning()=false for nil cmd")
	}

	// Test with a real but not started command
	cmd := exec.Command("echo", "test")
	client = &Client{
		cmd: cmd,
	}
	if client.IsRunning() {
		t.Error("expected IsRunning()=false for unstarted command")
	}
}

// TestCallWithMalformedResponse tests handling of invalid JSON responses.
func TestCallWithMalformedResponse(t *testing.T) {
	response := `not valid json`

	mp := newMockProcess([]string{response})
	defer mp.Close()

	client := &Client{
		stdin:   mp.stdin,
		stdout:  mp.stdout,
		scanner: bufio.NewScanner(mp.stdout),
		encoder: json.NewEncoder(mp.stdin),
		timeout: 5 * time.Second,
	}

	err := client.call("test_method", nil, nil)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}

	if !strings.Contains(err.Error(), "failed to parse response") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

// TestCallWithParams tests that parameters are correctly serialized.
func TestCallWithParams(t *testing.T) {
	response := `{"jsonrpc":"2.0","id":1,"result":{}}`

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	client := &Client{
		stdin:   stdinWriter,
		stdout:  stdoutReader,
		scanner: bufio.NewScanner(stdoutReader),
		encoder: json.NewEncoder(stdinWriter),
		timeout: 5 * time.Second,
	}

	// Capture the request
	var capturedRequest jsonrpcRequest
	go func() {
		scanner := bufio.NewScanner(stdinReader)
		if scanner.Scan() {
			json.Unmarshal(scanner.Bytes(), &capturedRequest)
		}
		stdoutWriter.Write([]byte(response + "\n"))
	}()

	params := struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}{
		Name:  "test",
		Value: 42,
	}

	err := client.call("test_method", params, nil)
	if err != nil {
		t.Fatalf("call() returned error: %v", err)
	}

	stdinWriter.Close()
	stdoutWriter.Close()

	if capturedRequest.Params["name"] != "test" {
		t.Errorf("expected params.name=test, got %v", capturedRequest.Params["name"])
	}
	if capturedRequest.Params["value"] != float64(42) { // JSON numbers are float64
		t.Errorf("expected params.value=42, got %v", capturedRequest.Params["value"])
	}
}

// TestCallWithToolCallError tests handling of tool call errors.
func TestCallWithToolCallError(t *testing.T) {
	mcpResult := mcpToolCallResult{
		IsError: true,
		Content: []mcpContent{
			{Type: "text", Text: "Something went wrong"},
		},
	}
	mcpResultJSON, _ := json.Marshal(mcpResult)
	response := `{"jsonrpc":"2.0","id":1,"result":` + string(mcpResultJSON) + `}`

	mp := newMockProcess([]string{response})
	defer mp.Close()

	client := &Client{
		stdin:       mp.stdin,
		stdout:      mp.stdout,
		scanner:     bufio.NewScanner(mp.stdout),
		encoder:     json.NewEncoder(mp.stdin),
		timeout:     5 * time.Second,
		initialized: true,
	}

	result, err := client.Call("failing_tool", nil)
	if err != nil {
		t.Fatalf("Call() returned error: %v", err)
	}

	// The result should indicate an error
	if result.Success {
		t.Error("expected success=false for tool error")
	}
	if result.Error == nil {
		t.Error("expected error details to be present")
	}
	if result.Error != nil && result.Error.Message != "Something went wrong" {
		t.Errorf("expected error message 'Something went wrong', got: %s", result.Error.Message)
	}
}

// TestNotify tests the notify method for notifications.
func TestNotify(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, _ := io.Pipe()

	client := &Client{
		stdin:   stdinWriter,
		stdout:  stdoutReader,
		scanner: bufio.NewScanner(stdoutReader),
		encoder: json.NewEncoder(stdinWriter),
		timeout: 5 * time.Second,
	}

	// Capture the notification
	var capturedNotification struct {
		JSONRPC string         `json:"jsonrpc"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params,omitempty"`
	}
	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdinReader)
		if scanner.Scan() {
			json.Unmarshal(scanner.Bytes(), &capturedNotification)
		}
		close(done)
	}()

	err := client.notify("test/notification", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("notify() returned error: %v", err)
	}

	stdinWriter.Close()
	<-done

	if capturedNotification.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc=2.0, got %v", capturedNotification.JSONRPC)
	}
	if capturedNotification.Method != "test/notification" {
		t.Errorf("expected method=test/notification, got %v", capturedNotification.Method)
	}
}
